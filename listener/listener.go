package listener

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
)

// ListenConfig is a generator of net.Listener.
type ListenConfig interface {
	Listen(ctx context.Context, network, address string) (net.Listener, error)
}

// PortEnvName is the environment name for server_starter configures.
// copied from the starter package.
const PortEnvName = "SERVER_STARTER_PORT"

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

	// Addr returns the address.
	Addr() string

	// return a string compatible with SERVER_STARTER_PORT
	String() string
}

// ListenSpecs holds a list of ListenConfig. This is here just for convenience
// so that you can do
//	list.String()
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
// The network must be "tcp", "tcp4", "tcp6", "unix" or "unixpacket".
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
			if v4 == nil && v6 != nil && (network == "tcp" || network == "tcp6") {
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
		addrlist = []string{address}
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

// ListenAll announces on the local network address.
func (ll ListenSpecs) ListenAll(ctx context.Context) ([]net.Listener, error) {
	ret := make([]net.Listener, 0, len(ll))
	for _, lc := range ll {
		l, err := lc.Listen()
		if err != nil {
			for _, l := range ret {
				l.Close()
			}
			return nil, err
		}
		ret = append(ret, l)
	}
	return ret, nil
}

type listenSpec struct {
	addr string
	fd   uintptr
}

func (l listenSpec) Addr() string {
	return l.addr
}

func (l listenSpec) String() string {
	return fmt.Sprintf("%s=%d", l.addr, l.fd)
}

func (l listenSpec) Fd() uintptr {
	return l.fd
}

func (l listenSpec) Listen() (net.Listener, error) {
	return net.FileListener(os.NewFile(l.fd, l.addr))
}

func parseListenTargets(str string, ok bool) (ListenSpecs, error) {
	if !ok {
		return nil, ErrNoListeningTarget
	}
	if str == "" {
		return []ListenSpec{}, nil
	}

	rawspec := strings.Split(str, ";")
	ret := make([]ListenSpec, len(rawspec))

	for i, pairString := range rawspec {
		pair := strings.SplitN(pairString, "=", 2)
		if len(pair) != 2 {
			return nil, fmt.Errorf("failed to parse '%s' as listen target", pairString)
		}
		addr := strings.TrimSpace(pair[0])
		fdString := strings.TrimSpace(pair[1])
		fd, err := strconv.ParseUint(fdString, 10, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to parse '%s' as listen target: %s", pairString, err)
		}
		ret[i] = listenSpec{
			addr: addr,
			fd:   uintptr(fd),
		}
	}

	return ret, nil
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
	ll, err := parseListenTargets(PortsSpecification())
	if err != nil {
		return nil, err
	}

	// emulate Perl's hash randomization
	// to eeproduce the original behavior of https://metacpan.org/pod/Server::Starter#server_ports
	rand.Shuffle(len(ll), func(i, j int) {
		ll[i], ll[j] = ll[j], ll[i]
	})

	return ll, nil
}

// PortsFallback returns the same result as Ports, if SERVER_STARTER_PORT is defined.
// Otherwise returns net.ListenConfig instead of ListenSpecs.
// Regardless of whether the process starts from the start_server command or not,
// you can call Listen method.
//
//  lc, err := listener.PortsFallback()
//  l, err := lc.Listen(ctx, "tcp", ":8080")
func PortsFallback() (ListenConfig, error) {
	ll, err := parseListenTargets(PortsSpecification())
	if err == nil {
		// emulate Perl's hash randomization
		// to eeproduce the original behavior of https://metacpan.org/pod/Server::Starter#server_ports
		rand.Shuffle(len(ll), func(i, j int) {
			ll[i], ll[j] = ll[j], ll[i]
		})

		return ll, nil
	}
	if err != ErrNoListeningTarget {
		return nil, err
	}
	return &net.ListenConfig{}, nil
}
