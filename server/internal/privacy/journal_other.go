//go:build !linux && !darwin && !freebsd

package privacy

import (
	"context"
	"os"
)

// Unsupported hosts refuse the fixture instead of pretending to serialize files.
func privateOwned(os.FileInfo, bool) bool         { return false }
func lockJournal(context.Context, *os.File) error { return ErrJournal }
func unlockJournal(*os.File)                      {}
