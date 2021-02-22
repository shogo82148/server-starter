package starter

import "syscall"

func init() {
	signalNameTable = append(signalNameTable,
		// cat zerrors_darwin_amd64.go | perl -nle 'print "signalName{syscall.SIG$1, \"$1\"}," if /SIG(\w+)\s*=\s*Signal/'
		signalName{syscall.SIGABRT, "ABRT"},
		signalName{syscall.SIGALRM, "ALRM"},
		signalName{syscall.SIGBUS, "BUS"},
		signalName{syscall.SIGCHLD, "CHLD"},
		signalName{syscall.SIGCONT, "CONT"},
		signalName{syscall.SIGEMT, "EMT"},
		signalName{syscall.SIGFPE, "FPE"},
		signalName{syscall.SIGHUP, "HUP"},
		signalName{syscall.SIGILL, "ILL"},
		signalName{syscall.SIGINFO, "INFO"},
		signalName{syscall.SIGINT, "INT"},
		signalName{syscall.SIGIO, "IO"},
		signalName{syscall.SIGIOT, "IOT"},
		signalName{syscall.SIGKILL, "KILL"},
		signalName{syscall.SIGPIPE, "PIPE"},
		signalName{syscall.SIGPROF, "PROF"},
		signalName{syscall.SIGQUIT, "QUIT"},
		signalName{syscall.SIGSEGV, "SEGV"},
		signalName{syscall.SIGSTOP, "STOP"},
		signalName{syscall.SIGSYS, "SYS"},
		signalName{syscall.SIGTERM, "TERM"},
		signalName{syscall.SIGTRAP, "TRAP"},
		signalName{syscall.SIGTSTP, "TSTP"},
		signalName{syscall.SIGTTIN, "TTIN"},
		signalName{syscall.SIGTTOU, "TTOU"},
		signalName{syscall.SIGURG, "URG"},
		signalName{syscall.SIGUSR1, "USR1"},
		signalName{syscall.SIGUSR2, "USR2"},
		signalName{syscall.SIGVTALRM, "VTALRM"},
		signalName{syscall.SIGWINCH, "WINCH"},
		signalName{syscall.SIGXCPU, "XCPU"},
		signalName{syscall.SIGXFSZ, "XFSZ"},
	)
}
