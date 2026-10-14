package embedding

import (
	"fmt"

	"github.com/gomlx/go-huggingface/tokenizers/api"
)

// specialTokenIDs is the read-only lookup the sparse lane uses.
// It does not expose With: the shared tokenizer's encode options stay as the
// loader set them (vaultmind#279).
type specialTokenIDs interface {
	SpecialTokenID(api.SpecialToken) (int, error)
}

// sparseSpecialIDs is the set the sparse map must not contain: CLS, SEP
// (end of sentence), PAD, and UNK, plus beginning-of-sentence when that id
// is a different token. The mask token stays in the vector. The model's
// reference sparse output drops the same four.
func sparseSpecialIDs(tok specialTokenIDs) (map[uint32]struct{}, error) {
	if tok == nil {
		return nil, fmt.Errorf("sparse specials: tokenizer is nil")
	}
	ids := make(map[uint32]struct{}, 5)
	required := []api.SpecialToken{
		api.TokClassification,
		api.TokEndOfSentence,
		api.TokPad,
		api.TokUnknown,
	}
	for _, kind := range required {
		id, err := tok.SpecialTokenID(kind)
		if err != nil {
			return nil, fmt.Errorf("sparse specials: %v: %w", kind, err)
		}
		uid, ok := sparseTokenID(id)
		if !ok {
			return nil, fmt.Errorf("sparse specials: %v id %d", kind, id)
		}
		ids[uid] = struct{}{}
	}
	if id, err := tok.SpecialTokenID(api.TokBeginningOfSentence); err == nil {
		if uid, ok := sparseTokenID(id); ok {
			ids[uid] = struct{}{}
		}
	}
	return ids, nil
}

func sparseTokenID(id int) (uint32, bool) {
	if id < 0 {
		return 0, false
	}
	return uint32(id), true //nolint:gosec // token ids are non-negative and far below 2^32
}

// sparseMaskFromTokenIDs marks positions whose token id is in specials.
// An empty token list or an empty special set returns nil, which keeps every
// position: there is nothing to drop. The mask token is not in specials, so
// it stays 0.
func sparseMaskFromTokenIDs(tokenIDs []uint32, specials map[uint32]struct{}) []uint32 {
	if len(tokenIDs) == 0 || len(specials) == 0 {
		return nil
	}
	mask := make([]uint32, len(tokenIDs))
	for i, id := range tokenIDs {
		if _, ok := specials[id]; ok {
			mask[i] = 1
		}
	}
	return mask
}
