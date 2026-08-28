package starter

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStderr(t *testing.T, f func()) []byte {
	t.Helper()

	// Redirect os.Stderr to a pipe to capture the output
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = original
	})

	// Use a buffer to capture the output
	buf := new(bytes.Buffer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := buf.ReadFrom(r); err != nil {
			t.Errorf("failed to read from pipe: %v", err)
		}
		if err := r.Close(); err != nil {
			t.Errorf("failed to close pipe reader: %v", err)
		}
	}()

	// Call the function that writes to os.Stderr
	f()
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}

	<-done
	return buf.Bytes()
}

func TestStdLogger(t *testing.T) {
	logger := newStdLogger()

	if logger.Stdout() != os.Stdout {
		t.Errorf("Stdout() = %v, want %v", logger.Stdout(), os.Stdout)
	}
	if logger.Stderr() != os.Stderr {
		t.Errorf("Stderr() = %v, want %v", logger.Stderr(), os.Stderr)
	}
	if logger.Done() != nil {
		t.Errorf("Done() = %v, want nil", logger.Done())
	}

	got := captureStderr(t, func() {
		logger.Logf("Hello, %s!", "world")
	})
	want := "Hello, world!\n"
	if string(got) != want {
		t.Errorf("Logf() wrote %q, want %q", got, want)
	}

	got = captureStderr(t, func() {
		logger.Logf("Hello, %s!\n", "world")
	})
	want = "Hello, world!\n"
	if string(got) != want {
		t.Errorf("Logf() wrote %q, want %q", got, want)
	}

	if err := logger.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
}

func BenchmarkStdLogger(b *testing.B) {
	var err error
	original := os.Stderr
	os.Stderr, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatalf("failed to open os.DevNull: %v", err)
	}
	b.Cleanup(func() {
		os.Stderr.Close() //nolint:errcheck // Ignore error on cleanup
		os.Stderr = original
	})

	s := strings.Repeat("Hello World!", 1000)

	b.ResetTimer()
	logger := stdLogger{}
	for b.Loop() {
		logger.Logf("%s", s)
	}
}

func captureFileLoggerOutput(t *testing.T, f func(l *fileLogger)) []byte {
	t.Helper()

	// Create a temporary file to capture the output
	tmpfile, err := os.CreateTemp("", "filelogger_test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name()) //nolint:errcheck // Ignore error on cleanup

	// Call the function that writes to the file logger
	logger := &fileLogger{f: tmpfile}
	f(logger)

	// Read the contents of the temporary file
	if _, err := tmpfile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("failed to seek to start of temp file: %v", err)
	}
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(tmpfile)
	if err != nil {
		t.Fatalf("failed to read from temp file: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	return buf.Bytes()
}

func TestFileLogger(t *testing.T) {
	logger, err := newFileLogger(os.DevNull)
	if err != nil {
		t.Fatalf("failed to create file logger: %v", err)
	}

	if logger.Stdout().Name() != os.DevNull {
		t.Errorf("Stdout() = %v, want %v", logger.Stdout().Name(), os.DevNull)
	}
	if logger.Stderr().Name() != os.DevNull {
		t.Errorf("Stderr() = %v, want %v", logger.Stderr().Name(), os.DevNull)
	}
	if logger.Done() != nil {
		t.Errorf("Done() = %v, want nil", logger.Done())
	}

	got := captureFileLoggerOutput(t, func(l *fileLogger) {
		l.Logf("Hello, %s!", "world")
	})
	want := "Hello, world!\n"
	if string(got) != want {
		t.Errorf("Logf() wrote %q, want %q", got, want)
	}

	got = captureFileLoggerOutput(t, func(l *fileLogger) {
		l.Logf("Hello, %s!\n", "world")
	})
	want = "Hello, world!\n"
	if string(got) != want {
		t.Errorf("Logf() wrote %q, want %q", got, want)
	}

	if err := logger.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
}

func BenchmarkFileLogger(b *testing.B) {
	logger, err := newFileLogger(os.DevNull)
	if err != nil {
		b.Fatalf("failed to create file logger: %v", err)
	}
	b.Cleanup(func() {
		logger.Close() //nolint:errcheck // Ignore error on cleanup
	})

	s := strings.Repeat("Hello World!", 1000)

	b.ResetTimer()
	for b.Loop() {
		logger.Logf("%s", s)
	}
}
