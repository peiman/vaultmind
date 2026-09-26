package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

// askRetriever builds the vault's retriever, scoped by --type and --tag. Every
// ask path — direct, --read and each federated vault — builds through here, so
// none of them can search unscoped.
func askRetriever(cmd *cobra.Command, db *index.DB) query.AutoRetrieverResult {
	ret := query.BuildAutoRetrieverFull(cmd.Context(), db)
	filters := index.SearchFilters{
		Type: getConfigValueWithFlags[string](cmd, "type", config.KeyAppAskType),
		Tag:  getConfigValueWithFlags[string](cmd, "tag", config.KeyAppAskTag),
	}
	if filters != (index.SearchFilters{}) {
		ret.Retriever = query.FilteredRetriever{Base: ret.Retriever, Filters: filters}
	}
	return ret
}
