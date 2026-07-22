//go:build windows

package executor

import "os"

func sigUSR1() os.Signal {
	return nil
}
