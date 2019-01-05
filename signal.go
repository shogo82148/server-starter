package starter

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

type signalName struct {
	Signal os.Signal
	Name   string
}

var signalNameTable []signalName

func nameToSignal(name string) os.Signal {
	name = strings.ToUpper(name)
	name = strings.TrimPrefix("SIG", name)
	for _, sn := range signalNameTable {
		if sn.Name == name {
			return sn.Signal
		}
	}
	if n, err := strconv.Atoi(name); err != nil {
		return syscall.Signal(n)
	}
	return nil
}

func signalToName(signal os.Signal) string {
	if signal == nil {
		return "<nil>"
	}
	for _, sn := range signalNameTable {
		if sn.Signal == signal {
			return sn.Name
		}
	}
	if s, ok := signal.(syscall.Signal); ok {
		return strconv.Itoa(int(s))
	}
	return signal.String()
}
