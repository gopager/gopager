// Package migrations owns the release's reviewed, embedded Goose SQL collection.
package migrations

import "embed"

// Files is the complete ordered domain migration collection.
//
//go:embed *.sql
var Files embed.FS
