package starter

import (
	"bytes"
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
		defer r.Close()
		_, err := buf.ReadFrom(r)
		if err != nil {
			t.Errorf("failed to read from pipe: %v", err)
		}
	}()

	// Call the function that writes to os.Stderr
	f()
	w.Close()

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
		os.Stderr.Close()
		os.Stderr = original
	})

	s := strings.Repeat("Hello World!", 1000)

	b.ResetTimer()
	logger := stdLogger{}
	for b.Loop() {
		logger.Logf("%s", s)
	}
}
