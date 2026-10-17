//go:build windows

package contract

import (
	"os/exec"
	"syscall"
)

// Job objects through kernel32 directly, so that the tree kill needs no
// new dependency (spec 0053, "Documentation").
var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = kernel32.NewProc("TerminateJobObject")
)

const (
	processSetQuota  = 0x0100
	processTerminate = 0x0001
)

// procTree kills a run together with its child processes: the run is
// assigned to a job object, and the kill terminates the job (spec 0053,
// "Time limit"). A child the run starts after the assignment belongs to
// the job too.
type procTree struct {
	cmd *exec.Cmd
	job syscall.Handle
}

// newProcTree prepares a tree for cmd. Call it before cmd.Start.
func newProcTree(cmd *exec.Cmd) *procTree { return &procTree{cmd: cmd} }

// attach assigns the started process to a new job object. When that
// fails, kill falls back to killing the process alone.
func (t *procTree) attach() {
	if t.cmd.Process == nil {
		return
	}
	job, _, _ := procCreateJobObjectW.Call(0, 0)
	if job == 0 {
		return
	}
	h, err := syscall.OpenProcess(processSetQuota|processTerminate, false, uint32(t.cmd.Process.Pid)) // #nosec G115 -- a Windows process ID fits in a DWORD
	if err != nil {
		_ = syscall.CloseHandle(syscall.Handle(job))
		return
	}
	defer func() { _ = syscall.CloseHandle(h) }()
	if ok, _, _ := procAssignProcessToJobObject.Call(job, uintptr(h)); ok == 0 {
		_ = syscall.CloseHandle(syscall.Handle(job))
		return
	}
	t.job = syscall.Handle(job)
}

// kill terminates every process of the run's job.
func (t *procTree) kill() error {
	if t.job != 0 {
		if ok, _, _ := procTerminateJobObject.Call(uintptr(t.job), 1); ok != 0 {
			return nil
		}
	}
	if t.cmd.Process == nil {
		return nil
	}
	return t.cmd.Process.Kill()
}

// close releases the job object. Processes still in it keep running.
func (t *procTree) close() {
	if t.job != 0 {
		_ = syscall.CloseHandle(t.job)
		t.job = 0
	}
}
