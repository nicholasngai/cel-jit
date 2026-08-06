package runtime

import "embed"

//go:embed runtime.go types.go
var Source embed.FS
