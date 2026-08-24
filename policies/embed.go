package policies

import "embed"

// FS contains the mandatory baseline and example policy modules.
//
//go:embed baseline/*.rego examples/*.rego
var FS embed.FS
