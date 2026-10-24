//go:build windows

package worker

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows a run's processes are one Job Object (ADR-0006). The command starts suspended, joins a job named after its
// pid and creation time, and only then runs, so everything it starts is in the job. The job kills its processes when the
// worker's handle closes, as the parent-death signal does on Linux: a worker that dies leaves no agent running, and the
// job's name goes with the last handle. While the worker holds the handle, the job's process count is the proof.

const (
	jobObjectQuery     = 0x0004 // JOB_OBJECT_QUERY
	jobObjectTerminate = 0x0008 // JOB_OBJECT_TERMINATE
	terminatedExitCode = 1
)

var procOpenJobObject = windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenJobObjectW")

// jobs holds the worker's handle to each job it made, by name, until the job is proven empty.
var jobs = struct {
	sync.Mutex
	held map[string]windows.Handle
}{held: map[string]windows.Handle{}}

// basicAccounting is JOBOBJECT_BASIC_ACCOUNTING_INFORMATION.
type basicAccounting struct {
	TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
	TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
}

// isolate starts cmd suspended, so adopt can put it in its job before it runs, and without a console window.
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW}
}

// adopt puts the suspended process in a new job and lets it run. A process that cannot join one is killed before it
// ran, so the failure is certain.
func adopt(cmd *exec.Cmd) (*LaunchRecord, error) {
	l, err := join(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("cannot put the process in a Job Object: %w", err)
	}
	return l, nil
}

func join(pid int) (*LaunchRecord, error) {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(process)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
		return nil, err
	}
	l := &LaunchRecord{PID: pid, PGID: pid, StartTicks: uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)}
	name, err := windows.UTF16PtrFromString(jobName(l))
	if err != nil {
		return nil, err
	}
	job, err := windows.CreateJobObject(nil, name)
	if err != nil {
		return nil, err
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	if err := resumeThreads(pid); err != nil {
		windows.CloseHandle(job) // kills the process, which never ran
		return nil, err
	}
	jobs.Lock()
	jobs.held[jobName(l)] = job
	jobs.Unlock()
	return l, nil
}

// resumeThreads starts the suspended process. Go keeps no handle to its main thread, so the threads are found by owner.
func resumeThreads(pid int) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := 0
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		_, err = windows.ResumeThread(thread)
		windows.CloseHandle(thread)
		if err != nil {
			return err
		}
		resumed++
	}
	if resumed == 0 {
		return errors.New("its thread was not found")
	}
	return nil
}

func jobName(l *LaunchRecord) string {
	return fmt.Sprintf(`Local\tern-kata-%d-%d`, l.PID, l.StartTicks)
}

// job returns a handle to the run's job, the worker's own when it holds one; release closes one it opened. ok is false
// when no job of that name exists: its last handle closed, which killed whatever was left in it.
func job(l *LaunchRecord, access uint32) (h windows.Handle, release func(), ok bool, err error) {
	jobs.Lock()
	held, have := jobs.held[jobName(l)]
	jobs.Unlock()
	if have {
		return held, func() {}, true, nil
	}
	name, err := windows.UTF16PtrFromString(jobName(l))
	if err != nil {
		return 0, nil, false, err
	}
	r, _, callErr := procOpenJobObject.Call(uintptr(access), 0, uintptr(unsafe.Pointer(name)))
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_FILE_NOT_FOUND) {
			return 0, nil, false, nil
		}
		return 0, nil, false, callErr
	}
	h = windows.Handle(r)
	return h, func() { windows.CloseHandle(h) }, true, nil
}

// signalGroup ends every process of the run's job. Windows has no SIGTERM for processes without a console, so both
// steps of a stop terminate.
func signalGroup(l *LaunchRecord, _ bool) {
	h, release, ok, err := job(l, jobObjectTerminate)
	if err != nil || !ok {
		return
	}
	defer release()
	_ = windows.TerminateJobObject(h, terminatedExitCode)
}

// checkGroup is the group proof: gone when the job is gone or empty, present while it has processes, unchecked (with
// the reason) when opening or querying the job fails for any other reason. Once the worker sees its job empty it closes
// its handle, and the job and its name go.
func checkGroup(l *LaunchRecord) GroupProof {
	h, release, ok, err := job(l, jobObjectQuery)
	if err != nil {
		return GroupProof{State: GroupUnchecked, Reason: "the Job Object cannot be opened: " + err.Error()}
	}
	if !ok {
		return GroupProof{State: GroupGone}
	}
	defer release()
	var info basicAccounting
	if err := windows.QueryInformationJobObject(h, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return GroupProof{State: GroupUnchecked, Reason: "the Job Object cannot be queried: " + err.Error()}
	}
	if info.ActiveProcesses > 0 {
		return GroupProof{State: GroupPresent}
	}
	jobs.Lock()
	if held, have := jobs.held[jobName(l)]; have && held == h {
		delete(jobs.held, jobName(l))
		windows.CloseHandle(held)
	}
	jobs.Unlock()
	return GroupProof{State: GroupGone}
}

// exitSignal is empty: Windows processes end with an exit code only.
func exitSignal(interface{ Sys() any }) string { return "" }
