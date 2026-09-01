package starter

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

const interopRequiredEnv = "SERVER_STARTER_INTEROP_REQUIRED"

func TestPerlInteroperabilityPerlSupervisorGoWorkerRuns(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	perl, startServer := requirePerlServerStarter(t)

	// build echod
	binFile := filepath.Join(dir, "echod")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binFile, filepath.Join("testdata", "interop", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	// start the go worker with the perl supervisor.
	addrFile := filepath.Join(dir, "addr.txt")
	cmd = exec.CommandContext(ctx, perl, startServer, "--port=127.0.0.1:0", "--", binFile, addrFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start %s: %s", startServer, err)
	}

	time.Sleep(1000 * time.Millisecond) // wait for starting worker

	// connect to the worker.
	addr, err := os.ReadFile(addrFile)
	if err != nil {
		t.Fatalf("Failed to read addr file: %s", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+string(addr), nil)
	if err != nil {
		t.Fatalf("Failed to create request: %s", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to do request: %s", err)
	}
	defer resp.Body.Close() //nolint:errcheck // ignore error on cleanup
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Unexpected status code: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %s", err)
	}
	if string(body) != "Hello, World!" {
		t.Fatalf("Unexpected response body: %q", body)
	}

	// shutdown the worker.
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Failed to send SIGTERM to %s: %s", startServer, err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("Failed to wait for %s: %s", startServer, err)
	}
}

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

func TestPerlInteroperabilityGoSupervisorRunsPerlWorkerUDP(t *testing.T) {
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		r.Close() //nolint:errcheck // ignore error on cleanup
		w.Close() //nolint:errcheck // ignore error on cleanup
		os.Stdout = original
	})

	ctx := t.Context()
	perl, _ := requirePerlServerStarter(t)

	// start the perl worker with the go supervisor.
	sd := &Starter{
		Command: perl,
		Args:    []string{filepath.Join("testdata", "15-udp-server.pl")},
		Ports:   []string{"u0"},
	}
	t.Cleanup(func() { sd.Close() }) //nolint:errcheck // ignore error on cleanup
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("Run() returned error: %v", err)
		}
	}()

	scanner := bufio.NewScanner(r)
	// wait for starting worker
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "success") {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("failed to read from pipe: %v", err)
	}

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
