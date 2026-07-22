//go:build linux || darwin || freebsd || netbsd || openbsd

package executor

import (
	"os"
	"syscall"
)

func sigUSR1() os.Signal {
	return syscall.SIGUSR1
}
