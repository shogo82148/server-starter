package starter

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func loadEnv(dir string) []string {
	env := []string{}

	// ignore errors
	if dir == "" {
		return env
	}
	stat, err := os.Stat(dir)
	if err != nil || !stat.IsDir() {
		return env
	}

	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		// ignore errors
		if err != nil {
			return nil
		}

		// skip sub directories.
		if fi.IsDir() && !os.SameFile(stat, fi) {
			return filepath.SkipDir
		}

		// skip dotfiles
		if strings.HasPrefix(fi.Name(), ".") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		name := filepath.Base(path)
		scanner := bufio.NewScanner(f)
		if scanner.Scan() {
			value := scanner.Text()
			env = append(env, name+"="+value)
		}
		return nil
	})

	return env
}
