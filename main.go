// main.go

package main

import (
	"os"

	"github.com/peiman/vaultmind/cmd"
)

// run is one OS process. cmd.Main runs the CLI and releases the process
// embedder when that command returns. A request inside the resident server
// does not call Main, so it does not release the model.
func run(args []string) int {
	return cmd.Main(args)
}

// main is intentionally not covered by tests because it's the program's entry point.
// All logic is tested via the run() function and other commands. The main function’s
// sole purpose is to call run() and exit accordingly. Attempting to cover main directly
// would require integration tests or running the built binary separately.
func main() {
	os.Exit(run(os.Args[1:]))
}
