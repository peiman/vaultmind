// Package personahealth measures what identity and arc text reached a keeper's
// current context after the latest reconstruction point.
package personahealth

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/vault"
	"gopkg.in/yaml.v3"
)

// Keeper identifies one transcript stream and its identity vault.
type Keeper struct {
	Name        string `yaml:"name" json:"name"`
	Transcripts string `yaml:"transcripts" json:"transcripts"`
	Vault       string `yaml:"vault" json:"vault"`
	Anchor      string `yaml:"anchor,omitempty" json:"anchor,omitempty"`
}

// IdentityReport reports body-byte coverage after whitespace normalization.
type IdentityReport struct {
	Coverage     float64 `json:"coverage"`
	CoveredBytes int     `json:"covered_bytes"`
	TotalBytes   int     `json:"total_bytes"`
}

// ArcBodyReport reports normalized sentence-unit coverage for one arc body.
type ArcBodyReport struct {
	ID           string             `json:"id"`
	BodyBytes    int                `json:"body_bytes"`
	ArrivedBytes int                `json:"arrived_bytes"`
	Sections     []ArcSectionReport `json:"sections"`
}

// ArcSectionReport measures one present Trigger, Push, or Principle section.
// Arcs without any of those sections contribute nothing to moment totals.
type ArcSectionReport struct {
	Name         string  `json:"name"`
	Bytes        int     `json:"bytes"`
	ArrivedBytes int     `json:"arrived_bytes"`
	Coverage     float64 `json:"coverage"`
}

// ArcReport counts complete, partial, and absent Principle sections and reports
// coverage of every arc body, including the exchanges around its Principle.
type ArcReport struct {
	Full               int             `json:"full"`
	Partial            int             `json:"partial"`
	Missing            int             `json:"missing"`
	BodyBytes          int             `json:"body_bytes"`
	ArrivedBytes       int             `json:"arrived_bytes"`
	Coverage           float64         `json:"coverage"`
	MomentBytes        int             `json:"moment_bytes"`
	MomentArrivedBytes int             `json:"moment_arrived_bytes"`
	MomentCoverage     float64         `json:"moment_coverage"`
	Items              []ArcBodyReport `json:"items"`
}

// MemoryFileReport records one loaded memory file from an instructions attachment.
type MemoryFileReport struct {
	Path  string `json:"path"`
	Type  string `json:"type"`
	Lines int    `json:"lines"`
}

// MemoryReport records whether an AutoMem file was seen after the current
// reload point, its line-count notice when present, and the loaded files.
type MemoryReport struct {
	Seen      bool `json:"seen"`
	Truncated bool `json:"truncated"`
	CutLines  int  `json:"cut_lines,omitempty"`
	// NoticeProven says whether the truncation check has been confirmed against
	// a real Claude Code record. It remains present for JSON compatibility.
	NoticeProven bool               `json:"notice_proven"`
	LoadedLines  int                `json:"loaded_lines,omitempty"`
	TotalLines   int                `json:"total_lines,omitempty"`
	Notice       string             `json:"notice,omitempty"`
	Files        []MemoryFileReport `json:"files"`
}

// Report is the stable per-keeper health result.
type Report struct {
	Name            string         `json:"name"`
	Status          string         `json:"status"`
	Identity        IdentityReport `json:"identity"`
	Arcs            ArcReport      `json:"arcs"`
	Memory          MemoryReport   `json:"memory"`
	UnreadableLines int            `json:"unreadable_lines"`
	AfterCompact    string         `json:"after_compact,omitempty"`
	Error           string         `json:"error,omitempty"`
	Warning         string         `json:"warning,omitempty"`
}

// Result is the multi-keeper JSON result.
type Result struct {
	Keepers []Report `json:"keepers"`
}

// Config selects the keepers and the minimum acceptable coverage thresholds.
type Config struct {
	Keepers           []Keeper
	MinCoverage       float64
	MinArcCoverage    float64
	MinMomentCoverage float64
}

// ValidateMinCoverage rejects a threshold that cannot be a fraction.
func ValidateMinCoverage(value float64) error {
	if value < 0 || value > 1 {
		return fmt.Errorf("--min-coverage must be between 0 and 1")
	}
	return nil
}

// ValidateMinArcCoverage rejects a threshold that cannot be a fraction.
func ValidateMinArcCoverage(value float64) error {
	if value < 0 || value > 1 {
		return fmt.Errorf("--min-arc-coverage must be between 0 and 1")
	}
	return nil
}

// ValidateMinMomentCoverage rejects a threshold that cannot be a fraction.
func ValidateMinMomentCoverage(value float64) error {
	if value < 0 || value > 1 {
		return fmt.Errorf("--min-moment-coverage must be between 0 and 1")
	}
	return nil
}

// memoryTruncationPattern is from a real transcript record verified on
// 2026-11-25. Only a blockquote warning line containing "Only part of it was
// loaded:" is a notice; the optional captures are cut lines and total lines.
var memoryTruncationPattern = regexp.MustCompile(`(?im)^> WARNING:[^\r\n]*Only part of it was loaded[:.](?:[^\r\n]*?\b([0-9]+)\s+of\s+([0-9]+)\s+lines\s+were\s+cut\s+off\b)?[^\r\n]*$`)

// memoryNoticeProven records that the notice's wording and location are
// confirmed against a real Claude Code record.
const memoryNoticeProven = true

// memoryFileTypes records file types observed in Claude Code's
// instructions attachments; AutoMem is the currently observed memory type.
var memoryFileTypes = [...]string{"AutoMem"}

var arcBlockHeader = regexp.MustCompile(`^── ([^—\r\n]+?)(?: \(part [0-9]+ of [0-9]+\))? — .*$`)

type transcriptEvidence struct {
	Texts           []string
	ArcBlocks       map[string][]string
	AfterCompact    string
	PointPrefix     string
	Memory          MemoryReport
	UnreadableLines int
}

type transcriptFile struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type transcriptAttachment struct {
	Type      string           `json:"type"`
	HookEvent string           `json:"hookEvent"`
	Content   json.RawMessage  `json:"content"`
	Files     []transcriptFile `json:"files"`
}

type transcriptRecord struct {
	Type       string                `json:"type"`
	Subtype    string                `json:"subtype"`
	Timestamp  string                `json:"timestamp"`
	Attachment *transcriptAttachment `json:"attachment"`
}

// CheckKeeper reads only the newest transcript and returns a per-keeper
// failure instead of hiding unreadable inputs behind a process error.
func CheckKeeper(keeper Keeper, minCoverage float64) Report {
	return checkKeeper(keeper, minCoverage, 0, 0)
}

func checkKeeper(keeper Keeper, minCoverage, minArcCoverage, minMomentCoverage float64) Report {
	report := Report{Name: keeper.Name, Status: "fail"}
	evidence, err := readTranscript(keeper.Transcripts)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	report.AfterCompact = evidence.AfterCompact
	report.Memory = evidence.Memory
	report.UnreadableLines = evidence.UnreadableLines

	anchorBody, arcs, err := readVault(keeper.Vault, keeper.Anchor)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	report.Identity = coverage(anchorBody, evidence.Texts)
	report.Arcs = arcCoverage(arcs, evidence.Texts, evidence.ArcBlocks)

	var reasons []string
	if len(evidence.Texts) == 0 {
		reasons = append(reasons, fmt.Sprintf("NO RELOAD %s at %s", evidence.PointPrefix, evidence.AfterCompact))
	}
	if report.Identity.Coverage < minCoverage {
		reasons = append(reasons, fmt.Sprintf("identity coverage %.0f%% below %.0f%%", report.Identity.Coverage*100, minCoverage*100))
	}
	if report.Memory.Truncated {
		reasons = append(reasons, memoryTruncationReason(report.Memory))
	}
	var warnings []string
	if minArcCoverage > 0 && report.Arcs.Coverage < minArcCoverage {
		warnings = append(warnings, fmt.Sprintf("arc body coverage %d%% below %.0f%%", arcCoveragePercentTotal(report.Arcs), minArcCoverage*100))
	}
	if minMomentCoverage > 0 && report.Arcs.MomentCoverage < minMomentCoverage {
		warnings = append(warnings, fmt.Sprintf("arc moment coverage %d%% below %.0f%%", arcMomentCoveragePercentTotal(report.Arcs), minMomentCoverage*100))
	}
	if !report.Memory.Seen {
		warnings = append(warnings, fmt.Sprintf("memory file not seen %s at %s", evidence.PointPrefix, evidence.AfterCompact))
	}
	report.Warning = strings.Join(warnings, "; ")
	report.Error = strings.Join(reasons, "; ")
	switch {
	case len(reasons) > 0:
		report.Status = "fail"
	case report.Warning != "" || report.Arcs.Partial > 0 || report.Arcs.Missing > 0:
		report.Status = "warn"
	default:
		report.Status = "ok"
	}
	return report
}

// Check evaluates every keeper in order and preserves a report for each one.
func Check(cfg Config) Result {
	result := Result{Keepers: make([]Report, 0, len(cfg.Keepers))}
	for _, keeper := range cfg.Keepers {
		result.Keepers = append(result.Keepers, checkKeeper(keeper, cfg.MinCoverage, cfg.MinArcCoverage, cfg.MinMomentCoverage))
	}
	return result
}

// KeepersFromFlags enforces the mutually exclusive single-keeper and YAML
// forms before any transcript or vault is opened.
func KeepersFromFlags(name, transcripts, root, anchor, keepersFile string) ([]Keeper, error) {
	if keepersFile != "" {
		if name != "" || transcripts != "" || root != "" || anchor != "" {
			return nil, fmt.Errorf("--keepers cannot be combined with --name, --transcripts, --vault, or --anchor")
		}
		return LoadKeepers(keepersFile)
	}
	if transcripts == "" || root == "" {
		return nil, fmt.Errorf("provide exactly one form: --transcripts and --vault, or --keepers")
	}
	if name == "" {
		name = filepath.Base(filepath.Clean(transcripts))
	}
	return []Keeper{{Name: name, Transcripts: transcripts, Vault: root, Anchor: anchor}}, nil
}

// Text renders one line per keeper for the command layer.
func Text(result Result) string {
	var b strings.Builder
	for _, report := range result.Keepers {
		status := strings.ToUpper(report.Status)
		memory := "not seen"
		if report.Memory.Seen && report.Memory.Truncated {
			if report.Memory.TotalLines == 0 {
				memory = "truncated (numbers unknown)"
			} else {
				memory = fmt.Sprintf("truncated (loaded %d of %d lines, %d cut off)", report.Memory.LoadedLines, report.Memory.TotalLines, report.Memory.CutLines)
			}
		} else if report.Memory.Seen {
			memory = "ok"
		}
		fmt.Fprintf(&b, "%s  %-4s  identity %.0f%% (%s of %s bytes)  arcs %d full / %d partial / %d missing  arcs body %s of %s bytes, %d%%  arcs moment %d%%  memory %s  after compact %s",
			report.Name, status, report.Identity.Coverage*100, formatNumber(report.Identity.CoveredBytes), formatNumber(report.Identity.TotalBytes), report.Arcs.Full, report.Arcs.Partial, report.Arcs.Missing, formatNumber(report.Arcs.ArrivedBytes), formatNumber(report.Arcs.BodyBytes), arcCoveragePercentTotal(report.Arcs), arcMomentCoveragePercentTotal(report.Arcs), memory, report.AfterCompact)
		if report.UnreadableLines > 0 {
			lineWord := "line"
			if report.UnreadableLines != 1 {
				lineWord += "s"
			}
			fmt.Fprintf(&b, "  %d unreadable %s", report.UnreadableLines, lineWord)
		}
		if report.Error != "" {
			b.WriteString("  ")
			b.WriteString(report.Error)
		}
		if report.Warning != "" {
			b.WriteString("  ")
			b.WriteString(report.Warning)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func formatNumber(value int) string {
	text := strconv.Itoa(value)
	for i := len(text) - 3; i > 0; i -= 3 {
		text = text[:i] + "," + text[i:]
	}
	return text
}

// LoadKeepers loads the exact list form accepted by --keepers.
func LoadKeepers(path string) ([]Keeper, error) {
	data, err := os.ReadFile(path) //nolint:gosec // nosemgrep: go-path-traversal -- the operator names the keepers file; it is only read
	if err != nil {
		return nil, fmt.Errorf("reading keepers file %q: %w", path, err)
	}
	var keepers []Keeper
	if err := yaml.Unmarshal(data, &keepers); err != nil {
		return nil, fmt.Errorf("parsing keepers file %q: %w", path, err)
	}
	if len(keepers) == 0 {
		return nil, fmt.Errorf("keepers file %q contains no keepers", path)
	}
	for i := range keepers {
		if keepers[i].Name == "" || keepers[i].Transcripts == "" || keepers[i].Vault == "" {
			return nil, fmt.Errorf("keepers file %q entry %d needs name, transcripts, and vault", path, i+1)
		}
	}
	return keepers, nil
}

func readTranscript(dir string) (transcriptEvidence, error) {
	if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() && filepath.Ext(dir) == ".jsonl" {
		return readTranscriptFile(dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return transcriptEvidence{}, fmt.Errorf("transcript directory %q: %w", dir, err)
	}
	var newest os.DirEntry
	var newestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return transcriptEvidence{}, fmt.Errorf("transcript %q: %w", entry.Name(), statErr)
		}
		if newest == nil || info.ModTime().After(newestTime) {
			newest, newestTime = entry, info.ModTime()
		}
	}
	if newest == nil {
		return transcriptEvidence{}, fmt.Errorf("transcript directory %q contains no .jsonl file", dir)
	}

	return readTranscriptFile(filepath.Join(dir, newest.Name()))
}

func readTranscriptFile(path string) (transcriptEvidence, error) {
	file, err := os.Open(path) //nolint:gosec // nosemgrep: go-path-traversal -- path is a transcript found by listing the operator-named directory; read-only stream
	if err != nil {
		return transcriptEvidence{}, fmt.Errorf("transcript %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	var result = transcriptEvidence{ArcBlocks: make(map[string][]string), Memory: emptyMemoryReport()}
	seenPoint := false
	compactSeen := false
	validRecords := 0
	resetBoundary := func(timestamp string) {
		compactSeen = true
		seenPoint = true
		result.AfterCompact = formatPoint(timestamp)
		result.PointPrefix = "after compaction"
		result.Texts = nil
		result.ArcBlocks = make(map[string][]string)
		result.Memory = emptyMemoryReport()
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var object map[string]json.RawMessage
		if err := json.Unmarshal(line, &object); err != nil || object == nil {
			result.UnreadableLines++
			if strings.Contains(string(line), "compact_boundary") {
				resetBoundary("")
			}
			continue
		}
		var record transcriptRecord
		if err := json.Unmarshal(line, &record); err != nil {
			result.UnreadableLines++
			if strings.Contains(string(line), "compact_boundary") {
				resetBoundary("")
			}
			continue
		}
		validRecords++
		if record.Type == "system" && record.Subtype == "compact_boundary" {
			resetBoundary(record.Timestamp)
			continue
		}
		if record.Type == "attachment" && record.Attachment != nil && record.Attachment.Type == "instructions" {
			memory := memoryReportFromFiles(record.Attachment.Files)
			if memory.Seen {
				result.Memory = memory
			}
		}
		if !seenPoint {
			seenPoint = true
			result.AfterCompact = formatPoint(record.Timestamp)
			result.PointPrefix = "after session start"
		}
		content := attachmentContent(record.Attachment)
		if record.Attachment != nil && record.Attachment.Type == "hook_success" && record.Attachment.HookEvent == "SessionStart" && content != "" {
			result.Texts = append(result.Texts, content)
			for id, blocks := range arcBlocks(content) {
				result.ArcBlocks[id] = append(result.ArcBlocks[id], blocks...)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return transcriptEvidence{}, fmt.Errorf("transcript %q: reading: %w", path, err)
	}
	if validRecords == 0 {
		return transcriptEvidence{}, fmt.Errorf("transcript %q contains no valid records", path)
	}
	if !compactSeen && result.AfterCompact == "" {
		result.AfterCompact = formatPoint("")
		result.PointPrefix = "after session start"
	}
	return result, nil
}

type memoryNotice struct {
	Text        string
	LoadedLines int
	TotalLines  int
	CutLines    int
}

func emptyMemoryReport() MemoryReport {
	return MemoryReport{NoticeProven: memoryNoticeProven, Files: make([]MemoryFileReport, 0)}
}

func memoryReportFromFiles(files []transcriptFile) MemoryReport {
	result := emptyMemoryReport()
	for _, file := range files {
		if !isMemoryFileType(file.Type) {
			continue
		}
		result.Seen = true
		result.Files = append(result.Files, MemoryFileReport{Path: file.Path, Type: file.Type, Lines: lineCount(file.Content)})
		if match := memoryNoticeInContent(file.Content); match != nil {
			result.Truncated = true
			result.LoadedLines = match.LoadedLines
			result.TotalLines = match.TotalLines
			result.CutLines = match.CutLines
			result.Notice = match.Text
		}
	}
	return result
}

func isMemoryFileType(fileType string) bool {
	for _, memoryFileType := range memoryFileTypes {
		if fileType == memoryFileType {
			return true
		}
	}
	return false
}

// lineCount counts newline-separated segments without trimming content, so an
// empty file is one segment and a trailing newline contributes an empty final
// segment.
func lineCount(content string) int { return len(strings.Split(content, "\n")) }

func memoryNoticeInContent(content string) *memoryNotice {
	match := memoryTruncationPattern.FindStringSubmatch(content)
	if match == nil {
		return nil
	}
	if match[1] == "" || match[2] == "" {
		return &memoryNotice{Text: match[0]}
	}
	cut, _ := strconv.Atoi(match[1])
	total, _ := strconv.Atoi(match[2])
	return &memoryNotice{Text: match[0], LoadedLines: total - cut, TotalLines: total, CutLines: cut}
}

func memoryTruncationReason(report MemoryReport) string {
	if report.TotalLines == 0 {
		return "memory truncated (numbers unknown)"
	}
	return fmt.Sprintf("memory truncated (loaded %d of %d lines, %d cut off)", report.LoadedLines, report.TotalLines, report.CutLines)
}

func attachmentContent(attachment *transcriptAttachment) string {
	if attachment == nil || len(attachment.Content) == 0 || string(attachment.Content) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(attachment.Content, &text) == nil {
		return text
	}
	var value interface{}
	if json.Unmarshal(attachment.Content, &value) != nil {
		return ""
	}
	var collect func(interface{})
	collect = func(item interface{}) {
		switch typed := item.(type) {
		case []interface{}:
			for _, child := range typed {
				collect(child)
			}
		case map[string]interface{}:
			if blockText, ok := typed["text"].(string); ok {
				text += blockText
			}
			for key, child := range typed {
				if key != "text" {
					collect(child)
				}
			}
		}
	}
	collect(value)
	return text
}

type vaultArc struct {
	ID        string
	Body      string
	Principle string
	Fallback  string
	Sections  []memory.ArcSection
}

func readVault(root, anchorID string) (string, []vaultArc, error) {
	cfg, err := vault.LoadConfig(root)
	if err != nil {
		return "", nil, fmt.Errorf("vault %q: %w", root, err)
	}
	scan, err := vault.Scan(root, cfg.Vault.Exclude)
	if err != nil || !scan.Walkable {
		if err != nil {
			return "", nil, fmt.Errorf("vault %q: %w", root, err)
		}
		return "", nil, fmt.Errorf("vault %q is not readable", root)
	}
	var anchorBody string
	var arcs []vaultArc
	for _, file := range scan.Files {
		data, readErr := os.ReadFile(file.AbsPath) //nolint:gosec // nosemgrep: go-path-traversal -- path comes from scanning the operator-named vault
		if readErr != nil {
			return "", nil, fmt.Errorf("vault note %q: %w", file.RelPath, readErr)
		}
		note, parseErr := parser.Parse(data)
		if parseErr != nil {
			return "", nil, fmt.Errorf("vault note %q: %w", file.RelPath, parseErr)
		}
		if !note.IsDomain {
			continue
		}
		if note.NoteType == "arc" {
			arcs = append(arcs, vaultArc{ID: note.ID, Body: note.Body, Principle: memory.PrincipleSection(note.Body), Fallback: memory.RecitePassage(note.Body), Sections: memory.ExtractArcSections(note.Body)})
		}
		if anchorID != "" && note.ID == anchorID {
			anchorBody = note.Body
		}
		if anchorID == "" && note.NoteType == "identity" && (note.ID == "identity-who-i-am" || note.ID == "identity-who-am-i") {
			anchorID = note.ID
			anchorBody = note.Body
		}
	}
	if anchorBody == "" {
		if anchorID == "" {
			return "", nil, fmt.Errorf("vault %q has no identity anchor (type identity, id identity-who-i-am or identity-who-am-i)", root)
		}
		return "", nil, fmt.Errorf("vault %q has no anchor %q", root, anchorID)
	}
	sort.Slice(arcs, func(i, j int) bool { return arcs[i].ID < arcs[j].ID })
	return anchorBody, arcs, nil
}

func coverage(body string, texts []string) IdentityReport {
	units := textUnits(body)
	result := IdentityReport{}
	for _, unit := range units {
		result.TotalBytes += len(unit)
		if containsNormalized(texts, unit) {
			result.CoveredBytes += len(unit)
		}
	}
	if result.TotalBytes > 0 {
		result.Coverage = float64(result.CoveredBytes) / float64(result.TotalBytes)
	}
	return result
}

func arcCoverage(arcs []vaultArc, texts []string, arcBlocks map[string][]string) ArcReport {
	result := ArcReport{Items: make([]ArcBodyReport, 0, len(arcs))}
	for _, arc := range arcs {
		body := coverage(arc.Body, texts)
		item := ArcBodyReport{ID: arc.ID, BodyBytes: body.TotalBytes, ArrivedBytes: body.CoveredBytes, Sections: make([]ArcSectionReport, 0)}
		for _, section := range arc.Sections {
			if section.Name != "trigger" && section.Name != "push" && section.Name != "principle" {
				continue
			}
			sectionCoverage := momentCoverage(section.Text, arcBlocks[arc.ID])
			item.Sections = append(item.Sections, ArcSectionReport{Name: section.Name, Bytes: sectionCoverage.TotalBytes, ArrivedBytes: sectionCoverage.CoveredBytes, Coverage: sectionCoverage.Coverage})
			result.MomentBytes += sectionCoverage.TotalBytes
			result.MomentArrivedBytes += sectionCoverage.CoveredBytes
		}
		result.Items = append(result.Items, item)
		result.BodyBytes += body.TotalBytes
		result.ArrivedBytes += body.CoveredBytes
		principle := normalize(arc.Principle)
		if principle == "" {
			if containsNormalized(texts, arc.Fallback) {
				result.Partial++
			} else {
				result.Missing++
			}
			continue
		}
		if containsNormalized(texts, principle) {
			result.Full++
			continue
		}
		partial := false
		for _, unit := range longTextUnits(arc.Principle) {
			if containsNormalized(texts, unit) {
				partial = true
				break
			}
		}
		if !partial {
			prefix := principle
			if len(prefix) > 24 {
				prefix = prefix[:24]
			}
			partial = len(prefix) >= minimumCoverageUnitLength && containsNormalized(texts, prefix)
		}
		if partial {
			result.Partial++
		} else {
			result.Missing++
		}
	}
	if result.BodyBytes > 0 {
		result.Coverage = float64(result.ArrivedBytes) / float64(result.BodyBytes)
	}
	if result.MomentBytes > 0 {
		result.MomentCoverage = float64(result.MomentArrivedBytes) / float64(result.MomentBytes)
	}
	return result
}

func arcCoveragePercentTotal(arcs ArcReport) int {
	if arcs.BodyBytes == 0 {
		return 0
	}
	return int(float64(arcs.ArrivedBytes) * 100 / float64(arcs.BodyBytes))
}

func arcMomentCoveragePercentTotal(arcs ArcReport) int {
	if arcs.MomentBytes == 0 {
		return 0
	}
	return int(float64(arcs.MomentArrivedBytes) * 100 / float64(arcs.MomentBytes))
}

func textUnits(text string) []string {
	var units []string
	for _, line := range strings.Split(text, "\n") {
		line = normalize(line)
		if line == "" {
			continue
		}
		start := 0
		for i, r := range line {
			if !strings.ContainsRune(".!?。！？", r) {
				continue
			}
			end := i + len(string(r))
			if unit := normalize(line[start:end]); unit != "" {
				units = append(units, unit)
			}
			start = end
		}
		if unit := normalize(line[start:]); unit != "" {
			units = append(units, unit)
		}
	}
	return units
}

const minimumCoverageUnitLength = 12

func longTextUnits(text string) []string {
	var units []string
	for _, unit := range textUnits(text) {
		if len(unit) >= minimumCoverageUnitLength {
			units = append(units, unit)
		}
	}
	return units
}

func momentCoverage(section string, blocks []string) IdentityReport {
	result := IdentityReport{}
	for _, unit := range longTextUnits(section) {
		if strings.EqualFold(unit, "Read them as yours before you respond.") {
			continue
		}
		result.TotalBytes += len(unit)
		if containsNormalized(blocks, unit) {
			result.CoveredBytes += len(unit)
		}
	}
	if result.TotalBytes > 0 {
		result.Coverage = float64(result.CoveredBytes) / float64(result.TotalBytes)
	}
	return result
}

func arcBlocks(text string) map[string][]string {
	lines := strings.Split(text, "\n")
	result := make(map[string][]string)
	for i, line := range lines {
		match := arcBlockHeader.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if match == nil {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimRight(lines[j], "\r"), "── ") {
				end = j
				break
			}
		}
		id := strings.TrimSpace(match[1])
		result[id] = append(result[id], strings.Join(lines[i:end], "\n"))
	}
	return result
}

func containsNormalized(texts []string, needle string) bool {
	needle = normalize(needle)
	if needle == "" {
		return false
	}
	for _, text := range texts {
		if strings.Contains(normalize(text), needle) {
			return true
		}
	}
	return false
}

func normalize(text string) string { return strings.Join(strings.Fields(text), " ") }

func formatPoint(raw string) string {
	if raw == "" {
		return "unknown"
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return raw
	}
	return parsed.UTC().Format("2006-01-02 15:04Z")
}
