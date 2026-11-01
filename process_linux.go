package starter

import (
	"bytes"
	"errors"
	"os"
	"strconv"
)

// processUIDs returns the real and saved user IDs of the process.
func processUIDs(pid int) (ruid, suid int, err error) {
	buf, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, 0, err
	}
	for line := range bytes.SplitSeq(buf, []byte{'\n'}) {
		// Uid: real effective saved filesystem
		value, ok := bytes.CutPrefix(line, []byte("Uid:"))
		if !ok {
			continue
		}
		fields := bytes.Fields(value)
		if len(fields) < 3 {
			break
		}
		ruid, err := strconv.Atoi(string(fields[0]))
		if err != nil {
			return 0, 0, err
		}
		suid, err := strconv.Atoi(string(fields[2]))
		if err != nil {
			return 0, 0, err
		}
		return ruid, suid, nil
	}
	return 0, 0, errors.New("uid not found in /proc/" + strconv.Itoa(pid) + "/status")
}
