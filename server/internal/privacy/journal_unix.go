//go:build linux || darwin || freebsd

package privacy

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

func privateOwned(info os.FileInfo, directory bool) bool {
	s, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(s.Uid) == os.Geteuid() && info.Mode().Perm()&0077 == 0 && ((directory && info.IsDir()) || (!directory && info.Mode().IsRegular() && s.Nlink == 1))
}
func lockJournal(ctx context.Context, f *os.File) error {
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		e := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			return nil
		}
		if !errors.Is(e, syscall.EWOULDBLOCK) && !errors.Is(e, syscall.EAGAIN) {
			return ErrJournal
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func unlockJournal(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
