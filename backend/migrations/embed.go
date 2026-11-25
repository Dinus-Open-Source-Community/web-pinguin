package migrations

import "embed"

// Files contains the SQL migrations bundled into the backend tools.
//
//go:embed *.sql
var Files embed.FS
