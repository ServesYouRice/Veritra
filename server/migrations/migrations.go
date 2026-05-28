package migrations

import "embed"

// FS embeds SQL migrations into the server binary.
//
//go:embed *.sql
var FS embed.FS
