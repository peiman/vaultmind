package cmd

// Consumer-side envelope shapes. These are the struct types a downstream
// tool (a persona hook, chat app, custom integration) might define
// to decode `vaultmind ask --json` output. They deliberately live in a
// test file so they stay independent of VaultMind's internal
// representations — if someone renames a field in internal/envelope, the
// contract tests must break, not silently track the rename.
//
// These structs intentionally use `json:"..."` tags that match the stable
// public names. Any change here requires a major-version schema_version
// bump AND a migration note in AGENTS.md.

// AskEnvelope is the contract shape for `vaultmind ask --json`.
// Fields that are optional (may be absent on zero-hit queries) use
// pointer types so consumer code can null-check rather than guessing.
type AskEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Command       string `json:"command"`
	Result        struct {
		Query       string `json:"query"`
		QueryCapped *struct {
			EmbeddedTokens int `json:"embedded_tokens"`
			TotalTokens    int `json:"total_tokens,omitempty"`
			TotalChars     int `json:"total_chars,omitempty"`
			SalientTerms   int `json:"salient_terms"`
		} `json:"query_capped,omitempty"`
		RetrievalMode string `json:"retrieval_mode"`
		TopHits       []struct {
			ID    string  `json:"id"`
			Type  string  `json:"type"`
			Title string  `json:"title"`
			Path  string  `json:"path"`
			Score float64 `json:"score"`
		} `json:"top_hits"`
		Context *struct {
			TargetID     string `json:"target_id"`
			UsedTokens   int    `json:"used_tokens"`
			BudgetTokens int    `json:"budget_tokens"`
		} `json:"context,omitempty"`
	} `json:"result"`
	Meta struct {
		VaultPath string `json:"vault_path"`
	} `json:"meta"`
}

// ContextPackEnvelope is the contract shape for `vaultmind memory pack --json`.
type ContextPackEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Result        struct {
		TargetID     string `json:"target_id"`
		UsedTokens   int    `json:"used_tokens"`
		BudgetTokens int    `json:"budget_tokens"`
		Truncated    bool   `json:"truncated"`
		Target       struct {
			ID string `json:"id"`
		} `json:"target"`
		Context []struct {
			ID           string `json:"id"`
			EdgeType     string `json:"edge_type"`
			BodyIncluded bool   `json:"body_included"`
		} `json:"context"`
	} `json:"result"`
}

// DoctorEnvelope is the contract shape for the draft fields on `vaultmind doctor --json`.
// schema_version stays v1: the fields are additive.
type DoctorEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Command       string `json:"command"`
	Result        struct {
		Drafts struct {
			Count int `json:"count"`
			Notes []struct {
				ID      string `json:"id"`
				Path    string `json:"path"`
				AgeDays int    `json:"age_days"`
			} `json:"notes"`
		} `json:"drafts"`
		UncommittedNotes struct {
			ThresholdDays int    `json:"threshold_days"`
			Count         int    `json:"count"`
			Skipped       bool   `json:"skipped"`
			Reason        string `json:"reason"`
			Notes         []struct {
				Path    string `json:"path"`
				AgeDays int    `json:"age_days"`
				State   string `json:"state"`
			} `json:"notes"`
		} `json:"uncommitted_notes"`
	} `json:"result"`
}

// SearchEnvelope is the contract shape for `vaultmind search --json`.
type SearchEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Result        struct {
		Hits []struct {
			ID    string  `json:"id"`
			Title string  `json:"title"`
			Score float64 `json:"score"`
		} `json:"hits"`
		Total int `json:"total"`
	} `json:"result"`
}

// NoteProposeEnvelope is the contract shape for `vaultmind note propose --json`.
// schema_version stays v1: the result fields are additive.
type NoteProposeEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Command       string `json:"command"`
	Status        string `json:"status"`
	Result        struct {
		Mode     string `json:"mode"`
		Path     string `json:"path"`
		ID       string `json:"id"`
		TargetID string `json:"target_id,omitempty"`
		NextStep string `json:"next_step"`
	} `json:"result"`
	Meta struct {
		VaultPath string `json:"vault_path"`
		IndexHash string `json:"index_hash"`
	} `json:"meta"`
}

// NoteAcceptEnvelope is the contract shape for `vaultmind note accept --json`.
type NoteAcceptEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Command       string `json:"command"`
	Status        string `json:"status"`
	Result        struct {
		Mode     string `json:"mode"`
		Path     string `json:"path"`
		ID       string `json:"id"`
		TargetID string `json:"target_id,omitempty"`
		Status   string `json:"status,omitempty"`
	} `json:"result"`
}

// NoteGetEnvelope is the contract shape for `vaultmind note get --json`.
type NoteGetEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Result        struct {
		ID    string `json:"id"`
		Type  string `json:"type"`
		Title string `json:"title"`
	} `json:"result"`
}
