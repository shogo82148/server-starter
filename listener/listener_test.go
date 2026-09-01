package listener

import (
	"fmt"
	"net"
	"os"
	"testing"
)

type socket interface {
	File() (*os.File, error)
	Close() error
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
