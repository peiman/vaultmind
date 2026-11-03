package identitycli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/keeperctl"
)

// SignKeeper strictly parses and signs an owner document through the keyless
// signer. Stamping fills only absent fields. Errors before output fail closed.
func SignKeeper(out io.Writer, client keeperctl.SignerClient, docJSON []byte, stamp bool, now func() time.Time, rand io.Reader) error {
	kind, raw, err := keeperDocument(docJSON, stamp, now, rand)
	if err != nil {
		return err
	}
	canonical, sig, err := signKeeperDocument(client, kind, raw)
	if err != nil {
		return err
	}
	// Embed the exact signed bytes. Marshaling a RawMessage would HTML-escape
	// strings (including U+2028/U+2029), changing the canonical document bytes.
	_, err = fmt.Fprintf(out, "{\"document\":%s,\"sig\":%q}\n", canonical.Bytes(), base64.StdEncoding.EncodeToString(sig))
	return err
}

func keeperDocument(raw []byte, stamp bool, now func() time.Time, rand io.Reader) (string, []byte, error) {
	if stamp {
		if err := identity.ValidateSchema(raw); err != nil {
			return "", nil, err
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "", nil, fmt.Errorf("parsing keeper document JSON: %w", err)
	}
	var kind string
	if err := json.Unmarshal(fields[keeperctl.FieldKind], &kind); err != nil || !keeperKind(kind) {
		return "", nil, fmt.Errorf("keeper document kind must be one of: %s, %s, %s", keeperctl.KindControl, keeperctl.KindGrant, keeperctl.KindGrantRevoke)
	}
	if stamp {
		stamped, err := stampKeeperDocument(raw, fields, kind, now, rand)
		return kind, stamped, err
	}
	return kind, raw, nil
}

func keeperKind(kind string) bool {
	return kind == keeperctl.KindControl || kind == keeperctl.KindGrant || kind == keeperctl.KindGrantRevoke
}

func stampKeeperDocument(raw []byte, fields map[string]json.RawMessage, kind string, now func() time.Time, rand io.Reader) ([]byte, error) {
	// Append only new members to the validated object. Re-marshaling fields
	// would reorder keys or rewrite string escapes in the original document.
	doc := bytes.TrimSpace(raw)
	doc = append([]byte(nil), doc[:len(doc)-1]...)
	if _, present := fields[keeperctl.FieldIssuedAt]; !present {
		doc = appendKeeperField(doc, keeperctl.FieldIssuedAt, strconv.AppendInt(nil, now().Unix(), 10))
	}
	if _, present := fields[keeperctl.FieldNonce]; !present && kind != keeperctl.KindGrant {
		var nonce [18]byte
		if _, err := io.ReadFull(rand, nonce[:]); err != nil {
			return nil, fmt.Errorf("stamping keeper nonce: %w", err)
		}
		encoded := `"` + base64.StdEncoding.EncodeToString(nonce[:]) + `"`
		doc = appendKeeperField(doc, keeperctl.FieldNonce, []byte(encoded))
	}
	return append(doc, '}'), nil
}

func appendKeeperField(doc []byte, key string, value []byte) []byte {
	// A supported kind is already present, so the object is never empty.
	doc = append(doc, ',')
	doc = append(doc, '"')
	doc = append(doc, key...)
	doc = append(doc, '"', ':')
	return append(doc, value...)
}

func signKeeperDocument(client keeperctl.SignerClient, kind string, raw []byte) (identity.CanonicalBytes, []byte, error) {
	switch kind {
	case keeperctl.KindControl:
		return signKeeperForm(client, raw, keeperctl.ParseControlRequest, keeperctl.CanonicalizeControlRequest, keeperctl.SignControlRequest)
	case keeperctl.KindGrant:
		return signKeeperForm(client, raw, keeperctl.ParseGrant, keeperctl.CanonicalizeGrant, keeperctl.SignGrant)
	default: // keeperDocument has already checked the three accepted kinds.
		return signKeeperForm(client, raw, keeperctl.ParseGrantRevoke, keeperctl.CanonicalizeGrantRevoke, keeperctl.SignGrantRevoke)
	}
}

func signKeeperForm[T any](client keeperctl.SignerClient, raw []byte, parse func([]byte) (T, error), canonicalize func(T) (identity.CanonicalBytes, error), sign func(keeperctl.SignerClient, T) ([]byte, error)) (identity.CanonicalBytes, []byte, error) {
	form, err := parse(raw)
	if err != nil {
		return identity.CanonicalBytes{}, nil, err
	}
	canonical, err := canonicalize(form)
	if err != nil {
		return identity.CanonicalBytes{}, nil, err
	}
	sig, err := sign(client, form)
	if err != nil {
		return identity.CanonicalBytes{}, nil, fmt.Errorf("signing keeper document via signer: %w", err)
	}
	return canonical, sig, nil
}
