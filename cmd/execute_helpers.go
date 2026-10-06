package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/peiman/vaultmind/.ckeletin/pkg/output"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Main runs the CLI for args and returns its exit code. An ask goes to this
// version's warm server when one is running (see serve); otherwise the
// command runs in this process, exactly as before.
func Main(args []string) int {
	if code, ok := tryServer(args); ok {
		return code
	}
	return ExecuteWithIO(args, os.Stdout, os.Stderr)
}

// ExecuteWithIO runs the CLI for args with its output on out and errw, and
// returns the exit code. It is the one mapping from a command's error to an
// exit code, used by main and by the server for each request.
func ExecuteWithIO(args []string, out, errw io.Writer) int {
	RootCmd.SetArgs(args)
	RootCmd.SetOut(out)
	RootCmd.SetErr(errw)
	defer func() {
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
	}()
	err := Execute()
	if err == nil {
		return 0
	}
	// The command already wrote its own error envelope and said so: exit
	// non-zero, so a caller checking only the status doesn't read success,
	// and stay silent, so the failure is described exactly once.
	if errors.Is(err, cmdutil.ErrAlreadyWritten) {
		return 1
	}
	if output.IsJSONMode() {
		_ = output.RenderJSON(out, output.JSONEnvelope{
			Status:  "error",
			Command: output.CommandName(),
			Error:   &output.JSONError{Message: err.Error()},
		})
	} else {
		_, _ = fmt.Fprintf(errw, "Error: %v\n", err)
	}
	return 1
}

// flagBinding is one command flag bound to its config key at registration.
type flagBinding struct {
	key  string
	flag *pflag.Flag
}

// flagBindings are replayed after a config reset: commands bind their flags
// once, when they are registered, and viper.Reset forgets them.
var flagBindings []flagBinding

// resetCLIState returns the CLI to the state of a fresh process: every flag
// at its default, the config and output mode cleared. A warm server runs many commands in
// one process; without this, one request's flags, config file or config
// search path would carry into the next.
func resetCLIState() {
	resetFlagsRecursive(RootCmd)
	resetContexts(RootCmd)
	output.SetOutputMode("")
	output.SetCommandName("")
	configFileUsed, configFileStatus = "", ""
	viper.Reset()
	for _, b := range flagBindings {
		_ = viper.BindPFlag(b.key, b.flag)
	}
}

// resetContexts clears every command's context. Cobra gives a command its
// parent's context only while it has none, and the pre-run hook wraps the
// experiment session into it, so a second run would still see the first
// run's session (closed by then) and the chain would grow with every run.
func resetContexts(cmd *cobra.Command) {
	cmd.SetContext(context.Background())
	for _, c := range cmd.Commands() {
		resetContexts(c)
	}
}

// resetFlagsRecursive walks cmd and all descendants, resetting every flag to
// its declared default. Covers persistent and local flags.
func resetFlagsRecursive(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
		// A flag that remembers its occurrences must forget them here: this
		// simulates a FRESH PROCESS, and a real one has parsed nothing yet.
		// Order matters — clearing BEFORE the reset above would record the
		// default assignment as a user-supplied occurrence, which made a
		// single --vault look like a repeat.
		if rv, ok := f.Value.(*repeatedFlagValue); ok {
			rv.Reset()
		}
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, c := range cmd.Commands() {
		resetFlagsRecursive(c)
	}
}
