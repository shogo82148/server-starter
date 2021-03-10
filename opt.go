package starter

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type errorList []error

func (l errorList) Error() string {
	msg := make([]string, len(l))
	for i, e := range l {
		msg[i] = e.Error()
	}
	return strings.Join(msg, ", ")
}

// ParseArgs parses command line arguments,
// and return configured Starter.
func ParseArgs(args []string) (*Starter, error) {
	var errs errorList
	var err error
	s := &Starter{}
	var killOldDelay, autoRestartInterval string

	// read from the environment value.
	s.EnvDir = os.Getenv("ENVDIR")
	s.EnableAutoRestart = os.Getenv("ENABLE_AUTO_RESTART") != "" && os.Getenv("ENABLE_AUTO_RESTART") != "0"
	killOldDelay = os.Getenv("KILL_OLD_DELAY")
	autoRestartInterval = os.Getenv("AUTO_RESTART_INTERVAL")

	// parse args
	var cmd []string
	for i := 1; i < len(args); i++ {
		// the end of options.
		if args[i] == "--" {
			cmd = args[i+1:]
			break
		}
		if !strings.HasPrefix(args[i], "--") {
			cmd = args[i:]
			break
		}

		// parse boolean options.
		parsed := true
		switch args[i] {
		case "--enable-auto-restart":
			s.EnableAutoRestart = true
		case "--daemonize":
			s.Daemonize = true
		case "--restart":
			s.Restart = true
		case "--stop":
			s.Stop = true
		case "--help":
			s.Help = true
		case "--version":
			s.Version = true
		default:
			parsed = false
		}
		if parsed {
			continue
		}

		// parse options with values.
		var opt, value string
		if idx := strings.IndexByte(args[i], '='); idx >= 0 {
			opt = args[i][:idx]
			value = args[i][idx+1:]
		} else {
			opt = args[i]
			i++
			if i >= len(args) {
				errs = append(errs, fmt.Errorf("missing the value for option %s", opt))
				break
			}
			value = args[i]
		}
		switch opt {
		case "--port":
			s.Ports = append(s.Ports, value)
		case "--path":
			s.Paths = append(s.Paths, value)
		case "--interval":
			s.Interval, err = parseDuration(value)
			if err != nil {
				errs = append(errs, fmt.Errorf("invalid --interval format: %s", value))
			}
		case "--log-file":
			s.LogFile = value
		case "--pid-file":
			s.PidFile = value
		case "--dir":
			s.Dir = value
		case "--signal-on-hup":
			if signal := nameToSignal(value); signal != nil {
				s.SignalOnHUP = signal
			} else {
				errs = append(errs, fmt.Errorf("unknown signal name for --signal-on-hup: %s", value))
			}
		case "--signal-on-term":
			if signal := nameToSignal(value); signal != nil {
				s.SignalOnTERM = signal
			} else {
				errs = append(errs, fmt.Errorf("unknown signal name for --signal-on-term: %s", value))
			}
		case "--backlog":
			errs = append(errs, errors.New("--backlog is not supported"))
		case "--envdir":
			s.EnvDir = value
		case "--auto-restart-interval":
			autoRestartInterval = value
		case "--kill-old-delay":
			killOldDelay = value
		case "--status-file":
			s.StatusFile = value
		default:
			errs = append(errs, fmt.Errorf("unknown option %s", opt))
		}
	}
	if len(cmd) > 0 {
		s.Command = cmd[0]
		s.Args = cmd[1:]
	}

	if killOldDelay != "" {
		s.KillOldDelay, err = parseDuration(killOldDelay)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid --kill-old-delay format: %s", killOldDelay))
		}
	}
	if autoRestartInterval != "" {
		s.AutoRestartInterval, err = parseDuration(autoRestartInterval)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid --auto-restart-interval format: %s", autoRestartInterval))
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return s, nil
}

func parseDuration(s string) (time.Duration, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err == nil {
		return time.Duration(v * float64(time.Second)), nil
	}
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	return 0, fmt.Errorf("invalid format: %s", s)
}
