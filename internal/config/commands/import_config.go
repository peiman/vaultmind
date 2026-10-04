package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ImportMetadata defines the `import` command — a folder of docs as notes.
var ImportMetadata = config.CommandMetadata{
	Use:   "import <dir|file|url>",
	Short: "Import a folder or archive of docs (markdown, PDF, Office, HTML, CSV, EPUB, images), one file, one web page or PDF, or a whole site, as notes and keep them in step",
	Long: "Copy every *.md under <dir> into the vault as a note under imported/<repo>/, so " +
		"a project's existing docs are found by search, ask and the code hooks like any " +
		"other note. Each *.pdf in the folder becomes a note of its text, <name>-pdf.md, and " +
		"each Word, PowerPoint or Excel file (.docx, .pptx, .xlsx) a markdown note of its " +
		"headings, slides or sheets, <name>-docx.md and so on. A saved web page or exported " +
		"HTML (.html, .htm) becomes the article it holds, read as a fetched page is, " +
		"<name>-html.md; a CSV or TSV becomes a table like an Excel sheet (the first 200 rows " +
		"and 50 columns, its delimiter and encoding detected), <name>-csv.md. An EPUB book " +
		"becomes a folder <name>-epub/ with a note per chapter, in reading order and titled " +
		"from its table of contents (a file holding several long chapters is split at them), " +
		"and book.md listing them; a book whose chapters are DRM-encrypted is skipped. A zip or " +
		"tar archive (.zip, .tar, .tar.gz, .tgz) imports like the folder it holds, into " +
		"<name>-zip/ or <name>-tar/: its members are unpacked to a temporary folder outside the " +
		"vault (unsafe paths, links and encrypted members are skipped, archives inside it are " +
		"not opened, and an archive over 512 MiB unpacked or 64 MiB in one member is refused) " +
		"and imported as files on disk are. An image (.png, .jpg, .jpeg, .webp, .gif, .heic, " +
		".heif) becomes <name>-png.md and so on, holding the text tesseract reads in it (when " +
		"tesseract is installed; results are cached, icons under 100 pixels are not read), the alt " +
		"text the import's markdown gives it, and its metadata, GPS included; an SVG becomes " +
		"the words it shows. An image with nothing but its pixels is counted, not written. A file that " +
		"cannot be read (a PDF scan, a broken file, a PDF over 20 MiB, an HTML file over 5 MiB " +
		"or an Office or CSV file over 512 MiB) is skipped " +
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
		"A URL import fetches exactly the page you name and nothing else, unless --crawl is " +
		"given. Only http and https are accepted. The body must be HTML or text of at most " +
		"5 MiB, or a PDF of at most 20 MiB; the request times out after 30 seconds, and at " +
		"most five redirects are followed, only when they stay on http or https. Scripts on " +
		"the page are not run. A PDF's text is read by pdfium running in a WebAssembly " +
		"sandbox, for at most 60 seconds; a PDF with no text layer (a scan) is refused. " +
		"The first PDF import compiles pdfium once (a few seconds) and caches it. A URL that " +
		"names a public host never reaches a private address (localhost, 10.x, 192.168.x, " +
		"the cloud metadata address), not through a redirect, its DNS answer or a proxy; a URL " +
		"that names a private host is your choice, and is fetched.\n\n" +
		"--crawl imports the site the URL leads to, one note per page, each the note a " +
		"single-page import of that page would write. Pages come from the site's llms.txt, " +
		"its sitemaps (from robots.txt, else /sitemap.xml), then the links on each page, " +
		"breadth first. The crawl stays on the URL's host, in the URL's folder (/docs/intro " +
		"and /docs/ both mean /docs/), narrowed by --include and --exclude globs on the URL " +
		"path. Links to images, scripts, archives and other files that are not pages are not " +
		"followed. One request is made at a time, --delay apart (default 1s); --max-pages " +
		"(default 100) and --depth (link hops, default 5) bound it. robots.txt is read for " +
		"its sitemaps but not obeyed unless --respect-robots is given, which also honours " +
		"its Crawl-delay (up to 30s). A page that fails is skipped with the reason and the " +
		"crawl goes on. Pages the crawl no longer finds are reported as orphaned only when " +
		"it saw the whole site: no page failed and no cap stopped it.\n\n" +
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
		"  vaultmind import https://arxiv.org/pdf/2402.03216 --vault ./kb\n" +
		"  vaultmind import https://docusaurus.io/docs --crawl --vault ./kb\n" +
		"  vaultmind import https://example.com/docs/ --crawl --exclude 'docs/v1/**' --max-pages 300 --vault ./kb",
	ConfigPrefix: "app.import",
	FlagOverrides: map[string]string{
		"app.import.vault":          "vault",
		"app.import.dry_run":        "dry-run",
		"app.import.prune":          "prune",
		"app.import.force":          "force",
		"app.import.json":           "json",
		"app.import.crawl":          "crawl",
		"app.import.max_pages":      "max-pages",
		"app.import.depth":          "depth",
		"app.import.include":        "include",
		"app.import.exclude":        "exclude",
		"app.import.respect_robots": "respect-robots",
		"app.import.delay":          "delay",
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
		{Key: "app.import.crawl", DefaultValue: false, Description: "Import the site a URL leads to, one note per page", Type: "bool"},
		{Key: "app.import.max_pages", DefaultValue: 100, Description: "Most pages a crawl fetches", Type: "int"},
		{Key: "app.import.depth", DefaultValue: 5, Description: "Most link hops a crawl follows from the URL (0: the URL alone)", Type: "int"},
		{Key: "app.import.include", DefaultValue: []string{}, Description: "Crawl only URL paths matching one of these globs", Type: "[]string"},
		{Key: "app.import.exclude", DefaultValue: []string{}, Description: "Never crawl URL paths matching these globs", Type: "[]string"},
		{Key: "app.import.respect_robots", DefaultValue: false, Description: "Obey robots.txt rules and Crawl-delay when crawling", Type: "bool"},
		{Key: "app.import.delay", DefaultValue: "1s", Description: "Pause between a crawl's requests", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(ImportOptions)
}
