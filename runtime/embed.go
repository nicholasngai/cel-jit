package runtime

import "embed"

//go:embed go.mod embed.go overloads.go path.go runtime.go types.go
var Source embed.FS
