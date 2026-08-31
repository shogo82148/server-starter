package starter

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvDirEnvName is the environment variable name for EnvDir.
const EnvDirEnvName = "ENVDIR"

// EnableAutoRestartEnvName is the environment variable name for EnableAutoRestart.
const EnableAutoRestartEnvName = "ENABLE_AUTO_RESTART"

// KillOldDelayEnvName is the environment variable name for KillOldDelay.
const KillOldDelayEnvName = "KILL_OLD_DELAY"

// AutoRestartIntervalEnvName is the environment variable name for AutoRestartInterval.
const AutoRestartIntervalEnvName = "AUTO_RESTART_INTERVAL"

// defaultAutoRestartInterval is the default interval for auto-restart when EnableAutoRestart is set to true.
const defaultAutoRestartInterval = time.Hour

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
	s.EnvDir = os.Getenv(EnvDirEnvName)
	enableAutoRestart, err := parseBool(os.Getenv(EnableAutoRestartEnvName))
	if err != nil {
		errs = append(errs, fmt.Errorf("invalid %s format: %s", EnableAutoRestartEnvName, os.Getenv(EnableAutoRestartEnvName)))
	}
	s.EnableAutoRestart = enableAutoRestart
	killOldDelay = os.Getenv(KillOldDelayEnvName)
	autoRestartInterval = os.Getenv(AutoRestartIntervalEnvName)

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
		opt, value, found := strings.Cut(args[i], "=")
		if !found {
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
		d, err := parseDuration(killOldDelay)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid --kill-old-delay format: %s", killOldDelay))
		} else {
			s.KillOldDelay = &d
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

func parseBool(s string) (bool, error) {
	if s == "" {
		return false, nil
	}
	return strconv.ParseBool(s)
}

func formatBool(b bool) string {
	if b {
		return "1"
	}
	return ""
}

func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err == nil {
		if d < 0 {
			return 0, fmt.Errorf("duration must be non-negative: %q", s)
		}
		return d, nil
	}
	for _, ch := range s {
		if (ch < '0' || ch > '9') && ch != '.' {
			return 0, fmt.Errorf("invalid format: %q", s)
		}
	}
	d, err = time.ParseDuration(s + "s")
	if err == nil {
		if d < 0 {
			return 0, fmt.Errorf("duration must be non-negative: %q", s)
		}
		return d, nil
	}
	return 0, fmt.Errorf("invalid format: %q", s)
}

func formatDuration(d time.Duration) string {
	sec, nsec := int64(d/time.Second), int64(d%time.Second)
	if nsec == 0 {
		return strconv.FormatInt(sec, 10)
	}
	s := fmt.Sprintf("%d.%09d", sec, nsec)
	s = strings.TrimRight(s, "0")
	return s
}
