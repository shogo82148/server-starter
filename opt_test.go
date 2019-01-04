package starter

import (
	"reflect"
	"testing"
	"time"
)

func TestParseArgs(t *testing.T) {
	t.Run("no args", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server"})
		if err != nil {
			t.Error(err)
		}
		if s.Command != "" {
			t.Errorf("want empty, got %s", s.Command)
		}
	})

	t.Run("program name only", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server", "server"})
		if err != nil {
			t.Error(err)
		}
		if s.Command != "server" {
			t.Errorf("want server, got %s", s.Command)
		}
		if len(s.Args) > 0 {
			t.Errorf("want 0, got %d", len(s.Args))
		}
	})

	t.Run("program name with option", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server", "server", "--some-options", "foo", "bar"})
		if err != nil {
			t.Error(err)
		}
		if s.Command != "server" {
			t.Errorf("want server, got %s", s.Command)
		}
		if !reflect.DeepEqual(s.Args, []string{"--some-options", "foo", "bar"}) {
			t.Errorf("want --some-options foo bar, got %#v", s.Args)
		}
	})

	t.Run("boolean option", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server", "--version"})
		if err != nil {
			t.Error(err)
		}
		if !s.Version {
			t.Error("want true, got false")
		}
	})

	t.Run("the options terminator", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server", "--", "--version"})
		if err != nil {
			t.Error(err)
		}
		if s.Command != "--version" {
			t.Errorf("want --versionr, got %s", s.Command)
		}
		if s.Version {
			t.Error("want false, got true")
		}
	})

	t.Run("string option", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server", "--status-file", "/tmp/foo/bar"})
		if err != nil {
			t.Error(err)
		}
		if s.StatusFile != "/tmp/foo/bar" {
			t.Errorf("want /tmp/foo/bar, got %s", s.StatusFile)
		}
	})

	t.Run("gnu_compat", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server", "--status-file=/tmp/foo/bar"})
		if err != nil {
			t.Error(err)
		}
		if s.StatusFile != "/tmp/foo/bar" {
			t.Errorf("want /tmp/foo/bar, got %s", s.StatusFile)
		}
	})

	t.Run("seconds", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server", "--interval", "1234"})
		if err != nil {
			t.Error(err)
		}
		if s.Interval != 1234*time.Second {
			t.Errorf("want 1234s, got %s", s.Interval)
		}
	})

	t.Run("go-style duration", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server", "--interval", "12h34m"})
		if err != nil {
			t.Error(err)
		}
		if s.Interval != 12*time.Hour+34*time.Minute {
			t.Errorf("want 12h34m, got %s", s.Interval)
		}
	})

	t.Run("slice", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server", "--port", "1234", "--port", "2345"})
		if err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(s.Ports, []string{"1234", "2345"}) {
			t.Errorf("want 1234,2345, got %#v", s.Ports)
		}
	})
}
