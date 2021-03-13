package starter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var errShutdown = errors.New("starter: now shutdown")

type socket interface {
	File() (*os.File, error)
	Close() error
}

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

	// working directory, start_server do chdir to before exec (optional)
	Dir string

	// enables automatic restart by time.
	EnableAutoRestart bool

	// automatic restart interval (default 360). It is used with EnableAutoRestart option.
	AutoRestartInterval time.Duration

	// directory that contains environment variables to the server processes.
	EnvDir string

	// prints the version number
	Version bool

	// prints the help message.
	Help bool

	// daemonize the server. (UNIMPLEMENTED)
	Daemonize bool

	// if set, redirects STDOUT and STDERR to given file or command
	LogFile string

	// this is a wrapper command that reads the pid of the start_server process from --pid-file,
	// sends SIGHUP to the process and waits until the server(s) of the older generation(s) die by monitoring the contents of the --status-file
	Restart bool

	// this is a wrapper command that reads the pid of the start_server process from --pid-file, sends SIGTERM to the process.
	Stop bool

	logger logger

	sockets    []socket
	generation int
	ctx        context.Context
	cancel     context.CancelFunc
	pidFile    *os.File

	// wait group for workers
	wgWorker sync.WaitGroup

	// wait group for server starter
	wg sync.WaitGroup

	mu          sync.RWMutex
	shutdown    atomicBool
	chreload    chan struct{}
	chstarter   chan struct{}
	chrestarter chan struct{}
	workers     map[*worker]struct{}
	onceClose   sync.Once
}

// Run starts the specified command.
func (s *Starter) Run() error {
	if s.Version {
		fmt.Printf("Yet Another Go port of start_server by shogo82148 version %s (rev %s) %s/%s (built by %s)\n", version, commit, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return nil
	}
	if s.Help {
		showHelp()
		return nil
	}
	if err := s.openLogFile(); err != nil {
		return err
	}
	if s.Restart {
		return s.restart()
	}
	if s.Stop {
		return s.stop()
	}
	if s.Daemonize {
		s.logf("WARNING: --daemonize is UNIMPLEMENTED")
	}
	if s.Command == "" {
		return errors.New("command is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel
	defer s.Close()

	// block reload during start up
	s.lockReload()

	// start background goroutines
	go s.waitSignal()
	if s.EnableAutoRestart {
		go s.autoRestarter()
	}

	if err := s.openPidFile(); err != nil {
		return err
	}

	if err := s.listen(); err != nil {
		if err == errShutdown {
			return nil
		}
		return err
	}

	// start first generation
	w, err := s.startWorker()
	if err != nil {
		if err == errShutdown {
			return nil
		}
		return err
	}
	w.Watch()

	// enable reload
	s.unlockReload()

	s.wgWorker.Wait()
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
	if err := flock(f.Fd(), syscall.LOCK_EX); err != nil {
		return err
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	s.pidFile = f
	return nil
}

func (s *Starter) openLogFile() error {
	if s.LogFile == "" {
		s.logger = newStdLogger()
		return nil
	}
	if s.LogFile[0] == '|' {
		l, err := newCmdLogger(s.LogFile[1:])
		if err != nil {
			return err
		}
		s.logger = l
		go s.watchLogger()
		return nil
	}
	l, err := newFileLogger(s.LogFile)
	if err != nil {
		return err
	}
	s.logger = l
	return nil
}

func (s *Starter) watchLogger() {
	<-s.logger.Done()

	if s.shutdown.IsSet() {
		// It is in the shutting down process.
		// this is the expected behavior.
		return
	}

	s.logf("the logger dies unexpectedly. shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.Shutdown(ctx)

	s.logf("exit.")
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
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.shutdownBySignal(sig)
			}()
		}
	}
}

func (s *Starter) autoRestarter() {
	interval := s.autoRestartInterval()
	var cnt int
	var ticker *time.Ticker
	var ch <-chan time.Time
	for {
		select {
		case <-s.getChRestarter():
			log.Println("reset")
			cnt = 0
			if ticker != nil {
				ticker.Stop()
			}
			ticker = time.NewTicker(interval)
			ch = ticker.C
		case <-ch:
			cnt++
			if cnt == 1 {
				s.logf("autorestart triggered (interval=%s)", interval)
			} else {
				s.logf("autorestart triggered (forced, interval=%s)", interval)
			}
			if s.tryToLockReload() {
				go func() {
					defer s.unlockReload()
					s.reload()
				}()
			}
		case <-s.ctx.Done():
			return
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
		if s.shutdown.IsSet() {
			return nil, errShutdown
		}
		s.logf("failed to exec %s:%s", s.Command, err)
		time.Sleep(s.interval())
		goto RETRY
	}
	s.logf("starting new worker %d", w.Pid())

	var state *os.ProcessState
	timer := time.NewTimer(s.interval())
	select {
	case <-w.done:
		if s.shutdown.IsSet() {
			return nil, errShutdown
		}
		state = w.cmd.ProcessState
		s.logf("new worker %d seems to have failed to start, exit status: %d", w.Pid(), state.ExitCode())
		<-timer.C
		goto RETRY
	case <-timer.C:
	}

	// notify that starting new worker succeed to the restarter.
	if ch := s.getChRestarter(); ch != nil {
		ch <- struct{}{}
	}

	return w, nil
}

func (s *Starter) tryToStartWorker() (*worker, error) {
	if s.shutdown.IsSet() {
		return nil, errShutdown
	}
	ch := s.getChStarter()
	select {
	case ch <- struct{}{}:
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
	defer func() {
		<-ch
	}()
	addr := func(sock socket) string {
		if addr, ok := sock.(interface{ Addr() net.Addr }); ok {
			return addr.Addr().String()
		}
		if addr, ok := sock.(interface{ LocalAddr() net.Addr }); ok {
			return addr.LocalAddr().String()
		}
		panic("fail to get addr")
	}

	sockets := s.getSockets()
	files := make([]*os.File, len(sockets))
	ports := make([]string, len(sockets))
	for i, sock := range sockets {
		f, err := sock.File()
		if err != nil {
			return nil, err
		}
		files[i] = f

		// file descriptor numbers in ExtraFiles turn out to be
		// index + 3, so we can just hard code it
		ports[i] = fmt.Sprintf("%s=%d", addr(sock), i+3)
	}

	s.generation++
	ctx, cancel := context.WithCancel(s.ctx)
	cmd := exec.CommandContext(ctx, s.Command, s.Args...)
	cmd.Stdout = s.logger.Stdout()
	cmd.Stderr = s.logger.Stderr()
	cmd.ExtraFiles = files
	env := os.Environ()
	env = append(env, fmt.Sprintf("%s=%s", PortEnvName, strings.Join(ports, ";")))
	env = append(env, fmt.Sprintf("%s=%d", GenerationEnvName, s.generation))
	env = append(env, loadEnv(s.EnvDir)...)
	cmd.Env = env
	cmd.Dir = s.Dir
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

	s.addWorker(w)
	w.Wait()

	return w, nil
}

func (w *worker) Wait() {
	w.starter.wgWorker.Add(1)
	go w.wait()
}

func (w *worker) wait() {
	defer w.starter.wgWorker.Done()
	defer w.close()
	w.cmd.Wait()
}

// start to watch the worker itself.
// after call the Watch, the worker watches its process and restart itself if necessary.
func (w *worker) Watch() {
	w.starter.wgWorker.Add(1)
	go w.watch()
}

func (w *worker) watch() {
	s := w.starter
	defer s.wgWorker.Done()
	state := workerStateInit
	for {
		select {
		case sig := <-w.chsig:
			state = sig.state
			err := w.cmd.Process.Signal(sig.signal)
			if err != nil {
				s.logf("failed to send signal %s to %d: %s", signalToName(sig.signal), w.Pid(), err)
			}
		case <-w.done:
			st := w.cmd.ProcessState
			switch state {
			case workerStateInit:
				s.logf("worker %d died unexpectedly with status %d, restarting", w.Pid(), st.ExitCode())
				if s.tryToLockReload() {
					w.starter.wgWorker.Add(1)
					go func() {
						defer s.wgWorker.Done()
						defer s.unlockReload()
						w, err := s.startWorker()
						if err != nil {
							return
						}
						w.Watch()
					}()
				}
			case workerStateOld:
				s.logf("old worker %d died, status %d", w.Pid(), st.ExitCode())
			case workerStateShutdown:
				s.logf("worker %d died, status %d", w.Pid(), st.ExitCode())
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
	w.starter.removeWorker(w)
	return nil
}

func (s *Starter) listen() error {
	var errListen error
	var sockets []socket
	var lc net.ListenConfig

	for _, hostport := range s.Ports {
		suffix := ""
		if idx := strings.LastIndexByte(hostport, '='); idx >= 0 {
			s.logf("%s: fd options are not supported", hostport)
			if errListen == nil {
				errListen = errors.New("fd options are not supported")
			}
			continue
		}
		host, port, err := net.SplitHostPort(hostport)
		if err != nil {
			// try to parse the hostport as a port number
			// by default, only bind to IPv4 (for compatibility)
			host = "0.0.0.0"
			port = hostport
			suffix = "4"
		}

		var sock socket
		var ok bool
		if strings.HasPrefix(port, "u") {
			// Listen UDP Port
			port = strings.TrimPrefix(port, "u")
			hostport = net.JoinHostPort(host, port)
			conn, err := lc.ListenPacket(s.ctx, "udp"+suffix, hostport)
			if err != nil {
				s.logf("%s: failed to listen: %s", hostport, err)
				if errListen == nil {
					errListen = err
				}
				continue
			}
			sock, ok = conn.(socket)
		} else {
			// Listen TCP Port
			hostport = net.JoinHostPort(host, port)
			l, err := lc.Listen(s.ctx, "tcp"+suffix, hostport)
			if err != nil {
				s.logf("%s: failed to listen: %s", hostport, err)
				if errListen == nil {
					errListen = err
				}
				continue
			}
			sock, ok = l.(socket)
		}
		if !ok {
			s.logf("%s: fail to get file description", hostport)
			if errListen == nil {
				errListen = errors.New("fail to get file description")
			}
			continue
		}
		sockets = append(sockets, sock)
	}

	for _, path := range s.Paths {
		if stat, err := os.Lstat(path); err == nil && stat.Mode()&os.ModeSocket == os.ModeSocket {
			s.logf("removing existing socket file: %s", path)
			if err := os.Remove(path); err != nil {
				s.logf("failed to remove existing socket file: %s: %s", path, err)
				if errListen == nil {
					errListen = err
				}
				continue
			}
		}
		_ = os.Remove(path)
		l, err := lc.Listen(s.ctx, "unix", path)
		if err != nil {
			s.logf("%s: failed to listen: %s", path, err)
			if errListen == nil {
				errListen = err
			}
			continue
		}
		if err := os.Chmod(path, 0777); err != nil {
			s.logf("%s: failed to chmod: %s", path, err)
			if errListen == nil {
				errListen = err
			}
			continue
		}
		socket, ok := l.(socket)
		if !ok {
			s.logf("%s: fail to get file description", path)
			if errListen == nil {
				errListen = errors.New("fail to get file description")
			}
			continue
		}
		sockets = append(sockets, socket)
	}

	if errListen != nil {
		for _, sock := range sockets {
			sock.Close()
		}
		return errListen
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sockets = sockets
	return nil
}

// Listeners returns the listeners.
func (s *Starter) Listeners() []net.Listener {
	s.mu.RLock()
	defer s.mu.RUnlock()
	listeners := make([]net.Listener, 0, len(s.sockets))
	for _, sock := range s.sockets {
		if l, ok := sock.(net.Listener); ok {
			listeners = append(listeners, l)
		}
	}
	return listeners
}

// PacketConns returns the PacketConns.
func (s *Starter) PacketConns() []net.PacketConn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conns := make([]net.PacketConn, 0, len(s.sockets))
	for _, sock := range s.sockets {
		if conn, ok := sock.(net.PacketConn); ok {
			conns = append(conns, conn)
		}
	}
	return conns
}

func (s *Starter) getSockets() []socket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sockets
}

// Reload starts a new worker and stop the current worker.
func (s *Starter) Reload() error {
	s.lockReload()
	defer s.unlockReload()
	return s.reload()
}

func (s *Starter) reload() error {
RETRY:
	w, err := s.startWorker()
	if err != nil {
		if err == errShutdown {
			return nil
		}
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
			if s.shutdown.IsSet() {
				return nil
			}

			// the new worker dies during sleep, restarting.
			state := w.cmd.ProcessState
			s.logf("worker %d died unexpectedly with status %d, restarting", w.Pid(), state.ExitCode())
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

func (s *Starter) getChReload() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chreload == nil {
		s.chreload = make(chan struct{}, 1)
	}
	return s.chreload
}

func (s *Starter) tryToLockReload() (locked bool) {
	ch := s.getChReload()
	select {
	case ch <- struct{}{}:
		return true
	default:
	}
	return false
}

func (s *Starter) lockReload() {
	s.getChReload() <- struct{}{}
}

func (s *Starter) unlockReload() {
	<-s.getChReload()
}

func (s *Starter) getChStarter() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chstarter == nil {
		s.chstarter = make(chan struct{}, 1)
	}
	return s.chstarter
}

func (s *Starter) getChRestarter() chan struct{} {
	if !s.EnableAutoRestart {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chrestarter == nil {
		s.chrestarter = make(chan struct{})
	}
	return s.chrestarter
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
	if s.EnableAutoRestart {
		return 5 * time.Second
	}
	return 0
}

func (s *Starter) autoRestartInterval() time.Duration {
	if s.AutoRestartInterval > 0 {
		return s.AutoRestartInterval
	}
	return 360 * time.Second
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

/// Worker Pool Utilities

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

func (s *Starter) addWorker(w *worker) {
	s.mu.Lock()
	if s.workers == nil {
		s.workers = make(map[*worker]struct{})
	}
	s.workers[w] = struct{}{}
	s.updateStatusLocked()
	s.mu.Unlock()
}

func (s *Starter) removeWorker(w *worker) {
	s.mu.Lock()
	delete(s.workers, w)
	s.updateStatusLocked()
	s.mu.Unlock()
}

// updateStatus writes the workers' status into StatusFile.
func (s *Starter) updateStatusLocked() {
	if s.StatusFile == "" {
		return // nothing to do
	}
	workers := make([]*worker, 0, len(s.workers))
	for w := range s.workers {
		workers = append(workers, w)
	}

	sort.Slice(workers, func(i, j int) bool {
		return workers[i].generation < workers[j].generation
	})

	var buf bytes.Buffer
	for _, w := range workers {
		fmt.Fprintf(&buf, "%d:%d\n", w.generation, w.Pid())
	}
	tmp := fmt.Sprintf("%s.%d", s.StatusFile, os.Getegid())
	if err := os.WriteFile(tmp, buf.Bytes(), 0666); err != nil {
		s.logf("failed to create temporary file:%s:%s", tmp, err)
		return
	}
	if err := os.Rename(tmp, s.StatusFile); err != nil {
		s.logf("failed to rename %s to %s:%s", tmp, s.StatusFile, err)
		return
	}
}

// Shutdown terminates all workers.
func (s *Starter) Shutdown(ctx context.Context) error {
	s.wg.Add(1)
	defer s.wg.Done()

	// stop starting new worker
	if s.shutdown.TrySet(true) {
		// wait for a worker that is currently starting
		ch := s.getChStarter()
		select {
		case ch <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		case <-s.ctx.Done():
			return nil
		}
		defer func() {
			<-ch
		}()
	}

	// notify shutdown signal to the workers
	workers := s.listWorkers()
	for _, w := range workers {
		w.Signal(s.signalOnTERM(), workerStateShutdown)
	}

	// wait for shutting down the workers
	for _, w := range workers {
		select {
		case <-w.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.wgWorker.Wait()

	// gracefully shutdown the logger
	if err := s.logger.Shutdown(ctx); err != nil {
		return err
	}

	return s.Close()
}

func (s *Starter) shutdownBySignal(recv os.Signal) {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// stop starting new worker
	if s.shutdown.TrySet(true) {
		// wait for a worker that is currently starting
		ch := s.getChStarter()
		select {
		case ch <- struct{}{}:
		case <-s.ctx.Done():
			return
		}
		defer func() {
			<-ch
		}()
	}

	// notify shutdown signal to the workers
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

	// wait for shutting down the workers
	for _, w := range workers {
		select {
		case <-w.done:
		case <-ctx.Done():
			s.Close()
			s.logf("exiting")
			return
		}
	}

	// gracefully shutdown the logger
	s.logger.Shutdown(ctx)

	s.Close()
	s.logf("exiting")
}

// Close terminates all workers immediately.
func (s *Starter) Close() error {
	s.onceClose.Do(s.close)
	return nil
}

func (s *Starter) close() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wgWorker.Wait()
	s.logger.Close()
	for _, sock := range s.getSockets() {
		sock.Close()
		if l, ok := sock.(*net.UnixListener); ok {
			os.Remove(l.Addr().String())
		}
	}
	if f := s.pidFile; f != nil {
		os.Remove(f.Name())
		f.Close()
	}
}

func (s *Starter) logf(format string, args ...interface{}) {
	s.logger.Logf(format, args...)
}

func (s *Starter) restart() error {
	if s.PidFile == "" || s.StatusFile == "" {
		return errors.New("--restart option requires --pid-file and --status-file to be set as well")
	}

	// get pid
	buf, err := os.ReadFile(s.PidFile)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(string(bytes.TrimSpace(buf)))
	if err != nil {
		return err
	}

	getGenerations := func() ([]int, error) {
		buf, err := os.ReadFile(s.StatusFile)
		if err != nil {
			return nil, err
		}
		gens := []int{}
		for _, line := range bytes.Split(buf, []byte{'\n'}) {
			idx := bytes.IndexByte(line, ':')
			if idx < 0 {
				continue
			}
			g, err := strconv.Atoi(string(line[:idx]))
			if err != nil {
				continue
			}
			gens = append(gens, g)
		}
		sort.Ints(gens)
		return gens, nil
	}
	var waitFor int
	if gens, err := getGenerations(); err != nil {
		return err
	} else if len(gens) == 0 {
		return errors.New("no active process found in the status file")
	} else {
		waitFor = gens[len(gens)-1] + 1
	}

	// send HUP
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Signal(syscall.SIGHUP); err != nil {
		return err
	}

	// wait for the generation
	for {
		gens, err := getGenerations()
		if err != nil {
			return err
		}
		if len(gens) == 1 && gens[0] == waitFor {
			break
		}
		time.Sleep(time.Second)
	}
	return nil
}

func (s *Starter) stop() error {
	if s.PidFile == "" {
		return errors.New("--stop option requires --pid-file to be set as well")
	}
	f, err := os.OpenFile(s.PidFile, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	buf, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(string(bytes.TrimSpace(buf)))
	if err != nil {
		return err
	}

	s.logf("stop_server (pid:%d) stopping now (pid:%d)...", os.Getpid(), pid)

	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return err
	}

	if err := flock(f.Fd(), syscall.LOCK_EX); err != nil {
		return err
	}
	return nil
}

// flock is same as syscall.Flock, but it ignores EINTR error.
// because of the changes from Go 1.14
//
// https://golang.org/doc/go1.14#runtime
// This means that programs that use packages like syscall or golang.org/x/sys/unix will see more slow system calls fail with EINTR errors.
// Those programs will have to handle those errors in some way, most likely looping to try the system call again.
func flock(fd uintptr, how int) error {
	for {
		err := syscall.Flock(int(fd), how)
		if err != syscall.EINTR {
			return err
		}
	}
}
