package starter

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxEnvValueBytes = 128 * 1024 // 128KiB

func loadEnv(dir string) (map[string]string, error) {
	if dir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	env := make(map[string]string, len(entries))
	for _, entry := range entries {
		// skip dotfiles and directories
		name := entry.Name()
		if strings.HasPrefix(name, ".") || entry.Type()&os.ModeType != 0 {
			continue
		}

		// read the value from the file
		val, err := readEnvFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		env[name] = val
	}
	return env, nil
}

func readEnvFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck // ignore error on cleanup

	data, err := io.ReadAll(io.LimitReader(f, maxEnvValueBytes+1))
	if err != nil {
		return "", err
	}
	if line, _, ok := bytes.Cut(data, []byte{'\n'}); ok {
		return string(line), nil
	}
	if len(data) > maxEnvValueBytes {
		return "", errors.New("env value exceeds maximum size of 128KiB")
	}
	return string(data), nil
}
