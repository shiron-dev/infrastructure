package remote

import (
	"testing"
)

func TestParseDirStatOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  string
		wantErr bool
		want    *DirMetadata
	}{
		{
			name:   "standard output",
			output: "755 root root\n",
			want:   &DirMetadata{Permission: "755", Owner: "root", Group: "root"},
		},
		{
			name:   "trimmed output",
			output: "  750 app staff  ",
			want:   &DirMetadata{Permission: "750", Owner: "app", Group: "staff"},
		},
		{
			name:   "setuid permission",
			output: "3755 deploy deploy\n",
			want:   &DirMetadata{Permission: "3755", Owner: "deploy", Group: "deploy"},
		},
		{
			name:    "too few fields",
			output:  "755 root",
			wantErr: true,
		},
		{
			name:    "too many fields",
			output:  "755 root extra",
			wantErr: true,
		},
		{
			name:    "empty output",
			output:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseDirStatOutput(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Permission != tt.want.Permission {
				t.Errorf("Permission = %q, want %q", got.Permission, tt.want.Permission)
			}

			if got.Owner != tt.want.Owner {
				t.Errorf("Owner = %q, want %q", got.Owner, tt.want.Owner)
			}

			if got.Group != tt.want.Group {
				t.Errorf("Group = %q, want %q", got.Group, tt.want.Group)
			}
		})
	}
}
