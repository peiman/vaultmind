package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runRecallHook drives vault-recall.sh with a given UserPromptSubmit payload and
// a stub `vaultmind` that always returns pointers, so the only thing under test
// is whether the hook decided to query at all.
func runRecallHook(t *testing.T, prompt string) (stdout string, queried bool) {
	t.Helper()
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	script, err := filepath.Abs("vault-recall.sh")
	require.NoError(t, err)

	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "vaultmind-identity"), 0o750))

	// The stub records every invocation, so "did the hook query the vault?" is
	// observable rather than inferred from output shape.
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "invoked")
	stub := "#!/bin/bash\necho \"$@\" >> " + marker + "\n" +
		"if [ \"$1\" = ask ]; then echo '  0.42  some-note   A Note'; fi\nexit 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "vaultmind"), []byte(stub), 0o700))

	payload, err := json.Marshal(map[string]string{"prompt": prompt, "session_id": "test"})
	require.NoError(t, err)

	cmd := exec.Command(bashPath, script)
	cmd.Env = []string{
		"PATH=" + binDir + ":/usr/bin:/bin",
		"CLAUDE_PROJECT_DIR=" + projectDir,
		"HOME=" + t.TempDir(),
	}
	cmd.Stdin = strings.NewReader(string(payload))
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	require.NoErrorf(t, cmd.Run(), "recall hook must always exit 0 (stderr: %s)", errb.String())

	_, statErr := os.Stat(marker)
	return out.String(), statErr == nil
}

// Not every UserPromptSubmit payload is a human asking something. Background
// task completions, hook output and system reminders arrive through the SAME
// `prompt` field, and ranking notes against a task-id envelope produces pointers
// about nothing — retrieval working correctly on garbage input.
//
// Measured on one real session before this guard existed: 93 injections, 48% of
// them ranked against a prompt over 250 chars that was a <task-notification>
// envelope, ~103k pointer characters total. Half the channel's output was noise,
// which is exactly how an agent learns to skip the channel — and then misses the
// half that mattered. The fix belongs in the hook, not in a resolution to read
// more carefully.
func TestRecallHook_SkipsMachineGeneratedPrompts(t *testing.T) {
	cases := map[string]string{
		"task notification": `<task-notification>
<task-id>bx22yo6wo</task-id>
<status>completed</status>
</task-notification> plus enough text to clear the length threshold comfortably`,
		"system reminder": `<system-reminder>As you answer, remember the following context about the user's preferences and setup</system-reminder>`,
		"local command":   `<local-command-stdout>Compacted (ctrl+o to see full summary) and some more output text here</local-command-stdout>`,
		"hook output":     `UserPromptSubmit hook success: Success — vault pointers were already injected for this turn`,
	}

	for name, prompt := range cases {
		t.Run(name, func(t *testing.T) {
			out, queried := runRecallHook(t, prompt)
			assert.False(t, queried,
				"a machine-generated payload must not reach the vault — ranking against it produces pointers about nothing")
			assert.Empty(t, strings.TrimSpace(out), "and must inject nothing")
		})
	}
}

// The guard must not silence the channel it exists to protect. A real question
// still queries and still injects.
func TestRecallHook_RealQuestionStillQueries(t *testing.T) {
	out, queried := runRecallHook(t, "how does spreading activation interact with the noise floor?")
	assert.True(t, queried, "a genuine question must still reach the vault")
	assert.NotEmpty(t, strings.TrimSpace(out), "and must still inject pointers")
}

// Prose ABOUT a marker is still a question. The bracketed form is what separates
// a payload from a conversation about one — which matters because a question
// about the hook's own behaviour is exactly the kind the vault may hold
// something on, and silencing it would be the noise problem inverted.
func TestRecallHook_MentioningAMarkerIsStillAQuestion(t *testing.T) {
	_, queried := runRecallHook(t,
		"why does the recall hook skip a system-reminder payload instead of ranking it?")
	assert.True(t, queried,
		"talking about a marker is a question; only the bracketed payload form is noise")
}
