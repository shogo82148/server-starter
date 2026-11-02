package starter

import (
	"encoding/binary"
	"errors"
	"runtime"

	"golang.org/x/sys/unix"
)

// offsets in struct kinfo_proc (sys/sysctl.h).
const (
	kernProcPID = 1 // KERN_PROC_PID

	kinfoPidOffset  = 108
	kinfoRuidOffset = 132
)

// kinfoSvuidOffset returns the offset of p_svuid.
// It follows p_uru_maxrss, which is 8-byte aligned except on 386.
func kinfoSvuidOffset() int {
	if runtime.GOARCH == "386" {
		return 564
	}
	return 568
}

// processUIDs returns the real and saved user IDs of the process.
func processUIDs(pid int) (ruid, suid int, err error) {
	svuidOffset := kinfoSvuidOffset()
	size := svuidOffset + 4
	buf, err := unix.SysctlRaw("kern.proc", kernProcPID, pid, size, 1)
	if err != nil {
		return 0, 0, err
	}
	if len(buf) < size {
		return 0, 0, errors.New("process not found")
	}
	if int(int32(binary.NativeEndian.Uint32(buf[kinfoPidOffset:]))) != pid {
		return 0, 0, errors.New("unexpected kinfo_proc layout")
	}
	ruid = int(binary.NativeEndian.Uint32(buf[kinfoRuidOffset:]))
	suid = int(binary.NativeEndian.Uint32(buf[svuidOffset:]))
	return ruid, suid, nil
}
