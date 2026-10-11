package migrations

import "embed"

//go:embed postgres/*.sql sqlite/*.sql
var FS embed.FS
