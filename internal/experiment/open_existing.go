package experiment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNoUsageLog reports that no usage log exists at the path asked for.
var ErrNoUsageLog = errors.New("no usage log")

// OpenExisting opens the usage log only if it is already there.
//
// Open creates the file, runs migrations, and leaves a schema behind. That is
// correct for the write path and wrong for every READ command: asking "what is
// in the log?" must not be the thing that creates one.
//
// It also made a documented promise false. `experiments.telemetry: off` says
// nothing is written — and `vaultmind experiment trace` under off would write a
// fresh database as a side effect of reporting that there was nothing to
// report. A read that creates its own subject is the same silent-write shape
// this codebase keeps closing.
//
// Reading an EXISTING log under off stays allowed on purpose: turning telemetry
// off is a decision about new data, not a reason to lock the operator out of
// what was already collected.
func OpenExisting(dbPath string) (*DB, error) {
	cleanPath := filepath.Clean(dbPath)
	if _, err := os.Stat(cleanPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w at %s", ErrNoUsageLog, cleanPath)
		}
		return nil, fmt.Errorf("checking for usage log: %w", err)
	}
	return Open(cleanPath)
}

// OpenMemory returns an empty, migrated log that exists only in memory.
//
// For the read path when no log is on disk: the caller gets a database with the
// real schema and zero rows, so every reader, manifest and rollup behaves
// exactly as it would against an empty file — without creating one. The
// alternative, a separate "there is nothing to export" code path, is a second
// implementation of the output format that drifts from the first.
func OpenMemory() (*DB, error) {
	return Open(":memory:")
}
