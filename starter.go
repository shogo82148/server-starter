package starter

import (
	"log"
	"net"
	"os"
	"os/exec"
)

// Starter is an implement of Server::Starter.
type Starter struct {
	Command string
	Args    []string
	Logger  *log.Logger
}

// Run starts the specified command.
func (s *Starter) Run() error {
	l, err := net.Listen("tcp", "0.0.0.0:12345") // TODO: read Port param
	if err != nil {
		return err
	}
	f, err := l.(*net.TCPListener).File()
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command(s.Command, s.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{f}
	cmd.Env = append(os.Environ(), "SERVER_STARTER_PORT=0.0.0.0:12345=3")
	if err := cmd.Start(); err != nil {
		s.logf("failed to exec %s: %s", s.Command, err)
	} else {
		pid := cmd.Process.Pid
		s.logf("starting new worker %d", pid)
	}
	cmd.Wait()
	return nil
}

func (s *Starter) logf(format string, args ...interface{}) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}
