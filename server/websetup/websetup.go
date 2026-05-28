package websetup

import "embed"

// FS embeds the setup web UI into the server binary.
//
//go:embed index.html
var FS embed.FS
