package shellparse

import "testing"

// TestStripShellQuoting is the table-driven port of workhorse's
// test-auto-rag-guard.sh v0.3 fixtures plus the regression guards
// they came with. Each case exercises one transition in the 4-state
// machine (OUTSIDE / IN_SINGLE / IN_DOUBLE / IN_HEREDOC). The
// load-bearing assertions are the v0.3 cases — drift verbs inside
// heredoc bodies and quoted strings must be stripped from the
// OUTSIDE skeleton so downstream regex matchers don't false-positive
// on literal-quoted content.
//
// Source contract:
// /Users/peiman/dev/workhorse/.claude/scripts/shell-strip.sh
// (workhorse v0.3 stable, 2026-05-07 handoff).
func TestStripShellQuoting(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// ─── Trivial OUTSIDE-only ────────────────────────────────
		{"empty string", "", ""},
		{"plain command", "ls -la", "ls -la"},
		{"command with pipe", "true | vaultmind index --embed", "true | vaultmind index --embed"},
		{"command with semicolons", "a; b; c", "a; b; c"},
		{"command with redirect", "cmd > /tmp/out", "cmd > /tmp/out"},

		// ─── Single-quoted ───────────────────────────────────────
		{"single-quoted body stripped", "echo 'hello world'", "echo "},
		{"single-quoted body with backslash (shell: ' never escapes)", `echo 'a\b' c`, "echo  c"},
		{"single-quote preserves OUTSIDE around", "echo 'a' b 'c'", "echo  b "},

		// ─── Double-quoted ───────────────────────────────────────
		{"double-quoted body stripped", `echo "hello world"`, "echo "},
		{"double-quoted with escaped quote", `echo "a\"b"`, "echo "},
		{"double-quoted with backslash-anything", `echo "a\nb"`, "echo "},
		{"double-quote preserves OUTSIDE around", `echo "x" outside "y"`, "echo  outside "},

		// ─── Heredoc basic ───────────────────────────────────────
		{
			"heredoc <<EOF body stripped, marker line preserved",
			"cat <<EOF\nhello\nEOF",
			"cat <<EOF\nEOF",
		},
		{
			"heredoc <<EOF with body containing pipes and verbs",
			"cat <<EOF\ntrue | vaultmind index --embed\nEOF",
			"cat <<EOF\nEOF",
		},

		// ─── Heredoc quoted markers ──────────────────────────────
		{
			"heredoc <<'EOF' (single-quoted marker)",
			"cat <<'EOF'\nbody\nEOF",
			"cat <<'EOF'\nEOF",
		},
		{
			"heredoc <<\"EOF\" (double-quoted marker)",
			"cat <<\"EOF\"\nbody\nEOF",
			"cat <<\"EOF\"\nEOF",
		},

		// ─── Heredoc <<- (tab-stripped close marker) ─────────────
		{
			"heredoc <<-EOF strips leading tabs on close marker",
			"\tcat <<-EOF\n\tbody\n\tEOF",
			"\tcat <<-EOF\n\tEOF",
		},
		{
			"heredoc <<-'EOF' (tabs + single-quoted marker)",
			"cat <<-'EOF'\n\tbody\n\tEOF",
			"cat <<-'EOF'\n\tEOF",
		},

		// ─── v0.3 LOAD-BEARING cases — workhorse false-positive guards ───
		// Without the preprocessor these tripped auto-rag-guard's drift
		// regex by emitting `|`-separator matches inside literal-quoted
		// or heredoc body. The preprocessor exists because reality
		// surfaced these in workhorse commit-authoring.
		{
			"v0.3: drift inside <<EOF body never reaches output",
			"cat <<EOF\ntrue | vaultmind index --embed\nEOF",
			"cat <<EOF\nEOF",
		},
		{
			"v0.3: drift inside <<'EOF' body never reaches output",
			"cat <<'EOF'\ntrue | vaultmind index --embed\nEOF",
			"cat <<'EOF'\nEOF",
		},
		{
			"v0.3: drift inside <<-EOF body (tab-stripped) never reaches output",
			"\tcat <<-EOF\n\ttrue | vaultmind index --embed\n\tEOF",
			"\tcat <<-EOF\n\tEOF",
		},
		{
			"v0.3: drift inside single-quoted string never reaches output",
			"grep -E 'foo|vaultmind index --embed' file.txt",
			"grep -E  file.txt",
		},
		{
			"v0.3: drift inside double-quoted string never reaches output",
			`echo "step 1 | vaultmind index --embed step 2"`,
			"echo ",
		},

		// ─── Edge cases — unclosed / malformed ───────────────────
		{
			"unclosed single quote drops rest of input",
			"echo 'unclosed body never closes",
			"echo ",
		},
		{
			"unclosed double quote drops rest of input",
			`echo "unclosed body never closes`,
			"echo ",
		},
		{
			"unclosed heredoc drops rest of input (no close-marker line)",
			"cat <<EOF\nbody body body",
			"cat <<EOF\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripShellQuoting(tc.in)
			if got != tc.want {
				t.Errorf("StripShellQuoting(%q):\n  got  %q\n  want %q", tc.in, got, tc.want)
			}
		})
	}
}
