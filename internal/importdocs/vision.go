package importdocs

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Vision is the opt-in model that describes images: an OpenAI-compatible
// endpoint (Ollama's is http://localhost:11434/v1, LM Studio and OpenAI
// work too), the model, and the key when the endpoint needs one. With no
// endpoint, no image leaves the machine.
type Vision struct {
	Endpoint string
	Model    string
	APIKey   string
}

func (v Vision) on() bool {
	return v.Endpoint != "" && v.Model != ""
}

const (
	// visionPromptVersion keys the cache: bump it when the prompt changes,
	// and every image is described again.
	visionPromptVersion = "v1"
	// visionMaxBytes is the largest image sent: endpoints refuse bigger
	// payloads, and a base64 body is a third larger again.
	visionMaxBytes = 20 << 20
	// visionMaxTokens leaves a reasoning model room to think and still
	// answer: with 200, a local gemma4 spent the budget thinking and
	// returned nothing (probe, 2026-10-04).
	visionMaxTokens = 1500
)

// visionTimeout bounds one description; a local model takes seconds to
// tens of seconds. A test lowers it.
var visionTimeout = 180 * time.Second

// visionReads are the formats sent for description, with their media type.
// HEIC is not: most endpoints do not accept it.
var visionReads = map[string]string{
	"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg", "webp": "image/webp", "gif": "image/gif",
}

// visionPrompt asks for what an index needs: what the image shows, its text
// verbatim, its entities, its kind.
const visionPrompt = "You are indexing this image for a searchable knowledge base. In three to six sentences, say " +
	"what it shows, quote any text in it verbatim, name the key concepts or entities, and say what kind of " +
	"image it is (screenshot, diagram, photo, chart, illustration)."

// describe is the vision model's description of an image, from the cache
// when it was described before. An image the model may not take, or one
// whose description fails, gets none; a failure is counted, to be said once.
func (r *imageReading) describe(raw []byte, kind string, w, h int, alts []string) string {
	mime, ok := visionReads[kind]
	if !r.vision.on() || !ok || w < minOCRSide || h < minOCRSide || len(raw) > visionMaxBytes {
		return ""
	}
	key := visionCacheKey(raw, r.vision.Model)
	if text, ok := readCache("vision", key); ok {
		return text
	}
	text, err := describeImage(r.vision, raw, mime, alts)
	if err != nil {
		r.undescribed++
		if r.describeErr == "" {
			r.describeErr = err.Error()
		}
		return ""
	}
	writeCache("vision", key, text)
	return text
}

func visionCacheKey(raw []byte, model string) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]) + "-" + slug(model) + "-" + visionPromptVersion
}

// describeImage sends one image to the endpoint's chat completions, with
// the captions the docs give it as context, and returns the answer. Only
// the message content is the description: a reasoning model's thinking
// comes separately and is left out. An empty answer is a failure.
func describeImage(v Vision, raw []byte, mime string, alts []string) (string, error) {
	prompt := visionPrompt
	if len(alts) > 0 {
		prompt += " The document that shows it captions it: " + strings.Join(alts, "; ") + "."
	}
	body, err := json.Marshal(map[string]any{
		"model":      v.Model,
		"max_tokens": visionMaxTokens,
		"messages": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": prompt},
			map[string]any{"type": "image_url", "image_url": map[string]any{
				"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)}},
		}}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimSuffix(v.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if v.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+v.APIKey)
	}
	resp, err := (&http.Client{Timeout: visionTimeout}).Do(req) //nolint:gosec // G704: the endpoint the operator configured for vision
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	return visionAnswer(resp)
}

// visionAnswer reads a chat completion's first message content.
func visionAnswer(resp *http.Response) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the vision endpoint returned %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("the vision endpoint's answer is not a chat completion: %w", err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", errors.New("the model returned no description (a reasoning model may need a larger token budget)")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
