//go:build !linux && !darwin

package starter

import "errors"

// processUIDs returns the real and saved user IDs of the process.
func processUIDs(pid int) (ruid, suid int, err error) {
	return 0, 0, errors.New("getting the owner of a process is not supported on this platform")
}
