package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateSchemaJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		schemaType     string
		wantErr        bool
		wantErrMsg     string
		wantFields     []string
		wantJSONFields []string
	}{
		{
			name:           "cmt schema",
			schemaType:     "cmt",
			wantErr:        false,
			wantJSONFields: []string{"$schema", "$defs"},
			wantFields:     []string{"basePath", "hosts"},
		},
		{
			name:       "host schema",
			schemaType: "host",
			wantErr:    false,
			wantFields: []string{"remotePath", "projects"},
		},
		{
			name:       "unknown schema type",
			schemaType: "unknown",
			wantErr:    true,
			wantErrMsg: "unknown schema type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := GenerateSchemaJSON(tt.schemaType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateSchemaJSON(%q) error = %v, wantErr %v", tt.schemaType, err, tt.wantErr)
			}

			if tt.wantErr {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error message = %q, want to contain %q", err.Error(), tt.wantErrMsg)
				}
				return
			}

			var obj map[string]any
			err = json.Unmarshal(data, &obj)
			if err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			for _, field := range tt.wantJSONFields {
				if _, ok := obj[field]; !ok {
					t.Errorf("missing JSON field %q", field)
				}
			}

			raw := string(data)
			for _, field := range tt.wantFields {
				if !strings.Contains(raw, field) {
					t.Errorf("schema should reference %q", field)
				}
			}
		})
	}
}
