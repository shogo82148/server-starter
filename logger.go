package starter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// logger is an interface of server-starter's logger.
type logger interface {
	// Stdout returns the output file for workers' stdout.
	Stdout() *os.File

	// Stderr returns the output file for workers' stderr.
	Stderr() *os.File

	// Logf outputs a log into Stderr()
	Logf(format string, args ...interface{})

	// Done returns a channel that's closed when work the logger stopped.
	Done() <-chan struct{}

	// Shutdown gracefully shuts down the logger without losing any logs.
	Shutdown(ctx context.Context) error

	// Close immediately closes the logger.
	// For a graceful shutdown, use Shutdown.
	Close() error
}

// stdLogger is a logger that outputs into os.Stdout and os.Stderr
type stdLogger struct{}

func newStdLogger() logger {
	return stdLogger{}
}

func (stdLogger) Stdout() *os.File {
	return os.Stdout
}

func (stdLogger) Stderr() *os.File {
	return os.Stderr
}

func (stdLogger) Done() <-chan struct{} {
	return nil
}

func (stdLogger) Logf(format string, args ...interface{}) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, format, args...)
	if buf.Len() == 0 || buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	os.Stderr.Write(buf.Bytes())
}

func (stdLogger) Shutdown(ctx context.Context) error {
	return nil
}

func (stdLogger) Close() error {
	return nil
}

type fileLogger struct {
	f *os.File
}

func newFileLogger(filename string) (logger, error) {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &fileLogger{f: f}, nil
}

func (l *fileLogger) Stdout() *os.File {
	return l.f
}

func (l *fileLogger) Stderr() *os.File {
	return l.f
}

func (*fileLogger) Done() <-chan struct{} {
	return nil
}

func (l *fileLogger) Logf(format string, args ...interface{}) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, format, args...)
	if buf.Len() == 0 || buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	l.f.Write(buf.Bytes())
}

func (l *fileLogger) Shutdown(ctx context.Context) error {
	return nil
}

func (l *fileLogger) Close() error {
	return l.f.Close()
}

// cmdLogger is a logger that outputs into stdin of a command.
type cmdLogger struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	cmd    *exec.Cmd

	closed atomicBool
	pr, pw *os.File
}

func newCmdLogger(command string) (logger, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sh", "-c", strings.TrimSpace(command))

	// make a logger process a group leader
	// https://junkyard.song.mu/slides/gocon2019-spring/#48 (written in Japanese)
	// https://github.com/Songmu/timeout/blob/9710262dc02f66fdd69a6cd4c8143204006d5843/timeout_unix.go#L14-L19
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// set log output the stdin of the logger process
	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	cmd.Stdin = pr

	// configure stdout and stderr of the logger process
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	l := &cmdLogger{
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		cmd:    cmd,
		pr:     pr,
		pw:     pw,
	}
	if err := cmd.Run(); err != nil {
		cancel()
		pr.Close()
		pw.Close()
		return nil, err
	}
	go l.wait()
	return l, nil
}

func (l *cmdLogger) Stdout() *os.File {
	return l.pw
}

func (l *cmdLogger) Stderr() *os.File {
	return l.pw
}

func (l *cmdLogger) Done() <-chan struct{} {
	return l.done
}

func (l *cmdLogger) Logf(format string, args ...interface{}) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, format, args...)
	if buf.Len() == 0 || buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	if l.closed.IsSet() {
		os.Stderr.Write(buf.Bytes())
	} else {
		l.pw.Write(buf.Bytes())
	}
}

func (l *cmdLogger) Shutdown(ctx context.Context) error {
	l.closePipe()

	// send SIGTERM signal to the process group
	// https://junkyard.song.mu/slides/gocon2019-spring/#53 (written in Japanese)
	// https://github.com/Songmu/timeout/blob/9710262dc02f66fdd69a6cd4c8143204006d5843/timeout_unix.go#L21-L35
	if err := syscall.Kill(-l.cmd.Process.Pid, syscall.SIGTERM); err != nil {
		return err
	}
	if err := syscall.Kill(-l.cmd.Process.Pid, syscall.SIGCONT); err != nil {
		return err
	}

	// wait for shutdown
	select {
	case <-l.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (l *cmdLogger) Close() error {
	l.closePipe()
	select {
	case <-l.done:
	default:
		// force to terminate the logger process
		syscall.Kill(-l.cmd.Process.Pid, syscall.SIGKILL)
		l.cancel()
		<-l.done
	}
	return nil
}

func (l *cmdLogger) wait() {
	l.cmd.Wait()

	// notify the logger process is stopped.
	l.closePipe()
	close(l.done)
}

func (l *cmdLogger) closePipe() {
	if l.closed.TrySet(true) {
		l.pw.Close()
		l.pr.Close()
	}
}
