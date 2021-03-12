package listener

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestListenConfigs(t *testing.T) {
	wantOK := func(ctx context.Context, t *testing.T, ll ListenSpecs, network, address string) {
		t.Helper()
		l, err := ll.Listen(ctx, network, address)
		if err != nil {
			t.Errorf("%s, %s: unexpected error: %v", network, address, err)
			return
		}
		l.Close()
	}
	wantNG := func(ctx context.Context, t *testing.T, ll ListenSpecs, network, address string) {
		t.Helper()
		l, err := ll.Listen(ctx, network, address)
		if err != nil {
			return
		}
		l.Close()
		t.Errorf("%s, %s: error expected, got nil", network, address)
	}

	t.Run("default", func(t *testing.T) {
		l, err := net.Listen("tcp4", "0.0.0.0:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()

		f, err := l.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, port, _ := net.SplitHostPort(l.Addr().String())

		ll := ListenSpecs{
			listenSpec{
				addr: port, // no host, just only port
				fd:   f.Fd(),
			},
		}

		// If host is not specified, then the program will bind to the default address of IPv4 ("0.0.0.0").
		// https://metacpan.org/pod/distribution/Server-Starter/script/start_server#-port=(port|host:port|port=fd|host:port=fd)
		wantOK(ctx, t, ll, "tcp", ":"+port)
		wantOK(ctx, t, ll, "tcp4", ":"+port)
		wantOK(ctx, t, ll, "tcp", "0.0.0.0:"+port)
		wantOK(ctx, t, ll, "tcp4", "0.0.0.0:"+port)
		wantNG(ctx, t, ll, "tcp6", ":"+port)
		wantOK(ctx, t, ll, "tcp", "[::]:"+port)
		wantNG(ctx, t, ll, "unix", "0.0.0.0:"+port)
	})

	t.Run("ipv4", func(t *testing.T) {
		l, err := net.Listen("tcp4", "0.0.0.0:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()

		f, err := l.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, port, _ := net.SplitHostPort(l.Addr().String())

		ll := ListenSpecs{
			listenSpec{
				addr: "0.0.0.0:" + port,
				fd:   f.Fd(),
			},
		}
		wantOK(ctx, t, ll, "tcp", ":"+port)
		wantOK(ctx, t, ll, "tcp4", ":"+port)
		wantOK(ctx, t, ll, "tcp", "0.0.0.0:"+port)
		wantOK(ctx, t, ll, "tcp4", "0.0.0.0:"+port)
		wantNG(ctx, t, ll, "tcp6", ":"+port)
		wantOK(ctx, t, ll, "tcp", "[::]:"+port)
		wantNG(ctx, t, ll, "unix", "0.0.0.0:"+port)
	})

	t.Run("ipv4-loopback", func(t *testing.T) {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()

		f, err := l.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, port, _ := net.SplitHostPort(l.Addr().String())

		ll := ListenSpecs{
			listenSpec{
				addr: "127.0.0.1:" + port,
				fd:   f.Fd(),
			},
		}
		wantOK(ctx, t, ll, "tcp", "127.0.0.1:"+port)
		wantOK(ctx, t, ll, "tcp4", "127.0.0.1:"+port)
		wantOK(ctx, t, ll, "tcp", "[::1]:"+port)
		wantNG(ctx, t, ll, "unix", "127.0.0.1:"+port)
	})

	t.Run("ipv6", func(t *testing.T) {
		l, err := net.Listen("tcp6", ":0")
		if err != nil {
			t.Skip("IPv6 is not supported?")
			return
		}
		defer l.Close()

		f, err := l.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, port, _ := net.SplitHostPort(l.Addr().String())

		ll := ListenSpecs{
			listenSpec{
				addr: "[::]:" + port,
				fd:   f.Fd(),
			},
		}
		wantOK(ctx, t, ll, "tcp", ":"+port)
		wantOK(ctx, t, ll, "tcp6", ":"+port)
		wantOK(ctx, t, ll, "tcp", "[::]:"+port)
		wantOK(ctx, t, ll, "tcp6", "[::]:"+port)
		wantNG(ctx, t, ll, "tcp", "0.0.0.0:"+port)
		wantNG(ctx, t, ll, "tcp4", ":"+port)
		wantNG(ctx, t, ll, "unix", ":"+port)
	})

	t.Run("ipv6-loopback", func(t *testing.T) {
		l, err := net.Listen("tcp6", "[::1]:0")
		if err != nil {
			t.Skip("IPv6 is not supported?")
			return
		}
		defer l.Close()

		f, err := l.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, port, _ := net.SplitHostPort(l.Addr().String())

		ll := ListenSpecs{
			listenSpec{
				addr: "[::1]:" + port,
				fd:   f.Fd(),
			},
		}
		wantOK(ctx, t, ll, "tcp", "[::1]:"+port)
		wantOK(ctx, t, ll, "tcp6", "[::1]:"+port)
		wantNG(ctx, t, ll, "tcp", "127.0.0.1:"+port)
		wantNG(ctx, t, ll, "unix", "[::1]:"+port)
	})

	t.Run("unix", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "server-starter-test")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %s", err)
		}
		defer os.RemoveAll(dir)

		pwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("fail to getwd:%s", err)
		}
		os.Chdir(dir)
		defer os.Chdir(pwd)

		sock := filepath.Join(dir, "127.0.0.1:8000")
		l, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()

		f, err := l.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		ll := ListenSpecs{
			listenSpec{
				addr: "127.0.0.1:8000",
				fd:   f.Fd(),
			},
		}
		wantOK(ctx, t, ll, "unix", sock)
		wantOK(ctx, t, ll, "unix", "127.0.0.1:8000")
		wantNG(ctx, t, ll, "tcp", "127.0.0.1:8000")
		wantNG(ctx, t, ll, "tcp4", "127.0.0.1:8000")
	})
}

func TestListenConfigs_ListenPacket(t *testing.T) {
	wantOK := func(ctx context.Context, t *testing.T, ll ListenSpecs, network, address string) {
		t.Helper()
		conn, err := ll.ListenPacket(ctx, network, address)
		if err != nil {
			t.Errorf("%s, %s: unexpected error: %v", network, address, err)
			return
		}
		conn.Close()
	}
	wantNG := func(ctx context.Context, t *testing.T, ll ListenSpecs, network, address string) {
		t.Helper()
		conn, err := ll.ListenPacket(ctx, network, address)
		if err != nil {
			return
		}
		conn.Close()
		t.Errorf("%s, %s: error expected, got nil", network, address)
	}

	t.Run("default", func(t *testing.T) {
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		f, err := conn.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, port, _ := net.SplitHostPort(conn.LocalAddr().String())

		ll := ListenSpecs{
			listenSpec{
				addr: port, // no host, just only port
				fd:   f.Fd(),
			},
		}

		// If host is not specified, then the program will bind to the default address of IPv4 ("0.0.0.0").
		// https://metacpan.org/pod/distribution/Server-Starter/script/start_server#-port=(port|host:port|port=fd|host:port=fd)
		wantOK(ctx, t, ll, "udp", ":"+port)
		wantOK(ctx, t, ll, "udp4", ":"+port)
		wantOK(ctx, t, ll, "udp", "0.0.0.0:"+port)
		wantOK(ctx, t, ll, "udp4", "0.0.0.0:"+port)
		wantNG(ctx, t, ll, "udp6", ":"+port)
		wantOK(ctx, t, ll, "udp", "[::]:"+port)
	})

	t.Run("ipv4", func(t *testing.T) {
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		f, err := conn.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, port, _ := net.SplitHostPort(conn.LocalAddr().String())

		ll := ListenSpecs{
			listenSpec{
				addr: "0.0.0.0:" + port,
				fd:   f.Fd(),
			},
		}
		wantOK(ctx, t, ll, "udp", ":"+port)
		wantOK(ctx, t, ll, "udp4", ":"+port)
		wantOK(ctx, t, ll, "udp", "0.0.0.0:"+port)
		wantOK(ctx, t, ll, "udp4", "0.0.0.0:"+port)
		wantNG(ctx, t, ll, "udp6", ":"+port)
		wantOK(ctx, t, ll, "udp", "[::]:"+port)
	})

	t.Run("ipv4-loopback", func(t *testing.T) {
		conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		f, err := conn.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, port, _ := net.SplitHostPort(conn.LocalAddr().String())

		ll := ListenSpecs{
			listenSpec{
				addr: "127.0.0.1:" + port,
				fd:   f.Fd(),
			},
		}
		wantOK(ctx, t, ll, "udp", "127.0.0.1:"+port)
		wantOK(ctx, t, ll, "udp4", "127.0.0.1:"+port)
		wantOK(ctx, t, ll, "udp", "[::1]:"+port)
	})

	t.Run("ipv6", func(t *testing.T) {
		conn, err := net.ListenPacket("udp6", ":0")
		if err != nil {
			t.Skip("IPv6 is not supported?")
			return
		}
		defer conn.Close()

		f, err := conn.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, port, _ := net.SplitHostPort(conn.LocalAddr().String())

		ll := ListenSpecs{
			listenSpec{
				addr: "[::]:" + port,
				fd:   f.Fd(),
			},
		}
		wantOK(ctx, t, ll, "udp", ":"+port)
		wantOK(ctx, t, ll, "udp6", ":"+port)
		wantOK(ctx, t, ll, "udp", "[::]:"+port)
		wantOK(ctx, t, ll, "udp6", "[::]:"+port)
		wantNG(ctx, t, ll, "udp", "0.0.0.0:"+port)
		wantNG(ctx, t, ll, "udp4", ":"+port)
	})

	t.Run("ipv6-loopback", func(t *testing.T) {
		conn, err := net.ListenPacket("udp6", "[::1]:0")
		if err != nil {
			t.Skip("IPv6 is not supported?")
			return
		}
		defer conn.Close()

		f, err := conn.(interface{ File() (*os.File, error) }).File()
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, port, _ := net.SplitHostPort(conn.LocalAddr().String())

		ll := ListenSpecs{
			listenSpec{
				addr: "[::1]:" + port,
				fd:   f.Fd(),
			},
		}
		wantOK(ctx, t, ll, "udp", "[::1]:"+port)
		wantOK(ctx, t, ll, "udp6", "[::1]:"+port)
		wantNG(ctx, t, ll, "udp", "127.0.0.1:"+port)
	})
}

func TestPort(t *testing.T) {
	cases := []struct {
		in string
		ll []listenSpec
	}{
		{
			in: "0.0.0.0:80=3",
			ll: []listenSpec{
				{
					addr: "0.0.0.0:80",
					fd:   3,
				},
			},
		},
		{
			in: "0.0.0.0:80=3;/tmp/foo.sock=4",
			ll: []listenSpec{
				{
					addr: "0.0.0.0:80",
					fd:   3,
				},
				{
					addr: "/tmp/foo.sock",
					fd:   4,
				},
			},
		},
		{
			in: "50908=4",
			ll: []listenSpec{
				{
					addr: "50908",
					fd:   4,
				},
			},
		},
		{
			in: "",
			ll: []listenSpec{},
		},
	}

	for i, tc := range cases {
		ll, err := parseListenTargets(tc.in, true)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(ll) != len(tc.ll) {
			t.Errorf("#%d: want %d, got %d", i, len(tc.ll), len(ll))
		}
		for i, l := range ll {
			l := l.(listenSpec)
			if !reflect.DeepEqual(l, tc.ll[i]) {
				t.Errorf("#%d, want %#v, got %#v", i, tc.ll[i], l)
			}
		}
		if ll.String() != tc.in {
			t.Errorf("#%d, want %s, got %s", i, tc.in, ll.String())
		}
	}

	errs := []string{
		"0.0.0.0:80=foo", // invalid fd
		"0.0.0.0:80",     // missing fd
	}
	for i, tc := range errs {
		ll, err := parseListenTargets(tc, true)
		if err == nil {
			t.Errorf("#%d: want error, got nil", i)
		}
		if ll != nil {
			t.Errorf("#%d: want nil, got %#v", i, ll)
		}
	}
}

func TestPortNoEnv(t *testing.T) {
	ports, err := parseListenTargets("", false)
	if err != ErrNoListeningTarget {
		t.Error("Ports must return error if no env")
	}

	if ports != nil {
		t.Errorf("Ports must return nil if no env")
	}
}
