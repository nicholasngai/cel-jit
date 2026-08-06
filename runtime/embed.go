package runtime

import "embed"

//go:embed overloads.go runtime.go types.go
var Source embed.FS
