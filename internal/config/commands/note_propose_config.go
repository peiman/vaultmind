package commands

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/noisefloor"
)

// NoteProposeMetadata defines metadata for note propose.
var NoteProposeMetadata = config.CommandMetadata{
	Use:   "propose",
	Short: "Turn a finding into a reviewable draft",
	Long: `Write a draft from a question and a finding, without choosing an id or a path.

propose retrieves on the question and the finding the same way ask does, drafts
excluded, and reads the relevance band ask already computed for the top hit.
It does not invent a second score.

  - patch, when that band is the close band (default: strong). A draft is
    written next to the closest note. status is draft, patches names the
    target, and the body is the finding plus a Sources section. An accepted
    note cannot hold the addition yet: recall would return it before a person
    had reviewed it.
  - new, when nothing is that close (moderate, weak, no_match, or no band).
    The type is the top hit's type when it has one, otherwise the vault's
    default knowledge type (concept when the schema registers it, otherwise
    the first type that can hold a draft). The id is the note-create id of a
    filename slugged from the question. The file is expanded through that
    type's template, so required fields hold.
  - existing, when a draft already has the same question (spacing and case
    aside). That draft is reported and nothing new is written.

A person reviews the draft and promotes it with: vaultmind note accept <id>

--finding is required. A source that contains :// must be a URL. Any other
source must be a file inside the vault or the repository that contains it.
--close-band names which of ask's bands counts as close. The default is the
strong band, so a weaker match becomes a new note instead of a patch on the
wrong one.

After the draft is written, propose re-runs the original question through the
same ranking ask uses, with the same retrieval options and the default hit
window (app.ask.search_limit). It reports where the closest existing note
ranked with drafts excluded, and where this draft ranks with drafts included.
A patch is measured on the proposal note, which holds the finding. The draft's
frontmatter records rank_before, rank_after, and rank_checked_at. 0 means the
note was not in the window. The check does not record an access. Keyword mode,
when no embedder is available, still runs and is labeled. --json adds
rank_check to the v1 envelope.

FLAGS

  --question <text>   The question the finding answers.
  --finding <text>    The finding. Required.
  --source <url|path> A source URL or a file inside the vault or its repository. Repeatable.
  --close-band <band> The ask relevance band that means the top hit is close enough to patch.
  --gap <id>          An open gap this finding answers. The draft stores closes_gap.
                      When --question is omitted, the gap's question is used.
                      Accepting the draft closes that gap.
  --vault <dir>       Path to the vault root.
  --json              Output a v1 envelope.`,
	ConfigPrefix: "app.notepropose",
	FlagOverrides: map[string]string{
		"app.notepropose.vault":      "vault",
		"app.notepropose.json":       "json",
		"app.notepropose.question":   "question",
		"app.notepropose.finding":    "finding",
		"app.notepropose.close_band": "close-band",
		"app.notepropose.gap":        "gap",
	},
}

// NoteProposeOptions returns config options for note propose.
func NoteProposeOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.notepropose.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.notepropose.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.notepropose.question", DefaultValue: "", Description: "Question the finding answers", Type: "string"},
		{Key: "app.notepropose.finding", DefaultValue: "", Description: "Finding to record as a draft", Type: "string"},
		{
			Key:          "app.notepropose.close_band",
			DefaultValue: noisefloor.ConfidenceStrong,
			Description:  "Relevance band that means the top hit is close enough to patch. The band ask already reports for that hit. Default is the strong band.",
			Type:         "string",
		},
		{Key: "app.notepropose.gap", DefaultValue: "", Description: "Open gap this finding answers. The draft stores closes_gap. When the question is omitted, the gap's question is used.", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(NoteProposeOptions)
}
