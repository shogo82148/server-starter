package starter

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

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
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	return "", scanner.Err()
}
