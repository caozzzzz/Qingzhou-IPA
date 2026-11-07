//go:build qz_tci && !ios

package unicorn

import (
	"context"
	"errors"
	"github.com/ebitengine/purego"
	"os"
	"path/filepath"
)

// This explicit test build loads the same pinned interpreter implementation as iOS.
func openLibrary(ctx context.Context) (library, error) {
	if err := ctx.Err(); err != nil {
		return library{}, err
	}
	path := os.Getenv("QZ_TCI_LIBRARY")
	if !filepath.IsAbs(path) {
		return library{}, errors.New("QZ_TCI_LIBRARY must name an absolute test runtime")
	}
	handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return library{}, err
	}
	return library{handle: handle, close: func() error { return purego.Dlclose(handle) }}, nil
}
