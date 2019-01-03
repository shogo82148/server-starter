package starter

import "testing"

func TestRun(t *testing.T) {
	sd := &Starter{
		Command: "echo",
	}
	if err := sd.Run(); err != nil {
		t.Errorf("sd.Run() failed: %s", err)
	}
}
