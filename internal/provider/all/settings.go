package all

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/ginkcode/agent-sessions/internal/configfile"
)

func configPath(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("agents: empty config directory")
	}
	return filepath.Join(dir, "config.toml"), nil
}

// LoadEnabled defaults to all agents only when the selection is absent.
func LoadEnabled(dir string) ([]string, error) {
	if dir == "" {
		return IDs(), nil
	}
	path, _ := configPath(dir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return IDs(), nil
	}
	if err != nil {
		return nil, err
	}
	return decodeEnabled(data)
}

func decodeEnabled(data []byte) ([]string, error) {
	var cfg struct {
		Agents struct {
			Enabled []string `toml:"enabled"`
		} `toml:"agents"`
	}
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, fmt.Errorf("agents: parse config: %w", err)
	}
	if !md.IsDefined("agents", "enabled") {
		return IDs(), nil
	}
	return Canonical(cfg.Agents.Enabled)
}

// SaveEnabled shares the config-file lock with all other settings writers.
func SaveEnabled(dir string, ids []string) ([]string, error) {
	ids, err := Canonical(ids)
	if err != nil {
		return nil, err
	}
	path, err := configPath(dir)
	if err != nil {
		return nil, err
	}
	lock, err := configfile.Lock(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = lock.Unlock() }()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	// Validate before editing; never silently rewrite malformed configuration.
	var cfg map[string]any
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(map[string]any{"enabled": ids}); err != nil {
		return nil, err
	}
	replacement := strings.TrimSuffix(encoded.String(), "\n")
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	out := make([]string, 0, len(lines)+3)
	inAgents, seen, written := false, false, false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			if inAgents && !written {
				out = append(out, replacement)
				written = true
			}
			header := strings.TrimSpace(strings.SplitN(trim, "#", 2)[0])
			inAgents = header == "[agents]"
			seen = seen || inAgents
		}
		if inAgents {
			parts := strings.SplitN(trim, "=", 2)
			if len(parts) == 2 && strings.Trim(strings.TrimSpace(parts[0]), "\"'") == "enabled" {
				// A TOML array may span lines. Keep consuming until the assignment parses.
				assignment := line
				for {
					var value map[string]any
					if _, e := toml.Decode(assignment, &value); e == nil {
						break
					}
					i++
					if i >= len(lines) {
						return nil, fmt.Errorf("agents: invalid enabled assignment")
					}
					assignment += "\n" + lines[i]
				}
				if !written {
					out = append(out, replacement)
					written = true
				}
				continue
			}
		}
		out = append(out, line)
	}
	if !seen {
		out = append(out, "", "[agents]")
	}
	if !written {
		out = append(out, replacement)
	}
	updated := []byte(strings.Join(out, "\n") + "\n")
	if _, err := decodeEnabled(updated); err != nil {
		return nil, err
	}
	if err := configfile.WriteAtomic(path, updated); err != nil {
		return nil, err
	}
	return ids, nil
}
