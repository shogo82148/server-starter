package listener

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const wildcardIPv4 = "0.0.0.0"

// ListenConfig is a generator of net.Listener.
type ListenConfig interface {
	Listen(ctx context.Context, network, address string) (net.Listener, error)
	ListenPacket(ctx context.Context, network, address string) (net.PacketConn, error)
}

// ErrNoListeningTarget is returned by ListenAll calls
// when the process is not started using server_starter.
var ErrNoListeningTarget = errors.New("listener: no listening target")

// ListenSpec is the interface for things that listen on file descriptors
// specified by Start::Server / server_starter
type ListenSpec interface {
	// Fd returns the underlying file descriptor
	Fd() uintptr

	// Listen creates a new Listener
	Listen() (net.Listener, error)

	// ListenPacket creates new PacketConn
	ListenPacket() (net.PacketConn, error)

	// Addr returns the address.
	Addr() string

	// return a string compatible with SERVER_STARTER_PORT
	String() string
}

var _ ListenSpec = (*TCPListener)(nil)

type TCPListener struct {
	addr string
	port int
	fd   uintptr
}

func newTCPListener(addr string, port int, fd uintptr) *TCPListener {
	if addr == "" {
		addr = wildcardIPv4
	}
	return &TCPListener{
		addr: addr,
		port: port,
		fd:   fd,
	}
}

func (l *TCPListener) Fd() uintptr {
	return l.fd
}

func (l *TCPListener) Listen() (net.Listener, error) {
	file := os.NewFile(l.fd, l.Addr())
	if file == nil {
		return nil, fmt.Errorf("listener: invalid file descriptor: %d", l.fd)
	}
	listener, err := net.FileListener(file)
	closeErr := file.Close()
	if err != nil {
		return nil, fmt.Errorf("listener: failed to create TCP listener: %w", err)
	}
	if closeErr != nil {
		listener.Close() //nolint:errcheck // ignore error on cleanup
		return nil, fmt.Errorf("listener: failed to close file descriptor: %w", closeErr)
	}
	return listener, err
}

func (l *TCPListener) ListenPacket() (net.PacketConn, error) {
	return nil, errors.New("listener: TCPListener does not support ListenPacket")
}

func (l *TCPListener) Addr() string {
	return net.JoinHostPort(l.addr, strconv.Itoa(l.port))
}

func (l *TCPListener) String() string {
	if l.addr == wildcardIPv4 {
		return fmt.Sprintf("%d=%d", l.port, l.fd)
	}
	return fmt.Sprintf("%s=%d", l.Addr(), l.fd)
}

var _ ListenSpec = (*UDPListener)(nil)

type UDPListener struct {
	addr string
	port int
	fd   uintptr
}

func newUDPListener(addr string, port int, fd uintptr) *UDPListener {
	if addr == "" {
		addr = wildcardIPv4
	}
	return &UDPListener{
		addr: addr,
		port: port,
		fd:   fd,
	}
}

func (l *UDPListener) Fd() uintptr {
	return l.fd
}

func (l *UDPListener) Listen() (net.Listener, error) {
	return nil, errors.New("listener: UDPListener does not support Listen")
}

func (l *UDPListener) ListenPacket() (net.PacketConn, error) {
	file := os.NewFile(l.fd, l.Addr())
	if file == nil {
		return nil, fmt.Errorf("listener: invalid file descriptor: %d", l.fd)
	}
	conn, err := net.FilePacketConn(file)
	closeErr := file.Close()
	if err != nil {
		return nil, fmt.Errorf("listener: failed to create UDP listener: %w", err)
	}
	if closeErr != nil {
		conn.Close() //nolint:errcheck // ignore error on cleanup
		return nil, fmt.Errorf("listener: failed to close file descriptor: %w", closeErr)
	}
	return conn, err
}

func (l *UDPListener) Addr() string {
	return net.JoinHostPort(l.addr, strconv.Itoa(l.port))
}

func (l *UDPListener) String() string {
	if l.addr == wildcardIPv4 {
		return fmt.Sprintf("%d=%d", l.port, l.fd)
	}
	return fmt.Sprintf("%s=%d", l.Addr(), l.fd)
}

var _ ListenSpec = (*UnixListener)(nil)

type UnixListener struct {
	path string
	fd   uintptr
}

func newUnixListener(path string, fd uintptr) *UnixListener {
	return &UnixListener{
		path: path,
		fd:   fd,
	}
}

func (l *UnixListener) Fd() uintptr {
	return l.fd
}

func (l *UnixListener) Listen() (net.Listener, error) {
	file := os.NewFile(l.fd, l.path)
	if file == nil {
		return nil, fmt.Errorf("listener: invalid file descriptor: %d", l.fd)
	}
	listener, err := net.FileListener(file)
	closeErr := file.Close()
	if err != nil {
		return nil, fmt.Errorf("listener: failed to create Unix listener: %w", err)
	}
	if closeErr != nil {
		listener.Close() //nolint:errcheck // ignore error on cleanup
		return nil, fmt.Errorf("listener: failed to close file descriptor: %w", closeErr)
	}
	return listener, err
}

func (l *UnixListener) ListenPacket() (net.PacketConn, error) {
	return nil, errors.New("listener: UnixListener does not support ListenPacket")
}

func (l *UnixListener) Addr() string {
	return l.path
}

func (l *UnixListener) String() string {
	return fmt.Sprintf("%s=%d", l.path, l.fd)
}

// ListenSpecs holds a list of ListenConfig. This is here just for convenience
// so that you can do
//
//	list.String()
//
// to get a string compatible with SERVER_STARTER_PORT
type ListenSpecs []ListenSpec

func (ll ListenSpecs) String() string {
	if len(ll) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, l := range ll {
		builder.WriteString(l.String())
		builder.WriteByte(';')
	}
	s := builder.String()
	return s[:len(s)-1] // remove last ';'
}

// Listen announces on the local network address.
// The network must be "tcp", "tcp4", "tcp6", "unix".
func (ll ListenSpecs) Listen(ctx context.Context, network, address string) (net.Listener, error) {
	var addrlist []string
	switch network {
	case "tcp", "tcp4", "tcp6":
		var ips []net.IPAddr
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		portnum, err := net.DefaultResolver.LookupPort(ctx, network, port)
		if err != nil {
			return nil, err
		}
		port = strconv.Itoa(portnum)

		if host != "" {
			ips, err = net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
		} else {
			if network == "tcp" || network == "tcp4" {
				ips = append(ips, net.IPAddr{IP: net.IPv4zero})
			}
			if network == "tcp" || network == "tcp6" {
				ips = append(ips, net.IPAddr{IP: net.IPv6unspecified})
			}
		}
		for _, ip := range ips {
			v4 := ip.IP.To4()
			if v4 != nil && (network == "tcp" || network == "tcp4") {
				addrlist = append(addrlist, net.JoinHostPort(v4.String(), port))
			}

			v6 := ip.IP.To16()
			if v6 != nil && (network == "tcp" || v4 == nil && network == "tcp6") {
				addrlist = append(addrlist, net.JoinHostPort(v6.String(), port))
			}

			// fallback v6 to v4
			if (network == "tcp" || network == "tcp4") && ip.IP.IsUnspecified() {
				addrlist = append(addrlist, port, "0.0.0.0:"+port)
			}
			if (network == "tcp" || network == "tcp4") && ip.IP.IsLoopback() {
				addrlist = append(addrlist, "127.0.0.1:"+port)
			}
		}
	case "unix":
		stat1, err := os.Stat(address)
		if err != nil {
			return nil, err
		}
		for _, l := range ll {
			stat2, err := os.Stat(l.Addr())
			if err != nil {
				continue
			}
			if os.SameFile(stat1, stat2) {
				ln, err := l.Listen()
				if err != nil {
					continue
				}
				if _, ok := ln.(*net.UnixListener); !ok {
					ln.Close()
					continue
				}
				return ln, nil
			}
		}
		return nil, fmt.Errorf("listener: address %s is not being bound to the server", address)
	default:
		return nil, net.UnknownNetworkError(network)
	}

	for _, l := range ll {
		a := l.Addr()
		for _, addr := range addrlist {
			if addr != a {
				continue
			}
			ln, err := l.Listen()
			if err != nil {
				continue
			}
			if _, ok := ln.(*net.TCPListener); !ok {
				ln.Close()
				continue
			}
			return ln, nil
		}
	}

	return nil, fmt.Errorf("listener: address %s is not being bound to the server", address)
}

// ListenPacket announces on the local network address.
// The network must be "udp", "udp4", "udp6".
func (ll ListenSpecs) ListenPacket(ctx context.Context, network, address string) (net.PacketConn, error) {
	var addrlist []string
	switch network {
	case "udp", "udp4", "udp6":
		var ips []net.IPAddr
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		portnum, err := net.DefaultResolver.LookupPort(ctx, network, port)
		if err != nil {
			return nil, err
		}
		port = strconv.Itoa(portnum)

		if host != "" {
			ips, err = net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
		} else {
			if network == "udp" || network == "udp4" {
				ips = append(ips, net.IPAddr{IP: net.IPv4zero})
			}
			if network == "udp" || network == "udp6" {
				ips = append(ips, net.IPAddr{IP: net.IPv6unspecified})
			}
		}
		for _, ip := range ips {
			v4 := ip.IP.To4()
			if v4 != nil && (network == "udp" || network == "udp4") {
				addrlist = append(addrlist, net.JoinHostPort(v4.String(), port))
			}

			v6 := ip.IP.To16()
			if v6 != nil && (network == "udp" || v4 == nil && network == "udp6") {
				addrlist = append(addrlist, net.JoinHostPort(v6.String(), port))
			}

			// fallback v6 to v4
			if (network == "udp" || network == "udp4") && ip.IP.IsUnspecified() {
				addrlist = append(addrlist, port, "0.0.0.0:"+port)
			}
			if (network == "udp" || network == "udp4") && ip.IP.IsLoopback() {
				addrlist = append(addrlist, "127.0.0.1:"+port)
			}
		}
	default:
		return nil, net.UnknownNetworkError(network)
	}

	for _, l := range ll {
		a := l.Addr()
		for _, addr := range addrlist {
			if addr != a {
				continue
			}
			conn, err := l.ListenPacket()
			if err != nil {
				continue
			}
			if _, ok := conn.(*net.UDPConn); !ok {
				conn.Close()
				continue
			}
			return conn, nil
		}
	}

	return nil, fmt.Errorf("listener: address %s is not being bound to the server", address)
}

// ListenAll announces on the local network address.
func (ll ListenSpecs) ListenAll(ctx context.Context) ([]net.Listener, error) {
	ret := make([]net.Listener, 0, len(ll))
	for _, lc := range ll {
		l, err := lc.Listen()
		if err != nil {
			for _, ln := range ret {
				ln.Close() //nolint:errcheck // ignore error on cleanup
			}
			return nil, err
		}
		ret = append(ret, l)
	}
	return ret, nil
}

// ListenPacketAll announces on the local network address.
func (ll ListenSpecs) ListenPacketAll(ctx context.Context) ([]net.PacketConn, error) {
	ret := make([]net.PacketConn, 0, len(ll))
	for _, lc := range ll {
		conn, err := lc.ListenPacket()
		if err != nil {
			for _, ln := range ret {
				ln.Close() //nolint:errcheck // ignore error on cleanup
			}
			return nil, err
		}
		ret = append(ret, conn)
	}
	return ret, nil
}

// cutLast is a port of strings.Cut, which is available since Go 1.27.
func cutLast(s, sep string) (string, string, bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

func splitHostPort(hostport string) (host string, port int, err error) {
	port, err = strconv.Atoi(hostport)
	if err == nil {
		return wildcardIPv4, port, nil
	}
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return "", 0, err
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func parseListenTargets(str string) (ListenSpecs, error) {
	if str == "" {
		return []ListenSpec{}, nil
	}

	rawspec := strings.Split(str, ";")
	ret := make([]ListenSpec, 0, len(rawspec))
	for _, pairString := range rawspec {
		spec, err := parseListenTarget(pairString)
		if err != nil {
			return nil, err
		}
		ret = append(ret, spec)
	}
	return ret, nil
}

func parseListenTarget(s string) (ListenSpec, error) {
	addr, fdString, ok := cutLast(s, "=")
	if !ok {
		return nil, fmt.Errorf("listener: failed to parse '%s' as listen target", s)
	}
	fd, err := strconv.ParseUint(fdString, 10, 0)
	if err != nil {
		return nil, fmt.Errorf("listener: failed to parse '%s' as listen target: %w", s, err)
	}

	sa, err := unix.Getsockname(int(fd))
	if err != nil {
		return nil, fmt.Errorf("listener: failed to parse '%s' as listen target: %w", s, err)
	}
	soType, err := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_TYPE)
	if err != nil {
		return nil, fmt.Errorf("listener: failed to parse '%s' as listen target: %w", s, err)
	}

	switch sa.(type) {
	case *unix.SockaddrUnix:
		// Unix socket
		return newUnixListener(addr, uintptr(fd)), nil
	case *unix.SockaddrInet4, *unix.SockaddrInet6:
		// TCP or UDP socket
		host, port, err := splitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("listener: failed to parse '%s' as listen target: %w", s, err)
		}
		switch soType {
		case unix.SOCK_STREAM:
			// TCP socket
			return newTCPListener(host, port, uintptr(fd)), nil
		case unix.SOCK_DGRAM:
			// UDP socket
			return newUDPListener(host, port, uintptr(fd)), nil
		}
	}
	return nil, fmt.Errorf("listener: failed to parse '%s' as listen target: unknown socket type", s)
}

// PortsSpecification returns the value of SERVER_STARTER_PORT
// environment variable.
// If the process starts from the start_server command,
// returns the port specification and the boolean is true.
// Otherwise the returned value will be empty and the boolean will be false.
func PortsSpecification() (string, bool) {
	return os.LookupEnv(PortEnvName)
}

// Ports parses the environment variable SERVER_STARTER_PORT,
// and return ListenSpecs.
// If SERVER_STARTER_PORT is not defined, return ErrNoListeningTarget.
func Ports() (ListenSpecs, error) {
	portSpec, ok := PortsSpecification()
	if !ok {
		return nil, ErrNoListeningTarget
	}
	ll, err := parseListenTargets(portSpec)
	if err != nil {
		return nil, err
	}
	return ll, nil
}

// PortsFallback returns the same result as Ports, if SERVER_STARTER_PORT is defined.
// Otherwise returns net.ListenConfig instead of ListenSpecs.
// Regardless of whether the process starts from the start_server command or not,
// you can call Listen method.
//
//	lc, err := listener.PortsFallback()
//	l, err := lc.Listen(ctx, "tcp", ":8080")
func PortsFallback() (ListenConfig, error) {
	portSpec, ok := PortsSpecification()
	if !ok {
		return &net.ListenConfig{}, nil
	}
	ll, err := parseListenTargets(portSpec)
	if err != nil {
		return nil, err
	}
	return ll, nil
}

// ListenAll parses SERVER_STARTER_PORT and creates net.Listener objects.
func ListenAll(ctx context.Context) ([]net.Listener, error) {
	ll, err := Ports()
	if err != nil {
		return nil, err
	}
	return ll.ListenAll(ctx)
}

// ListenPacketAll parses SERVER_STARTER_PORT and creates net.PacketConn objects.
func ListenPacketAll(ctx context.Context) ([]net.PacketConn, error) {
	ll, err := Ports()
	if err != nil {
		return nil, err
	}
	return ll.ListenPacketAll(ctx)
}
