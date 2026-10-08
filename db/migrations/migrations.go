// Package migrations contains the schema shipped with this application version.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS

const Version int32 = 5
