package syncer

import (
	"bufio"
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

func LoadTemplateVars(basePath, hostName, projectName string) (map[string]any, error) {
	hostProjectDir := filepath.Join(basePath, "hosts", hostName, projectName)
	vars := make(map[string]any)

	envPath := filepath.Join(hostProjectDir, ".env")

	envVars, err := parseEnvFile(envPath)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", envPath, err)
	}

	maps.Copy(vars, envVars)

	secretsPath := filepath.Join(hostProjectDir, "env.secrets.yml")

	secretVars, err := parseSecretsYAML(secretsPath)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", secretsPath, err)
	}

	maps.Copy(vars, secretVars)

	return vars, nil
}

func parseEnvFile(path string) (map[string]any, error) {
	vars := make(map[string]any)
	cleanPath := filepath.Clean(path)

	file, err := os.Open(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return vars, nil
		}

		return nil, err
	}

	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		value = trimSurroundingQuotes(value)

		vars[key] = value
	}

	scanErr := scanner.Err()
	if scanErr != nil {
		return nil, scanErr
	}

	return vars, nil
}

func trimSurroundingQuotes(value string) string {
	const minQuotedLength = 2
	if len(value) < minQuotedLength {
		return value
	}

	firstChar := value[0]
	lastChar := value[len(value)-1]

	if (firstChar == '"' && lastChar == '"') || (firstChar == '\'' && lastChar == '\'') {
		return value[1 : len(value)-1]
	}

	return value
}

func parseSecretsYAML(path string) (map[string]any, error) {
	vars := make(map[string]any)
	cleanPath := filepath.Clean(path)

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return vars, nil
		}

		return nil, err
	}

	raw := make(map[string]any)

	unmarshalErr := yaml.Unmarshal(data, &raw)
	if unmarshalErr != nil {
		return nil, fmt.Errorf("invalid YAML: %w", unmarshalErr)
	}

	maps.Copy(vars, raw)

	return vars, nil
}

func RenderTemplate(data []byte, vars map[string]any) ([]byte, error) {
	if isBinary(data) {
		return data, nil
	}

	if len(vars) == 0 {
		return data, nil
	}

	tmpl, err := template.New("").Option("missingkey=error").Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer

	executeErr := tmpl.Execute(&buf, vars)
	if executeErr != nil {
		return nil, fmt.Errorf("template render error: %w", executeErr)
	}

	return buf.Bytes(), nil
}
