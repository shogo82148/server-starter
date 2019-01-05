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
	"os/signal"
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

	// Paths at where to listen using unix socket.
	Paths []string

	Interval time.Duration

	// Signal to send when HUP is received
	SignalOnHUP os.Signal

	// Signal to send when TERM is received
	SignalOnTERM os.Signal

	// KillOlddeplay is time to suspend to send a signal to the old worker.
	KillOldDelay time.Duration

	// if set, writes the status of the server process(es) to the file
	StatusFile string

	// if set, writes the process id of the start_server process to the file
	PidFile string

	// TODO:
	EnvDir              string
	EnableAutoRestart   bool
	AutoRestartInterval time.Duration
	Restart             bool
	Stop                bool
	Help                bool
	Version             bool
	Daemonize           bool
	LogFile             string
	Dir                 string

	Logger *log.Logger

	listeners  []net.Listener
	generation int
	ctx        context.Context
	cancel     context.CancelFunc
	pidFile    *os.File

	wg       sync.WaitGroup
	mu       sync.RWMutex
	chreload chan struct{}
	chstart  chan struct{}
	workers  map[*worker]struct{}
}

// Run starts the specified command.
func (s *Starter) Run() error {
	go s.waitSignal()

	if err := s.openPidFile(); err != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel
	if err := s.listen(); err != nil {
		return err
	}
	w, err := s.startWorker()
	if err != nil {
		return err
	}
	w.Watch()

	s.wg.Wait()
	return nil
}

func (s *Starter) openPidFile() error {
	if s.PidFile == "" {
		return nil
	}
	f, err := os.OpenFile(s.PidFile, os.O_EXCL|os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	return nil
}

func (s *Starter) waitSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(
		ch,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)
	for sig := range ch {
		sig := sig
		switch sig {
		case syscall.SIGHUP:
			s.logf("received HUP, spawning a new worker")
			go s.Reload()
		default:
			go s.shutdownBySignal(sig)
		}
	}
}

type worker struct {
	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd

	// done is closed if cmd.Wait has finished.
	// after closed, cmd.ProcessState is available.
	done chan struct{}

	generation int
	starter    *Starter
	chsig      chan workerSignal
}

type workerState int

const (
	// workerStateInit is initail status of worker.
	// thw worker restarts itself if necessary.
	workerStateInit workerState = iota

	// workerStateOld means the worker is marked as old.
	// The Stater starts another new worker, so the worker does nothing.
	workerStateOld

	// workerStateShutdown means the Starter is shutting down.
	workerStateShutdown
)

type workerSignal struct {
	// signal to send the worker process.
	signal os.Signal

	state workerState
}

func (s *Starter) startWorker() (*worker, error) {
RETRY:
	w, err := s.tryToStartWorker()
	if err != nil {
		return nil, err
	}
	s.logf("starting new worker %d", w.Pid())

	var state *os.ProcessState
	timer := time.NewTimer(s.interval())
	select {
	case <-w.done:
		state = w.cmd.ProcessState
	case <-timer.C:
		timer.Reset(0)
	}

	if state != nil {
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

func (s *Starter) tryToStartWorker() (*worker, error) {
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
	ctx, cancel := context.WithCancel(s.ctx)
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
		done:       make(chan struct{}),
		generation: s.generation,
		starter:    s,
		chsig:      make(chan workerSignal),
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
	w.Wait()

	return w, nil
}

func (w *worker) Wait() {
	w.starter.wg.Add(1)
	go w.wait()
}

func (w *worker) wait() {
	defer w.starter.wg.Done()
	defer w.close()
	w.cmd.Wait()
}

// start to watch the worker itself.
// after call the Watch, the worker watches its process and restart itself if necessary.
func (w *worker) Watch() {
	w.starter.wg.Add(1)
	go w.watch()
}

func (w *worker) watch() {
	s := w.starter
	defer s.wg.Done()
	state := workerStateInit
	for {
		select {
		case sig := <-w.chsig:
			state = sig.state
			err := w.cmd.Process.Signal(sig.signal)
			if err != nil {
				s.logf("failed to send signal %s to %d", signalToName(sig.signal), w.Pid())
			}
		case <-w.done:
			var msg string
			st := w.cmd.ProcessState
			if s, ok := st.Sys().(syscall.WaitStatus); ok && s.Exited() {
				msg = "status: " + strconv.Itoa(s.ExitStatus())
			} else {
				msg = st.String()
			}
			switch state {
			case workerStateInit:
				s.logf("worker %d died unexpectedly with %s, restarting", w.Pid(), msg)
				go func() {
					w, err := s.startWorker()
					if err != nil {
						return
					}
					w.Watch()
				}()
			case workerStateOld:
				s.logf("old worker %d died, %s", w.Pid(), msg)
			case workerStateShutdown:
				s.logf("worker %d died, %s", w.Pid(), msg)
			default:
				panic(fmt.Sprintf("unknown state: %d", state))
			}
			return
		}
	}
}

func (w *worker) Pid() int {
	return w.cmd.Process.Pid
}

func (w *worker) Signal(sig os.Signal, state workerState) {
	s := workerSignal{
		signal: sig,
		state:  state,
	}
	select {
	case w.chsig <- s:
	case <-w.ctx.Done():
	}
}

func (w *worker) close() error {
	for _, f := range w.cmd.ExtraFiles {
		f.Close()
	}
	w.cancel()
	close(w.done)

	w.starter.mu.Lock()
	delete(w.starter.workers, w)
	w.starter.mu.Unlock()
	w.starter.updateStatus()
	return nil
}

func (s *Starter) listen() error {
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
		l, err := lc.Listen(s.ctx, "tcp", port)
		if err != nil {
			s.logf("failed to listen to %s:%s", port, err)
			return err
		}
		listeners = append(listeners, l)
	}

	for _, path := range s.Paths {
		if stat, err := os.Lstat(path); err == nil && stat.Mode()&os.ModeSocket == os.ModeSocket {
			s.logf("removing existing socket file:%s", path)
			if err := os.Remove(path); err != nil {
				s.logf("failed to remove existing socket file:%s:%s", path, err)
			}
		}
		_ = os.Remove(path)
		l, err := lc.Listen(s.ctx, "unix", path)
		if err != nil {
			s.logf("failed to listen to file %s:%s", path, err)
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
func (s *Starter) Reload() error {
	chreload := s.getChReaload()
	select {
	case chreload <- struct{}{}:
		defer func() {
			<-chreload
		}()
	default:
		return nil
	}

RETRY:
	w, err := s.startWorker()
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
	s.logf("new worker is now running, sending %s to old workers: %s", signalToName(s.signalOnHUP()), pids)

	if delay := s.killOldDelay(); delay > 0 {
		s.logf("sleeping %d secs before killing old workers", int64(delay/time.Second))
		timer := time.NewTimer(s.killOldDelay())
		select {
		case <-timer.C:
		case <-w.done:
			timer.Stop()
			// the new worker dies during sleep, restarting.
			state := w.cmd.ProcessState
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
	w.Watch()

	s.logf("killing old workers")
	for _, w := range workers {
		w.Signal(s.signalOnHUP(), workerStateOld)
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
		w.Signal(s.signalOnTERM(), workerStateShutdown)
	}
	for _, w := range workers {
		select {
		case <-w.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.Close()
}

func (s *Starter) shutdownBySignal(recv os.Signal) {
	signal := os.Signal(syscall.SIGTERM)
	if recv == syscall.SIGTERM {
		signal = s.signalOnTERM()
	}
	workers := s.listWorkers()
	var buf strings.Builder
	for _, w := range workers {
		buf.WriteByte(',')
		buf.WriteString(strconv.Itoa(w.Pid()))
	}
	if len(workers) == 0 {
		buf.WriteString(",none")
	}
	s.logf("received %s, sending %s to all workers:%s", recv, signalToName(signal), buf.String()[1:])

	for _, w := range workers {
		w.Signal(signal, workerStateShutdown)
	}
	s.Close()
	s.logf("exiting")
}

// Close terminates all workers immediately.
func (s *Starter) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	for _, l := range s.Listeners() {
		l.Close()
		if l, ok := l.(*net.UnixListener); ok {
			os.Remove(l.Addr().String())
		}
	}
	s.wg.Wait()
	if f := s.pidFile; f != nil {
		if err := os.Remove(f.Name()); err != nil {
			s.logf("failed to unlink file:%s:%s", f.Name(), err)
		}
		f.Close()
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
		return workers[i].generation < workers[j].generation
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
