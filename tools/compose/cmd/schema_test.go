package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateSchemaJSON_Cmt(t *testing.T) {
	t.Parallel()

	data, err := generateSchemaJSON("cmt")
	if err != nil {
		t.Fatalf("generateSchemaJSON(cmt): %v", err)
	}

	// Must be valid JSON.
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Must contain expected top-level schema fields.
	if _, ok := obj["$schema"]; !ok {
		t.Error("missing $schema field")
	}

	if _, ok := obj["$defs"]; !ok {
		t.Error("missing $defs field")
	}

	// Must reference basePath and hosts from CmtConfig in the JSON.
	raw := string(data)
	if !strings.Contains(raw, "basePath") {
		t.Error("schema should reference basePath")
	}

	if !strings.Contains(raw, "hosts") {
		t.Error("schema should reference hosts")
	}
}

func TestGenerateSchemaJSON_Host(t *testing.T) {
	t.Parallel()

	data, err := generateSchemaJSON("host")
	if err != nil {
		t.Fatalf("generateSchemaJSON(host): %v", err)
	}

	// Must be valid JSON.
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Must reference remotePath and projects from HostConfig.
	raw := string(data)
	if !strings.Contains(raw, "remotePath") {
		t.Error("schema should reference remotePath")
	}

	if !strings.Contains(raw, "projects") {
		t.Error("schema should reference projects")
	}
}

func TestGenerateSchemaJSON_Unknown(t *testing.T) {
	t.Parallel()

	_, err := generateSchemaJSON("unknown")
	if err == nil {
		t.Fatal("expected error for unknown schema type")
	}

	if !strings.Contains(err.Error(), "unknown schema type") {
		t.Errorf("unexpected error message: %v", err)
	}
}
