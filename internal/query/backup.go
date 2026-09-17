package query

import (
	"errors"
	"fmt"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Backup health — is this vault's work anywhere but this machine?
//
// WHY. A vault is the one artifact in this system that cannot be rebuilt. The
// index can be regenerated, the binary reinstalled, the embeddings recomputed;
// the arcs cannot. On 2026-09-17 this repo's own vault was found 42 commits and
// 28 days from its remote — not because backup had never been set up, but
// because the local clone had no `origin` at all while carrying an unrelated
// remote from a scaffold. Everything looked normal and nothing was leaving the
// laptop.
//
// The check therefore asks the question that actually failed: does the current
// branch have an UPSTREAM, and how far ahead of it are we? "A remote exists" is
// not the same question, and it is the one that would have answered yes.

// Backup states, most severe first.
const (
	// BackupStateNoRepo — the vault is not inside a git repository at all.
	BackupStateNoRepo = "no_repo"
	// BackupStateNoUpstream — versioned, but the branch tracks nothing. Every
	// commit exists on exactly one disk.
	BackupStateNoUpstream = "no_upstream"
	// BackupStateBehind — an upstream exists and local work has not reached it.
	BackupStateBehind = "behind"
	// BackupStateCurrent — the upstream has everything local has.
	BackupStateCurrent = "current"
)

// Backup warnings. Phrased as what is AT RISK, not as what is misconfigured:
// the reader needs to know what they lose, not which setting is unset.
const (
	WarnBackupNoRepo = "this vault is not in a git repository — nothing is versioning it, " +
		"and an accidental delete is unrecoverable"
	WarnBackupNoUpstream = "this vault's branch tracks no remote — every commit exists only on this machine. " +
		"Set one with: git remote add origin <url> && git push -u origin <branch>"
	WarnBackupBehindFmt = "%d commit(s) have never left this machine, the oldest %d day(s) ago — " +
		"push, or accept losing that work if the disk does"
)

// BackupStaleDays is when "behind" becomes worth saying out loud.
//
// Zero unpushed commits is silent, and a handful from today is normal working
// state. The failure this guards against is the SLOW one: the remote quietly
// stops receiving and nobody notices for weeks.
const BackupStaleDays = 3

// DoctorBackup reports whether the vault's work exists anywhere else.
type DoctorBackup struct {
	State string `json:"state"`
	// Upstream is the tracked remote branch ("origin/main"), empty when none.
	Upstream string `json:"upstream,omitempty"`
	// Unpushed counts commits the upstream does not have.
	Unpushed int `json:"unpushed"`
	// OldestUnpushedDays is the age of the oldest such commit. The COUNT alone
	// under-reports the risk: 3 commits from this hour and 3 from last month
	// are very different amounts of unrecoverable work.
	OldestUnpushedDays int      `json:"oldest_unpushed_days"`
	Warnings           []string `json:"warnings,omitempty"`
}

// AtRisk reports whether anything would be lost with this disk.
func (b *DoctorBackup) AtRisk() bool {
	return b != nil && b.State != BackupStateCurrent
}

// CheckBackup inspects the git repository containing vaultPath.
//
// A vault that is not in a repository is reported, not skipped: "no answer" and
// "the answer is fine" must not look alike, which is the whole reason this
// check exists.
func CheckBackup(vaultPath string, now time.Time) (*DoctorBackup, error) {
	repo, err := gogit.PlainOpenWithOptions(vaultPath, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		if errors.Is(err, gogit.ErrRepositoryNotExists) {
			return &DoctorBackup{State: BackupStateNoRepo, Warnings: []string{WarnBackupNoRepo}}, nil
		}
		return nil, fmt.Errorf("backup check: opening repository for %q: %w", vaultPath, err)
	}
	return backupStateOf(repo, now)
}

// backupStateOf resolves the upstream and measures the gap.
//
// Split out so the three "no upstream" paths return a VERDICT rather than a
// nil error beside a handled failure: git errors here are states to report,
// and a function that returns (result, nil) after checking err reads — to a
// linter and to a person — like a swallowed failure.
func backupStateOf(repo *gogit.Repository, now time.Time) (*DoctorBackup, error) {
	// An unborn branch (no commits yet) is a young repo, not a failure — and
	// also not backed up, so it reports as such rather than as OK.
	head := headRef(repo)
	if head == nil {
		return unbackedUp(""), nil
	}

	upstreamRef, ok := upstreamOf(repo, head.Name())
	if !ok {
		return unbackedUp(""), nil
	}

	// Configured but never fetched — the remote-tracking ref does not exist.
	// Practically identical: nothing here is known to be anywhere else.
	upstreamHash := resolveUpstream(repo, upstreamRef)
	if upstreamHash == nil {
		return unbackedUp(upstreamRef), nil
	}

	count, oldest, err := commitsAhead(repo, head.Hash(), *upstreamHash)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return &DoctorBackup{State: BackupStateCurrent, Upstream: upstreamRef}, nil
	}

	days := int(now.Sub(oldest).Hours() / 24)
	if days < 0 {
		days = 0
	}
	b := &DoctorBackup{
		State:              BackupStateBehind,
		Upstream:           upstreamRef,
		Unpushed:           count,
		OldestUnpushedDays: days,
	}
	if days >= BackupStaleDays {
		b.Warnings = append(b.Warnings, fmt.Sprintf(WarnBackupBehindFmt, count, days))
	}
	return b, nil
}

// resolveUpstream returns the upstream hash, or nil when the tracking ref does
// not exist locally. Absence is the answer, not an error.
func resolveUpstream(repo *gogit.Repository, upstreamRef string) *plumbing.Hash {
	h, err := repo.ResolveRevision(plumbing.Revision(upstreamRef))
	if err != nil {
		return nil
	}
	return h
}

// upstreamOf resolves the remote-tracking ref for a branch, e.g. "origin/main".
func upstreamOf(repo *gogit.Repository, ref plumbing.ReferenceName) (string, bool) {
	if !ref.IsBranch() {
		return "", false // detached HEAD tracks nothing
	}
	cfg, err := repo.Config()
	if err != nil {
		return "", false
	}
	branch, ok := cfg.Branches[ref.Short()]
	if !ok || branch.Remote == "" {
		return "", false
	}
	return branch.Remote + "/" + ref.Short(), true
}

// commitsAhead counts commits reachable from head but not from upstream, and
// returns the oldest one's time.
func commitsAhead(repo *gogit.Repository, head, upstream plumbing.Hash) (int, time.Time, error) {
	seen := map[plumbing.Hash]bool{}
	upstreamIter, err := repo.Log(&gogit.LogOptions{From: upstream})
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("backup check: reading upstream history: %w", err)
	}
	if err := upstreamIter.ForEach(func(c *object.Commit) error {
		seen[c.Hash] = true
		return nil
	}); err != nil {
		return 0, time.Time{}, fmt.Errorf("backup check: walking upstream history: %w", err)
	}

	headIter, err := repo.Log(&gogit.LogOptions{From: head})
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("backup check: reading local history: %w", err)
	}
	var count int
	oldest := time.Time{}
	if err := headIter.ForEach(func(c *object.Commit) error {
		if seen[c.Hash] {
			return nil
		}
		count++
		when := c.Committer.When
		if oldest.IsZero() || when.Before(oldest) {
			oldest = when
		}
		return nil
	}); err != nil {
		return 0, time.Time{}, fmt.Errorf("backup check: walking local history: %w", err)
	}
	return count, oldest, nil
}

// unbackedUp is the shared "this exists on one disk" verdict. One constructor
// so the three ways of getting there cannot drift into three different
// messages about the same risk.
func unbackedUp(upstream string) *DoctorBackup {
	return &DoctorBackup{
		State:    BackupStateNoUpstream,
		Upstream: upstream,
		Warnings: []string{WarnBackupNoUpstream},
	}
}

// headRef returns HEAD, or nil when the branch is unborn. Absence is a state
// this check reports, not a failure it raises.
func headRef(repo *gogit.Repository) *plumbing.Reference {
	ref, err := repo.Head()
	if err != nil {
		return nil
	}
	return ref
}
