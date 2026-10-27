//go:build linux || darwin

package diagnostics

import (
	"github.com/napisani/secret_inject/internal/privatefs"
	"os"
)

func tryLock(f *os.File) error { return privatefs.TryLock(f) }
func unlock(f *os.File) error  { return privatefs.Unlock(f) }
