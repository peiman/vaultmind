package onboard

import "fmt"

// AskQuestion is the question the first ask runs, in the README and in
// init's next steps.
const AskQuestion = "what did we decide about retries?"

// QuickstartVault is the vault path the README Quickstart types.
const QuickstartVault = "./knowledge"

// ShellExpandSentence is why the quickstart passes *.md. The shell expands
// it, and dotfiles stay out.
const ShellExpandSentence = "The shell expands *.md to the root Markdown (README, CONTRIBUTING, AGENTS, and the rest) and skips dotfiles, so tool chat logs such as .aider.chat.history.md stay out; drop docs if the project has no docs/, and if the project has no root Markdown, leave out *.md (zsh aborts the whole command with no matches found otherwise)."

// InitFlagsLine names the two init flags that sit beside the first steps.
const InitFlagsLine = "vaultmind init --wire-hooks installs the hooks while scaffolding. vaultmind init --print-instructions prints the agent setup guide."

// FirstSteps are the commands a new vault's next steps and the README
// Quickstart both print, in order (Principle 7 — one list). vault is the
// path the operator types.
func FirstSteps(vault string) []string {
	return []string{
		fmt.Sprintf("vaultmind import docs *.md --vault %s", vault),
		fmt.Sprintf("vaultmind index --embed --vault %s", vault),
		fmt.Sprintf("vaultmind ask %q --vault %s", AskQuestion, vault),
		fmt.Sprintf("vaultmind hooks install . --vault %s --merge", vault),
	}
}

// IdentityFirstSteps starts retrieval and installs hooks for an identity
// vault (persona or full profile), filled by editing its starter notes.
func IdentityFirstSteps(vault, profile string) []string {
	return []string{
		fmt.Sprintf("vaultmind index --embed --vault %s", vault),
		fmt.Sprintf("vaultmind ask %q --vault %s", "who am I", vault),
		fmt.Sprintf("vaultmind hooks install . --vault %s --profile %s --merge", vault, profile),
	}
}
