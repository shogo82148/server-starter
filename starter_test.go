package starter

import (
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

	go func() {
		sd := &Starter{
			Command: binFile,
			Ports:   []string{"12345"},
		}
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // wait for starting worker
	conn, err := net.Dial("tcp", "127.0.0.1:12345")
	if err != nil {
		t.Fatalf("fail to dial: %s", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("fail to write: %s", err)
	}
	var buf [1024 * 1024]byte
	if n, err := conn.Read(buf[:]); err != nil {
		t.Fatalf("fail to read: %s", err)
	} else {
		t.Logf("%s", buf[:n])
	}
}
