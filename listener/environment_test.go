package listener

import (
	"os"
	"testing"
)

// unsetEnv unsets the environment variable with the given key and restores it after the test.
func unsetEnv(t *testing.T, key string) {
	t.Helper()

	prev, ok := os.LookupEnv(key)
	if !ok {
		return
	}

	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("failed to unset env: %v", err)
	}

	t.Cleanup(func() {
		if err := os.Setenv(key, prev); err != nil {
			t.Fatalf("failed to restore env: %v", err)
		}
	})
}

func TestIsUnderStartServer(t *testing.T) {
	t.Run("not under start_server", func(t *testing.T) {
		unsetEnv(t, GenerationEnvName)
		if IsUnderStartServer() {
			t.Errorf("IsUnderStartServer() = true, want false")
		}
	})

	t.Run("under start_server", func(t *testing.T) {
		t.Setenv(GenerationEnvName, "1")
		if !IsUnderStartServer() {
			t.Errorf("IsUnderStartServer() = false, want true")
		}
	})
}

func TestGeneration(t *testing.T) {
	t.Run("not under start_server", func(t *testing.T) {
		unsetEnv(t, GenerationEnvName)
		if gen, ok := Generation(); ok || gen != 0 {
			t.Errorf("Generation() = %d, %v, want 0, false", gen, ok)
		}
	})

	t.Run("under start_server", func(t *testing.T) {
		t.Setenv(GenerationEnvName, "1")
		if gen, ok := Generation(); !ok || gen != 1 {
			t.Errorf("Generation() = %d, %v, want 1, true", gen, ok)
		}
	})

	t.Run("generation 0 is acceptable", func(t *testing.T) {
		t.Setenv(GenerationEnvName, "0")
		if gen, ok := Generation(); !ok || gen != 0 {
			t.Errorf("Generation() = %d, %v, want 0, true", gen, ok)
		}
	})

	t.Run("invalid generation", func(t *testing.T) {
		t.Setenv(GenerationEnvName, "invalid")
		if gen, ok := Generation(); ok || gen != 0 {
			t.Errorf("Generation() = %d, %v, want 0, false", gen, ok)
		}
	})

	t.Run("negative generation", func(t *testing.T) {
		t.Setenv(GenerationEnvName, "-1")
		if gen, ok := Generation(); ok || gen != 0 {
			t.Errorf("Generation() = %d, %v, want 0, false", gen, ok)
		}
	})
}
