package hooks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/peiman/vaultmind/internal/shellparse"
)

// ScriptState is what a project's copy of a canonical hook script is doing
// relative to the one embedded in this binary.
type ScriptState string

const (
	// ScriptInSync means the installed copy behaves identically to the
	// canonical one. Comments and blank lines are ignored — a hook's behaviour
	// is its code, and treating annotations as drift makes the whole signal
	// unusable, which is how a drift report teaches people to ignore it.
	ScriptInSync ScriptState = "in_sync"
	// ScriptDrifted means the installed copy differs behaviourally. Either the
	// project changed it (and the change should go upstream to survive an
	// update) or the binary moved ahead (and the project should update).
	ScriptDrifted ScriptState = "drifted"
	// ScriptMissing means the binary ships this hook and the project does not
	// have it. Distinct from in-sync for a reason: two hooks lived only in one
	// consumer for months while every adopter silently lacked them, and no
	// command would say so — absence reported as nothing looks like health.
	ScriptMissing ScriptState = "missing"
)

// ScriptStatus is one canonical script's state in a project.
type ScriptStatus struct {
	Name  string      `json:"name"`
	State ScriptState `json:"state"`
}

// StatusReport is the full comparison between the hooks this binary ships and
// the ones a project has installed.
type StatusReport struct {
	ProjectDir string         `json:"project_dir"`
	Installed  bool           `json:"installed"`
	Scripts    []ScriptStatus `json:"scripts"`
	// Events reports whether each canonical event is wired in settings.json.
	// Script contents and event wiring are independent failures: a project can
	// hold every script byte-identical and still run none of them.
	Events []EventStatus `json:"events"`
}

// Counts returns how many scripts are in each state — the summary line.
func (r StatusReport) Counts() (inSync, drifted, missing int) {
	for _, s := range r.Scripts {
		switch s.State {
		case ScriptInSync:
			inSync++
		case ScriptDrifted:
			drifted++
		case ScriptMissing:
			missing++
		}
	}
	return inSync, drifted, missing
}

// Status compares every canonical hook script against the project's installed
// copy. It is the full picture; CompareInstalled is the drift-only view that
// doctor consumes.
func Status(projectDir string) (StatusReport, error) {
	report := StatusReport{ProjectDir: projectDir}
	report.Events = eventWiring(projectDir)
	scriptsDir := filepath.Join(projectDir, ".claude", "scripts")
	if _, err := os.Stat(scriptsDir); err == nil {
		report.Installed = true
	} else if !os.IsNotExist(err) {
		return StatusReport{}, fmt.Errorf("reading %s: %w", scriptsDir, err)
	}

	for _, name := range hookscripts.Names() {
		canonical, ok := hookscripts.Get(name)
		if !ok {
			continue
		}
		// Canonical names come from the embedded FS and are bare filenames by
		// construction. Checked here anyway rather than asserted in a comment:
		// the guarantee lives in another package, and this is the line that
		// would do the damage if it ever stopped holding.
		if name != filepath.Base(name) {
			return StatusReport{}, fmt.Errorf("canonical script name %q is not a bare filename", name)
		}
		dst := filepath.Join(scriptsDir, name)
		existing, err := os.ReadFile(dst) // #nosec G304 -- name validated as a bare filename just above
		switch {
		case os.IsNotExist(err):
			report.Scripts = append(report.Scripts, ScriptStatus{Name: name, State: ScriptMissing})
		case err != nil:
			return StatusReport{}, fmt.Errorf("reading %s: %w", dst, err)
		case shellparse.StripCommentsAndBlanks(string(existing)) == shellparse.StripCommentsAndBlanks(string(canonical)):
			report.Scripts = append(report.Scripts, ScriptStatus{Name: name, State: ScriptInSync})
		default:
			report.Scripts = append(report.Scripts, ScriptStatus{Name: name, State: ScriptDrifted})
		}
	}
	return report, nil
}
