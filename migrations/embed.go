package migrations

import "embed"

// FS contains the ordered SQL migrations used by the Themisy control plane and tests.
//
//go:embed *.sql
var FS embed.FS
