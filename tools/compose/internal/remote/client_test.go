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
			output: "755 0 0 root root\n",
			want:   &DirMetadata{Permission: "755", OwnerID: "0", GroupID: "0", Owner: "root", Group: "root"},
		},
		{
			name:   "trimmed output",
			output: "  750 1000 50 app staff  ",
			want:   &DirMetadata{Permission: "750", OwnerID: "1000", GroupID: "50", Owner: "app", Group: "staff"},
		},
		{
			name:   "setuid permission",
			output: "3755 1000 1000 deploy deploy\n",
			want:   &DirMetadata{Permission: "3755", OwnerID: "1000", GroupID: "1000", Owner: "deploy", Group: "deploy"},
		},
		{
			name:    "too few fields",
			output:  "755 0 0 root",
			wantErr: true,
		},
		{
			name:    "too many fields",
			output:  "755 0 0 root wheel extra",
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

			if got.OwnerID != tt.want.OwnerID {
				t.Errorf("OwnerID = %q, want %q", got.OwnerID, tt.want.OwnerID)
			}

			if got.GroupID != tt.want.GroupID {
				t.Errorf("GroupID = %q, want %q", got.GroupID, tt.want.GroupID)
			}
		})
	}
}
