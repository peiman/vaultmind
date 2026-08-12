// main_test.go

package main

import (
	"fmt"
	"testing"

	"github.com/peiman/vaultmind/cmd"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/spf13/cobra"
)

func TestMainFunction(t *testing.T) {
	// Define test cases
	tests := []struct {
		name     string
		cmd      string
		cmdError error
		wantCode int
	}{
		{
			name:     "Success scenario",
			cmd:      "success",
			cmdError: nil,
			wantCode: 0,
		},
		{
			name:     "Failure scenario",
			cmd:      "fail",
			cmdError: fmt.Errorf("simulated failure"),
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SETUP PHASE
			// Save the original RootCmd
			originalRoot := cmd.RootCmd
			// Create a test root command
			testRoot := &cobra.Command{Use: "test"}
			// Replace global RootCmd with our test root
			cmd.RootCmd = testRoot
			// Restore after the test
			defer func() { cmd.RootCmd = originalRoot }()

			// Add a dummy command with the specified behavior
			testRoot.AddCommand(&cobra.Command{
				Use: tt.cmd,
				RunE: func(cmd *cobra.Command, args []string) error {
					return tt.cmdError
				},
			})

			// Set command arguments
			testRoot.SetArgs([]string{tt.cmd})

			// EXECUTION PHASE
			code := run()

			// ASSERTION PHASE
			if code != tt.wantCode {
				t.Errorf("expected exit code %d, got %d", tt.wantCode, code)
			}
		})
	}
}

// A command that has already written a JSON error envelope signals it with
// cmdutil.ErrAlreadyWritten, so the envelope isn't printed twice. Every such
// command returned nil afterwards — which made the process exit 0 while the
// envelope it had just printed said status "error".
//
// That is the silent failure this tool exists to argue against, sitting in its
// own agent-facing contract: `vaultmind ask --json ... || handle_failure` never
// fired, and a hook wrapping any --json call read success on every error. The
// sentinel must reach the exit code while still suppressing the second write.
func TestRun_AlreadyWrittenEnvelopeStillExitsNonZero(t *testing.T) {
	originalRoot := cmd.RootCmd
	testRoot := &cobra.Command{Use: "test"}
	cmd.RootCmd = testRoot
	defer func() { cmd.RootCmd = originalRoot }()

	testRoot.AddCommand(&cobra.Command{
		Use: "wrote-envelope",
		RunE: func(*cobra.Command, []string) error {
			return cmdutil.ErrAlreadyWritten
		},
	})
	testRoot.SetArgs([]string{"wrote-envelope"})

	if code := run(); code != 1 {
		t.Errorf("a written error envelope must exit non-zero; got %d", code)
	}
}

// The sentinel may be wrapped on its way up; matching must be errors.Is, not ==.
func TestRun_WrappedAlreadyWrittenStillExitsNonZero(t *testing.T) {
	originalRoot := cmd.RootCmd
	testRoot := &cobra.Command{Use: "test"}
	cmd.RootCmd = testRoot
	defer func() { cmd.RootCmd = originalRoot }()

	testRoot.AddCommand(&cobra.Command{
		Use: "wrapped",
		RunE: func(*cobra.Command, []string) error {
			return fmt.Errorf("running query: %w", cmdutil.ErrAlreadyWritten)
		},
	})
	testRoot.SetArgs([]string{"wrapped"})

	if code := run(); code != 1 {
		t.Errorf("a wrapped sentinel must still exit non-zero; got %d", code)
	}
}
