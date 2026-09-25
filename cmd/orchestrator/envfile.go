package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// envFile is where the orchestrator's settings live: the Raenil URL and
// token, models, repo routing. Read on every start, so `orchestrator` works
// from any directory without sourcing anything first.
func envFile() string {
	if p := os.Getenv("ORCHESTRATOR_ENV"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "raenil", "orchestrator.env")
}

// loadEnvFile sets each KEY=VALUE in the file that the environment does not
// already set — a variable given on the command line wins. Blank lines and
// # comments are skipped; `export ` and surrounding quotes are allowed. A
// missing file is not an error.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' || val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
		if _, set := os.LookupEnv(key); set || key == "" {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			return err
		}
	}
	return sc.Err()
}
