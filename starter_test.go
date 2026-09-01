package starter

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"testing"
	"time"

	// avoid caching test results
	_ "github.com/shogo82148/server-starter/listener"
)

func Test_Start(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// build echod
	binFile := filepath.Join(dir, "echod")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binFile, filepath.Join("testdata", "echod", "echod.go"))
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
		t.Cleanup(func() {
			sd.Close() //nolint:errcheck // ignore error on cleanup
		})
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
		if err := conn.Close(); err != nil {
			t.Fatalf("fail to close: %s", err)
		}

		time.Sleep(3 * time.Second)
		status, err := os.ReadFile(statusFile)
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
		go func() {
			if err := sd.Reload(); err != nil {
				t.Errorf("sd.Reload() failed: %s", err)
			}
		}()
		time.Sleep(2 * time.Second)
		status, err = os.ReadFile(statusFile)
		if err != nil {
			t.Errorf("fail to read status file %s: %s", statusFile, err)
		}
		if ok, _ := regexp.Match(`^1:\d+\n2:\d+\n$`, status); !ok {
			t.Errorf(`want /^1:\d+\n2:\d+\n$/, got %s`, status)
		}

		time.Sleep(2 * time.Second)
		status, err = os.ReadFile(statusFile)
		if err != nil {
			t.Errorf("fail to read status file %s: %s", statusFile, err)
		}
		if ok, _ := regexp.Match(`^2:\d+\n$`, status); !ok {
			t.Errorf(`want /^2:\d+\n$/, got %s`, status)
		}

		signameGot, err := os.ReadFile(filepath.Join(dir, "signame"))
		if err != nil {
			t.Fatal(err)
		}
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

		if err := sd.Shutdown(ctx); err != nil {
			t.Errorf("sd.Shutdown() failed: %s", err)
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
	ctx := t.Context()
	dir := t.TempDir()

	// build a server.
	binFile := filepath.Join(dir, "echod")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binFile, filepath.Join("testdata", "startfail", "main.go"))
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

	// wait until server succeeds in reboot
	time.Sleep(5 * time.Second)
	generation = getGeneration()
	if generation != "5" {
		t.Errorf("want %s, got %s", "5", generation)
	}
}

func Test_KillOldDelay(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// build echod
	binFile := filepath.Join(dir, "killolddelay")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binFile, filepath.Join("testdata", "killolddelay", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	statusFile := filepath.Join(dir, "status")
	sd := &Starter{
		Command:      binFile,
		Ports:        []string{"0"},
		KillOldDelay: new(3 * time.Second),
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
	// 1sec: if the new worker is still alive, sleep kill_old_delay sec.
	// 4sec: send SIGTERM to the old worker.
	// 5sec: the old worker stops.
	go sd.Reload()

	time.Sleep(4 * time.Second)
	status, err := os.ReadFile(statusFile)
	if err != nil {
		t.Errorf("fail to read status file %s: %s", statusFile, err)
	}
	if ok, _ := regexp.Match(`^1:\d+\n2:\d+\n$`, status); !ok {
		t.Errorf(`want /1:\d+\n2:\d+\n/, got %s`, status)
	}

	time.Sleep(2 * time.Second)
	status, err = os.ReadFile(statusFile)
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
	ctx := t.Context()
	dir := t.TempDir()

	// build echod
	binFile := filepath.Join(dir, "unix")
	sockFile := filepath.Join(dir, "sock")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binFile, filepath.Join("testdata", "unix", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	statusFile := filepath.Join(dir, "status")
	sd := &Starter{
		Command:      binFile,
		Paths:        []string{sockFile},
		KillOldDelay: new(3 * time.Second),
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
	ctx := t.Context()
	dir := t.TempDir()

	// build server
	binFile := filepath.Join(dir, "dir")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binFile, filepath.Join("testdata", "dir", "main.go"))
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
	ctx := t.Context()
	dir := t.TempDir()

	// build autorestart
	binFile := filepath.Join(dir, "autorestart")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binFile, filepath.Join("testdata", "autorestart", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	statusFile := filepath.Join(dir, "status")
	sd := &Starter{
		Command:             binFile,
		Ports:               []string{"0"},
		KillOldDelay:        new(2 * time.Second),
		StatusFile:          statusFile,
		EnableAutoRestart:   true,
		AutoRestartInterval: 6 * time.Second,
	}
	defer sd.Close() //nolint:errcheck // ignore error on cleanup
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
	status, err := os.ReadFile(statusFile)
	if err != nil {
		t.Errorf("fail to read status file %s: %s", statusFile, err)
	}
	if ok, _ := regexp.Match(`^1:\d+\n$`, status); !ok {
		t.Errorf(`want /1:\d+\n/, got %s`, status)
	}

	// status during transient state
	time.Sleep(3 * time.Second)
	status, err = os.ReadFile(statusFile)
	if err != nil {
		t.Errorf("fail to read status file %s: %s", statusFile, err)
	}
	if ok, _ := regexp.Match(`^1:\d+\n2:\d+\n$`, status); !ok {
		t.Errorf(`want /1:\d+\n2:\d+\n/, got %s`, status)
	}

	// status after auto-restart
	time.Sleep(3 * time.Second)
	status, err = os.ReadFile(statusFile)
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

	if err := sd.Shutdown(ctx); err != nil {
		t.Errorf("sd.Shutdown() failed: %s", err)
	}
}

func Test_Env(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// set up envdir
	envdir := filepath.Join(dir, "envdir")
	if err := os.Mkdir(envdir, 0755); err != nil {
		t.Fatal(err)
	}

	// build server
	binFile := filepath.Join(dir, "env")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binFile, filepath.Join("testdata", "env", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	sd := &Starter{
		Command:           binFile,
		Ports:             []string{"0"},
		EnvDir:            envdir,
		EnableAutoRestart: true,
	}
	defer sd.Close() //nolint:errcheck // ignore error on cleanup
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // wait for starting worker

	getEnv := func(ctx context.Context, key string) (string, error) {
		// connect to the worker.
		addr := sd.Listeners()[0].Addr().String()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/"+key, nil)
		if err != nil {
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close() //nolint:errcheck // ignore error on cleanup
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}

	v, err := getEnv(ctx, EnvDirEnvName)
	if err != nil {
		t.Fatal(err)
	}
	if v != envdir {
		t.Errorf("want %q, got %q", envdir, v)
	}

	v, err = getEnv(ctx, EnableAutoRestartEnvName)
	if err != nil {
		t.Fatal(err)
	}
	if v != "1" {
		t.Errorf("want 1, got %q", v)
	}

	v, err = getEnv(ctx, AutoRestartIntervalEnvName)
	if err != nil {
		t.Fatal(err)
	}
	if v != "3600" {
		t.Errorf("want 3600, got %q", v)
	}

	if err := sd.Shutdown(ctx); err != nil {
		t.Fatalf("sd.Shutdown() failed: %s", err)
	}
}

func Test_EnvDir(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	const envName = "FOO"
	original, ok := os.LookupEnv(envName)
	if ok {
		t.Cleanup(func() {
			if err := os.Setenv(envName, original); err != nil {
				t.Fatalf("os.Setenv(%q) failed: %s", envName, err)
			}
		})
	}
	if err := os.Unsetenv(envName); err != nil {
		t.Fatalf("os.Unsetenv(%q) failed: %s", envName, err)
	}

	// set up envdir
	envdir := filepath.Join(dir, "envdir")
	if err := os.Mkdir(envdir, 0755); err != nil {
		t.Fatal(err)
	}
	envfile := filepath.Join(envdir, envName)
	if err := os.WriteFile(envfile, []byte(" old env \nsecond line will be ignored.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// build server
	binFile := filepath.Join(dir, "env")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binFile, filepath.Join("testdata", "env", "main.go"))
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

	getEnv := func(ctx context.Context, key string) (string, error) {
		// connect to the worker.
		addr := sd.Listeners()[0].Addr().String()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/"+key, nil)
		if err != nil {
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close() //nolint:errcheck // ignore error on cleanup
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}

	v, err := getEnv(ctx, envName)
	if err != nil {
		t.Fatal(err)
	}
	if v != " old env " {
		t.Errorf("want  old env , got %q", v)
	}

	// rewrite envdir...
	if err := os.WriteFile(envfile, []byte("new env\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// ... but the worker returns the old environment value before reload.
	v, err = getEnv(ctx, envName)
	if err != nil {
		t.Fatal(err)
	}
	if v != " old env " {
		t.Errorf("want  old env , got %q", v)
	}

	// after reload, we can get the new environment value
	time.Sleep(1 * time.Second)
	go sd.Reload()
	time.Sleep(2 * time.Second)
	v, err = getEnv(ctx, envName)
	if err != nil {
		t.Fatal(err)
	}
	if v != "new env" {
		t.Errorf("want new env, got %q", v)
	}

	if err := os.Remove(envfile); err != nil {
		t.Fatal(err)
	}
	go sd.Reload()
	time.Sleep(2 * time.Second)
	v, err = getEnv(ctx, envName)
	if err != nil {
		t.Fatal(err)
	}
	if v != "not found!" {
		t.Errorf("want not found!, got %q", v)
	}
}

func Test_OverrideAutoRestartByEnvDir(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// set up envdir
	envdir := filepath.Join(dir, "envdir")
	if err := os.Mkdir(envdir, 0755); err != nil {
		t.Fatal(err)
	}

	// override auto-restart settings by envdir
	envfile := filepath.Join(envdir, EnableAutoRestartEnvName)
	if err := os.WriteFile(envfile, []byte("1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	envfile = filepath.Join(envdir, KillOldDelayEnvName)
	if err := os.WriteFile(envfile, []byte("3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	envfile = filepath.Join(envdir, AutoRestartIntervalEnvName)
	if err := os.WriteFile(envfile, []byte("6\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// build autorestart
	binFile := filepath.Join(dir, "autorestart")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binFile, filepath.Join("testdata", "autorestart", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	statusFile := filepath.Join(dir, "status")
	sd := &Starter{
		Command:    binFile,
		Ports:      []string{"0"},
		StatusFile: statusFile,
		EnvDir:     envdir,
	}
	defer sd.Close() //nolint:errcheck // ignore error on cleanup
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
	status, err := os.ReadFile(statusFile)
	if err != nil {
		t.Errorf("fail to read status file %s: %s", statusFile, err)
	}
	if ok, _ := regexp.Match(`^1:\d+\n$`, status); !ok {
		t.Errorf(`want /1:\d+\n/, got %s`, status)
	}

	// status during transient state
	time.Sleep(3 * time.Second)
	status, err = os.ReadFile(statusFile)
	if err != nil {
		t.Errorf("fail to read status file %s: %s", statusFile, err)
	}
	if ok, _ := regexp.Match(`^1:\d+\n2:\d+\n$`, status); !ok {
		t.Errorf(`want /1:\d+\n2:\d+\n/, got %s`, status)
	}

	// status after auto-restart
	time.Sleep(3 * time.Second)
	status, err = os.ReadFile(statusFile)
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

	if err := sd.Shutdown(ctx); err != nil {
		t.Errorf("sd.Shutdown() failed: %s", err)
	}
}

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		log     string
		wantCmd string
		wantOk  bool
	}{
		{"| echo hello", "echo hello", true},
		{"   | echo hello", "echo hello", true},
		{"echo hello", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		cmd, ok := extractCommand(tt.log)
		if cmd != tt.wantCmd || ok != tt.wantOk {
			t.Errorf("extractCommand(%q) = %q, %v; want %q, %v", tt.log, cmd, ok, tt.wantCmd, tt.wantOk)
		}
	}
}

func Test_Logger(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// mock stderr
	stderr := os.Stderr
	defer func() {
		os.Stderr = stderr
	}()
	var wg sync.WaitGroup
	var buf bytes.Buffer
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = pw
	wg.Go(func() {
		defer pr.Close()
		if _, err := io.Copy(&buf, pr); err != nil {
			t.Error(err)
		}
	})

	// build the server
	serverBinFile := filepath.Join(dir, "server")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", serverBinFile, filepath.Join("testdata", "logger", "server", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", "testdata/logger/server/main.go", err, output)
	}

	// build the logger
	loggerBinFile := filepath.Join(dir, "logger")
	cmd = exec.CommandContext(ctx, "go", "build", "-o", loggerBinFile, filepath.Join("testdata", "logger", "logger", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", "testdata/logger/logger/main.go", err, output)
	}

	logFile := filepath.Join(dir, "server.log")

	sd := &Starter{
		Command: serverBinFile,
		Ports:   []string{"0"},
		LogFile: "|" + loggerBinFile + " " + logFile,
	}
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // wait for starting worker

	// shutdown
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := sd.Shutdown(ctx); err != nil {
			t.Errorf("sd.Shutdown() failed: %s", err)
		}
		pw.Close()
		wg.Wait()
	}()

	want := "logger: started\n" +
		"logger: received EOF\n" +
		"logger: received terminated\n"
	if buf.String() != want {
		t.Errorf("want %q, got %q", want, buf.String())
	}

	if log, err := os.ReadFile(logFile); err == nil {
		t.Logf("logfile: %s", string(log))
	} else {
		t.Error(err)
	}
}

func Test_LoggerDies(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// build the server
	serverBinFile := filepath.Join(dir, "server")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", serverBinFile, filepath.Join("testdata", "logger", "server", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", "testdata/logger/server/main.go", err, output)
	}

	// build the logger
	loggerBinFile := filepath.Join(dir, "sleep")
	cmd = exec.CommandContext(ctx, "go", "build", "-o", loggerBinFile, filepath.Join("testdata", "logger", "sleep", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", "testdata/logger/sleep/main.go", err, output)
	}

	sd := &Starter{
		Command: serverBinFile,
		Ports:   []string{"0"},
		LogFile: "|" + loggerBinFile,
	}
	if err := sd.Run(); err != nil {
		t.Errorf("sd.Run() failed: %s", err)
	}
}

func Test_UDP(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// build the server
	serverBinFile := filepath.Join(dir, "server")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", serverBinFile, filepath.Join("testdata", "udp", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", "testdata/udp/main.go", err, output)
	}

	sd := &Starter{
		Command: serverBinFile,
		Ports:   []string{"u0"},
	}
	defer sd.Close() //nolint:errcheck // ignore error on cleanup
	go func() {
		if err := sd.Run(); err != nil {
			t.Errorf("sd.Run() failed: %s", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // wait for starting worker

	// connect to the first worker.
	addr := sd.PacketConns()[0].LocalAddr().String()
	conn, err := net.Dial("udp", addr)
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
	if string(buf[:n]) != "HELLO" {
		t.Errorf("want HELLO, got %s", buf[:n])
	}
	if err := sd.Shutdown(ctx); err != nil {
		t.Errorf("sd.Shutdown() failed: %s", err)
	}
}

func Test_RestartAndStop(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// build echod
	echod := filepath.Join(dir, "echod")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", echod, filepath.Join("testdata", "echod", "echod.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	// build start_server
	startServer := filepath.Join(dir, "start_server")
	cmd = exec.CommandContext(ctx, "go", "build", "-o", startServer, filepath.Join("cmd", "start_server", "main.go"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile %s: %s\n%s", dir, err, output)
	}

	pidFile := filepath.Join(dir, "echod.pid")
	statusFile := filepath.Join(dir, "echod.status")
	signame := filepath.Join(dir, "signame")

	cmd = exec.CommandContext(
		ctx,
		startServer, "--port=0", "--interval=5", "--pid-file="+pidFile, "--status-file="+statusFile,
		"--", echod, signame,
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	cmd.Start()

	var sd *Starter

	// wait for starting echod
	time.Sleep(time.Second)

	sd = &Starter{
		PidFile:    pidFile,
		StatusFile: statusFile,
		Restart:    true,
	}
	if err := sd.Run(); err != nil {
		t.Fatal(err)
	}

	sd = &Starter{
		PidFile:    pidFile,
		StatusFile: statusFile,
		Stop:       true,
	}
	if err := sd.Run(); err != nil {
		t.Fatal(err)
	}

	cmd.Wait()
}
