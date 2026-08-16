package runtime

import "embed"

//go:embed go.mod *.go
var Source embed.FS
