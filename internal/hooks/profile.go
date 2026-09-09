package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/hookscripts"
)

// Capability profiles: what this adopter CHOSE to run, so status judges the
// choice rather than the binary's full inventory.
//
// Why this exists. `hooks status` compared every project against every script
// the binary ships. Run against focalc's knowledge vault — correctly
// configured, deliberately without persona, episodes or mesh — it reported
// "3 unwired, 1 missing" on every single run. A report that is wrong every
// time is not a strict check; it is a channel the reader learns to skip, and
// then it is wrong on the day it is right.
//
// DECLARED, never inferred. Deriving the profile from what happens to be
// installed would make the check vacuous — nothing could ever be missing,
// because whatever is present would define the expectation. Same rule the
// mesh daemon URL and the registry staleness bound already follow: a guessed
// value is worse than none, because it is confidently wrong.
type Profile string

const (
	// ProfileFull is every canonical hook: persona, episodes, mesh, recall.
	// It is the default when nothing is declared, so existing adopters see
	// exactly today's behaviour.
	ProfileFull Profile = "full"
	// ProfileKnowledge is a curated knowledge vault: recall, decision-time
	// reach, read tracking and health. No persona to load, no transcript to
	// capture, no mesh to watch. (focalc's shape, and the first adopter this
	// tool was actively wrong about.)
	ProfileKnowledge Profile = "knowledge"
	// ProfilePersona is an agent identity vault without the mesh: persona,
	// episodes and recall, but no wake-watcher.
	ProfilePersona Profile = "persona"
)

// profileFilename holds the one declared word, under the project's .claude dir.
// Deliberately a single value and nothing else: recording WHICH scripts are
// installed here too would duplicate what the filesystem already answers, and
// the two copies would disagree the first time someone edited one.
const profileFilename = "vaultmind-profile"

// knowledgeScripts is the knowledge profile's set. Written as an explicit
// allowlist rather than "full minus persona" so adding a canonical script
// later cannot silently enrol every knowledge vault into running it.
var knowledgeScripts = map[string]bool{
	hookHealthScript:           true,
	hookUserPromptSubmitScript: true,
	hookPreToolUseScript:       true,
	hookReachScript:            true,
}

// personaScripts is everything canonical except the mesh watcher.
func personaScripts() map[string]bool {
	out := map[string]bool{}
	for _, es := range CanonicalEventScripts() {
		out[es.Script] = true
	}
	return out
}

// ScriptsForProfile returns the script names a profile expects to be present
// and wired. Anything outside the set is neither missing nor unwired for this
// adopter — it is not part of what they run.
func ScriptsForProfile(p Profile) []string {
	names := hookscripts.Names()
	want := func(n string) bool { return true }
	switch p {
	case ProfileKnowledge:
		want = func(n string) bool { return knowledgeScripts[n] }
	case ProfilePersona:
		ps := personaScripts()
		want = func(n string) bool { return ps[n] }
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if want(n) {
			out = append(out, n)
		}
	}
	return out
}

// EventScriptsForProfile filters the canonical event map the same way, so the
// wiring check and the script check cannot disagree about what this adopter
// signed up for.
func EventScriptsForProfile(p Profile) []EventScript {
	allowed := map[string]bool{}
	for _, n := range ScriptsForProfile(p) {
		allowed[n] = true
	}
	out := make([]EventScript, 0, len(allowed))
	for _, es := range CanonicalEventScripts() {
		if allowed[es.Script] {
			out = append(out, es)
		}
	}
	return out
}

// DeclaredProfile reads the project's declared profile. Absent ⇒ ProfileFull,
// preserving today's behaviour for every existing adopter. An unrecognised
// value is an ERROR: silently treating a typo as "full" would report a
// misconfigured project as a healthy one.
func DeclaredProfile(projectDir string) (Profile, error) {
	path := filepath.Join(projectDir, ".claude", profileFilename)
	// projectDir is operator-supplied (same trust class as the vault path);
	// the filename is a package constant, so no component is caller-controlled.
	// #nosec G304 G703
	// nosemgrep: go-path-traversal
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProfileFull, nil
		}
		return "", fmt.Errorf("reading declared profile %s: %w", path, err)
	}
	switch p := Profile(strings.TrimSpace(string(raw))); p {
	case ProfileFull, ProfileKnowledge, ProfilePersona:
		return p, nil
	default:
		return "", fmt.Errorf(
			"unknown vaultmind profile %q in %s (expected %s, %s or %s)",
			p, path, ProfileFull, ProfileKnowledge, ProfilePersona)
	}
}

// WriteDeclaredProfile records the choice. Callers pass a validated Profile.
func WriteDeclaredProfile(projectDir string, p Profile) error {
	dir := filepath.Join(projectDir, ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, profileFilename)
	if err := os.WriteFile(path, []byte(string(p)+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing declared profile %s: %w", path, err)
	}
	return nil
}

// ParseProfile validates an operator-supplied profile name. Empty means
// ProfileFull. An unrecognised value is an error rather than a fallback: a
// typo that silently installs the full set is the failure this feature exists
// to remove.
func ParseProfile(s string) (Profile, error) {
	switch p := Profile(strings.TrimSpace(s)); p {
	case "":
		return ProfileFull, nil
	case ProfileFull, ProfileKnowledge, ProfilePersona:
		return p, nil
	default:
		return "", fmt.Errorf("unknown --profile %q (expected %s, %s or %s)",
			p, ProfileFull, ProfileKnowledge, ProfilePersona)
	}
}
