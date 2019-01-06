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

// PortEnvName is the environment name for server_starter configures.
// copied from the starter package.
const PortEnvName = "SERVER_STARTER_PORT"

// ErrNoListeningTarget is returned by ListenAll calls
// when the process is not started using server_starter.
var ErrNoListeningTarget = errors.New("listener: no listening target")

// ListenConfig is the interface for things that listen on file descriptors
// specified by Start::Server / server_starter
type ListenConfig interface {
	// Fd returns the underlying file descriptor
	Fd() uintptr

	// Listen creates a new Listener
	Listen() (net.Listener, error)

	// Addr returns the address.
	Addr() string

	// return a string compatible with SERVER_STARTER_PORT
	String() string
}

// ListenConfigs holds a list of ListenConfig. This is here just for convenience
// so that you can do
//	list.String()
// to get a string compatible with SERVER_STARTER_PORT
type ListenConfigs []ListenConfig

func (ll ListenConfigs) String() string {
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
func (ll ListenConfigs) Listen(ctx context.Context, network, address string) (net.Listener, error) {
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
			if network == "tcp4" && ip.IP.To4() == nil {
				continue
			}
			if network == "tcp6" && ip.IP.To4() != nil {
				continue
			}
			addrlist = append(addrlist, net.JoinHostPort(ip.String(), port))
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
func (ll ListenConfigs) ListenAll(ctx context.Context) ([]net.Listener, error) {
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

type listenConfig struct {
	addr string
	fd   uintptr
}

func (l listenConfig) Addr() string {
	return l.addr
}

func (l listenConfig) String() string {
	return fmt.Sprintf("%s=%d", l.addr, l.fd)
}

func (l listenConfig) Fd() uintptr {
	return l.fd
}

func (l listenConfig) Listen() (net.Listener, error) {
	return net.FileListener(os.NewFile(l.fd, l.addr))
}

func parseListenTargets(str string, ok bool) (ListenConfigs, error) {
	if !ok {
		return nil, ErrNoListeningTarget
	}
	if str == "" {
		return []ListenConfig{}, nil
	}

	rawspec := strings.Split(str, ";")
	ret := make([]ListenConfig, len(rawspec))

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
		ret[i] = listenConfig{
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

// Ports parses environment variable SERVER_STARTER_PORT
func Ports() (ListenConfigs, error) {
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
