package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadDotEnv parses a .env file into a map of KEY=VALUE pairs per ADR-0007
// §D5: whole-line "#" comments, optional surrounding quotes, no "export"
// prefix and no variable expansion. A missing file yields an empty map and a
// nil error — the .env tier is optional, not required.
func LoadDotEnv(path string) (map[string]string, error) {
	// The path is a fixed developer-controlled config location (env var or
	// default), never user input — G304 does not apply.
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("open dotenv file: %w", err)
	}
	defer func() { _ = f.Close() }()

	vars := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue // not KEY=VALUE; ignore the line
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" {
			vars[key] = value
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan dotenv file: %w", err)
	}
	return vars, nil
}
