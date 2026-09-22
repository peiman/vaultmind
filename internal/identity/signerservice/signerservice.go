// Package signerservice supervises the custody signer under launchd.
//
// WHY. The signer is a foreground daemon, and nothing restarted it. On
// 2026-09-22 a member's signer died with a session reset and every signed send
// failed "signer unreachable" until it was restarted by hand; Mira's own had
// run unsupervised for a month. The only supervised signer on either machine
// was a hand-written plist. This makes that plist a command.
package signerservice

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	labelPrefix    = "com.vaultmind.signer."
	launchAgentDir = "Library/LaunchAgents"
	plistPerm      = 0o644
	agentDirPerm   = 0o755
	launchctl      = "launchctl"
)

// Spec is everything the launchd job needs. Every path is absolute: launchd
// runs the job with its own environment, so a default resolved there is not
// the default the operator's shell would resolve.
type Spec struct {
	Name       string
	Binary     string
	KeyPath    string
	SocketPath string
	LogDir     string
}

// Label is the launchd job label for a signer name.
func Label(name string) string { return labelPrefix + name }

// NameFromKeyPath derives a signer name from its key file ("mira.key" → "mira"),
// so two signers on one machine get two jobs rather than overwriting one.
func NameFromKeyPath(keyPath string) string {
	base := filepath.Base(keyPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// RenderPlist produces the LaunchAgent plist for s.
func RenderPlist(s Spec) ([]byte, error) {
	for what, p := range map[string]string{"binary": s.Binary, "key": s.KeyPath, "socket": s.SocketPath, "log dir": s.LogDir} {
		if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("signer service: %s path %q must be absolute", what, p)
		}
	}
	args := []string{s.Binary, "identity", "signer", "--signer-key", s.KeyPath, "--signer-socket", s.SocketPath}

	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	fmt.Fprintf(&b, "  <key>Label</key><string>%s</string>\n", esc(Label(s.Name)))
	b.WriteString("  <key>ProgramArguments</key>\n  <array>\n")
	for _, a := range args {
		fmt.Fprintf(&b, "    <string>%s</string>\n", esc(a))
	}
	b.WriteString("  </array>\n")
	b.WriteString("  <key>RunAtLoad</key><true/>\n")
	b.WriteString("  <key>KeepAlive</key><true/>\n")
	b.WriteString("  <key>ProcessType</key><string>Background</string>\n")
	logBase := filepath.Join(s.LogDir, "vaultmind-signer-"+s.Name)
	fmt.Fprintf(&b, "  <key>StandardOutPath</key><string>%s</string>\n", esc(logBase+".out.log"))
	fmt.Fprintf(&b, "  <key>StandardErrorPath</key><string>%s</string>\n", esc(logBase+".err.log"))
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes(), nil
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s)) // writes to a strings.Builder; cannot fail
	return b.String()
}

// Installer writes and loads the job. Its effects are injected so the refusal
// and idempotency rules are testable without touching launchd.
type Installer struct {
	Home       string
	UID        int
	Run        func(ctx context.Context, name string, args ...string) error
	SocketLive func(path string) bool
}

// PlistPath is where the job for name is written.
func (i Installer) PlistPath(name string) string {
	return filepath.Join(i.Home, launchAgentDir, Label(name)+".plist")
}

// Install writes the plist and (re)loads it, returning the plist path.
//
// It REFUSES while another signer already serves the socket: the launchd copy
// would fail to bind, and KeepAlive would turn that into a restart loop that
// looks like a running service.
func (i Installer) Install(ctx context.Context, s Spec) (string, error) {
	if i.SocketLive(s.SocketPath) {
		return "", fmt.Errorf("a signer is already serving %s — stop it first (it was started by hand), "+
			"then rerun install so launchd owns the only one", s.SocketPath)
	}
	body, err := RenderPlist(s)
	if err != nil {
		return "", err
	}
	path := i.PlistPath(s.Name)
	if err := os.MkdirAll(filepath.Dir(path), agentDirPerm); err != nil {
		return "", fmt.Errorf("signer service: creating %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, body, plistPerm); err != nil {
		return "", fmt.Errorf("signer service: writing %s: %w", path, err)
	}
	if err := os.Chmod(path, plistPerm); err != nil { // WriteFile keeps an existing file's mode
		return "", fmt.Errorf("signer service: chmod %s: %w", path, err)
	}
	domain := fmt.Sprintf("gui/%d", i.UID)
	// Unload any previous version so a rerun replaces it. Failure here means
	// nothing was loaded, which is the fresh-install case — not an error.
	_ = i.Run(ctx, launchctl, "bootout", domain+"/"+Label(s.Name))
	if err := i.Run(ctx, launchctl, "bootstrap", domain, path); err != nil {
		return "", fmt.Errorf("signer service: launchctl bootstrap %s: %w", path, err)
	}
	return path, nil
}
