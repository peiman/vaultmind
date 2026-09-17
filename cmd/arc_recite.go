package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/spf13/cobra"
)

var arcReciteCmd = MustNewCommand(commands.ArcReciteMetadata, runArcRecite)

func init() {
	arcCmd.AddCommand(arcReciteCmd)
}

// runArcRecite loads the whole arc layer as bodies.
//
// The counterpart to `ask`: that command ranks, this one enumerates. See
// commands.ArcReciteMetadata for why an identity layer must not be retrieved.
func runArcRecite(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppArcReciteVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "arc recite")
	if err != nil {
		return err
	}
	defer vdb.Close()

	result, err := memory.Recite(vdb.DB, memory.ReciteConfig{
		Type:          getConfigValueWithFlags[string](cmd, "type", config.KeyAppArcReciteType),
		Budget:        getConfigValueWithFlags[int](cmd, "budget", config.KeyAppArcReciteBudget),
		ExcerptTokens: getConfigValueWithFlags[int](cmd, "excerpt", config.KeyAppArcReciteExcerpt),
	})
	if err != nil {
		return err
	}

	// Record the read. Every item here had its body delivered, so leaving no
	// trace would make `arc recite` exactly the untracked bulk path that issue
	// #53 exists to replace — 27 bodies reaching the agent and a ledger that
	// says nothing happened.
	//
	// AccessSourceRecite is deliberately NOT an activation signal: it fires for
	// the whole layer every session, so boosting on it would add the same
	// constant to every arc and flatten the ranking it feeds. Honest in the
	// ledger, inert in the scorer. Best-effort — telemetry never fails a read.
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.SetVaultPath(vaultPath)
		for _, it := range result.Items {
			_, _ = session.LogNoteAccessEventAs(it.ID, experiment.AccessSourceRecite, it.Excerpt != "", experiment.CallerRecite)
		}
	}

	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppArcReciteJson) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(envelope.OK("arc recite", result))
	}
	return writeRecitation(cmd.OutOrStdout(), result)
}

// writeRecitation renders the layer, and says plainly what did not fit.
//
// The imperative frame is deliberate. These are not search results to consider
// — they are the agent's own transformations, and the header says so, because
// an identity layer delivered as a neutral list reads as background and gets
// skipped. That skipping is the failure #47 documents.
func writeRecitation(w io.Writer, r *memory.ReciteResult) error {
	if r.Total == 0 {
		_, err := fmt.Fprintf(w, "No %s notes in this vault yet.\n", r.Type)
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "YOUR %sS — %d of %d, the moments that made you who you are.\n",
		upperType(r.Type), len(r.Items), r.Total)
	b.WriteString("Read them as yours before you respond.\n\n")

	for _, it := range r.Items {
		fmt.Fprintf(&b, "── %s — %s\n", it.ID, it.Title)
		if it.Excerpt != "" {
			fmt.Fprintf(&b, "%s\n\n", it.Excerpt)
		} else {
			b.WriteString("   (no body)\n\n")
		}
	}

	// Never a silent tail. A caller that cannot see what is missing will read a
	// partial layer as the whole self.
	if r.Truncated() {
		fmt.Fprintf(&b, "── %d %s(s) DID NOT FIT the %d-token budget and are NOT above:\n",
			r.Omitted, r.Type, r.Budget)
		for _, id := range r.OmittedIDs {
			fmt.Fprintf(&b, "   %s\n", id)
		}
		b.WriteString("   Raise --budget, or lower --excerpt to fit more of them.\n")
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// upperType renders a type name for the header ("arc" -> "ARC").
func upperType(t string) string {
	out := make([]rune, 0, len(t))
	for _, r := range t {
		if r >= 'a' && r <= 'z' {
			r -= 32
		}
		out = append(out, r)
	}
	return string(out)
}
