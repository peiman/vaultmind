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

	// launcherDirSuffix is relative to the user's home directory.
	// macOS Login Items lists a LaunchAgent by ProgramArguments[0]'s filename.
	// By pointing the plist at a per-signer script here, every job gets its own
	// readable name in System Settings instead of all showing "vaultmind".
	launcherDirSuffix = "Library/Application Support/vaultmind/launchers"
	launcherPerm      = 0o755
	launcherDirPerm   = 0o700
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

// LauncherAbsPath returns the absolute path of the per-signer launcher script
// for name inside home. macOS Login Items shows this script's filename as the
// entry label, so each signer gets a readable name ("VaultMind Signer - mira")
// rather than a row of identical "vaultmind" entries.
func LauncherAbsPath(home, name string) string {
	return filepath.Join(home, launcherDirSuffix, "VaultMind Signer - "+name)
}

// posixQuote wraps s in single quotes, escaping any embedded single quotes so
// the result is safe as a shell word even when s contains spaces or quotes.
// Rule: end the single-quoted segment, write '"'"', restart it.
func posixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// RenderScript produces the launcher shell script for s. The script execs the
// signer with every argument explicitly single-quoted so paths with spaces or
// single quotes are passed as-is to the signer process.
func RenderScript(s Spec) ([]byte, error) {
	for what, p := range map[string]string{"binary": s.Binary, "key": s.KeyPath, "socket": s.SocketPath} {
		if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("signer service: %s path %q must be absolute", what, p)
		}
	}
	var b bytes.Buffer
	b.WriteString("#!/bin/sh\n")
	fmt.Fprintf(&b, "# VaultMind Signer - %s: custody signer launcher (managed by vaultmind identity signer install).\n", s.Name)
	fmt.Fprintf(&b, "exec %s identity signer --signer-key %s --signer-socket %s \"$@\"\n",
		posixQuote(s.Binary), posixQuote(s.KeyPath), posixQuote(s.SocketPath))
	return b.Bytes(), nil
}

// RenderPlist produces the LaunchAgent plist for s. ProgramArguments contains
// exactly one element: launcherPath. This makes macOS Login Items display the
// launcher's filename ("VaultMind Signer - <name>") instead of "vaultmind".
// Everything else (label, KeepAlive, logs) is unchanged.
func RenderPlist(s Spec, launcherPath string) ([]byte, error) {
	for what, p := range map[string]string{"launcher": launcherPath, "log dir": s.LogDir} {
		if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("signer service: %s path %q must be absolute", what, p)
		}
	}
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	fmt.Fprintf(&b, "  <key>Label</key><string>%s</string>\n", esc(Label(s.Name)))
	b.WriteString("  <key>ProgramArguments</key>\n  <array>\n")
	fmt.Fprintf(&b, "    <string>%s</string>\n", esc(launcherPath))
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

// LauncherPath is where the launcher script for name is written.
func (i Installer) LauncherPath(name string) string {
	return LauncherAbsPath(i.Home, name)
}

// Install writes the launcher script and plist, then (re)loads the job,
// returning the plist path.
//
// It REFUSES while another signer already serves the socket: the launchd copy
// would fail to bind, and KeepAlive would turn that into a restart loop that
// looks like a running service.
//
// A re-run with an identical spec is a no-op for file writes: byte-identical
// files are left untouched. The job reload happens regardless.
func (i Installer) Install(ctx context.Context, s Spec) (string, error) {
	if i.SocketLive(s.SocketPath) {
		return "", fmt.Errorf("a signer is already serving %s — stop it first (it was started by hand), "+
			"then rerun install so launchd owns the only one", s.SocketPath)
	}

	launcherPath := i.LauncherPath(s.Name)
	launcherDir := filepath.Dir(launcherPath)

	// Refuse symlinks at both the dir and the final path so we never follow
	// one and write outside the intended location.
	if isSymlink(launcherDir) {
		return "", fmt.Errorf("signer service: launcher dir %q is a symlink — refusing to follow it", launcherDir)
	}
	if err := os.MkdirAll(launcherDir, launcherDirPerm); err != nil {
		return "", fmt.Errorf("signer service: creating launcher dir %s: %w", launcherDir, err)
	}
	if isSymlink(launcherPath) {
		return "", fmt.Errorf("signer service: launcher path %q is a symlink — refusing to follow it", launcherPath)
	}

	scriptBody, err := RenderScript(s)
	if err != nil {
		return "", err
	}
	if err := writeAtomicIfChanged(launcherDir, launcherPath, scriptBody, launcherPerm); err != nil {
		return "", fmt.Errorf("signer service: writing launcher %s: %w", launcherPath, err)
	}

	plistBody, err := RenderPlist(s, launcherPath)
	if err != nil {
		return "", err
	}
	plistPath := i.PlistPath(s.Name)
	if err := os.MkdirAll(filepath.Dir(plistPath), agentDirPerm); err != nil {
		return "", fmt.Errorf("signer service: creating %s: %w", filepath.Dir(plistPath), err)
	}
	if err := writeFileIfChanged(plistPath, plistBody, plistPerm); err != nil {
		return "", fmt.Errorf("signer service: writing %s: %w", plistPath, err)
	}

	domain := fmt.Sprintf("gui/%d", i.UID)
	// Unload any previous version so a rerun replaces it. Failure here means
	// nothing was loaded, which is the fresh-install case — not an error.
	_ = i.Run(ctx, launchctl, "bootout", domain+"/"+Label(s.Name))
	if err := i.Run(ctx, launchctl, "bootstrap", domain, plistPath); err != nil {
		return "", fmt.Errorf("signer service: launchctl bootstrap %s: %w", plistPath, err)
	}
	return plistPath, nil
}

// isSymlink reports whether path is a symlink without following it.
func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// writeAtomicIfChanged writes content to path via a temp file in dir only when
// the existing content differs. The temp file is renamed into place so readers
// never see a partial write, and it is cleaned up on any error before the rename.
func writeAtomicIfChanged(dir, path string, content []byte, perm os.FileMode) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) { //nolint:gosec // G304: path is under the caller-controlled Home directory
		return nil
	}
	tmp, err := os.CreateTemp(dir, ".launcher-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op after a successful rename
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// writeFileIfChanged writes content to path only when the existing content differs.
func writeFileIfChanged(path string, content []byte, perm os.FileMode) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) { //nolint:gosec // G304: path is under the caller-controlled Home directory
		return nil
	}
	if err := os.WriteFile(path, content, perm); err != nil {
		return err
	}
	return os.Chmod(path, perm) // WriteFile keeps an existing file's mode
}
