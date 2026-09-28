package cmd

// noIndexHash stands in for vdb.GetIndexHash where no vault was opened (an
// error, or `frontmatter validate --live`). Helpers hand back the accessor,
// not the hash, so a command pays for reading the whole index only on the
// JSON path that reports it.
func noIndexHash() string { return "" }
