package templates

import "embed"

// AuthComponents embeds the canonical login and register component templates
// so `gost make:auth` copies the exact same files that the framework tests (D49).
//
//go:embed components/login components/register
var AuthComponents embed.FS
