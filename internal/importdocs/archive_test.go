package importdocs_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// member is one entry of a test archive.
type member struct {
	name, body string
	symlink    string // a symlink to this target, instead of a file
	encrypted  bool   // zip only: the encrypted flag
}

func zipBytes(t *testing.T, members []member) []byte {
	t.Helper()
	require.NotEmpty(t, members, "an archive needs members")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, m := range members {
		h := &zip.FileHeader{Name: m.name, Method: zip.Deflate}
		body := m.body
		if m.symlink != "" {
			h.SetMode(os.ModeSymlink | 0o777)
			body = m.symlink
		}
		if m.encrypted {
			h.Flags |= 0x1
		}
		w, err := zw.CreateHeader(h)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func tgzBytes(t *testing.T, members []member) []byte {
	t.Helper()
	require.NotEmpty(t, members, "an archive needs members")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		h := &tar.Header{Name: m.name, Mode: 0o644, Size: int64(len(m.body)), Typeflag: tar.TypeReg}
		if m.symlink != "" {
			h = &tar.Header{Name: m.name, Linkname: m.symlink, Typeflag: tar.TypeSymlink, Mode: 0o777}
		}
		require.NoError(t, tw.WriteHeader(h))
		if m.symlink == "" {
			_, err := tw.Write([]byte(m.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// GitHub's release tarballs open with a global PAX header: metadata, not a
// member, and not worth a line in the report.
func TestImport_ATarballsPAXHeaderIsNotReported(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header",
		PAXRecords: map[string]string{"comment": "4f1c2b0"}, Format: tar.FormatPAX}))
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "cobra-1.0/README.md", Mode: 0o644, Size: 9, Typeflag: tar.TypeReg}))
	_, err := tw.Write([]byte("# Cobra\n\n"[:9]))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	write(t, filepath.Join(repo, "docs", "cobra.tar.gz"), buf.String())

	res := run(t, repo, vault, importdocs.Options{})
	assert.NotContains(t, skipReasons(res), "pax_global_header")
	assert.FileExists(t, filepath.Join(vault, "imported/demo-repo/docs/cobra-tar/cobra-1.0/readme.md"))
}

func bundle() []member {
	return []member{
		{name: "bundle/guide/intro.md", body: "# Intro\n\nThe bundle's introduction.\n"},
		{name: "bundle/guide/setup.md", body: "# Setup\n\nHow to set it up.\n"},
		{name: "bundle/data/prices.csv", body: "item,price\ntea,3\n"},
		{name: "bundle/logo.png", body: "PNG"},
		{name: "bundle/.git/config", body: "[core]\n"},
	}
}

func TestImport_AZipImportsLikeAFolderOfItsMembers(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "bundle.zip"), string(zipBytes(t, bundle())))

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 2+3, res.Count(importdocs.Added), "two markdown docs and three importable members")
	fm, body := noteAt(t, vault, "imported/demo-repo/docs/bundle-zip/bundle/guide/intro.md")
	assert.Equal(t, "Intro", fm["title"])
	assert.Equal(t, "demo-repo:docs/bundle.zip#bundle/guide/intro.md", fm["source"])
	assert.Equal(t, []interface{}{"demo-repo:docs/bundle.zip"}, fm["paths"])
	assert.Contains(t, body, "The bundle's introduction.")
	assert.FileExists(t, filepath.Join(vault, "imported/demo-repo/docs/bundle-zip/bundle/data/prices-csv.md"))
	assert.NoDirExists(t, filepath.Join(vault, "imported/demo-repo/docs/bundle-zip/bundle/.git"))

	again := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 5, again.Count(importdocs.Unchanged))
}

func TestImport_ATarGzImportsAndKeepsAnEPUBsChapters(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	epub := filepath.Join(t.TempDir(), "voyage.epub")
	writeEPUB(t, epub, twoChapterBook(true))
	raw, err := os.ReadFile(epub) //nolint:gosec // test path
	require.NoError(t, err)
	members := append(bundle()[:2], member{name: "bundle/books/voyage.epub", body: string(raw)})
	write(t, filepath.Join(repo, "docs", "release.tar.gz"), string(tgzBytes(t, members)))

	run(t, repo, vault, importdocs.Options{})
	dir := "imported/demo-repo/docs/release-tar/bundle/books/voyage-epub/"
	fm, _ := noteAt(t, vault, dir+"01-chapter-one-arrival.md")
	assert.Equal(t, "demo-repo:docs/release.tar.gz#bundle/books/voyage.epub#ch1.xhtml", fm["source"])
	assert.Equal(t, []interface{}{"demo-repo:docs/release.tar.gz"}, fm["paths"])
	_, body := noteAt(t, vault, dir+"book.md")
	assert.Contains(t, body, "[["+dir+"01-chapter-one-arrival|", "the book's links point at where its chapters landed")
}

func TestImport_AMemberGoneFromAnArchiveIsAnOrphanButAnUnreadableArchiveKeepsItsNotes(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	p := filepath.Join(repo, "docs", "bundle.zip")
	write(t, p, string(zipBytes(t, bundle())))
	run(t, repo, vault, importdocs.Options{})

	write(t, p, string(zipBytes(t, bundle()[1:])))
	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, []string{"imported/demo-repo/docs/bundle-zip/bundle/guide/intro.md"}, notesBy(res, importdocs.Orphaned))

	write(t, p, "not a zip")
	res = run(t, repo, vault, importdocs.Options{})
	assert.Empty(t, notesBy(res, importdocs.Orphaned))
	e, ok := entryFor(res, "bundle.zip")
	require.True(t, ok)
	assert.Equal(t, importdocs.Skipped, e.Action)
}

func TestImport_AnArchiveNeverWritesOutsideItsFolderNorFollowsLinks(t *testing.T) {
	for _, kind := range []string{"zip", "tar.gz"} {
		t.Run(kind, func(t *testing.T) {
			repo, vault := srcRepo(t), t.TempDir()
			members := []member{
				{name: "ok.md", body: "# OK\n\nSafe.\n"},
				{name: "../evil.md", body: "# Evil\n"},
				{name: "a/../../evil2.md", body: "# Evil\n"},
				{name: "/etc/evil3.md", body: "# Evil\n"},
				{name: "link.md", symlink: "/etc/passwd"},
				{name: "inner.zip", body: "PK"},
			}
			raw := zipBytes(t, members)
			if kind == "tar.gz" {
				raw = tgzBytes(t, members)
			}
			write(t, filepath.Join(repo, "docs", "box."+kind), string(raw))

			res := run(t, repo, vault, importdocs.Options{})
			assert.Equal(t, 2+1, res.Count(importdocs.Added), "only ok.md from the archive")
			reasons := skipReasons(res)
			for _, want := range []string{"../evil.md — an unsafe path", "a/../../evil2.md — an unsafe path", "/etc/evil3.md — an unsafe path",
				"link.md — not a regular file", "inner.zip — an archive inside an archive is not opened"} {
				assert.Contains(t, reasons, want)
			}
			assert.NoFileExists(t, filepath.Join(repo, "evil.md"))
			assert.NoFileExists(t, filepath.Join(filepath.Dir(repo), "evil2.md"))
		})
	}
}

func TestImport_AnEncryptedZipMemberIsSkipped(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "box.zip"), string(zipBytes(t, []member{
		{name: "open.md", body: "# Open\n"}, {name: "secret.md", body: "garbled", encrypted: true}})))
	res := run(t, repo, vault, importdocs.Options{})
	assert.Contains(t, skipReasons(res), "secret.md — encrypted")
	assert.FileExists(t, filepath.Join(vault, "imported/demo-repo/docs/box-zip/open.md"))
}

func TestImportFile_AnArchiveImportsAloneWithAllItsMembers(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "bundle.zip"), string(zipBytes(t, bundle())))
	res, err := importdocs.ImportFile(source(repo), "bundle.zip", vault, importdocs.Options{})
	require.NoError(t, err)
	assert.Equal(t, 3, res.Count(importdocs.Added))
}
