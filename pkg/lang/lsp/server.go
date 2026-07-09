package lsp

import (
	"context"
	"os"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

var supportedTokenTypes = []string{
	"namespace",  // 0: pacotes/imports
	"type",       // 1: struct types
	"parameter",  // 2: parâmetros de fluxo/handler
	"variable",   // 3: variáveis locais
	"property",   // 4: campos de struct
	"function",   // 5: fluxos/handlers
	"method",     // 6: chamadas de função
	"keyword",    // 7: palavras reservadas (flow, return, etc)
	"comment",    // 8: comentários (//)
	"string",     // 9: literais de string
	"number",     // 10: literais de número
	"operator",   // 11: operadores (=, |, ?)
	"enumMember", // 12: tags de match cases
	"boolean",    // 13: literais booleanos (true, false)
}

type HeddleLspServer struct {
	protocol.UnimplementedServer
	client        protocol.Client
	cache         *DocumentCache
	workspaceRoot string
}

func NewHeddleLspServer() *HeddleLspServer {
	return &HeddleLspServer{
		cache: NewDocumentCache(),
	}
}

// stdioReadWriteCloser bridges Stdin and Stdout for jsonrpc2
type stdioReadWriteCloser struct{}

func (s *stdioReadWriteCloser) Read(p []byte) (n int, err error) {
	return os.Stdin.Read(p)
}

func (s *stdioReadWriteCloser) Write(p []byte) (n int, err error) {
	return os.Stdout.Write(p)
}

func (s *stdioReadWriteCloser) Close() error {
	return nil
}

// StartServer inicializa o servidor LSP em stdio
func StartServer() error {
	rwc := &stdioReadWriteCloser{}
	stream := jsonrpc2.NewStream(rwc)

	server := NewHeddleLspServer()
	ctx := context.Background()

	// NewServer cria a conexão
	_, conn, client := protocol.NewServer(ctx, server, stream)
	server.client = client

	// Aguarda o término da conexão
	<-conn.Done()
	return nil
}

func (s *HeddleLspServer) Initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	if params.RootURI != nil {
		s.workspaceRoot = params.RootURI.FsPath()
	}

	tVal := true
	syncKind := protocol.TextDocumentSyncKindIncremental

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: &tVal,
				Change:    &syncKind,
				Save:      &protocol.SaveOptions{IncludeText: &tVal},
			},
			HoverProvider: protocol.Boolean(true),
			SignatureHelpProvider: &protocol.SignatureHelpOptions{
				TriggerCharacters: []string{"(", ","},
			},
			DocumentHighlightProvider: protocol.Boolean(true),
			InlayHintProvider:         protocol.Boolean(true),
			SemanticTokensProvider: &protocol.SemanticTokensOptions{
				Legend: protocol.SemanticTokensLegend{
					TokenTypes:     supportedTokenTypes,
					TokenModifiers: []string{},
				},
				Full: protocol.Boolean(true),
			},
			FoldingRangeProvider: protocol.Boolean(true),
			DocumentLinkProvider: &protocol.DocumentLinkOptions{
				ResolveProvider: &tVal,
			},
			DefinitionProvider: protocol.Boolean(true),
		},
		ServerInfo: protocol.ServerInfo{
			Name:    "heddle-lsp",
			Version: protocol.NewOptional("1.0.0"),
		},
	}, nil
}

func (s *HeddleLspServer) Initialized(ctx context.Context, params *protocol.InitializedParams) error {
	return nil
}

func (s *HeddleLspServer) Shutdown(ctx context.Context) error {
	return nil
}

func (s *HeddleLspServer) Exit(ctx context.Context) error {
	return nil
}

func (s *HeddleLspServer) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return err
	}
	doc := s.cache.Put(uriStr, params.TextDocument.Text)
	doc.PublishDiagnostics(ctx, s.client)
	return nil
}

func (s *HeddleLspServer) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return err
	}

	doc, exists := s.cache.Get(uriStr)
	if !exists {
		doc = s.cache.Put(uriStr, "")
	}

	content := doc.GetContent()

	for _, changeEvent := range params.ContentChanges {
		switch change := changeEvent.(type) {
		case *protocol.TextDocumentContentChangeWholeDocument:
			content = change.Text
		case *protocol.TextDocumentContentChangePartial:
			content = ApplyIncrementalChange(content, *change)
		}
	}

	doc = s.cache.Put(uriStr, content)
	doc.PublishDiagnostics(ctx, s.client)
	return nil
}

func (s *HeddleLspServer) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {
	uriStr := string(params.TextDocument.URI)
	if err := IsSafeURI(uriStr, s.workspaceRoot); err != nil {
		return err
	}
	s.cache.Remove(uriStr)
	return nil
}

func (s *HeddleLspServer) DidSave(ctx context.Context, params *protocol.DidSaveTextDocumentParams) error {
	return nil
}
