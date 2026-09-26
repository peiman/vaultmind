package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

// embedOnWriteFlag names the setting for getConfigValueWithFlags. No command
// declares it as a flag, so the value comes from the config file or
// VAULTMIND_APP_EMBED_ON_WRITE.
const embedOnWriteFlag = "embed-on-write"

// embedOnWriteLimit is how many notes a write embeds. A write embeds the note
// it wrote; a vault with a backlog of notes never embedded would turn one small
// edit into a long run, so above this the write names the backlog instead.
// A variable so a test can lower it.
var embedOnWriteLimit = 10

// runEmbedPass runs the incremental embed pass. A variable so tests can pin
// what a write reports without loading a real model.
var runEmbedPass = func(cmd *cobra.Command, vaultPath, dbPath string, cfg *vault.Config, model string) (*index.EmbedResult, error) {
	return index.NewIndexer(vaultPath, dbPath, cfg).RunEmbed(cmd.Context(), dbPath, model, false)
}

// embedAfterWrite embeds the notes a write left without embeddings, with the
// model the vault already uses. A changed note loses its vectors (the index
// clears them on a content change), so without this a note just written was
// invisible to semantic search until someone ran `vaultmind index --embed`.
//
// It never fails the write: a vault never embedded is left alone, BGE-M3 on the
// slow pure-Go backend is skipped, and a failure is reported on stderr with the
// command that finishes the job. app.embed_on_write=false turns it off.
func embedAfterWrite(cmd *cobra.Command, vaultPath string, cfg *vault.Config) {
	if !getConfigValueWithFlags[bool](cmd, embedOnWriteFlag, config.KeyAppEmbedOnWrite) {
		return
	}
	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	model, err := index.EmbeddedModel(dbPath)
	if err != nil || model == "" {
		return
	}
	finish := fmt.Sprintf("vaultmind index --embed --vault %s", vaultPath)
	if model == embedding.ModelBGEM3 && embedding.BackendName() != embedding.BackendNameORT {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "not embedded: this build has no ORT backend for BGE-M3; run %s with an ORT build\n", finish)
		return
	}
	if pending, err := index.PendingEmbeddings(dbPath, model); err == nil && pending > embedOnWriteLimit {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "not embedded: %d notes have no embeddings; embed them with %s\n", pending, finish)
		return
	}
	res, err := runEmbedPass(cmd, vaultPath, dbPath, cfg, model)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "written, but not embedded (%v); run %s\n", err, finish)
		return
	}
	if res != nil && res.Embedded > 0 {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "embedded %d note(s) [model: %s]\n", res.Embedded, model)
	}
}
