package cmd

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func momentReciteVault(t *testing.T) string {
	t.Helper()
	vault, _ := graphNeighborVault(t)
	writeRawNote(t, vault, "arcs/moment.md", `---
id: arc-moment
type: arc
title: Moment Arc
created: 2026-04-03
---
<!-- internal-only comment -->
## The Trigger

The request arrived.

## Push

The result pushed back.

## The Principle

Protect the boundary and verify the result.

## Source

Fixture source.

## What I did differently

I changed the delivery contract.
`)
	rebuildVault(t, vault)
	return vault
}

func TestArcRecite_SectionsTextAndJSONIncludeFullSectionsAndFallback(t *testing.T) {
	isolateXDG(t)
	vault := momentReciteVault(t)

	out, errOut, err := runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "trigger,push,principle", "--excerpt", "1")
	require.NoError(t, err, errOut.String())
	assert.Contains(t, out.String(), "## The Trigger")
	assert.Contains(t, out.String(), "The result pushed back.")
	assert.Contains(t, out.String(), "Protect the boundary and verify the result.")
	assert.Contains(t, out.String(), "(no trigger/push/principle section — excerpt)")

	out, errOut, err = runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "trigger,push,principle", "--json")
	require.NoError(t, err, errOut.String())
	var env struct {
		Result struct {
			Items []struct {
				ID       string `json:"id"`
				Fallback bool   `json:"fallback"`
				Sections []struct {
					Name string `json:"name"`
				} `json:"sections"`
			} `json:"items"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	var moment, fallback bool
	for _, item := range env.Result.Items {
		if item.ID == "arc-moment" {
			moment = true
			assert.Equal(t, []string{"trigger", "push", "principle"}, []string{item.Sections[0].Name, item.Sections[1].Name, item.Sections[2].Name})
		}
		if item.ID == "arc-kept-self" {
			fallback = item.Fallback
		}
	}
	assert.True(t, moment)
	assert.True(t, fallback)
}

func TestArcRecite_AllIncludesEveryBodySectionAndJSONOmitsSectionDetails(t *testing.T) {
	isolateXDG(t)
	vault := momentReciteVault(t)

	out, errOut, err := runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "all", "--excerpt", "1")
	require.NoError(t, err, errOut.String())
	assert.Contains(t, out.String(), "The request arrived.")
	assert.Contains(t, out.String(), "The result pushed back.")
	assert.Contains(t, out.String(), "Protect the boundary and verify the result.")
	assert.Contains(t, out.String(), "Fixture source.")
	assert.Contains(t, out.String(), "I changed the delivery contract.")
	assert.NotContains(t, out.String(), "(no trigger/push/principle section — excerpt)")
	assert.NotContains(t, out.String(), "id: arc-moment")
	assert.NotContains(t, out.String(), "internal-only comment")
	assert.NotContains(t, out.String(), "<!--")

	out, errOut, err = runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "all", "--json")
	require.NoError(t, err, errOut.String())
	var env struct {
		Result struct {
			Items []struct {
				ID       string            `json:"id"`
				Excerpt  string            `json:"excerpt"`
				Sections []json.RawMessage `json:"sections"`
				Fallback bool              `json:"fallback"`
			} `json:"items"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	for _, item := range env.Result.Items {
		if item.ID != "arc-moment" {
			continue
		}
		assert.Contains(t, item.Excerpt, "Fixture source.")
		assert.Contains(t, item.Excerpt, "I changed the delivery contract.")
		assert.Empty(t, item.Sections)
		assert.False(t, item.Fallback)
		assert.NotContains(t, item.Excerpt, "internal-only comment")
		return
	}
	t.Fatal("arc-moment was not recited")
}

func TestArcRecite_AllShardsAreDeterministicAndComplete(t *testing.T) {
	isolateXDG(t)
	vault := momentReciteVault(t)
	shardIDs := make([][]string, 0, 2)
	for shard := 1; shard <= 2; shard++ {
		var first, second struct {
			Result struct {
				Total int `json:"total"`
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			} `json:"result"`
		}
		for run, target := range []*struct {
			Result struct {
				Total int `json:"total"`
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			} `json:"result"`
		}{&first, &second} {
			out, errOut, err := runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "all", "--shard", fmt.Sprintf("%d/2", shard), "--json")
			require.NoError(t, err, errOut.String())
			require.NoError(t, json.Unmarshal(out.Bytes(), target))
			if run == 0 {
				continue
			}
			assert.Equal(t, first.Result.Items, second.Result.Items)
		}
		ids := make([]string, len(first.Result.Items))
		for i, item := range first.Result.Items {
			ids[i] = item.ID
		}
		shardIDs = append(shardIDs, ids)
	}

	seen := map[string]int{}
	for _, ids := range shardIDs {
		for _, id := range ids {
			seen[id]++
		}
	}
	assert.Equal(t, map[string]int{"arc-kept-self": 1, "arc-moment": 1}, seen)
}

func TestArcRecite_AllMaxBytesReportsOverflowID(t *testing.T) {
	isolateXDG(t)
	vault := momentReciteVault(t)
	out, errOut, err := runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "all", "--max-bytes", "120", "--json")
	require.NoError(t, err, errOut.String())
	var env struct {
		Result struct {
			OverflowIDs []string `json:"overflow_ids"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Contains(t, env.Result.OverflowIDs, "arc-moment")
}

func TestArcRecite_ShardAndMaxBytesReportOverflow(t *testing.T) {
	isolateXDG(t)
	vault := momentReciteVault(t)

	out, errOut, err := runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "trigger,push,principle", "--shard", "1/2", "--max-bytes", "120", "--json")
	require.NoError(t, err, errOut.String())
	var env struct {
		Result struct {
			Shard struct {
				Index int `json:"index"`
				Count int `json:"count"`
			} `json:"shard"`
			OverflowIDs []string `json:"overflow_ids"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, 1, env.Result.Shard.Index)
	assert.Equal(t, 2, env.Result.Shard.Count)
	assert.Contains(t, env.Result.OverflowIDs, "arc-kept-self")

	out, errOut, err = runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "trigger,push,principle", "--shard", "2/2", "--max-bytes", "300")
	require.NoError(t, err, errOut.String())
	assert.LessOrEqual(t, len(out.Bytes()), 300, "rendered text including overflow footer must stay below the cap")

	out, errOut, err = runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "trigger,push,principle", "--shard", "1/2", "--max-bytes", "120")
	require.NoError(t, err, errOut.String())
	assert.LessOrEqual(t, len(out.Bytes()), 120, "compact overflow report must stay below a tight cap")
	assert.Contains(t, out.String(), "arc-kept-self")
}

func TestArcRecite_RejectsUnknownSectionAndInvalidShard(t *testing.T) {
	isolateXDG(t)
	vault := momentReciteVault(t)

	_, _, err := runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "trigger,wat")
	assert.EqualError(t, err, `unknown arc section "wat" (want all, trigger, push, principle, or deeper-sight)`)
	_, _, err = runRootCmd(t, "arc", "recite", "--vault", vault, "--sections", "all,push")
	assert.EqualError(t, err, `arc section "all" cannot be combined with other sections`)
	_, _, err = runRootCmd(t, "arc", "recite", "--vault", vault, "--shard", "3/2")
	assert.EqualError(t, err, "shard index must be between 1 and 2")
}
