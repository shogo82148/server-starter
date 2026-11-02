package starter

import "golang.org/x/sys/unix"

// processUIDs returns the real and saved user IDs of the process.
func processUIDs(pid int) (ruid, suid int, err error) {
	kinfo, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, 0, err
	}
	return int(kinfo.Eproc.Pcred.P_ruid), int(kinfo.Eproc.Pcred.P_svuid), nil
}
