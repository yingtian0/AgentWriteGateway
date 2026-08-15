package migrations

import "embed"

// FS contains the ordered SQL migrations used by the gateway and tests.
//
//go:embed *.sql
var FS embed.FS
