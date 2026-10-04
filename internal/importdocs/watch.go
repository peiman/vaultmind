package importdocs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

// Fingerprint is a digest of every file the import of src would read: its
// path, size and modification time. It changes when one of those files is
// added, edited or removed, and only then: the walk is the import's own
// (walkImportable), so hidden, dependency and gitignored files never count.
// Nothing is read but the files' metadata.
func Fingerprint(src Source, vaultRoot string) (string, error) {
	_, vaultReal, srcRoot, err := roots(src.Dir, vaultRoot)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = walkImportable(srcRoot, vaultReal, func(rel, p string) ([]Entry, error) {
		// A file gone between the walk and the stat is left out: the next
		// poll sees it gone.
		if info, err := os.Stat(p); err == nil {
			_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\n", rel, info.Size(), info.ModTime().UnixNano())
		}
		return nil, nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// WatchConfig drives WatchLoop. Fingerprint, when set, is polled every
// Interval and Run called once a change has settled; without it Run is
// called every Interval (a URL, which has no cheap fingerprint). Wait
// pauses for an interval, or until the context is done; OnError hears every
// failure, which never ends the loop. A test injects Wait to take no time.
type WatchConfig struct {
	Interval    time.Duration
	Fingerprint func() (string, error)
	Run         func() error
	Wait        func(ctx context.Context, d time.Duration) error
	OnError     func(error)
}

// WatchLoop keeps a vault in step with its source until ctx is done. A
// change is acted on once the fingerprint has held still for one interval:
// an editor's write-then-rename, or a checkout of many files, is one
// re-sync, not one per file.
func WatchLoop(ctx context.Context, c WatchConfig) {
	if c.Wait == nil {
		c.Wait = sleep
	}
	if c.OnError == nil {
		c.OnError = func(error) {}
	}
	if c.Fingerprint == nil {
		for c.Wait(ctx, c.Interval) == nil {
			c.report(c.Run())
		}
		return
	}
	prev, err := c.Fingerprint()
	c.report(err)
	for c.Wait(ctx, c.Interval) == nil {
		cur, err := c.Fingerprint()
		if err != nil || cur == prev {
			c.report(err)
			continue
		}
		settled, ok := c.settle(ctx, cur)
		if !ok {
			continue
		}
		c.report(c.Run())
		prev = settled
	}
}

// settle waits until the fingerprint holds still for one interval and
// returns it. ok is false when the context ended or a poll failed first.
func (c WatchConfig) settle(ctx context.Context, cur string) (string, bool) {
	for {
		if c.Wait(ctx, c.Interval) != nil {
			return "", false
		}
		next, err := c.Fingerprint()
		if err != nil {
			c.report(err)
			return "", false
		}
		if next == cur {
			return cur, true
		}
		cur = next
	}
}

func (c WatchConfig) report(err error) {
	if err != nil {
		c.OnError(err)
	}
}
