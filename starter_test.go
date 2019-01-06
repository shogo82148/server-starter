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
	"syscall"
	"testing"
	"time"

	// avoid caching test results
	_ "github.com/shogo82148/server-starter/listener"
)

func Test_Start(t *testing.T) {
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

	testFunc := func(t *testing.T, signal os.Signal, signame string) {
		statusFile := filepath.Join(dir, "status")
		sd := &Starter{
			Command:     binFile,
			Args:        []string{filepath.Join(dir, "signame")},
			Ports:       []string{"0"},
			StatusFile:  statusFile,
			SignalOnHUP: signal,
		}
		defer sd.Shutdown(context.Background())
		go func() {
			if err := sd.Run(); err != nil {
				t.Errorf("sd.Run() failed: %s", err)
			}
		}()

		time.Sleep(500 * time.Millisecond) // wait for starting worker

		// connect to the first worker.
		addr := sd.Listeners()[0].Addr().String()
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("fail to dial: %s", err)
		}
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
		status, err := ioutil.ReadFile(statusFile)
		if err != nil {
			t.Errorf("fail to read status file %s: %s", statusFile, err)
		}
		if ok, _ := regexp.Match(`^1:\d+\n$`, status); !ok {
			t.Errorf(`want /^1:\d+\n$/, got %s`, status)
		}

		// Reload
		// 0sec: start a new worker
		// 1sec: if the new worker is still alive, send SIGTERM to the old one.
		// 3sec: the old worker stops.
		go sd.Reload()
		time.Sleep(2 * time.Second)
		status, err = ioutil.ReadFile(statusFile)
		if err != nil {
			t.Errorf("fail to read status file %s: %s", statusFile, err)
		}
		if ok, _ := regexp.Match(`^1:\d+\n2:\d+\n$`, status); !ok {
			t.Errorf(`want /^1:\d+\n2:\d+\n$/, got %s`, status)
		}

		time.Sleep(2 * time.Second)
		status, err = ioutil.ReadFile(statusFile)
		if err != nil {
			t.Errorf("fail to read status file %s: %s", statusFile, err)
		}
		if ok, _ := regexp.Match(`^2:\d+\n$`, status); !ok {
			t.Errorf(`want /^2:\d+\n$/, got %s`, status)
		}

		signameGot, err := ioutil.ReadFile(filepath.Join(dir, "signame"))
		if string(signameGot) != signame {
			t.Errorf("want %s, got %s", signame, string(signameGot))
		}

		// connect to the second worker.
		conn, err = net.Dial("tcp", addr)
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
	t.Run("TERM", func(t *testing.T) {
		testFunc(t, nil, syscall.SIGTERM.String())
	})
	t.Run("USR1", func(t *testing.T) {
		testFunc(t, syscall.SIGUSR1, syscall.SIGUSR1.String())
	})
}

func Test_StartFail(t *testing.T) {
	dir, err := ioutil.TempDir("", "server-starter-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	defer os.RemoveAll(dir)

	// build a server.
	binFile := filepath.Join(dir, "echod")
	cmd := exec.Command("go", "build", "-o", binFile, "testdata/startfail/main.go")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	var addr string
	getGeneration := func() string {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("fail to dial: %s", err)
		}
		defer conn.Close()
		var buf [1024 * 1024]byte
		n, err := conn.Read(buf[:])
		if err != nil {
			t.Fatalf("fail to read: %s", err)
		}
		return string(buf[:n])
	}

	sd := &Starter{
		Command: binFile,
		Ports:   []string{"0"},
	}
	defer sd.Shutdown(context.Background())
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(3 * time.Second) // wait for starting worker

	// connect to the first worker.
	addr = sd.Listeners()[0].Addr().String()
	// the first generation fails to start, so the generation number starts from 2.
	generation := getGeneration()
	if generation != "2" {
		t.Errorf("want %s, got %s", "2", generation)
	}

	go sd.Reload()
	time.Sleep(1 * time.Second)

	// the 3rd and 4th generation fails to start, so the generation number is still 2.
	generation = getGeneration()
	if generation != "2" {
		t.Errorf("want %s, got %s", "2", generation)
	}

	// wait until server succeds in reboot
	time.Sleep(5 * time.Second)
	generation = getGeneration()
	if generation != "5" {
		t.Errorf("want %s, got %s", "5", generation)
	}
}

func Test_KillOldDeplay(t *testing.T) {
	dir, err := ioutil.TempDir("", "server-starter-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	defer os.RemoveAll(dir)

	// build echod
	binFile := filepath.Join(dir, "killolddelay")
	cmd := exec.Command("go", "build", "-o", binFile, "testdata/killolddelay/main.go")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	statusFile := filepath.Join(dir, "status")
	sd := &Starter{
		Command:      binFile,
		Ports:        []string{"0"},
		KillOldDelay: 3 * time.Second,
		StatusFile:   statusFile,
	}
	defer sd.Shutdown(context.Background())
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // wait for starting worker

	// connect to the first worker.
	addr := sd.Listeners()[0].Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("fail to dial: %s", err)
	}
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

	// Reload
	// 0sec: start a new worker
	// 1sec: if the new worker is still alive, sleep kill_old_deplay sec.
	// 4sec: send SIGTERM to the old worker.
	// 5sec: the old worker stops.
	go sd.Reload()

	time.Sleep(4 * time.Second)
	status, err := ioutil.ReadFile(statusFile)
	if err != nil {
		t.Errorf("fail to read status file %s: %s", statusFile, err)
	}
	if ok, _ := regexp.Match(`^1:\d+\n2:\d+\n$`, status); !ok {
		t.Errorf(`want /1:\d+\n2:\d+\n/, got %s`, status)
	}

	time.Sleep(2 * time.Second)
	status, err = ioutil.ReadFile(statusFile)
	if err != nil {
		t.Errorf("fail to read status file %s: %s", statusFile, err)
	}
	if ok, _ := regexp.Match(`^2:\d+\n$`, status); !ok {
		t.Errorf(`want /2:\d+\n/, got %s`, status)
	}

	// connect to the second worker.
	conn, err = net.Dial("tcp", addr)
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

func Test_Unix(t *testing.T) {
	dir, err := ioutil.TempDir("", "server-starter-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	defer os.RemoveAll(dir)

	// build echod
	binFile := filepath.Join(dir, "unix")
	sockFile := filepath.Join(dir, "sock")
	cmd := exec.Command("go", "build", "-o", binFile, "testdata/unix/main.go")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	statusFile := filepath.Join(dir, "status")
	sd := &Starter{
		Command:      binFile,
		Paths:        []string{sockFile},
		KillOldDelay: 3 * time.Second,
		StatusFile:   statusFile,
	}
	defer sd.Close()
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // wait for starting worker

	conn, err := net.Dial("unix", sockFile)
	if err != nil {
		t.Errorf("fail to dial: %s", err)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Errorf("fail to write: %s", err)
	}
	var buf [1024 * 1024]byte
	n, err := conn.Read(buf[:])
	if err != nil {
		t.Errorf("fail to read: %s", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("want hello, got %s", buf[:n])
	}
	conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sd.Shutdown(ctx)

	if _, err := os.Lstat(sockFile); err == nil {
		t.Errorf("want %s is removed, but exists", sockFile)
	}
}

func Test_Dir(t *testing.T) {
	dir, err := ioutil.TempDir("", "server-starter-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	defer os.RemoveAll(dir)

	// build server
	binFile := filepath.Join(dir, "dir")
	cmd := exec.Command("go", "build", "-o", binFile, "testdata/dir/main.go")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	sd := &Starter{
		Command: binFile,
		Ports:   []string{"0"},
		Dir:     dir,
	}
	defer sd.Shutdown(context.Background())
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // wait for starting worker

	// connect to the first worker.
	addr := sd.Listeners()[0].Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("fail to dial: %s", err)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("fail to write: %s", err)
	}
	var buf [1024 * 1024]byte
	n, err := conn.Read(buf[:])
	if err != nil {
		t.Fatalf("fail to read: %s", err)
	}
	conn.Close()

	stat1, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	stat2, err := os.Stat(string(buf[:n]))
	if err != nil {
		t.Fatal(err)
	}

	if !os.SameFile(stat1, stat2) {
		t.Errorf("want %s, got %s", stat1.Name(), stat2.Name())
	}
}

func Test_AutoRestart(t *testing.T) {
	dir, err := ioutil.TempDir("", "server-starter-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	defer os.RemoveAll(dir)

	// build echod
	binFile := filepath.Join(dir, "autorestart")
	cmd := exec.Command("go", "build", "-o", binFile, "testdata/autorestart/main.go")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	statusFile := filepath.Join(dir, "status")
	sd := &Starter{
		Command:             binFile,
		Ports:               []string{"0"},
		KillOldDelay:        2 * time.Second,
		StatusFile:          statusFile,
		EnableAutoRestart:   true,
		AutoRestartInterval: 6 * time.Second,
	}
	defer sd.Shutdown(context.Background())
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(100 * time.Millisecond) // wait for starting worker

	// connect to the first worker.
	addr := sd.Listeners()[0].Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("fail to dial: %s", err)
	}
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

	// new worker spawn at 7sec (since start, interval(1sec) + auto_restart_interval(6sec))
	// status updated at 8sec (7sec + interval(1sec))
	// old dies at 11sec (8sec + kill_old_delay(2sec) + sleep(1sec) in the child source code

	// check status before auto-restart
	time.Sleep(6 * time.Second)
	status, err := ioutil.ReadFile(statusFile)
	if err != nil {
		t.Errorf("fail to read status file %s: %s", statusFile, err)
	}
	if ok, _ := regexp.Match(`^1:\d+\n$`, status); !ok {
		t.Errorf(`want /1:\d+\n/, got %s`, status)
	}

	// status during transient state
	time.Sleep(3 * time.Second)
	status, err = ioutil.ReadFile(statusFile)
	if err != nil {
		t.Errorf("fail to read status file %s: %s", statusFile, err)
	}
	if ok, _ := regexp.Match(`^1:\d+\n2:\d+\n$`, status); !ok {
		t.Errorf(`want /1:\d+\n2:\d+\n/, got %s`, status)
	}

	// status after auto-restart
	time.Sleep(3 * time.Second)
	status, err = ioutil.ReadFile(statusFile)
	if err != nil {
		t.Errorf("fail to read status file %s: %s", statusFile, err)
	}
	if ok, _ := regexp.Match(`^2:\d+\n$`, status); !ok {
		t.Errorf(`want /2:\d+\n/, got %s`, status)
	}

	// connect to the second worker.
	conn, err = net.Dial("tcp", addr)
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

func Test_EnvDir(t *testing.T) {
	dir, err := ioutil.TempDir("", "server-starter-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	defer os.RemoveAll(dir)

	// set up envdir
	envdir := filepath.Join(dir, "envdir")
	if err := os.Mkdir(envdir, 0755); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("FOO")
	envfile := filepath.Join(envdir, "FOO")
	if err := ioutil.WriteFile(envfile, []byte(" old env \nsecond line will be ignored.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// build server
	binFile := filepath.Join(dir, "env")
	cmd := exec.Command("go", "build", "-o", binFile, "testdata/env/main.go")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	sd := &Starter{
		Command: binFile,
		Ports:   []string{"0"},
		EnvDir:  envdir,
	}
	defer sd.Shutdown(context.Background())
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // wait for starting worker

	getEnv := func() string {
		// connect to the worker.
		addr := sd.Listeners()[0].Addr().String()
		conn, err := net.Dial("tcp", addr)
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
		return string(buf[:n])
	}

	// rewrite envdir...
	if err := ioutil.WriteFile(envfile, []byte("new env\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// ... but the worker returns the old environment value before reload.
	v := getEnv()
	if v != " old env " {
		t.Errorf("want old env, got %s", v)
	}

	// after reload, we can get the new environment value
	time.Sleep(1 * time.Second)
	go sd.Reload()
	time.Sleep(2 * time.Second)
	v = getEnv()
	if v != "new env" {
		t.Errorf("want new env, got %s", v)
	}

	if err := os.Remove(envfile); err != nil {
		t.Fatal(err)
	}
	go sd.Reload()
	time.Sleep(2 * time.Second)
	v = getEnv()
	if v != "" {
		t.Errorf("want \"\", got %s", v)
	}
}
