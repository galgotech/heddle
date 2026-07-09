package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSafeURI(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}
	workspaceRoot := filepath.Clean(wd)

	tests := []struct {
		name          string
		uri           string
		workspaceRoot string
		wantErr       bool
	}{
		{
			name:          "Valid file URI inside workspace",
			uri:           "file://" + filepath.ToSlash(filepath.Join(workspaceRoot, "pkg/lang/lsp/server.go")),
			workspaceRoot: workspaceRoot,
			wantErr:       false,
		},
		{
			name:          "Valid untitled URI inside workspace",
			uri:           "untitled://" + filepath.ToSlash(filepath.Join(workspaceRoot, "new_file.he")),
			workspaceRoot: workspaceRoot,
			wantErr:       false,
		},
		{
			name:          "Invalid scheme http",
			uri:           "http://example.com/file.he",
			workspaceRoot: workspaceRoot,
			wantErr:       true,
		},
		{
			name:          "Empty URI",
			uri:           "",
			workspaceRoot: workspaceRoot,
			wantErr:       true,
		},
		{
			name:          "Path traversal attempt escaping root",
			uri:           "file://" + filepath.ToSlash(filepath.Join(workspaceRoot, "../../../etc/passwd")),
			workspaceRoot: workspaceRoot,
			wantErr:       true,
		},
		{
			name:          "Path traversal using dot segments",
			uri:           "file://" + filepath.ToSlash(workspaceRoot) + "/../../etc/passwd",
			workspaceRoot: workspaceRoot,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsSafeURI(tt.uri, tt.workspaceRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsSafeURI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
