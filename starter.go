package starter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
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

	Interval time.Duration

	// Signal to send when HUP is received
	SignalOnHUP os.Signal

	// Signal to send when TERM is received
	SignalOnTERM os.Signal

	// KillOlddeplay is time to suspend to send a signal to the old worker.
	KillOldDelay time.Duration

	// if set, writes the status of the server process(es) to the file
	StatusFile string

	Logger *log.Logger

	listeners  []net.Listener
	generation int

	mu       sync.RWMutex
	chreload chan struct{}
	workers  map[*worker]struct{}
}

// Run starts the specified command.
func (s *Starter) Run() error {
	if err := s.listen(context.Background()); err != nil {
		return err
	}
	if _, err := s.startWorker(context.Background()); err != nil {
		return err
	}

	// TODO: watch the workers, and wail for stopping all.
	select {}
}

type worker struct {
	ctx        context.Context
	cancel     context.CancelFunc
	cmd        *exec.Cmd
	generation int
	starter    *Starter
	chsig      chan os.Signal
}

func (s *Starter) startWorker(ctx context.Context) (*worker, error) {
RETRY:
	w, err := s.tryToStartWorker(context.Background())
	if err != nil {
		return nil, err
	}
	s.logf("starting new worker %d", w.Pid())

	timer := time.NewTimer(s.interval())
	select {
	case <-w.ctx.Done():
	case <-timer.C:
		timer.Reset(0)
	}

	if state := w.ProcessState(); state != nil {
		var msg string
		if s, ok := state.Sys().(syscall.WaitStatus); ok && s.Exited() {
			msg = "exit status: " + strconv.Itoa(s.ExitStatus())
		} else {
			msg = state.String()
		}
		s.logf("new worker %d seems to have failed to start, %s", w.Pid(), msg)
		<-timer.C
		goto RETRY
	}
	timer.Stop()

	return w, nil
}

func (s *Starter) tryToStartWorker(ctx context.Context) (*worker, error) {
	type filer interface {
		File() (*os.File, error)
	}
	files := make([]*os.File, len(s.listeners))
	ports := make([]string, len(s.listeners))
	for i, l := range s.listeners {
		f, err := l.(filer).File()
		if err != nil {
			return nil, err
		}
		files[i] = f

		// file descriptor numbers in ExtraFiles turn out to be
		// index + 3, so we can just hard code it
		ports[i] = fmt.Sprintf("%s=%d", l.Addr().String(), i+3)
	}

	s.generation++
	ctx, cancel := context.WithCancel(ctx)
	env := os.Environ()
	cmd := exec.CommandContext(ctx, s.Command, s.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = files
	env = append(env, fmt.Sprintf("%s=%s", PortEnvName, strings.Join(ports, ";")))
	env = append(env, fmt.Sprintf("%s=%d", GenerationEnvName, s.generation))
	cmd.Env = env
	w := &worker{
		ctx:        ctx,
		cancel:     cancel,
		cmd:        cmd,
		generation: s.generation,
		starter:    s,
		chsig:      make(chan os.Signal, 1),
	}

	if err := w.cmd.Start(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	if s.workers == nil {
		s.workers = make(map[*worker]struct{})
	}
	s.workers[w] = struct{}{}
	s.mu.Unlock()
	s.updateStatus()

	go w.Wait()
	return w, nil
}

func (w *worker) Wait() error {
	defer w.close()

	done := make(chan struct{})
	go func() {
		w.cmd.Wait()
		close(done)

		var msg string
		state := w.cmd.ProcessState
		if s, ok := state.Sys().(syscall.WaitStatus); ok && s.Exited() {
			msg = "exit status: " + strconv.Itoa(s.ExitStatus())
		} else {
			msg = state.String()
		}

		// TODO: check the worker is currect.
		w.starter.logf("old worker %d died, %s", w.Pid(), msg)
	}()

	for {
		select {
		case sig := <-w.chsig:
			w.cmd.Process.Signal(sig)
		case <-done:
			return nil
		}
	}
}

func (w *worker) Pid() int {
	return w.cmd.Process.Pid
}

func (w *worker) Signal(sig os.Signal) {
	w.chsig <- sig
}

// ProcessState contains information about an exited process.
// Return nil while the worker is running.
func (w *worker) ProcessState() *os.ProcessState {
	select {
	case <-w.ctx.Done():
	default:
		return nil
	}
	return w.cmd.ProcessState
}

func (w *worker) close() error {
	for _, f := range w.cmd.ExtraFiles {
		f.Close()
	}
	w.cancel()

	w.starter.mu.Lock()
	delete(w.starter.workers, w)
	w.starter.mu.Unlock()
	w.starter.updateStatus()
	return nil
}

func (s *Starter) listen(ctx context.Context) error {
	var listeners []net.Listener
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
		listeners = append(listeners, l)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = listeners
	return nil
}

// Listeners returns the listeners.
func (s *Starter) Listeners() []net.Listener {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listeners
}

// Reload XX
func (s *Starter) Reload(ctx context.Context) error {
	chreload := s.getChReaload()
	select {
	case chreload <- struct{}{}:
		defer func() {
			<-chreload
		}()
	default:
		return nil
	}

	s.logf("received HUP, spawning a new worker")

RETRY:
	w, err := s.startWorker(context.Background())
	if err != nil {
		return err
	}

	tmp := s.listWorkers()
	workers := tmp[:0]
	for _, w2 := range tmp {
		if w2 != w {
			workers = append(workers, w2)
		}
	}
	pids := "none"
	if len(workers) > 0 {
		var b strings.Builder
		for _, w := range workers {
			fmt.Fprintf(&b, "%d,", w.Pid())
		}
		pids = b.String()
		pids = pids[:len(pids)-1] // remove last ','
	}
	s.logf("new worker is now running, sending SIGTERM to old workers: %s", pids)

	if delay := s.killOldDelay(); delay > 0 {
		s.logf("sleeping %d secs before killing old workers", int64(delay/time.Second))
		timer := time.NewTimer(s.killOldDelay())
		select {
		case <-timer.C:
		case <-w.ctx.Done():
			timer.Stop()
			// the new worker dies during sleep, restarting.
			state := w.ProcessState()
			var msg string
			if s, ok := state.Sys().(syscall.WaitStatus); ok && s.Exited() {
				msg = "exit status: " + strconv.Itoa(s.ExitStatus())
			} else {
				msg = state.String()
			}
			s.logf("worker %d died unexpectedly with %s, restarting", w.Pid(), msg)
			goto RETRY
		}
	}

	s.logf("killing old workers")
	for _, w := range workers {
		w.Signal(s.signalOnHUP())
	}

	return nil
}

func (s *Starter) getChReaload() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chreload == nil {
		s.chreload = make(chan struct{}, 1)
	}
	return s.chreload
}

func (s *Starter) interval() time.Duration {
	if s.Interval > 0 {
		return s.Interval
	}
	return time.Second
}

func (s *Starter) killOldDelay() time.Duration {
	if s.KillOldDelay > 0 {
		return s.KillOldDelay
	}
	return 0 // TODO: The default value is 5 when --enable-auto-restart is set
}

func (s *Starter) signalOnHUP() os.Signal {
	if s.SignalOnHUP != nil {
		return s.SignalOnHUP
	}
	return syscall.SIGTERM
}

func (s *Starter) signalOnTERM() os.Signal {
	if s.SignalOnTERM != nil {
		return s.SignalOnTERM
	}
	return syscall.SIGTERM
}

func (s *Starter) listWorkers() []*worker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workers := make([]*worker, 0, len(s.workers))
	for w := range s.workers {
		workers = append(workers, w)
	}
	sort.Slice(workers, func(i, j int) bool {
		return workers[i].Pid() < workers[i].Pid()
	})
	return workers
}

// Shutdown terminates all workers.
func (s *Starter) Shutdown(ctx context.Context) error {
	workers := s.listWorkers()
	for _, w := range workers {
		w.Signal(s.signalOnTERM())
	}
	for _, w := range workers {
		select {
		case <-w.ctx.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// updateStatus writes the workers' status into StatusFile.
func (s *Starter) updateStatus() {
	if s.StatusFile == "" {
		return // nothing to do
	}
	workers := s.listWorkers()
	sort.Slice(workers, func(i, j int) bool {
		return workers[i].generation < workers[i].generation
	})

	var buf bytes.Buffer
	for _, w := range workers {
		fmt.Fprintf(&buf, "%d:%d\n", w.generation, w.Pid())
	}
	tmp := fmt.Sprintf("%s.%d", s.StatusFile, os.Getegid())
	if err := ioutil.WriteFile(tmp, buf.Bytes(), 0666); err != nil {
		s.logf("failed to create temporary file:%s:%s", tmp, err)
		return
	}
	if err := os.Rename(tmp, s.StatusFile); err != nil {
		s.logf("failed to rename %s to %s:%s", tmp, s.StatusFile, err)
		return
	}
}

func (s *Starter) logf(format string, args ...interface{}) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}
