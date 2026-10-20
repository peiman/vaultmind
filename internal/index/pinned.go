package index

import "fmt"

// PinnedIDs lists notes whose frontmatter says pinned: true, ordered by id.
//
// The value compared here is the JSON encoding of a YAML boolean. A quoted
// scalar ("true") is a different value and does not count. The index already
// holds that encoding in frontmatter_kv, so this does not re-read the file.
func PinnedIDs(db *DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT DISTINCT n.id
		FROM notes n
		INNER JOIN frontmatter_kv kv ON kv.note_id = n.id
		WHERE kv.key = 'pinned' AND kv.value_json = 'true'
		ORDER BY n.id`)
	if err != nil {
		return nil, fmt.Errorf("listing pinned notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning pinned note id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pinned notes: %w", err)
	}
	return ids, nil
}
