// main.go

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/peiman/vaultmind/.ckeletin/pkg/output"
	"github.com/peiman/vaultmind/cmd"
	"github.com/peiman/vaultmind/internal/cmdutil"
)

func run() int {
	if err := cmd.Execute(); err != nil {
		// The command already wrote its own error envelope and said so. Exit
		// non-zero — the envelope reports the failure, but a caller checking
		// only the exit status must not read success — while staying silent
		// here, so the failure is described exactly once.
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return 1
		}
		if output.IsJSONMode() {
			_ = output.RenderJSON(os.Stdout, output.JSONEnvelope{
				Status:  "error",
				Command: output.CommandName(),
				Error:   &output.JSONError{Message: err.Error()},
			})
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		return 1
	}
	return 0
}

// main is intentionally not covered by tests because it's the program's entry point.
// All logic is tested via the run() function and other commands. The main function’s
// sole purpose is to call run() and exit accordingly. Attempting to cover main directly
// would require integration tests or running the built binary separately.
func main() {
	os.Exit(run())
}
