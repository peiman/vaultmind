package importdocs

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// The caps an archive is read under. Past any of them the archive is
// refused and nothing from it is written. Tests lower them.
var (
	// maxArchiveMember is the most one member may write, counted on the
	// bytes actually written, never on the size its header claims.
	maxArchiveMember int64 = 64 << 20
	// maxArchiveTotal is the most a whole archive may write.
	maxArchiveTotal int64 = maxOfficeBytes
	// maxArchiveMembers is the most members an archive may hold.
	maxArchiveMembers = 10000
)

// archiveTempRoot is where an archive's temporary folder is made: the
// system's temporary directory, outside any vault. A test points it
// elsewhere to see the folder removed.
var archiveTempRoot = ""

// archiveKind is "zip" for .zip and "tar" for .tar, .tar.gz and .tgz, the
// suffix of the folder its notes sit in; "" for anything else.
func archiveKind(p string) string {
	lower := strings.ToLower(p)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".tar"), strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar"
	}
	return ""
}

// archiveStem is the archive's name without its extension, .tar.gz whole.
func archiveStem(rel string) string {
	lower := strings.ToLower(rel)
	for _, ext := range []string{".tar.gz", ".tgz", ".tar", ".zip"} {
		if strings.HasSuffix(lower, ext) {
			return rel[:len(rel)-len(ext)]
		}
	}
	return rel
}

// readArchiveDocs imports an archive as the folder it holds: its members
// are written to a temporary folder outside the vault, scanned as a folder
// on disk is, and the folder removed. The notes sit in <name>-zip/ (or
// -tar); each member's source is the archive and the member, its `paths:`
// the archive. Members that cannot be written safely are returned as
// entries; a cap passed refuses the archive.
func readArchiveDocs(src Source, rel, p string) ([]doc, []Entry, error) {
	tmp, err := os.MkdirTemp(archiveTempRoot, "vaultmind-archive-*")
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	skipped, err := extractArchive(p, tmp)
	if err != nil {
		return nil, nil, err
	}
	folder := archiveStem(rel) + "-" + archiveKind(rel)
	inner := Source{Dir: tmp, Repo: src.Repo, Prefix: path.Join(src.Prefix, folder)}
	docs, innerSkips, err := scan(inner, tmp, "")
	if err != nil {
		return nil, nil, err
	}
	archive := src.Repo + ":" + path.Join(src.Prefix, rel)
	innerRoot := src.Repo + ":" + inner.Prefix + "/"
	for i := range docs {
		docs[i].Rel = path.Join(folder, docs[i].Rel)
		docs[i].Source = archive + "#" + strings.TrimPrefix(docs[i].Source, innerRoot)
		docs[i].PathsEntry = archive
	}
	var entries []Entry
	for _, s := range append(skipped, innerSkips...) {
		entries = append(entries, Entry{Action: Skipped, Note: rel + "#" + s.Note, Reason: s.Reason})
	}
	return docs, entries, nil
}

// extractArchive writes an archive's importable members under dir and
// returns the members it would not write, with the reason.
func extractArchive(p, dir string) ([]Entry, error) {
	x := &extractor{dir: dir}
	var err error
	if archiveKind(p) == "zip" {
		err = x.zip(p)
	} else {
		err = x.tar(p)
	}
	return x.skipped, err
}

// extractor is one archive's extraction: where it writes, what it wrote,
// and what it would not.
type extractor struct {
	dir     string
	members int
	written int64
	skipped []Entry
}

func (x *extractor) skip(name, reason string) {
	x.skipped = append(x.skipped, Entry{Note: name, Reason: reason})
}

// count counts one entry of any kind toward maxArchiveMembers: folders and
// links cost the reader too, and bound the skip list.
func (x *extractor) count() error {
	x.members++
	if x.members > maxArchiveMembers {
		return fmt.Errorf("the archive holds more than %d members", maxArchiveMembers)
	}
	return nil
}

func (x *extractor) zip(p string) error {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return fmt.Errorf("not a readable zip file: %w", err)
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if err := x.count(); err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Flags&0x1 != 0 {
			x.skip(f.Name, "encrypted, so it cannot be read")
			continue
		}
		if !f.Mode().IsRegular() {
			x.skip(f.Name, "not a regular file (a link or a device); not followed")
			continue
		}
		if err := x.member(f.Name, func() (io.ReadCloser, error) { return f.Open() }); err != nil {
			return err
		}
	}
	return nil
}

func (x *extractor) tar(p string) error {
	// nosemgrep: go-path-traversal -- a file found by walking the folder the operator named; symlinks were skipped
	f, err := os.Open(p) //nolint:gosec // same
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var r io.Reader = f
	lower := strings.ToLower(p)
	if strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".tgz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("not a readable tar.gz file: %w", err)
		}
		defer func() { _ = gz.Close() }()
		r = gz
	}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("not a readable tar file: %w", err)
		}
		if err := x.count(); err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir, tar.TypeXGlobalHeader, tar.TypeXHeader:
			continue // folders and PAX metadata are not members
		case tar.TypeReg:
		default:
			x.skip(h.Name, "not a regular file (a link or a device); not followed")
			continue
		}
		if err := x.member(h.Name, func() (io.ReadCloser, error) { return io.NopCloser(tr), nil }); err != nil {
			return err
		}
	}
}

// member writes one regular member, if it is one a folder import reads and
// its name is safe. A member past its cap, or the archive past its total,
// refuses the archive.
func (x *extractor) member(name string, open func() (io.ReadCloser, error)) error {
	dest, ok := safeMemberPath(x.dir, name)
	switch {
	case !ok:
		x.skip(name, "an unsafe path (absolute, or climbing out of the archive); not written")
		return nil
	case archiveKind(name) != "":
		x.skip(name, "an archive inside an archive is not opened")
		return nil
	case !importable(name) || hiddenOrDependency(name):
		return nil
	}
	rc, err := open()
	if err != nil {
		x.skip(name, err.Error())
		return nil
	}
	defer func() { _ = rc.Close() }()
	return x.write(name, dest, rc)
}

// write copies one member to dest, counting the bytes as they go.
func (x *extractor) write(name, dest string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		x.skip(name, err.Error())
		return nil
	}
	// nosemgrep: go-path-traversal -- dest is checked by safeMemberPath to stay under the temporary folder
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // same
	if errors.Is(err, fs.ErrExist) {
		x.skip(name, "another member has the same name here (names that differ only in case are one file on this filesystem)")
		return nil
	}
	if err != nil {
		x.skip(name, err.Error())
		return nil
	}
	// Read at most the room left, under the member cap and under the total,
	// and one byte more to see a member run past it: nothing past the total
	// is ever on disk.
	room := min(maxArchiveMember, maxArchiveTotal-x.written)
	n, err := io.Copy(out, io.LimitReader(r, room+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("reading %s from the archive: %w", name, err)
	}
	if n > maxArchiveMember {
		return fmt.Errorf("%s is larger than %d MiB uncompressed", name, maxArchiveMember>>20)
	}
	x.written += n
	if x.written > maxArchiveTotal {
		return fmt.Errorf("the archive holds more than %d MiB to import", maxArchiveTotal>>20)
	}
	return nil
}

// safeMemberPath is where a member is written under dir, and whether its
// name is safe: relative, free of backslashes and control characters, and
// landing strictly under dir once joined and cleaned.
func safeMemberPath(dir, name string) (string, bool) {
	if name == "" || strings.ContainsAny(name, `\`) || hasControl(name) || strings.ContainsRune(name, 0) {
		return "", false
	}
	if path.IsAbs(name) || filepath.IsAbs(name) {
		return "", false
	}
	// Containment covers every climb: ".." and "a/../../b" land above dir,
	// "." lands on dir itself, and neither is under it.
	dest := filepath.Join(dir, filepath.FromSlash(path.Clean(name)))
	if !strings.HasPrefix(dest, dir+string(filepath.Separator)) {
		return "", false
	}
	return dest, true
}

// hiddenOrDependency reports a member under a hidden or dependency folder,
// which a folder import leaves out: not worth writing.
func hiddenOrDependency(name string) bool {
	segs := strings.Split(path.Clean(name), "/")
	for _, s := range segs[:len(segs)-1] {
		if strings.HasPrefix(s, ".") || skippedFolders[s] {
			return true
		}
	}
	return false
}
