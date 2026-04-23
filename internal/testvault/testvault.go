// Package testvault provides shared, per-process test fixtures for the
// research vault. It builds the vault index once per test binary and hands
// out fresh copies to individual tests, cutting package-level test time
// from N*rebuild to 1*rebuild + N*file-copy. Test-only; not imported by
// production code.
package testvault

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
)

var (
	sharedOnce sync.Once
	sharedPath string
	sharedErr  error
)

// SharedIndexDBPath returns the filesystem path of an index DB built from
// vaultPath, rebuilt exactly once per test process. Callers must NOT mutate
// the returned file — use OpenSharedDB to obtain a writable per-test copy.
func SharedIndexDBPath(tb testing.TB, vaultPath string) string {
	tb.Helper()
	sharedOnce.Do(func() {
		dir, err := os.MkdirTemp("", "vm-test-shared-*")
		if err != nil {
			sharedErr = err
			return
		}
		dbPath := filepath.Join(dir, "shared.db")
		cfg, err := vault.LoadConfig(vaultPath)
		if err != nil {
			sharedErr = err
			return
		}
		idxr := index.NewIndexer(vaultPath, dbPath, cfg)
		if _, err := idxr.Rebuild(); err != nil {
			sharedErr = err
			return
		}
		sharedPath = dbPath
	})
	if sharedErr != nil {
		tb.Fatalf("testvault: build shared DB: %v", sharedErr)
	}
	return sharedPath
}

// OpenSharedDB copies the shared, pre-built DB to dstPath and opens it for
// exclusive use by the caller's test. The caller owns the returned *index.DB
// and is responsible for closing it (typically via t.Cleanup).
func OpenSharedDB(tb testing.TB, vaultPath, dstPath string) *index.DB {
	tb.Helper()
	src := SharedIndexDBPath(tb, vaultPath)
	if err := copyFile(src, dstPath); err != nil {
		tb.Fatalf("testvault: copy shared DB: %v", err)
	}
	db, err := index.Open(dstPath)
	if err != nil {
		tb.Fatalf("testvault: open copied DB: %v", err)
	}
	return db
}

func copyFile(src, dst string) error {
	// #nosec G304 -- test-only helper; both paths are controlled by test code.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	// #nosec G304 -- test-only helper; dst is a t.TempDir() path from the caller.
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
