package starter

import (
	"bytes"
	"context"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	// avoid caching test results
	_ "github.com/shogo82148/server-starter/listener"
)

func TestRun(t *testing.T) {
	dir, err := ioutil.TempDir("", "server-starter-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	defer os.RemoveAll(dir)

	// build echod
	binFile := filepath.Join(dir, "echod")
	cmd := exec.Command("go", "build", "-o", binFile, "testdata/echod/echod.go")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	sd := &Starter{
		Command: binFile,
		Ports:   []string{"12345"},
	}
	defer sd.Shutdown(context.Background())
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // wait for starting worker

	// connect to the first worker.
	conn, err := net.Dial("tcp", "127.0.0.1:12345")
	if err != nil {
		t.Fatalf("fail to dial: %s", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("fail to write: %s", err)
	}
	var buf [1024 * 1024]byte
	n, err := conn.Read(buf[:])
	if err != nil {
		t.Fatalf("fail to read: %s", err)
	}
	if ok, _ := regexp.Match(`^\d+:hello$`, buf[:n]); !ok {
		t.Errorf(`want /^\d+:hello$/, got %s`, buf[:n])
	}
	pid1 := string(buf[:bytes.IndexByte(buf[:], ':')])
	conn.Close()

	time.Sleep(3 * time.Second)
	// TODO: check status file

	// Reload
	// 0sec: start a new worker
	// 1sec: if the new worker is still alive, send SIGTERM to the old one.
	// 3sec: the old worker stops.
	sd.Reload(context.Background())
	time.Sleep(2 * time.Second)
	// TODO: check status file
	time.Sleep(2 * time.Second)
	// TODO: check status file

	// connect to the second worker.
	conn, err = net.Dial("tcp", "127.0.0.1:12345")
	if err != nil {
		t.Fatalf("fail to dial: %s", err)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("fail to write: %s", err)
	}
	n, err = conn.Read(buf[:])
	if err != nil {
		t.Fatalf("fail to read: %s", err)
	}
	if ok, _ := regexp.Match(`^\d+:hello$`, buf[:n]); !ok {
		t.Errorf(`want /^\d+:hello$/, got %s`, buf[:n])
	}
	pid2 := string(buf[:bytes.IndexByte(buf[:], ':')])
	if pid1 == pid2 {
		t.Errorf("want another, got %s", pid2)
	}
}
