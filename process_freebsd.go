package starter

import (
	"encoding/binary"
	"errors"
	"unsafe"

	"golang.org/x/sys/unix"
)

// offsets in struct kinfo_proc (sys/user.h).
// ki_structsize and ki_layout are followed by eight pointers.
const (
	kinfoPtrs      = 8 + 8*unsafe.Sizeof(uintptr(0))
	kinfoPidOffset = kinfoPtrs
	// ki_pid ... ki_tsid, ki_jobc, ki_spare_short1, ki_tdev_freebsd11 and four sigset_t.
	kinfoRuidOffset  = kinfoPtrs + 6*4 + 2*2 + 4 + 4*16 + 4
	kinfoSvuidOffset = kinfoRuidOffset + 4
)

// processUIDs returns the real and saved user IDs of the process.
func processUIDs(pid int) (ruid, suid int, err error) {
	buf, err := unix.SysctlRaw("kern.proc.pid", pid)
	if err != nil {
		return 0, 0, err
	}
	if len(buf) < int(kinfoSvuidOffset)+4 || int(int32(binary.NativeEndian.Uint32(buf))) != len(buf) {
		return 0, 0, errors.New("unexpected kinfo_proc size")
	}
	if int(int32(binary.NativeEndian.Uint32(buf[kinfoPidOffset:]))) != pid {
		return 0, 0, errors.New("unexpected kinfo_proc layout")
	}
	ruid = int(binary.NativeEndian.Uint32(buf[kinfoRuidOffset:]))
	suid = int(binary.NativeEndian.Uint32(buf[kinfoSvuidOffset:]))
	return ruid, suid, nil
}
