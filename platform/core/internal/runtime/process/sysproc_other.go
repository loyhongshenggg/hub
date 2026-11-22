//go:build !linux

package process

import "syscall"

// No Pdeathsig outside Linux (macOS release): the core stops its process
// groups on shutdown, and a crash leaves them to the session's next stop.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
