package celjit

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nicholasngai/cel-jit/runtime"
)

var (
	// The source runtime directory that we wrote to a temporary directory.
	stdRuntimeDir      string
	stdRuntimeRefCount int
	stdRuntimeMu       sync.Mutex
)

func getStdRuntimeDir() (string, error) {
	// Look for GOPATH directory first. Check for the module path prefix
	// followed by @ to detect -trimpath binaries.
	if runtimeSource, ok := runtime.ModuleSourceDirectory(); ok && !strings.HasPrefix(runtimeSource, "github.com/nicholasngai/cel-jit/runtime@") {
		return runtimeSource, nil
	}

	stdRuntimeMu.Lock()
	defer stdRuntimeMu.Unlock()

	// If GOPATH dir couldn't be found, look for one that we wrote already.
	if stdRuntimeDir != "" {
		return stdRuntimeDir, nil
	}

	// We haven't written a temporary runtime dir yet. Write it.
	writtenDir, err := writeStdRuntime()
	if err != nil {
		return "", err
	}

	stdRuntimeDir = writtenDir
	stdRuntimeRefCount += 1

	return writtenDir, nil
}

func closeStdRuntime() {
	stdRuntimeMu.Lock()
	defer stdRuntimeMu.Unlock()

	if stdRuntimeRefCount > 0 {
		stdRuntimeRefCount -= 1
		if stdRuntimeRefCount == 0 {
			if stdRuntimeDir != "" {
				_ = os.RemoveAll(stdRuntimeDir)
				stdRuntimeDir = ""
			}
		}
	}
}

func writeStdRuntime() (_ string, retErr error) {
	// Make runtime dir.
	dir, err := os.MkdirTemp("", "cel-jit-std-runtime-source-*")
	if err != nil {
		return "", fmt.Errorf("mkdir temp: %w", err)
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(dir)
		}
	}()

	// Write Go module file.
	if err := writeFilef(filepath.Join(dir, "go.mod"),
		`module github.com/nicholasngai/cel-jit/runtime

go 1.25.0
`,
	); err != nil {
		return "", err
	}

	// Write the runtime to it.
	if err := fs.WalkDir(runtime.Source, ".", func(runtimePath string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if runtimePath == "." {
			return nil
		}
		if dirEntry.IsDir() {
			if err := os.Mkdir(filepath.Join(dir, runtimePath), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Join(dir, runtimePath), err)
			}
		} else {
			contents, err := runtime.Source.ReadFile(runtimePath)
			if err != nil {
				return fmt.Errorf("read embedded %s: %w", runtimePath, err)
			}
			if err := writeFilef(filepath.Join(dir, runtimePath), "%s", string(contents)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return "", err
	}

	return dir, nil
}
