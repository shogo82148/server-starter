package listener

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

type socket interface {
	File() (*os.File, error)
	Close() error
}

func TestTCPListener_Fd(t *testing.T) {
	l := newTCPListener("127.0.0.1", 8080, 42)
	if l.Fd() != 42 {
		t.Errorf("Fd() = %d, want 42", l.Fd())
	}
}

func TestTCPListener_Addr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		port int
		want string
	}{
		{
			name: "specific host",
			addr: "127.0.0.1",
			port: 8080,
			want: "127.0.0.1:8080",
		},
		{
			name: "empty host defaults to wildcard",
			addr: "",
			port: 8080,
			want: "0.0.0.0:8080",
		},
		{
			name: "IPv6 host",
			addr: "::1",
			port: 8080,
			want: "[::1]:8080",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTCPListener(tt.addr, tt.port, 0)
			if got := l.Addr(); got != tt.want {
				t.Errorf("Addr() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTCPListener_String(t *testing.T) {
	tests := []struct {
		name string
		addr string
		port int
		fd   uintptr
		want string
	}{
		{
			name: "wildcard host omits address",
			addr: "",
			port: 8080,
			fd:   3,
			want: "8080=3",
		},
		{
			name: "explicit wildcard host omits address",
			addr: "0.0.0.0",
			port: 8080,
			fd:   3,
			want: "8080=3",
		},
		{
			name: "specific host",
			addr: "127.0.0.1",
			port: 8080,
			fd:   4,
			want: "127.0.0.1:8080=4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTCPListener(tt.addr, tt.port, tt.fd)
			if got := l.String(); got != tt.want {
				t.Errorf("String() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTCPListener_ListenPacket(t *testing.T) {
	l := newTCPListener("127.0.0.1", 8080, 0)
	conn, err := l.ListenPacket()
	if err == nil {
		conn.Close() //nolint:errcheck // ignore error on cleanup
		t.Fatal("ListenPacket() = nil, want error")
	}
}

func TestTCPListener_Listen(t *testing.T) {
	// create a real TCP listener to obtain a valid file descriptor.
	orig, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() = %v, want nil", err)
	}
	t.Cleanup(func() { orig.Close() }) //nolint:errcheck // ignore error on cleanup

	f, err := orig.(socket).File()
	if err != nil {
		t.Fatalf("File() = %v, want nil", err)
	}
	t.Cleanup(func() { f.Close() }) //nolint:errcheck // ignore error on cleanup

	addr := orig.Addr().(*net.TCPAddr)
	l := newTCPListener(addr.IP.String(), addr.Port, f.Fd())

	ln, err := l.Listen()
	if err != nil {
		t.Fatalf("Listen() = %v, want nil", err)
	}
	t.Cleanup(func() { ln.Close() }) //nolint:errcheck // ignore error on cleanup

	if _, ok := ln.(*net.TCPListener); !ok {
		t.Fatalf("Listen() returned %T, want *net.TCPListener", ln)
	}

	// the returned listener should accept a connection on the same address.
	go func() {
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			return
		}
		conn.Close() //nolint:errcheck // ignore error on cleanup
	}()

	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("Accept() = %v, want nil", err)
	}
	conn.Close() //nolint:errcheck // ignore error on cleanup
}

func TestTCPListener_Listen_invalidFd(t *testing.T) {
	// use a file descriptor that is not a socket.
	f, err := os.CreateTemp(t.TempDir(), "not-a-socket")
	if err != nil {
		t.Fatalf("os.CreateTemp() = %v, want nil", err)
	}
	t.Cleanup(func() { f.Close() }) //nolint:errcheck // ignore error on cleanup

	l := newTCPListener("127.0.0.1", 8080, f.Fd())
	ln, err := l.Listen()
	if err == nil {
		ln.Close() //nolint:errcheck // ignore error on cleanup
		t.Fatal("Listen() = nil, want error")
	}
}

func TestUDPListener_Fd(t *testing.T) {
	l := newUDPListener("127.0.0.1", 8080, 42)
	if l.Fd() != 42 {
		t.Errorf("Fd() = %d, want 42", l.Fd())
	}
}

func TestUDPListener_Addr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		port int
		want string
	}{
		{
			name: "specific host",
			addr: "127.0.0.1",
			port: 8080,
			want: "127.0.0.1:8080",
		},
		{
			name: "empty host defaults to wildcard",
			addr: "",
			port: 8080,
			want: "0.0.0.0:8080",
		},
		{
			name: "IPv6 host",
			addr: "::1",
			port: 8080,
			want: "[::1]:8080",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newUDPListener(tt.addr, tt.port, 0)
			if got := l.Addr(); got != tt.want {
				t.Errorf("Addr() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestUDPListener_String(t *testing.T) {
	tests := []struct {
		name string
		addr string
		port int
		fd   uintptr
		want string
	}{
		{
			name: "wildcard host omits address",
			addr: "",
			port: 8080,
			fd:   3,
			want: "8080=3",
		},
		{
			name: "explicit wildcard host omits address",
			addr: "0.0.0.0",
			port: 8080,
			fd:   3,
			want: "8080=3",
		},
		{
			name: "specific host",
			addr: "127.0.0.1",
			port: 8080,
			fd:   4,
			want: "127.0.0.1:8080=4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newUDPListener(tt.addr, tt.port, tt.fd)
			if got := l.String(); got != tt.want {
				t.Errorf("String() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestUDPListener_Listen(t *testing.T) {
	l := newUDPListener("127.0.0.1", 8080, 0)
	ln, err := l.Listen()
	if err == nil {
		ln.Close() //nolint:errcheck // ignore error on cleanup
		t.Fatal("Listen() = nil, want error")
	}
}

func TestUDPListener_ListenPacket(t *testing.T) {
	// create a real UDP socket to obtain a valid file descriptor.
	orig, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.ListenPacket() = %v, want nil", err)
	}
	t.Cleanup(func() { orig.Close() }) //nolint:errcheck // ignore error on cleanup

	f, err := orig.(socket).File()
	if err != nil {
		t.Fatalf("File() = %v, want nil", err)
	}
	t.Cleanup(func() { f.Close() }) //nolint:errcheck // ignore error on cleanup

	addr := orig.LocalAddr().(*net.UDPAddr)
	l := newUDPListener(addr.IP.String(), addr.Port, f.Fd())

	conn, err := l.ListenPacket()
	if err != nil {
		t.Fatalf("ListenPacket() = %v, want nil", err)
	}
	t.Cleanup(func() { conn.Close() }) //nolint:errcheck // ignore error on cleanup

	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		t.Fatalf("ListenPacket() returned %T, want *net.UDPConn", conn)
	}

	// the returned conn should receive a datagram sent to the same address.
	want := []byte("hello")
	client, err := net.Dial("udp", udpConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("net.Dial() = %v, want nil", err)
	}
	t.Cleanup(func() { client.Close() }) //nolint:errcheck // ignore error on cleanup
	if _, err := client.Write(want); err != nil {
		t.Fatalf("Write() = %v, want nil", err)
	}

	buf := make([]byte, len(want))
	n, _, err := udpConn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom() = %v, want nil", err)
	}
	if string(buf[:n]) != string(want) {
		t.Fatalf("ReadFrom() = %q, want %q", buf[:n], want)
	}
}

func TestUDPListener_ListenPacket_invalidFd(t *testing.T) {
	// use a file descriptor that is not a socket.
	f, err := os.CreateTemp(t.TempDir(), "not-a-socket")
	if err != nil {
		t.Fatalf("os.CreateTemp() = %v, want nil", err)
	}
	t.Cleanup(func() { f.Close() }) //nolint:errcheck // ignore error on cleanup

	l := newUDPListener("127.0.0.1", 8080, f.Fd())
	conn, err := l.ListenPacket()
	if err == nil {
		conn.Close() //nolint:errcheck // ignore error on cleanup
		t.Fatal("ListenPacket() = nil, want error")
	}
}

func TestUnixListener_Fd(t *testing.T) {
	l := newUnixListener("/tmp/test.sock", 42)
	if l.Fd() != 42 {
		t.Errorf("Fd() = %d, want 42", l.Fd())
	}
}

func TestUnixListener_Addr(t *testing.T) {
	l := newUnixListener("/tmp/test.sock", 0)
	if got := l.Addr(); got != "/tmp/test.sock" {
		t.Errorf("Addr() = %s, want /tmp/test.sock", got)
	}
}

func TestUnixListener_String(t *testing.T) {
	l := newUnixListener("/tmp/test.sock", 3)
	if got := l.String(); got != "/tmp/test.sock=3" {
		t.Errorf("String() = %s, want /tmp/test.sock=3", got)
	}
}

func TestUnixListener_ListenPacket(t *testing.T) {
	l := newUnixListener("/tmp/test.sock", 0)
	conn, err := l.ListenPacket()
	if err == nil {
		conn.Close() //nolint:errcheck // ignore error on cleanup
		t.Fatal("ListenPacket() = nil, want error")
	}
}

func TestUnixListener_Listen(t *testing.T) {
	// create a real Unix socket to obtain a valid file descriptor.
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	orig, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen() = %v, want nil", err)
	}
	t.Cleanup(func() { orig.Close() }) //nolint:errcheck // ignore error on cleanup

	f, err := orig.(socket).File()
	if err != nil {
		t.Fatalf("File() = %v, want nil", err)
	}
	t.Cleanup(func() { f.Close() }) //nolint:errcheck // ignore error on cleanup

	l := newUnixListener(sockPath, f.Fd())

	ln, err := l.Listen()
	if err != nil {
		t.Fatalf("Listen() = %v, want nil", err)
	}
	t.Cleanup(func() { ln.Close() }) //nolint:errcheck // ignore error on cleanup

	if _, ok := ln.(*net.UnixListener); !ok {
		t.Fatalf("Listen() returned %T, want *net.UnixListener", ln)
	}

	// the returned listener should accept a connection on the same path.
	go func() {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			return
		}
		conn.Close() //nolint:errcheck // ignore error on cleanup
	}()

	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("Accept() = %v, want nil", err)
	}
	conn.Close() //nolint:errcheck // ignore error on cleanup
}

func TestUnixListener_Listen_invalidFd(t *testing.T) {
	// use a file descriptor that is not a socket.
	f, err := os.CreateTemp(t.TempDir(), "not-a-socket")
	if err != nil {
		t.Fatalf("os.CreateTemp() = %v, want nil", err)
	}
	t.Cleanup(func() { f.Close() }) //nolint:errcheck // ignore error on cleanup

	l := newUnixListener("/tmp/test.sock", f.Fd())
	ln, err := l.Listen()
	if err == nil {
		ln.Close() //nolint:errcheck // ignore error on cleanup
		t.Fatal("Listen() = nil, want error")
	}
}

func TestParseListenTargets(t *testing.T) {
	t.Run("TCP sockets", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.Listen() = %v, want nil", err)
		}
		t.Cleanup(func() { l.Close() }) //nolint:errcheck // ignore error on cleanup

		sock := l.(socket)
		f, err := sock.File()
		if err != nil {
			t.Fatalf("sock.File() = %v, want nil", err)
		}

		// parse the listen target string
		s := fmt.Sprintf("%s=%d", l.Addr().String(), f.Fd())
		ll, err := parseListenTargets(s)
		if err != nil {
			t.Fatalf("parseListenTargets() = %v, want nil", err)
		}

		// verify
		if len(ll) != 1 {
			t.Fatalf("len(ll) = %d, want 1", len(ll))
		}
		if _, ok := ll[0].(*TCPListener); !ok {
			t.Fatalf("ll[0] is not *TCPListener, got %T", ll[0])
		}
		if ll[0].Addr() != l.Addr().String() {
			t.Fatalf("ll[0].Addr() = %s, want %s", ll[0].Addr(), l.Addr().String())
		}
	})

	t.Run("Unix sockets", func(t *testing.T) {
		sockPath := "./test.sock"
		l, err := net.Listen("unix", sockPath)
		if err != nil {
			t.Fatalf("net.Listen() = %v, want nil", err)
		}
		t.Cleanup(func() { l.Close() }) //nolint:errcheck // ignore error on cleanup

		sock := l.(socket)
		f, err := sock.File()
		if err != nil {
			t.Fatalf("sock.File() = %v, want nil", err)
		}

		// parse the listen target string
		s := fmt.Sprintf("%s=%d", sockPath, f.Fd())
		ll, err := parseListenTargets(s)
		if err != nil {
			t.Fatalf("parseListenTargets() = %v, want nil", err)
		}

		// verify
		if len(ll) != 1 {
			t.Fatalf("len(ll) = %d, want 1", len(ll))
		}
		if _, ok := ll[0].(*UnixListener); !ok {
			t.Fatalf("ll[0] is not *UnixListener, got %T", ll[0])
		}
		if ll[0].Addr() != sockPath {
			t.Fatalf("ll[0].Addr() = %s, want %s", ll[0].Addr(), sockPath)
		}
	})

	t.Run("UDP sockets", func(t *testing.T) {
		l, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.ListenPacket() = %v, want nil", err)
		}
		t.Cleanup(func() { l.Close() }) //nolint:errcheck // ignore error on cleanup

		sock := l.(socket)
		f, err := sock.File()
		if err != nil {
			t.Fatalf("sock.File() = %v, want nil", err)
		}

		// parse the listen target string
		s := fmt.Sprintf("%s=%d", l.LocalAddr().String(), f.Fd())
		ll, err := parseListenTargets(s)
		if err != nil {
			t.Fatalf("parseListenTargets() = %v, want nil", err)
		}

		// verify
		if len(ll) != 1 {
			t.Fatalf("len(ll) = %d, want 1", len(ll))
		}
		if _, ok := ll[0].(*UDPListener); !ok {
			t.Fatalf("ll[0] is not *UDPListener, got %T", ll[0])
		}
		if ll[0].Addr() != l.LocalAddr().String() {
			t.Fatalf("ll[0].Addr() = %s, want %s", ll[0].Addr(), l.LocalAddr().String())
		}
	})
}
