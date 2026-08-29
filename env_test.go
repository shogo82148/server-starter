package starter

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnv(t *testing.T) {
	t.Run("normal case", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "FOO"), []byte("foo"), 0644); err != nil {
			t.Fatalf("failed to write env file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "BAR"), []byte("bar"), 0644); err != nil {
			t.Fatalf("failed to write env file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "BAZ"), []byte("baz"), 0644); err != nil {
			t.Fatalf("failed to write env file: %v", err)
		}

		got, err := loadEnv(dir)
		if err != nil {
			t.Fatalf("loadEnv() returned error: %v", err)
		}
		want := map[string]string{
			"FOO": "foo",
			"BAR": "bar",
			"BAZ": "baz",
		}
		if !maps.Equal(got, want) {
			t.Errorf("loadEnv() = %v, want %v", got, want)
		}
	})

	t.Run("skip dotfiles and directories", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".FOO"), []byte("foo"), 0644); err != nil {
			t.Fatalf("failed to write env file: %v", err)
		}
		if err := os.Mkdir(filepath.Join(dir, "BAR"), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "BAZ"), []byte("baz"), 0644); err != nil {
			t.Fatalf("failed to write env file: %v", err)
		}

		got, err := loadEnv(dir)
		if err != nil {
			t.Fatalf("loadEnv() returned error: %v", err)
		}
		want := map[string]string{
			"BAZ": "baz",
		}
		if !maps.Equal(got, want) {
			t.Errorf("loadEnv() = %v, want %v", got, want)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		got, err := loadEnv(dir)
		if err != nil {
			t.Fatalf("loadEnv() returned error: %v", err)
		}
		want := map[string]string{}
		if !maps.Equal(got, want) {
			t.Errorf("loadEnv() = %v, want %v", got, want)
		}
	})

	t.Run("maximum size", func(t *testing.T) {
		dir := t.TempDir()
		largeValue := bytes.Repeat([]byte("A"), maxEnvValueBytes)
		if err := os.WriteFile(filepath.Join(dir, "FOO"), largeValue, 0644); err != nil {
			t.Fatalf("failed to write env file: %v", err)
		}

		got, err := loadEnv(dir)
		if err != nil {
			t.Fatalf("loadEnv() returned error: %v", err)
		}
		want := map[string]string{
			"FOO": string(largeValue),
		}
		if !maps.Equal(got, want) {
			t.Errorf("loadEnv() = %v, want %v", got, want)
		}
	})

	t.Run("env value exceeds maximum size", func(t *testing.T) {
		dir := t.TempDir()
		largeValue := bytes.Repeat([]byte("A"), maxEnvValueBytes+1)
		if err := os.WriteFile(filepath.Join(dir, "FOO"), largeValue, 0644); err != nil {
			t.Fatalf("failed to write env file: %v", err)
		}

		_, err := loadEnv(dir)
		if err == nil {
			t.Fatalf("loadEnv() did not return error for large env value")
		}
	})
}
