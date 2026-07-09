package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// mockClient implementa protocol.Client para capturar diagnósticos emitidos
type mockClient struct {
	protocol.Client
	diagnostics map[string][]protocol.Diagnostic
}

func (m *mockClient) PublishDiagnostics(ctx context.Context, params *protocol.PublishDiagnosticsParams) error {
	m.diagnostics[string(params.URI)] = params.Diagnostics
	return nil
}

func TestLspFeatures(t *testing.T) {
	server := NewHeddleLspServer()
	client := &mockClient{
		diagnostics: make(map[string][]protocol.Diagnostic),
	}
	server.client = client
	ctx := context.Background()
	docURI := "file:///test.he"

	content := `import "math"
flow main(in) {
	// Test link: http://example.com/doc
	x = in
	res = x | math.round()
	return res
}
`
	// 1. DidOpen & Passive Diagnostics
	t.Run("Passive Diagnostics", func(t *testing.T) {
		err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        uri.URI(docURI),
				LanguageID: "heddle",
				Version:    1,
				Text:       content,
			},
		})
		if err != nil {
			t.Fatalf("DidOpen error: %v", err)
		}

		diags, exists := client.diagnostics[docURI]
		if !exists {
			t.Fatalf("expected diagnostics to be published")
		}
		if len(diags) > 0 {
			t.Errorf("unexpected diagnostics errors: %+v", diags)
		}
	})

	// 2. Hover
	t.Run("Hover", func(t *testing.T) {
		params := &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
				Position:     protocol.Position{Line: 3, Character: 1}, // "x" em "x = in"
			},
		}
		hover, err := server.Hover(ctx, params)
		if err != nil {
			t.Fatalf("Hover error: %v", err)
		}
		if hover == nil {
			t.Fatalf("expected hover result")
		}
		markup, ok := hover.Contents.(*protocol.MarkupContent)
		if !ok {
			t.Fatalf("expected *protocol.MarkupContent, got %T", hover.Contents)
		}
		val := markup.Value
		if !strings.Contains(val, "x") {
			t.Errorf("expected hover contents to describe 'x', got %q", val)
		}
	})

	// 3. Document Highlight
	t.Run("Document Highlight", func(t *testing.T) {
		params := &protocol.DocumentHighlightParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
				Position:     protocol.Position{Line: 3, Character: 1}, // "x" em "x = in"
			},
		}
		highlights, err := server.DocumentHighlight(ctx, params)
		if err != nil {
			t.Fatalf("DocumentHighlight error: %v", err)
		}
		if len(highlights) < 2 {
			t.Errorf("expected at least 2 highlights, got %d", len(highlights))
		}
	})

	// 4. Inlay Hint
	t.Run("Inlay Hint", func(t *testing.T) {
		params := &protocol.InlayHintParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 7, Character: 0},
			},
		}
		hints, err := server.InlayHint(ctx, params)
		if err != nil {
			t.Fatalf("InlayHint error: %v", err)
		}
		_ = hints
	})

	// 5. Semantic Tokens
	t.Run("Semantic Tokens", func(t *testing.T) {
		params := &protocol.SemanticTokensParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
		}
		tokens, err := server.SemanticTokensFull(ctx, params)
		if err != nil {
			t.Fatalf("SemanticTokens error: %v", err)
		}
		if tokens == nil || len(tokens.Data) == 0 {
			t.Fatalf("expected semantic tokens data")
		}
		if len(tokens.Data)%5 != 0 {
			t.Errorf("invalid token data length: %d", len(tokens.Data))
		}
	})

	// 6. Folding Range
	t.Run("Folding Range", func(t *testing.T) {
		params := &protocol.FoldingRangeParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
		}
		ranges, err := server.FoldingRanges(ctx, params)
		if err != nil {
			t.Fatalf("FoldingRanges error: %v", err)
		}
		if len(ranges) == 0 {
			t.Errorf("expected folding ranges, got none")
		}
	})

	// 7. Document Link
	t.Run("Document Link", func(t *testing.T) {
		params := &protocol.DocumentLinkParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
		}
		links, err := server.DocumentLink(ctx, params)
		if err != nil {
			t.Fatalf("DocumentLink error: %v", err)
		}
		if len(links) != 1 {
			t.Fatalf("expected exactly 1 link, got %d", len(links))
		}
		expectedTarget := "http://example.com/doc"
		if string(*links[0].Target) != expectedTarget {
			t.Errorf("expected link target %q, got %q", expectedTarget, string(*links[0].Target))
		}
	})

	// 8. Go to Definition
	t.Run("Go to Definition (Local)", func(t *testing.T) {
		params := &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
				Position:     protocol.Position{Line: 4, Character: 7}, // "x" em "res = x"
			},
		}
		def, err := server.Definition(ctx, params)
		if err != nil {
			t.Fatalf("Definition error: %v", err)
		}
		locs, ok := def.(protocol.LocationSlice)
		if !ok || len(locs) != 1 {
			t.Fatalf("expected 1 location in definition result")
		}
		if locs[0].Range.Start.Line != 3 {
			t.Errorf("expected definition at line 3, got line %d", locs[0].Range.Start.Line)
		}
	})

	t.Run("Go to Definition (Go symbol)", func(t *testing.T) {
		params := &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
				Position:     protocol.Position{Line: 4, Character: 18}, // "round" em "math.round()" (Character 18 is 'u')
			},
		}
		def, err := server.Definition(ctx, params)
		if err != nil {
			t.Fatalf("Definition error: %v", err)
		}
		locs, ok := def.(protocol.LocationSlice)
		if !ok || len(locs) != 1 {
			t.Fatalf("expected 1 location in definition result")
		}
		targetURI := string(locs[0].URI)
		if !strings.Contains(targetURI, "math") || !strings.HasSuffix(targetURI, ".go") {
			t.Errorf("expected definition target to be math Go source file, got %q", targetURI)
		}
	})

	t.Run("Go to Definition (Custom Target Path)", func(t *testing.T) {
		tempDir := t.TempDir()
		// 1. Create heddle.toml
		tomlContent := `[project]
name = "temp-project"
[languages.golang]
path = "custom_go_src"`
		err := os.WriteFile(filepath.Join(tempDir, "heddle.toml"), []byte(tomlContent), 0644)
		if err != nil {
			t.Fatalf("failed to write heddle.toml: %v", err)
		}

		// 2. Create custom Go src folder & package
		pkgDir := filepath.Join(tempDir, "custom_go_src", "custom_pkg")
		err = os.MkdirAll(pkgDir, 0755)
		if err != nil {
			t.Fatalf("failed to create custom Go src folder: %v", err)
		}

		goContent := `package custom_pkg
func SayHello() string {
	return "Hello"
}
`
		err = os.WriteFile(filepath.Join(pkgDir, "custom_pkg.go"), []byte(goContent), 0644)
		if err != nil {
			t.Fatalf("failed to write custom_pkg.go: %v", err)
		}

		// 3. Create .he file contents
		heContent := `import "custom_pkg"
flow main() {
	res = custom_pkg.say_hello()
}
`
		heFileURI := "file://" + filepath.Join(tempDir, "test_custom.he")

		// Let's create a new server instance for this test so its cache/client is clean
		customServer := NewHeddleLspServer()
		customServer.client = client // reuse client

		err = customServer.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        uri.URI(heFileURI),
				LanguageID: "heddle",
				Version:    1,
				Text:       heContent,
			},
		})
		if err != nil {
			t.Fatalf("DidOpen failed: %v", err)
		}

		// Definition of "say_hello" in "res = custom_pkg.say_hello()"
		// Line 2 (0-based), character 22 is inside "say_hello"
		params := &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(heFileURI)},
				Position:     protocol.Position{Line: 2, Character: 22},
			},
		}
		def, err := customServer.Definition(ctx, params)
		if err != nil {
			t.Fatalf("Definition error: %v", err)
		}
		locs, ok := def.(protocol.LocationSlice)
		if !ok || len(locs) != 1 {
			t.Fatalf("expected 1 location in definition result, got %v", def)
		}
		targetURI := string(locs[0].URI)
		expectedSuffix := filepath.Join("custom_go_src", "custom_pkg", "custom_pkg.go")
		if !strings.HasSuffix(targetURI, expectedSuffix) {
			t.Errorf("expected target to end with %q, got %q", expectedSuffix, targetURI)
		}
	})

	t.Run("Go to Definition (Struct Constructor)", func(t *testing.T) {
		tempDir := t.TempDir()
		// 1. Create heddle.toml
		tomlContent := `[project]
name = "temp-project"
`
		err := os.WriteFile(filepath.Join(tempDir, "heddle.toml"), []byte(tomlContent), 0644)
		if err != nil {
			t.Fatalf("failed to write heddle.toml: %v", err)
		}

		// 2. Create .he file contents with http.server instantiation
		heContent := `import "net/http"
flow main() {
	srv = http.server {
		host: "127.0.0.1",
		port: 8080
	}
}
`
		heFileURI := "file://" + filepath.Join(tempDir, "test_server.he")

		// Create clean server instance
		customServer := NewHeddleLspServer()
		customServer.client = client

		err = customServer.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        uri.URI(heFileURI),
				LanguageID: "heddle",
				Version:    1,
				Text:       heContent,
			},
		})
		if err != nil {
			t.Fatalf("DidOpen failed: %v", err)
		}

		// Definition of "server" in "http.server {"
		// Line 2 (0-based), character 13 is inside "server"
		params := &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(heFileURI)},
				Position:     protocol.Position{Line: 2, Character: 13},
			},
		}
		def, err := customServer.Definition(ctx, params)
		if err != nil {
			t.Fatalf("Definition error: %v", err)
		}
		locs, ok := def.(protocol.LocationSlice)
		if !ok || len(locs) != 1 {
			t.Fatalf("expected 1 location in definition result, got %v", def)
		}
		targetURI := string(locs[0].URI)
		expectedSuffix := filepath.Join("net", "http", "http.go")
		if !strings.HasSuffix(targetURI, expectedSuffix) {
			t.Errorf("expected target to end with %q, got %q", expectedSuffix, targetURI)
		}
	})

	t.Run("Go to Definition (Method Call on Variable)", func(t *testing.T) {
		tempDir := t.TempDir()
		tomlContent := `[project]
name = "temp-project"
`
		err := os.WriteFile(filepath.Join(tempDir, "heddle.toml"), []byte(tomlContent), 0644)
		if err != nil {
			t.Fatalf("failed to write heddle.toml: %v", err)
		}

		// Create a custom Go package mimicking fhub/jwt
		pkgDir := filepath.Join(tempDir, "pkg/lib", "fhub/jwt")
		err = os.MkdirAll(pkgDir, 0755)
		if err != nil {
			t.Fatalf("failed to create package folder: %v", err)
		}

		goContent := `package jwt
type TokenManager struct {}
func (tm *TokenManager) GenerateAccessToken(userID string) (string, error) {
	return "token", nil
}
type SecretOptions struct {
	Secret string
}
func NewSecret(opts SecretOptions) *TokenManager {
	return &TokenManager{}
}
`
		err = os.WriteFile(filepath.Join(pkgDir, "jwt.go"), []byte(goContent), 0644)
		if err != nil {
			t.Fatalf("failed to write jwt.go: %v", err)
		}

		heContent := `import "fhub/jwt"
token_manager = jwt.secret {
	secret: "mysecret"
}
flow main(in) {
	res = in | token_manager.generate_access_token()
}
`
		heFileURI := "file://" + filepath.Join(tempDir, "test_method.he")

		customServer := NewHeddleLspServer()
		customServer.client = client

		err = customServer.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        uri.URI(heFileURI),
				LanguageID: "heddle",
				Version:    1,
				Text:       heContent,
			},
		})
		if err != nil {
			t.Fatalf("DidOpen failed: %v", err)
		}

		// Verify semantic tokens coloring of generate_access_token (index 6, method)
		tokensParams := &protocol.SemanticTokensParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(heFileURI)},
		}
		tokens, err := customServer.SemanticTokensFull(ctx, tokensParams)
		if err != nil {
			t.Fatalf("SemanticTokens error: %v", err)
		}

		// Let's decode the tokens to locate "generate_access_token"
		type token struct {
			line, col, length, typeIdx uint32
		}
		var decodedTokens []token
		currLine := uint32(0)
		currCol := uint32(0)
		for i := 0; i < len(tokens.Data); i += 5 {
			deltaL := tokens.Data[i]
			deltaC := tokens.Data[i+1]
			length := tokens.Data[i+2]
			typeIdx := tokens.Data[i+3]

			currLine += deltaL
			if deltaL == 0 {
				currCol += deltaC
			} else {
				currCol = deltaC
			}
			decodedTokens = append(decodedTokens, token{line: currLine, col: currCol, length: length, typeIdx: typeIdx})
		}

		var foundToken *token
		for _, tok := range decodedTokens {
			if tok.line == 5 && tok.length == 21 { // len("generate_access_token")
				foundToken = &tok
				break
			}
		}

		if foundToken == nil {
			t.Errorf("could not find generate_access_token token in decoded tokens")
		} else if foundToken.typeIdx != 6 {
			t.Errorf("expected generate_access_token to have type 6 (method), got %d", foundToken.typeIdx)
		}

		// Go to definition of "generate_access_token"
		// Line 5, character 28
		params := &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(heFileURI)},
				Position:     protocol.Position{Line: 5, Character: 28},
			},
		}
		def, err := customServer.Definition(ctx, params)
		if err != nil {
			t.Fatalf("Definition error: %v", err)
		}
		locs, ok := def.(protocol.LocationSlice)
		if !ok || len(locs) != 1 {
			t.Fatalf("expected 1 location in definition result, got %v", def)
		}
		targetURI := string(locs[0].URI)
		expectedSuffix := filepath.Join("fhub", "jwt", "jwt.go")
		if !strings.HasSuffix(targetURI, expectedSuffix) {
			t.Errorf("expected target to end with %q, got %q", expectedSuffix, targetURI)
		}
	})

	// 7. Match Case Parameter Definition and Semantic Tokens
	t.Run("Match Case Parameter Definition and Semantic Tokens", func(t *testing.T) {
		heContent := `flow main(in) {
	return in {
		success(res) {
			x = res
			return x
		}
	}
}
`
		heFileURI := "file:///test_match_case.he"

		err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        uri.URI(heFileURI),
				LanguageID: "heddle",
				Version:    1,
				Text:       heContent,
			},
		})
		if err != nil {
			t.Fatalf("DidOpen failed: %v", err)
		}

		// Verify semantic tokens coloring of parameter "res" on line 3
		tokensParams := &protocol.SemanticTokensParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(heFileURI)},
		}
		tokens, err := server.SemanticTokensFull(ctx, tokensParams)
		if err != nil {
			t.Fatalf("SemanticTokens error: %v", err)
		}

		type token struct {
			line, col, length, typeIdx uint32
		}
		var decodedTokens []token
		currLine := uint32(0)
		currCol := uint32(0)
		for i := 0; i < len(tokens.Data); i += 5 {
			deltaL := tokens.Data[i]
			deltaC := tokens.Data[i+1]
			length := tokens.Data[i+2]
			typeIdx := tokens.Data[i+3]

			currLine += deltaL
			if deltaL == 0 {
				currCol += deltaC
			} else {
				currCol = deltaC
			}
			decodedTokens = append(decodedTokens, token{line: currLine, col: currCol, length: length, typeIdx: typeIdx})
		}

		var foundToken *token
		for _, tok := range decodedTokens {
			if tok.line == 3 && tok.length == 3 {
				foundToken = &tok
				break
			}
		}

		if foundToken == nil {
			t.Errorf("could not find res token in decoded tokens")
		} else if foundToken.typeIdx != 2 {
			t.Errorf("expected res to have type 2 (parameter), got %d", foundToken.typeIdx)
		}

		var foundTagToken *token
		for _, tok := range decodedTokens {
			if tok.line == 2 && tok.length == 7 { // "success" on line 2
				foundTagToken = &tok
				break
			}
		}

		if foundTagToken == nil {
			t.Errorf("could not find success tag token in decoded tokens")
		} else if foundTagToken.typeIdx != 12 {
			t.Errorf("expected success tag to have type 12 (enumMember), got %d", foundTagToken.typeIdx)
		}

		// Go to definition of "res" on line 3, character 7
		params := &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(heFileURI)},
				Position:     protocol.Position{Line: 3, Character: 7},
			},
		}
		def, err := server.Definition(ctx, params)
		if err != nil {
			t.Fatalf("Definition error: %v", err)
		}
		locs, ok := def.(protocol.LocationSlice)
		if !ok || len(locs) != 1 {
			t.Fatalf("expected 1 location in definition result, got %v", def)
		}
		if locs[0].Range.Start.Line != 2 {
			t.Errorf("expected definition to point to line 2 (declaration of res), got line %d", locs[0].Range.Start.Line)
		}
	})

	// 8. Boolean Semantic Tokens
	t.Run("Boolean Semantic Tokens", func(t *testing.T) {
		heContent := `flow main(in) {
	x = true
	y = false
	return x
}
`
		heFileURI := "file:///test_bool.he"

		err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:        uri.URI(heFileURI),
				LanguageID: "heddle",
				Version:    1,
				Text:       heContent,
			},
		})
		if err != nil {
			t.Fatalf("DidOpen failed: %v", err)
		}

		tokensParams := &protocol.SemanticTokensParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(heFileURI)},
		}
		tokens, err := server.SemanticTokensFull(ctx, tokensParams)
		if err != nil {
			t.Fatalf("SemanticTokens error: %v", err)
		}

		type token struct {
			line, col, length, typeIdx uint32
		}
		var decodedTokens []token
		currLine := uint32(0)
		currCol := uint32(0)
		for i := 0; i < len(tokens.Data); i += 5 {
			deltaL := tokens.Data[i]
			deltaC := tokens.Data[i+1]
			length := tokens.Data[i+2]
			typeIdx := tokens.Data[i+3]

			currLine += deltaL
			if deltaL == 0 {
				currCol += deltaC
			} else {
				currCol = deltaC
			}
			decodedTokens = append(decodedTokens, token{line: currLine, col: currCol, length: length, typeIdx: typeIdx})
		}

		var foundTrueToken *token
		var foundFalseToken *token
		for _, tok := range decodedTokens {
			if tok.line == 1 && tok.length == 4 { // "true" on line 1 (0-indexed)
				foundTrueToken = &tok
			}
			if tok.line == 2 && tok.length == 5 { // "false" on line 2 (0-indexed)
				foundFalseToken = &tok
			}
		}

		if foundTrueToken == nil {
			t.Errorf("could not find true token in decoded tokens")
		} else if foundTrueToken.typeIdx != 13 {
			t.Errorf("expected true to have type 13 (boolean), got %d", foundTrueToken.typeIdx)
		}

		if foundFalseToken == nil {
			t.Errorf("could not find false token in decoded tokens")
		} else if foundFalseToken.typeIdx != 13 {
			t.Errorf("expected false to have type 13 (boolean), got %d", foundFalseToken.typeIdx)
		}
	})
}

func TestLspHoverEnhanced(t *testing.T) {
	server := NewHeddleLspServer()
	client := &mockClient{
		diagnostics: make(map[string][]protocol.Diagnostic),
	}
	server.client = client
	ctx := context.Background()
	docURI := "file:///test_hover.he"

	content := `import "net/http"
import "time"

http_server = http.server {
	host: "localhost",
	port: 8080
}

flow main {
	// Http callback handler
	http_server.post(path: "/user") {
		ok(request) {
			body_str = request.body
			return { status: 200, body: body_str }
		}
	}

	time.tick("1s") {
		each(now) {
			t_str = now
		}
	}
}
`

	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri.URI(docURI),
			LanguageID: "heddle",
			Version:    1,
			Text:       content,
		},
	})
	if err != nil {
		t.Fatalf("DidOpen error: %v", err)
	}

	t.Run("StructMethodHover", func(t *testing.T) {
		params := &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
				Position:     protocol.Position{Line: 10, Character: 14}, // "post" in "http_server.post"
			},
		}
		hover, err := server.Hover(ctx, params)
		if err != nil {
			t.Fatalf("Hover method error: %v", err)
		}
		if hover == nil {
			t.Fatalf("expected hover result for http_server.post")
		}
		markup, ok := hover.Contents.(*protocol.MarkupContent)
		if !ok {
			t.Fatalf("expected *protocol.MarkupContent, got %T", hover.Contents)
		}
		val := markup.Value
		if !strings.Contains(val, "func (s *HTTPServer) Post") {
			t.Errorf("expected hover value to contain 'Post' signature, got %q", val)
		}
	})

	t.Run("StructFieldHover", func(t *testing.T) {
		params := &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
				Position:     protocol.Position{Line: 12, Character: 23}, // "body" in "request.body"
			},
		}
		hover, err := server.Hover(ctx, params)
		if err != nil {
			t.Fatalf("Hover field error: %v", err)
		}
		if hover == nil {
			t.Fatalf("expected hover result for request.body")
		}
		markup, ok := hover.Contents.(*protocol.MarkupContent)
		if !ok {
			t.Fatalf("expected *protocol.MarkupContent, got %T", hover.Contents)
		}
		val := markup.Value
		if !strings.Contains(val, "struct field Request.body") && !strings.Contains(val, "struct field Request.Body") {
			t.Errorf("expected hover value to contain 'Request.body' or 'Request.Body', got %q", val)
		}
	})

	t.Run("CallbackParamHover", func(t *testing.T) {
		params := &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
				Position:     protocol.Position{Line: 11, Character: 6}, // "request"
			},
		}
		hover, err := server.Hover(ctx, params)
		if err != nil {
			t.Fatalf("Hover param error: %v", err)
		}
		if hover == nil {
			t.Fatalf("expected hover result for parameter request")
		}
		markup, ok := hover.Contents.(*protocol.MarkupContent)
		if !ok {
			t.Fatalf("expected *protocol.MarkupContent, got %T", hover.Contents)
		}
		val := markup.Value
		if !strings.Contains(val, "net/http.Request") {
			t.Errorf("expected hover value to contain 'net/http.Request', got %q", val)
		}
	})

	t.Run("NamedArgHover", func(t *testing.T) {
		params := &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
				Position:     protocol.Position{Line: 10, Character: 19}, // "path" in "http_server.post(path: ..."
			},
		}
		hover, err := server.Hover(ctx, params)
		if err != nil {
			t.Fatalf("Hover argument error: %v", err)
		}
		if hover == nil {
			t.Fatalf("expected hover result for named argument path")
		}
		markup, ok := hover.Contents.(*protocol.MarkupContent)
		if !ok {
			t.Fatalf("expected *protocol.MarkupContent, got %T", hover.Contents)
		}
		val := markup.Value
		if !strings.Contains(val, "argument path: string") {
			t.Errorf("expected hover value to be 'argument path: string', got %q", val)
		}
	})

	t.Run("TriggerParamHover", func(t *testing.T) {
		params := &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
				Position:     protocol.Position{Line: 18, Character: 8}, // "now" in "each(now)"
			},
		}
		hover, err := server.Hover(ctx, params)
		if err != nil {
			t.Fatalf("Hover trigger param error: %v", err)
		}
		if hover == nil {
			t.Fatalf("expected hover result for parameter now")
		}
		markup, ok := hover.Contents.(*protocol.MarkupContent)
		if !ok {
			t.Fatalf("expected *protocol.MarkupContent, got %T", hover.Contents)
		}
		val := markup.Value
		if !strings.Contains(val, "string") {
			t.Errorf("expected hover value to contain 'string' for now, got %q", val)
		}
	})
}
