//go:build !windows

package cli

import "syscall"

// detachSysProcAttr makes the mocks daemon a session leader so it survives
// the parent process and terminal closing (no SIGHUP from the controlling
// terminal's process group).
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
