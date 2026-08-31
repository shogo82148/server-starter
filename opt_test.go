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
			t.Errorf("want --version, got %s", s.Command)
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

	t.Run("AutoRestartInterval", func(t *testing.T) {
		s, err := ParseArgs([]string{"start_server", "--auto-restart-interval", "1234"})
		if err != nil {
			t.Fatal(err)
		}
		if s.AutoRestartInterval != 1234*time.Second {
			t.Errorf("want 1234s, got %s", s.AutoRestartInterval)
		}
	})
}

func TestParseDuration(t *testing.T) {
	testCases := []struct {
		input    string
		expected time.Duration
	}{
		{"1", 1 * time.Second},
		{"1.5", 1500 * time.Millisecond},
		{"2s", 2 * time.Second},
		{"3m", 3 * time.Minute},
		{"4h", 4 * time.Hour},
		{"12h34m", 12*time.Hour + 34*time.Minute},
		{"9223372036", 9223372036 * time.Second},
		{"9223372036.854775807", 9223372036*time.Second + 854775807*time.Nanosecond},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			d, err := parseDuration(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, d)
			}
		})
	}

	t.Run("too large float value", func(t *testing.T) {
		_, err := parseDuration("9223372037")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("too large duration", func(t *testing.T) {
		_, err := parseDuration("9223372037s")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("nan", func(t *testing.T) {
		_, err := parseDuration("nan")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("inf", func(t *testing.T) {
		_, err := parseDuration("inf")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("negative float value", func(t *testing.T) {
		_, err := parseDuration("-1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("negative duration", func(t *testing.T) {
		_, err := parseDuration("-1s")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := parseDuration("invalid")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
