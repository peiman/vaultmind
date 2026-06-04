package parser

import "fmt"

// ParsedNote is the complete result of parsing a single .md file.
type ParsedNote struct {
	Frontmatter map[string]interface{}
	Body        string
	FTSBody     string
	Links       []ExtractedLink
	Headings    []ExtractedHeading
	Blocks      []ExtractedBlock
	IsDomain    bool
	ID          string
	NoteType    string
}

// Parse converts raw .md file content into a ParsedNote.
// Returns an error only if YAML frontmatter is present but syntactically invalid.
func Parse(content []byte) (*ParsedNote, error) {
	fm, body, err := ExtractFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("extracting frontmatter: %w", err)
	}

	isDomain, id, noteType := ClassifyNote(fm)

	return &ParsedNote{
		Frontmatter: fm,
		Body:        body,
		FTSBody:     StripForFTS(body),
		Links:       ExtractLinks(body),
		Headings:    ExtractHeadings(body),
		Blocks:      ExtractBlocks(body),
		IsDomain:    isDomain,
		ID:          id,
		NoteType:    noteType,
	}, nil
}
