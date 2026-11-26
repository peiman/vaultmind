package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/personahealth"
)

func renderPersonaHealth(cmd interface{ OutOrStdout() io.Writer }, result personahealth.Result, jsonMode bool) error {
	if jsonMode {
		env := envelope.OK("persona health", result)
		if personaHealthFailures(result) > 0 {
			env.Status = "fail"
		}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(env); err != nil {
			return err
		}
		if personaHealthFailures(result) > 0 {
			return cmdutil.ErrAlreadyWritten
		}
		return nil
	}
	if _, err := io.WriteString(cmd.OutOrStdout(), personahealth.Text(result)); err != nil {
		return err
	}
	if failures := personaHealthFailures(result); failures > 0 {
		return fmt.Errorf("%d keeper(s) failed persona health", failures)
	}
	return nil
}

func personaHealthFailures(result personahealth.Result) int {
	count := 0
	for _, keeper := range result.Keepers {
		if keeper.Status == "fail" {
			count++
		}
	}
	return count
}
