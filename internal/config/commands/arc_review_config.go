package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ArcReviewMetadata defines the `arc review` command — the review queue for
// finding turning points in captured sessions (plasticity step 2).
var ArcReviewMetadata = config.CommandMetadata{
	Use:   "review",
	Short: "Review captured sessions for turning points, oldest first",
	Long: "List the person's messages from a captured session, each with what the agent had " +
		"just said, for you to judge which ones changed how you understand or approach " +
		"something. Measured on real sessions: phrase rules (`arc candidates`) found 0 of 5 " +
		"moments that became arcs; an agent reading this list found 4 of 5. Machine messages, " +
		"compaction summaries and one-word replies are left out.\n\n" +
		"It works as a queue. With no episode named it opens the oldest session nobody has " +
		"judged. Write each turning point as a desk entry (arcs are still written by hand), " +
		"then record the judgement with --mark-reviewed: it sets `reviewed: <date>` on the " +
		"episode, and the queue moves on. `vaultmind self` — run at session start — says how " +
		"many sessions await review.\n\n" +
		"  vaultmind arc review --vault ./vaultmind-identity\n" +
		"  vaultmind arc review --vault ./vaultmind-identity --mark-reviewed episode-2026-09-24-abcd1234\n" +
		"  vaultmind arc review --vault ./vaultmind-identity --episode episode-2026-09-24-abcd1234",
	ConfigPrefix: "app.arc.review",
	FlagOverrides: map[string]string{
		"app.arc.review.vault":         "vault",
		"app.arc.review.json":          "json",
		"app.arc.review.episode":       "episode",
		"app.arc.review.mark_reviewed": "mark-reviewed",
	},
}

// ArcReviewOptions returns configuration options for `arc review`.
func ArcReviewOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.arc.review.vault", DefaultValue: ".", Description: "Path to vault root (its episodes/ folder is reviewed)", Type: "string"},
		{Key: "app.arc.review.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.arc.review.episode", DefaultValue: "", Description: "The episode id(s) to review, comma-separated (default: the oldest session awaiting review)", Type: "string"},
		{Key: "app.arc.review.mark_reviewed", DefaultValue: "", Description: "Record that these episode id(s), comma-separated, have been judged — they leave the queue", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(ArcReviewOptions)
}
