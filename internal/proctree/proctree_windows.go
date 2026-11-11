package proctree

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Tree is a started command and its descendants.
type Tree struct {
	cmd *exec.Cmd
	job windows.Handle
}

// Start starts cmd suspended, assigns it to a kill-on-close job before any of
// its code can spawn descendants, then resumes it. A failed assignment kills
// the process and fails, rather than racing a later taskkill. It replaces
// cmd.SysProcAttr.
func Start(cmd *exec.Cmd) (*Tree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
		if err == nil {
			resume := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")
			if err = resume.Find(); err == nil {
				status, _, _ := resume.Call(uintptr(process))
				if status != 0 {
					err = syscall.EINVAL
				}
			}
		}
		_ = windows.CloseHandle(process)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = windows.CloseHandle(job)
		_ = cmd.Wait()
		return nil, err
	}
	return &Tree{cmd, job}, nil
}

// Interrupt is Kill: Windows has no signal a console-less tree can be asked
// to stop with.
func (t *Tree) Interrupt() error { return t.Kill() }

// Kill terminates every process in the job.
func (t *Tree) Kill() error { return windows.TerminateJobObject(t.job, 1) }

// Release closes the job handle, which also kills anything left in it.
func (t *Tree) Release() error { return windows.CloseHandle(t.job) }
