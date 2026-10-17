//go:build !windows

package contract

import (
	"os/exec"
	"syscall"
)

// procTree kills a run together with its child processes: the run starts
// a process group of its own, and the kill signals the whole group
// (spec 0053, "Time limit").
type procTree struct{ cmd *exec.Cmd }

// newProcTree prepares cmd to start its own process group. Call it
// before cmd.Start.
func newProcTree(cmd *exec.Cmd) *procTree {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &procTree{cmd: cmd}
}

// attach is called after cmd.Start. A process group needs nothing more.
func (t *procTree) attach() {}

// kill kills every process of the run's group.
func (t *procTree) kill() error {
	if t.cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return t.cmd.Process.Kill()
	}
	return nil
}

// close releases what attach acquired.
func (t *procTree) close() {}
