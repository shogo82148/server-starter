package listener

import (
	"reflect"
	"testing"
)

func TestPort(t *testing.T) {
	caces := []struct {
		in string
		ll []listenConfig
	}{
		{
			in: "0.0.0.0:80=3",
			ll: []listenConfig{
				{
					addr: "0.0.0.0:80",
					fd:   3,
				},
			},
		},
		{
			in: "0.0.0.0:80=3;/tmp/foo.sock=4",
			ll: []listenConfig{
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
			ll: []listenConfig{
				{
					addr: "50908",
					fd:   4,
				},
			},
		},
		{
			in: "",
			ll: []listenConfig{},
		},
	}

	for i, tc := range caces {
		ll, err := parseListenTargets(tc.in, true)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(ll) != len(tc.ll) {
			t.Errorf("#%d: want %d, got %d", i, len(tc.ll), len(ll))
		}
		for i, l := range ll {
			l := l.(listenConfig)
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
