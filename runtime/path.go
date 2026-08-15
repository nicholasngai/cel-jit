package runtime

import (
	"path/filepath"
	"runtime"
)

func ModuleSourceDirectory() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	return filepath.Dir(file), true
}
