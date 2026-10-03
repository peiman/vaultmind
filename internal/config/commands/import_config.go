package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ImportMetadata defines the `import` command — a folder of docs as notes.
var ImportMetadata = config.CommandMetadata{
	Use:   "import <dir|file|url>",
	Short: "Import a folder of docs, PDFs and Office files, one file, or one web page or PDF, as notes and keep them in step",
	Long: "Copy every *.md under <dir> into the vault as a note under imported/<repo>/, so " +
		"a project's existing docs are found by search, ask and the code hooks like any " +
		"other note. Each *.pdf in the folder becomes a note of its text, <name>-pdf.md, and " +
		"each Word, PowerPoint or Excel file (.docx, .pptx, .xlsx) a markdown note of its " +
		"headings, slides or sheets, <name>-docx.md and so on. A file that cannot be read (a " +
		"PDF scan, a broken file, a PDF over 20 MiB or an Office file over 512 MiB) is skipped " +
		"with the reason and the rest still imports. Inside a git work tree, what git ignores " +
		"is left out (naming an ignored folder imports it). Pass one such file instead and " +
		"only that " +
		"file is imported, named as importing its folder would name it; the folder's other " +
		"notes are left alone. Pass an http or https URL instead of a folder and that one page " +
		"becomes a note under imported/web/<host>/. A URL that serves a PDF becomes a note " +
		"holding the PDF's text, titled from its metadata or its first line.\n\n" +
		"Each folder note records its doc in `paths:` — reading or editing the doc brings the " +
		"note — and in `source:` and `source_hash:`, which is how a re-run knows what " +
		"changed. A page note records the URL you gave in `url:` and `source:` and has no " +
		"`paths:`. The doc stays the source of truth: edit the doc, then re-run. The page " +
		"is the source of a URL note: re-run the import to refresh it.\n\n" +
		"A re-run adds new docs, rewrites changed ones and leaves the rest alone. Nothing " +
		"is deleted without asking: a note whose doc is gone is reported as orphaned and " +
		"removed only with --prune. A note edited by hand whose doc or page also changed is " +
		"reported as a conflict and kept; --force overwrites it. Frontmatter you add to " +
		"an imported note (tags, links) survives a re-run.\n\n" +
		"A URL import fetches exactly the page you name and nothing else. Only http and " +
		"https are accepted, one page per run. The body must be HTML or text of at most " +
		"5 MiB, or a PDF of at most 20 MiB; the request times out after 30 seconds, and at " +
		"most five redirects are followed, only when they stay on http or https. Scripts on " +
		"the page are not run. A PDF's text is read by pdfium running in a WebAssembly " +
		"sandbox, for at most 60 seconds; a PDF with no text layer (a scan) is refused. " +
		"The first PDF import compiles pdfium once (a few seconds) and caches it.\n\n" +
		"Left out of a folder import: hidden folders, dependency folders (node_modules, vendor), symlinks, " +
		"and anything the vault's exclude list would hide from the index. A README is " +
		"imported as readme.md, since vaults exclude README.md as their own meta file. " +
		"A repository is known by its name (its origin remote's, else its folder's; " +
		"outside git, the folder's), so two repositories or folders of the same name " +
		"share imported/<name>/. Names that differ only in case are one note.\n\n" +
		"  vaultmind import docs --vault ./knowledge --dry-run\n" +
		"  vaultmind import docs --vault ./knowledge\n" +
		"  vaultmind import docs --vault ./knowledge --prune --json\n" +
		"  vaultmind import papers/attention.pdf --vault ./knowledge\n" +
		"  vaultmind import https://sqlite.org/fts5.html --vault ./kb\n" +
		"  vaultmind import https://arxiv.org/pdf/2402.03216 --vault ./kb",
	ConfigPrefix: "app.import",
	FlagOverrides: map[string]string{
		"app.import.vault":   "vault",
		"app.import.dry_run": "dry-run",
		"app.import.prune":   "prune",
		"app.import.force":   "force",
		"app.import.json":    "json",
	},
}

// ImportOptions returns configuration options for `import`.
func ImportOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.import.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.import.dry_run", DefaultValue: false, Description: "Report what the import would do and write nothing", Type: "bool"},
		{Key: "app.import.prune", DefaultValue: false, Description: "Remove imported notes whose doc is gone", Type: "bool"},
		{Key: "app.import.force", DefaultValue: false, Description: "Overwrite imported notes edited by hand when their doc changed", Type: "bool"},
		{Key: "app.import.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(ImportOptions)
}
