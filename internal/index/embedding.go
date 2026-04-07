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

// NoteEmbedding pairs a note ID with its embedding vector.
type NoteEmbedding struct {
	NoteID    string
	Embedding []float32
}

// StoreEmbedding writes an embedding BLOB for a note that already exists in the index.
func StoreEmbedding(d *DB, noteID string, vec []float32) error {
	_, err := d.Exec("UPDATE notes SET embedding = ? WHERE id = ?", EncodeEmbedding(vec), noteID)
	if err != nil {
		return fmt.Errorf("storing embedding for %q: %w", noteID, err)
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

// LoadAllEmbeddings returns all notes that have stored embeddings.
func LoadAllEmbeddings(d *DB) ([]NoteEmbedding, error) {
	rows, err := d.Query("SELECT id, embedding FROM notes WHERE embedding IS NOT NULL")
	if err != nil {
		return nil, fmt.Errorf("loading all embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []NoteEmbedding
	for rows.Next() {
		var ne NoteEmbedding
		var data []byte
		if err := rows.Scan(&ne.NoteID, &data); err != nil {
			return nil, fmt.Errorf("scanning embedding row: %w", err)
		}
		vec, decErr := DecodeEmbedding(data)
		if decErr != nil {
			return nil, fmt.Errorf("decoding embedding for %q: %w", ne.NoteID, decErr)
		}
		ne.Embedding = vec
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
