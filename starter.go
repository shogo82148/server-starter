package starter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// PortEnvName is the environment name for server_starter configures.
const PortEnvName = "SERVER_STARTER_PORT"

// GenerationEnvName is the environment name for the generation number.
const GenerationEnvName = "SERVER_STARTER_GENERATION"

// Starter is an implement of Server::Starter.
type Starter struct {
	Command string
	Args    []string

	// Ports to bind to (addr:port or port, so it's a string)
	Ports []string

	Logger *log.Logger

	listeners  []net.Listener
	generation int
}

// Run starts the specified command.
func (s *Starter) Run() error {
	if err := s.listen(context.Background()); err != nil {
		return err
	}
	return s.runWorker(context.Background())
}

func (s *Starter) runWorker(ctx context.Context) error {
	type filer interface {
		File() (*os.File, error)
	}
	files := make([]*os.File, len(s.listeners))
	ports := make([]string, len(s.listeners))
	for i, l := range s.listeners {
		f, err := l.(filer).File()
		if err != nil {
			return err
		}
		defer f.Close()
		files[i] = f

		// file descriptor numbers in ExtraFiles turn out to be
		// index + 3, so we can just hard code it
		ports[i] = fmt.Sprintf("%s=%d", l.Addr().String(), i+3)
	}

	env := os.Environ()
	cmd := exec.CommandContext(ctx, s.Command, s.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = files
	env = append(env, fmt.Sprintf("%s=%s", PortEnvName, strings.Join(ports, ";")))
	env = append(env, fmt.Sprintf("%s=%d", GenerationEnvName, s.generation))
	s.generation++
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		s.logf("failed to exec %s: %s", s.Command, err)
	} else {
		pid := cmd.Process.Pid
		s.logf("starting new worker %d", pid)
	}
	return cmd.Wait()
}

func (s *Starter) listen(ctx context.Context) error {
	var lc net.ListenConfig
	for _, port := range s.Ports {
		if idx := strings.LastIndexByte(port, '='); idx >= 0 {
			return errors.New("fd options are not supported")
		}
		if _, err := strconv.Atoi(port); err == nil {
			// by default, only bind to IPv4 (for compatibility)
			port = net.JoinHostPort("0.0.0.0", port)
		}
		l, err := lc.Listen(ctx, "tcp", port)
		if err != nil {
			// TODO: error handling.
			return err
		}
		s.listeners = append(s.listeners, l)
	}
	return nil
}

func (s *Starter) logf(format string, args ...interface{}) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}
