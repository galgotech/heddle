package lsp

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"go.lsp.dev/uri"
)

// IsSafeURI valida se a URI fornecida possui esquema permitido e se seu caminho 
// absoluto resolvido permanece estritamente sob o workspaceRoot.
func IsSafeURI(uriStr string, workspaceRoot string) error {
	if uriStr == "" {
		return fmt.Errorf("empty URI")
	}

	// 1. Validar parse da URI
	parsed, err := url.Parse(uriStr)
	if err != nil {
		return fmt.Errorf("failed to parse URI %q: %w", uriStr, err)
	}

	// 2. Permitir apenas file:// e untitled://
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "file" && scheme != "untitled" {
		return fmt.Errorf("unsafe URI scheme %q, only 'file' and 'untitled' are allowed", parsed.Scheme)
	}

	if workspaceRoot == "" {
		return nil
	}

	// 3. Obter o FsPath limpo
	fsPath := uri.URI(uriStr).FsPath()
	cleanPath, err := filepath.Abs(fsPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path of %q: %w", fsPath, err)
	}

	cleanWorkspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path of workspace root %q: %w", workspaceRoot, err)
	}

	// 4. Verificar se o caminho do arquivo começa com o caminho do workspace root
	if !strings.HasSuffix(cleanWorkspace, string(filepath.Separator)) {
		cleanWorkspace += string(filepath.Separator)
	}

	if !strings.HasPrefix(cleanPath+string(filepath.Separator), cleanWorkspace) && cleanPath != filepath.Clean(workspaceRoot) {
		return fmt.Errorf("path traversal detected: file %q lies outside workspace root %q", cleanPath, cleanWorkspace)
	}

	return nil
}
