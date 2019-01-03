package starter

import (
	"log"
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
	cmd := exec.Command(s.Command, s.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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
