package gap

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/peiman/vaultmind/internal/envelope"
)

type gapResult struct {
	Mode              string   `json:"mode"`
	Path              string   `json:"path"`
	ID                string   `json:"id"`
	Question          string   `json:"question"`
	Status            string   `json:"status"`
	AskedHits         []string `json:"asked_hits"`
	Opened            int      `json:"opened"`
	Closed            int      `json:"closed"`
	MedianDaysToClose *float64 `json:"median_days_to_close,omitempty"`
}

func fail(req Request, code, msg string) error {
	if !req.JSON {
		return fmt.Errorf("%s", msg)
	}
	return envelope.WriteError(stdout(req), envelope.Error(commandGap, code, msg, ""))
}

func failOp(req Request, code string, err error) error {
	if req.JSON {
		return fail(req, code, err.Error())
	}
	return err
}

func stdout(req Request) io.Writer {
	if req.Stdout == nil {
		return io.Discard
	}
	return req.Stdout
}

func render(req Request, v Vault, o outcome) error {
	if o.AskedHits == nil {
		o.AskedHits = []string{}
	}
	if !req.JSON {
		var b strings.Builder
		fmt.Fprintf(&b, "mode: %s\npath: %s\nid: %s\nstatus: %s\nquestion: %s\n",
			o.Mode, o.Path, o.ID, o.Status, o.Question)
		fmt.Fprintf(&b, "asked_hits: %s\n", strings.Join(o.AskedHits, ", "))
		b.WriteString(formatMeter(o.Report))
		_, err := io.WriteString(stdout(req), b.String())
		return err
	}
	env := envelope.OK(commandGap, gapResult{
		Mode:              o.Mode,
		Path:              o.Path,
		ID:                o.ID,
		Question:          o.Question,
		Status:            o.Status,
		AskedHits:         o.AskedHits,
		Opened:            o.Report.Opened,
		Closed:            o.Report.Closed,
		MedianDaysToClose: o.Report.Median,
	})
	env.Meta.VaultPath = v.Path
	if v.IndexHash != nil {
		env.Meta.IndexHash = v.IndexHash()
	}
	return json.NewEncoder(stdout(req)).Encode(env)
}

func formatMeter(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "gaps opened: %d\ngaps closed: %d\nmedian days to close: %s\n",
		r.Opened, r.Closed, formatMedian(r.Median))
	return b.String()
}

func formatMedian(m *float64) string {
	if m == nil {
		return "n/a"
	}
	return strconv.FormatFloat(*m, 'f', -1, 64)
}
