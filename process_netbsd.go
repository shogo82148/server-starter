package starter

import (
	"encoding/binary"
	"errors"

	"golang.org/x/sys/unix"
)

// offsets in struct kinfo_proc2 (sys/sysctl.h).
// its fields have fixed sizes and alignments on all architectures.
const (
	kernProcPID = 1 // KERN_PROC_PID

	kinfoPidOffset   = 116
	kinfoRuidOffset  = 140
	kinfoSvuidOffset = 640
	kinfoSize        = kinfoSvuidOffset + 4
)

// processUIDs returns the real and saved user IDs of the process.
func processUIDs(pid int) (ruid, suid int, err error) {
	buf, err := unix.SysctlRaw("kern.proc2", kernProcPID, pid, kinfoSize, 1)
	if err != nil {
		return 0, 0, err
	}
	if len(buf) < kinfoSize {
		return 0, 0, errors.New("process not found")
	}
	if int(int32(binary.NativeEndian.Uint32(buf[kinfoPidOffset:]))) != pid {
		return 0, 0, errors.New("unexpected kinfo_proc2 layout")
	}
	ruid = int(binary.NativeEndian.Uint32(buf[kinfoRuidOffset:]))
	suid = int(binary.NativeEndian.Uint32(buf[kinfoSvuidOffset:]))
	return ruid, suid, nil
}
