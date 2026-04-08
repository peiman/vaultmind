package index

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// EncodeEmbedding serializes a float32 slice to raw little-endian bytes for BLOB storage.
func EncodeEmbedding(vec []float32) []byte {
	if len(vec) == 0 {
		return nil
	}
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// DecodeEmbedding deserializes raw little-endian bytes back to a float32 slice.
func DecodeEmbedding(data []byte) ([]float32, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data)%4 != 0 {
		return nil, fmt.Errorf("invalid embedding data: length %d not divisible by 4", len(data))
	}
	vec := make([]float32, len(data)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return vec, nil
}

// EncodeSparseEmbedding serializes a sparse vector as packed (int32 token_id, float32 weight) pairs.
func EncodeSparseEmbedding(sparse map[int32]float32) []byte {
	if len(sparse) == 0 {
		return nil
	}
	buf := make([]byte, len(sparse)*8)
	i := 0
	for id, w := range sparse {
		binary.LittleEndian.PutUint32(buf[i:], uint32(id)) //nolint:gosec // G115: intentional bit-pattern reinterpretation, not arithmetic
		binary.LittleEndian.PutUint32(buf[i+4:], math.Float32bits(w))
		i += 8
	}
	return buf
}

// DecodeSparseEmbedding deserializes packed (int32, float32) pairs back to a sparse map.
// Returns an empty map (not nil) when data is empty.
func DecodeSparseEmbedding(data []byte) (map[int32]float32, error) {
	if len(data) == 0 {
		return map[int32]float32{}, nil
	}
	if len(data)%8 != 0 {
		return nil, fmt.Errorf("invalid sparse embedding data: length %d not divisible by 8", len(data))
	}
	sparse := make(map[int32]float32, len(data)/8)
	for i := 0; i < len(data); i += 8 {
		id := int32(binary.LittleEndian.Uint32(data[i:])) //nolint:gosec // G115: intentional bit-pattern reinterpretation, not arithmetic
		w := math.Float32frombits(binary.LittleEndian.Uint32(data[i+4:]))
		sparse[id] = w
	}
	return sparse, nil
}

// EncodeColBERTEmbedding serializes a per-token embedding matrix as flat float32 bytes.
func EncodeColBERTEmbedding(colbert [][]float32) []byte {
	if len(colbert) == 0 {
		return nil
	}
	dims := len(colbert[0])
	buf := make([]byte, len(colbert)*dims*4)
	offset := 0
	for _, vec := range colbert {
		for _, v := range vec {
			binary.LittleEndian.PutUint32(buf[offset:], math.Float32bits(v))
			offset += 4
		}
	}
	return buf
}

// DecodeColBERTEmbedding deserializes flat float32 bytes back to a per-token matrix.
// dims is the embedding dimensionality (e.g., 1024 for BGE-M3).
func DecodeColBERTEmbedding(data []byte, dims int) ([][]float32, error) {
	if len(data) == 0 {
		return nil, nil
	}
	bytesPerVec := dims * 4
	if len(data)%bytesPerVec != 0 {
		return nil, fmt.Errorf("invalid ColBERT embedding data: length %d not divisible by %d", len(data), bytesPerVec)
	}
	nTokens := len(data) / bytesPerVec
	result := make([][]float32, nTokens)
	for i := 0; i < nTokens; i++ {
		vec := make([]float32, dims)
		for j := 0; j < dims; j++ {
			offset := (i*dims + j) * 4
			vec[j] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
		}
		result[i] = vec
	}
	return result, nil
}

// NoteEmbedding pairs a note ID with its embedding vector and metadata.
type NoteEmbedding struct {
	NoteID    string
	Embedding []float32
	Type      string
	Title     string
	Path      string
	BodyText  string
	IsDomain  bool
}

// StoreEmbedding writes an embedding BLOB for a note that already exists in the index.
func StoreEmbedding(d *DB, noteID string, vec []float32) error {
	result, err := d.Exec("UPDATE notes SET embedding = ? WHERE id = ?", EncodeEmbedding(vec), noteID)
	if err != nil {
		return fmt.Errorf("storing embedding for %q: %w", noteID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no note found with id %q", noteID)
	}
	return nil
}

// LoadEmbedding reads the embedding for a single note. Returns nil, nil if no embedding stored.
func LoadEmbedding(d *DB, noteID string) ([]float32, error) {
	var data []byte
	err := d.QueryRow("SELECT embedding FROM notes WHERE id = ?", noteID).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("loading embedding for %q: %w", noteID, err)
	}
	if data == nil {
		return nil, nil
	}
	return DecodeEmbedding(data)
}

// LoadAllEmbeddings returns all notes that have stored embeddings, including metadata.
// This is a single query that avoids N+1 lookups when scoring and filtering results.
func LoadAllEmbeddings(d *DB) ([]NoteEmbedding, error) {
	rows, err := d.Query(`SELECT id, embedding, type, title, path, body_text, is_domain
		FROM notes WHERE embedding IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("loading all embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []NoteEmbedding
	for rows.Next() {
		var ne NoteEmbedding
		var data []byte
		var noteType, title, path, bodyText sql.NullString
		if err := rows.Scan(&ne.NoteID, &data, &noteType, &title, &path, &bodyText, &ne.IsDomain); err != nil {
			return nil, fmt.Errorf("scanning embedding row: %w", err)
		}
		vec, decErr := DecodeEmbedding(data)
		if decErr != nil {
			return nil, fmt.Errorf("decoding embedding for %q: %w", ne.NoteID, decErr)
		}
		ne.Embedding = vec
		ne.Type = noteType.String
		ne.Title = title.String
		ne.Path = path.String
		ne.BodyText = bodyText.String
		result = append(result, ne)
	}
	return result, rows.Err()
}

// HasEmbeddings returns true if any note in the index has a stored embedding.
func HasEmbeddings(d *DB) (bool, error) {
	var exists int
	err := d.QueryRow("SELECT 1 FROM notes WHERE embedding IS NOT NULL LIMIT 1").Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("checking for embeddings: %w", err)
	}
	return true, nil
}
