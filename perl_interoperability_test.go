package starter

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

const interopRequiredEnv = "SERVER_STARTER_INTEROP_REQUIRED"

func TestPerlInteroperabilityGoSupervisorRunsPerlWorker(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	perl, _ := requirePerlServerStarter(t)

	// start the perl worker with the go supervisor.
	signame := filepath.Join(dir, "signame")
	sd := &Starter{
		Command: perl,
		Args:    []string{"testdata/01-starter-echod.pl", signame},
		Ports:   []string{"0"},
	}
	t.Cleanup(func() { sd.Close() }) //nolint:errcheck // ignore error on cleanup
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("Run() returned error: %v", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // wait for starting worker

	// connect to the worker.
	addr := sd.Listeners()[0].Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	var buf [1024 * 1024]byte
	n, err := conn.Read(buf[:])
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	if ok, _ := regexp.Match(`^\d+:hello`, buf[:n]); !ok {
		t.Errorf("unexpected response: %q", buf[:n])
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("failed to close connection: %v", err)
	}

	// shutdown the worker.
	if err := sd.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}
}

// requirePerlServerStarter checks if the original Server::Starter is available.
// If not, it skips the test unless SERVER_STARTER_INTEROP_REQUIRED is set.
func requirePerlServerStarter(t *testing.T) (perlPath string, startServerPath string) {
	t.Helper()

	perlPath, err := exec.LookPath("perl")
	if err != nil {
		serverStarterUnavailable(t)
		return
	}
	startServerPath, err = exec.LookPath("start_server")
	if err != nil {
		serverStarterUnavailable(t)
		return
	}
	ctx := t.Context()
	cmd := exec.CommandContext(ctx, perlPath, "-MServer::Starter", "-e", "1")
	if err := cmd.Run(); err != nil {
		serverStarterUnavailable(t)
		return
	}
	cmd = exec.CommandContext(ctx, startServerPath, "--version")
	if err := cmd.Run(); err != nil {
		serverStarterUnavailable(t)
		return
	}
	return perlPath, startServerPath
}

func serverStarterUnavailable(t *testing.T) {
	message := "original Server::Starter is unavailable; install libserver-starter-perl"
	if os.Getenv(interopRequiredEnv) != "" {
		t.Fatal(message)
	} else {
		t.Skip(message)
	}
}
