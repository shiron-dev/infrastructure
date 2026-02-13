package syncer

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// parseEnvFile
// ---------------------------------------------------------------------------

func TestParseEnvFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	os.WriteFile(p, []byte(`# comment
KEY1=value1
KEY2=value2
QUOTED="hello world"
SINGLE_QUOTED='foo bar'
  SPACED = spaced_val  

EMPTY=
`), 0644)

	vars, err := parseEnvFile(p)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"KEY1":          "value1",
		"KEY2":          "value2",
		"QUOTED":        "hello world",
		"SINGLE_QUOTED": "foo bar",
		"SPACED":        "spaced_val",
		"EMPTY":         "",
	}

	for k, want := range tests {
		got, ok := vars[k]
		if !ok {
			t.Errorf("missing key %q", k)

			continue
		}

		if got != want {
			t.Errorf("key %q = %q, want %q", k, got, want)
		}
	}
}

func TestParseEnvFile_NotExist(t *testing.T) {
	t.Parallel()

	vars, err := parseEnvFile("/nonexistent/.env")
	if err != nil {
		t.Fatal(err)
	}

	if len(vars) != 0 {
		t.Errorf("expected empty map, got %v", vars)
	}
}

// ---------------------------------------------------------------------------
// parseSecretsYAML
// ---------------------------------------------------------------------------

func TestParseSecretsYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, "env.secrets.yml")
	os.WriteFile(p, []byte(`github_client_id: abc123
github_client_secret: secret456
smtp_port: 587
`), 0644)

	vars, err := parseSecretsYAML(p)
	if err != nil {
		t.Fatal(err)
	}

	if vars["github_client_id"] != "abc123" {
		t.Errorf("github_client_id = %v", vars["github_client_id"])
	}

	if vars["github_client_secret"] != "secret456" {
		t.Errorf("github_client_secret = %v", vars["github_client_secret"])
	}
	// YAML integers are parsed as int.
	if vars["smtp_port"] != 587 {
		t.Errorf("smtp_port = %v (%T)", vars["smtp_port"], vars["smtp_port"])
	}
}

func TestParseSecretsYAML_NotExist(t *testing.T) {
	t.Parallel()

	vars, err := parseSecretsYAML("/nonexistent/env.secrets.yml")
	if err != nil {
		t.Fatal(err)
	}

	if len(vars) != 0 {
		t.Errorf("expected empty map, got %v", vars)
	}
}

func TestParseSecretsYAML_InvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, "env.secrets.yml")
	os.WriteFile(p, []byte(`{invalid: yaml: [}`), 0644)

	_, err := parseSecretsYAML(p)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// ---------------------------------------------------------------------------
// LoadTemplateVars
// ---------------------------------------------------------------------------

func TestLoadTemplateVars_SecretsOverrideEnv(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	hostProjectDir := filepath.Join(base, "hosts", "server1", "grafana")
	os.MkdirAll(hostProjectDir, 0755)

	// .env has KEY1 and SHARED.
	os.WriteFile(filepath.Join(hostProjectDir, ".env"), []byte(`KEY1=from_env
SHARED=env_value
`), 0644)

	// env.secrets.yml has KEY2 and overrides SHARED.
	os.WriteFile(filepath.Join(hostProjectDir, "env.secrets.yml"), []byte(`KEY2: from_secrets
SHARED: secrets_value
`), 0644)

	vars, err := LoadTemplateVars(base, "server1", "grafana")
	if err != nil {
		t.Fatal(err)
	}

	if vars["KEY1"] != "from_env" {
		t.Errorf("KEY1 = %v", vars["KEY1"])
	}

	if vars["KEY2"] != "from_secrets" {
		t.Errorf("KEY2 = %v", vars["KEY2"])
	}
	// SHARED should come from env.secrets.yml.
	if vars["SHARED"] != "secrets_value" {
		t.Errorf("SHARED = %v, want secrets_value", vars["SHARED"])
	}
}

func TestLoadTemplateVars_NoFiles(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, "hosts", "server1", "grafana"), 0755)

	vars, err := LoadTemplateVars(base, "server1", "grafana")
	if err != nil {
		t.Fatal(err)
	}

	if len(vars) != 0 {
		t.Errorf("expected empty vars, got %v", vars)
	}
}

func TestLoadTemplateVars_OnlySecrets(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	hostProjectDir := filepath.Join(base, "hosts", "server1", "grafana")
	os.MkdirAll(hostProjectDir, 0755)

	os.WriteFile(filepath.Join(hostProjectDir, "env.secrets.yml"), []byte(`secret_key: s3cret
`), 0644)

	vars, err := LoadTemplateVars(base, "server1", "grafana")
	if err != nil {
		t.Fatal(err)
	}

	if vars["secret_key"] != "s3cret" {
		t.Errorf("secret_key = %v", vars["secret_key"])
	}
}

// ---------------------------------------------------------------------------
// RenderTemplate
// ---------------------------------------------------------------------------

func TestRenderTemplate(t *testing.T) {
	t.Parallel()

	data := []byte(`host = {{ .smtp_host }}
password = {{ .smtp_password }}`)

	vars := map[string]any{
		"smtp_host":     "mail.example.com:587",
		"smtp_password": "s3cret",
	}

	result, err := RenderTemplate(data, vars)
	if err != nil {
		t.Fatal(err)
	}

	expected := `host = mail.example.com:587
password = s3cret`
	if string(result) != expected {
		t.Errorf("got %q, want %q", string(result), expected)
	}
}

func TestRenderTemplate_NoVars(t *testing.T) {
	t.Parallel()

	data := []byte("plain text with no {{ templates }}")

	result, err := RenderTemplate(data, nil)
	if err != nil {
		t.Fatal(err)
	}

	if string(result) != string(data) {
		t.Errorf("expected unchanged data, got %q", string(result))
	}
}

func TestRenderTemplate_EmptyVars(t *testing.T) {
	t.Parallel()

	data := []byte("plain text")

	result, err := RenderTemplate(data, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	if string(result) != string(data) {
		t.Errorf("expected unchanged data, got %q", string(result))
	}
}

func TestRenderTemplate_BinarySkipped(t *testing.T) {
	t.Parallel()

	data := []byte("binary\x00content")
	vars := map[string]any{"key": "val"}

	result, err := RenderTemplate(data, vars)
	if err != nil {
		t.Fatal(err)
	}

	if string(result) != string(data) {
		t.Error("binary data should be returned unchanged")
	}
}

func TestRenderTemplate_MissingKeyError(t *testing.T) {
	t.Parallel()

	data := []byte("value = {{ .missing_key }}")
	vars := map[string]any{"other_key": "val"}

	_, err := RenderTemplate(data, vars)
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestRenderTemplate_InvalidTemplate(t *testing.T) {
	t.Parallel()

	data := []byte("bad {{ .unterminated")
	vars := map[string]any{"key": "val"}

	_, err := RenderTemplate(data, vars)
	if err == nil {
		t.Error("expected error for invalid template syntax")
	}
}

func TestRenderTemplate_ComplexTemplate(t *testing.T) {
	t.Parallel()

	data := []byte(`services:
  app:
    environment:
      - DB_HOST={{ .db_host }}
      - DB_PORT={{ .db_port }}
      - DB_NAME={{ .db_name }}`)

	vars := map[string]any{
		"db_host": "postgres.local",
		"db_port": 5432,
		"db_name": "myapp",
	}

	result, err := RenderTemplate(data, vars)
	if err != nil {
		t.Fatal(err)
	}

	expected := `services:
  app:
    environment:
      - DB_HOST=postgres.local
      - DB_PORT=5432
      - DB_NAME=myapp`
	if string(result) != expected {
		t.Errorf("got:\n%s\nwant:\n%s", string(result), expected)
	}
}
