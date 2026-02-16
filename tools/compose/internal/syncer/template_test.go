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

	envPath := filepath.Join(dir, ".env")

	err := os.WriteFile(envPath, []byte(`# comment
KEY1=value1
KEY2=value2
QUOTED="hello world"
SINGLE_QUOTED='foo bar'
  SPACED = spaced_val

EMPTY=
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	vars, err := parseEnvFile(envPath)
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

	for key, want := range tests {
		got, ok := vars[key]
		if !ok {
			t.Errorf("missing key %q", key)

			continue
		}

		if got != want {
			t.Errorf("key %q = %q, want %q", key, got, want)
		}
	}
}

func TestParseEnvFile_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantLen int
		wantErr bool
	}{
		{
			name:    "not exist",
			path:    "/nonexistent/.env",
			wantLen: 0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vars, err := parseEnvFile(tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseEnvFile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(vars) != tt.wantLen {
				t.Errorf("expected %d vars, got %v", tt.wantLen, vars)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseSecretsYAML
// ---------------------------------------------------------------------------

func TestParseSecretsYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	secretsPath := filepath.Join(dir, "env.secrets.yml")

	err := os.WriteFile(secretsPath, []byte(`github_client_id: abc123
github_client_secret: secret456
smtp_port: 587
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	vars, err := parseSecretsYAML(secretsPath)
	if err != nil {
		t.Fatal(err)
	}

	if vars["github_client_id"] != "abc123" {
		t.Errorf("github_client_id = %v", vars["github_client_id"])
	}

	if vars["github_client_secret"] != "secret456" {
		t.Errorf("github_client_secret = %v", vars["github_client_secret"])
	}
	// YAML の整数は int としてパースされます。
	if vars["smtp_port"] != 587 {
		t.Errorf("smtp_port = %v (%T)", vars["smtp_port"], vars["smtp_port"])
	}
}

func TestParseSecretsYAML_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantLen int
		wantErr bool
	}{
		{
			name: "not exist",
			setup: func(t *testing.T) string {
				t.Helper()

				return "/nonexistent/env.secrets.yml"
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "invalid YAML",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				secretsPath := filepath.Join(dir, "env.secrets.yml")

				err := os.WriteFile(secretsPath, []byte(`{invalid: yaml: [}`), 0600)
				if err != nil {
					t.Fatal(err)
				}

				return secretsPath
			},
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.setup(t)

			vars, err := parseSecretsYAML(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseSecretsYAML() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && len(vars) != tt.wantLen {
				t.Errorf("expected %d vars, got %v", tt.wantLen, vars)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LoadTemplateVars
// ---------------------------------------------------------------------------

func TestLoadTemplateVars_SecretsOverrideEnv(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	hostProjectDir := filepath.Join(base, "hosts", "server1", "grafana")

	err := os.MkdirAll(hostProjectDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	// .env は KEY1 と SHARED を持っています。
	err = os.WriteFile(filepath.Join(hostProjectDir, ".env"), []byte(`KEY1=from_env
SHARED=env_value
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// env.secrets.yml は KEY2 を持ち、SHARED を上書きします。
	err = os.WriteFile(filepath.Join(hostProjectDir, "env.secrets.yml"), []byte(`KEY2: from_secrets
SHARED: secrets_value
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

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
	// SHARED は env.secrets.yml から来るべきです。
	if vars["SHARED"] != "secrets_value" {
		t.Errorf("SHARED = %v, want secrets_value", vars["SHARED"])
	}
}

func TestLoadTemplateVars_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T, base string)
		wantVars map[string]any
	}{
		{
			name: "no files",
			setup: func(t *testing.T, base string) {
				t.Helper()

				err := os.MkdirAll(filepath.Join(base, "hosts", "server1", "grafana"), 0750)
				if err != nil {
					t.Fatal(err)
				}
			},
			wantVars: map[string]any{},
		},
		{
			name: "only secrets",
			setup: func(t *testing.T, base string) {
				t.Helper()

				hostProjectDir := filepath.Join(base, "hosts", "server1", "grafana")

				err := os.MkdirAll(hostProjectDir, 0750)
				if err != nil {
					t.Fatal(err)
				}

				err = os.WriteFile(filepath.Join(hostProjectDir, "env.secrets.yml"), []byte(`secret_key: s3cret
`), 0600)
				if err != nil {
					t.Fatal(err)
				}
			},
			wantVars: map[string]any{"secret_key": "s3cret"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			tt.setup(t, base)

			vars, err := LoadTemplateVars(base, "server1", "grafana")
			if err != nil {
				t.Fatal(err)
			}

			if len(vars) != len(tt.wantVars) {
				t.Errorf("expected %d vars, got %d: %v", len(tt.wantVars), len(vars), vars)
			}

			for key, want := range tt.wantVars {
				got, ok := vars[key]
				if !ok {
					t.Errorf("missing key %q", key)

					continue
				}

				if got != want {
					t.Errorf("%s = %v, want %v", key, got, want)
				}
			}
		})
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

func TestRenderTemplate_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		data       []byte
		vars       map[string]any
		wantErr    bool
		wantResult []byte
	}{
		{
			name:       "no vars",
			data:       []byte("plain text with no {{ templates }}"),
			vars:       nil,
			wantErr:    false,
			wantResult: []byte("plain text with no {{ templates }}"),
		},
		{
			name:       "empty vars",
			data:       []byte("plain text"),
			vars:       map[string]any{},
			wantErr:    false,
			wantResult: []byte("plain text"),
		},
		{
			name:       "binary skipped",
			data:       []byte("binary\x00content"),
			vars:       map[string]any{"key": "val"},
			wantErr:    false,
			wantResult: []byte("binary\x00content"),
		},
		{
			name:    "missing key error",
			data:    []byte("value = {{ .missing_key }}"),
			vars:    map[string]any{"other_key": "val"},
			wantErr: true,
		},
		{
			name:    "invalid template",
			data:    []byte("bad {{ .unterminated"),
			vars:    map[string]any{"key": "val"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := RenderTemplate(tt.data, tt.vars)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RenderTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && string(result) != string(tt.wantResult) {
				t.Errorf("RenderTemplate() = %q, want %q", string(result), string(tt.wantResult))
			}
		})
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
