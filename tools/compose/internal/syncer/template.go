package syncer

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Template variable loading
// ---------------------------------------------------------------------------

// LoadTemplateVars loads template variables from the host project directory.
// It reads .env (KEY=VALUE format) and env.secrets.yml (flat YAML key-value),
// merging them with env.secrets.yml values taking priority.
func LoadTemplateVars(basePath, hostName, projectName string) (map[string]interface{}, error) {
	hostProjectDir := filepath.Join(basePath, "hosts", hostName, projectName)
	vars := make(map[string]interface{})

	// Layer 1: .env (lower priority)
	envPath := filepath.Join(hostProjectDir, ".env")
	envVars, err := parseEnvFile(envPath)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", envPath, err)
	}
	for k, v := range envVars {
		vars[k] = v
	}

	// Layer 2: env.secrets.yml (higher priority, overrides .env)
	secretsPath := filepath.Join(hostProjectDir, "env.secrets.yml")
	secretVars, err := parseSecretsYAML(secretsPath)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", secretsPath, err)
	}
	for k, v := range secretVars {
		vars[k] = v
	}

	return vars, nil
}

// parseEnvFile reads a file in KEY=VALUE format (one per line).
// Lines starting with # and empty lines are ignored.
// Returns an empty map (not an error) if the file does not exist.
func parseEnvFile(path string) (map[string]interface{}, error) {
	vars := make(map[string]interface{})

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return vars, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		// Strip surrounding quotes from the value.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		vars[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return vars, nil
}

// parseSecretsYAML reads a flat YAML key-value file.
// Returns an empty map (not an error) if the file does not exist.
func parseSecretsYAML(path string) (map[string]interface{}, error) {
	vars := make(map[string]interface{})

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return vars, nil
		}
		return nil, err
	}

	raw := make(map[string]interface{})
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	for k, v := range raw {
		vars[k] = v
	}

	return vars, nil
}

// ---------------------------------------------------------------------------
// Template rendering
// ---------------------------------------------------------------------------

// RenderTemplate processes data as a Go text/template with the given variables.
// Binary data (containing null bytes) is returned as-is without processing.
func RenderTemplate(data []byte, vars map[string]interface{}) ([]byte, error) {
	// Skip binary files.
	if isBinary(data) {
		return data, nil
	}

	// Skip if no template variables are available.
	if len(vars) == 0 {
		return data, nil
	}

	tmpl, err := template.New("").Option("missingkey=error").Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("template render error: %w", err)
	}

	return buf.Bytes(), nil
}
