package lsp

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"heddle/pkg/lang/ast"
	"heddle/pkg/lang/lexer"
	"heddle/pkg/lang/parser"
	"heddle/pkg/lang/semantic"
)

// Document representa um arquivo .he aberto no editor
type Document struct {
	mu             sync.RWMutex
	URI            string
	Content        string
	Program        *ast.Program
	ParserErrors   []string
	SemanticErrors []semantic.AnalysisError
	InferredTypes  map[ast.Expression]*ast.TypeInfo
}

// DocumentCache gerencia a coleção de arquivos abertos de forma thread-safe
type DocumentCache struct {
	mu   sync.RWMutex
	docs map[string]*Document
}

func NewDocumentCache() *DocumentCache {
	return &DocumentCache{
		docs: make(map[string]*Document),
	}
}

func (c *DocumentCache) Put(uriStr, content string) *Document {
	c.mu.Lock()
	doc, exists := c.docs[uriStr]
	if !exists {
		doc = &Document{
			URI: uriStr,
		}
		c.docs[uriStr] = doc
	}
	c.mu.Unlock()

	doc.UpdateAndRebuild(content)
	return doc
}

func (c *DocumentCache) Get(uriStr string) (*Document, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	doc, ok := c.docs[uriStr]
	return doc, ok
}

func (c *DocumentCache) Remove(uriStr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.docs, uriStr)
}

// UpdateAndRebuild reconstrói a AST e analisa semanticamente o documento de forma concorrente e thread-safe.
func (d *Document) UpdateAndRebuild(content string) {
	l := lexer.New(content)
	p := parser.New(l)
	prog := p.ParseProgram()
	parserErrors := p.Errors()

	var semanticErrors []semantic.AnalysisError
	var inferredTypes map[ast.Expression]*ast.TypeInfo

	// Só rodar análise semântica se não houver erros de sintaxe
	if len(parserErrors) == 0 && prog != nil {
		analyzer := semantic.NewWithParser(&semantic.GoASTParser{})
		docFsPath := uri.URI(d.URI).FsPath()
		docDir := filepath.Dir(docFsPath)
		analyzer.SetProjectRootDir(docDir)
		analyzer.Analyze(prog)
		semanticErrors = analyzer.DetailedErrors()
		inferredTypes = analyzer.InferredTypes()
	}

	d.mu.Lock()
	d.Content = content
	d.Program = prog
	d.ParserErrors = parserErrors
	d.SemanticErrors = semanticErrors
	d.InferredTypes = inferredTypes
	d.mu.Unlock()
}


func (d *Document) GetContent() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Content
}

func (d *Document) GetProgram() *ast.Program {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Program
}

func (d *Document) GetParserErrors() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.ParserErrors == nil {
		return nil
	}
	errs := make([]string, len(d.ParserErrors))
	copy(errs, d.ParserErrors)
	return errs
}

func (d *Document) GetSemanticErrors() []semantic.AnalysisError {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.SemanticErrors == nil {
		return nil
	}
	errs := make([]semantic.AnalysisError, len(d.SemanticErrors))
	copy(errs, d.SemanticErrors)
	return errs
}

func (d *Document) GetInferredTypes() map[ast.Expression]*ast.TypeInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.InferredTypes == nil {
		return nil
	}
	// Não precisamos clonar o mapa, pois ele não é modificado após a análise
	return d.InferredTypes
}


// PublishDiagnostics envia os erros (sintaxe + semântica) para o cliente editor
func (d *Document) PublishDiagnostics(ctx context.Context, client protocol.Client) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var diagnostics []protocol.Diagnostic

	// 1. Mapear erros de parser (sintaxe)
	for _, errStr := range d.ParserErrors {
		line, col, msg, ok := parseParserError(errStr)
		if !ok {
			// Fallback se não conseguir parsear a string de erro
			diagnostics = append(diagnostics, protocol.Diagnostic{
				Range:    protocol.Range{},
				Severity: protocol.DiagnosticSeverityError,
				Source:   protocol.NewOptional("heddle-parser"),
				Message:  protocol.String(errStr),
			})
			continue
		}

		startPos := LexerToLSPPosition(line, col)
		endPos := startPos
		endPos.Character++

		diagnostics = append(diagnostics, protocol.Diagnostic{
			Range: protocol.Range{
				Start: startPos,
				End:   endPos,
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   protocol.NewOptional("heddle-parser"),
			Message:  protocol.String(msg),
		})
	}

	// 2. Mapear erros semânticos
	for _, semErr := range d.SemanticErrors {
		var rng protocol.Range
		if semErr.Node != nil {
			rng = LexerTokenToLSPRange(semErr.Node.GetToken())
		}

		diagnostics = append(diagnostics, protocol.Diagnostic{
			Range:    rng,
			Severity: protocol.DiagnosticSeverityError,
			Source:   protocol.NewOptional("heddle-semantic"),
			Message:  protocol.String(semErr.Msg),
		})
	}

	_ = client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         uri.URI(d.URI),
		Diagnostics: diagnostics,
	})
}

// parseParserError extrai linha, coluna e mensagem de erros no formato "line X, col Y: msg"
func parseParserError(errStr string) (line int, col int, msg string, ok bool) {
	if !strings.HasPrefix(errStr, "line ") {
		return 0, 0, "", false
	}
	rest := errStr[5:]
	commaIdx := strings.Index(rest, ", col ")
	if commaIdx == -1 {
		return 0, 0, "", false
	}
	lineStr := rest[:commaIdx]
	rest = rest[commaIdx+6:]
	colonIdx := strings.Index(rest, ": ")
	if colonIdx == -1 {
		return 0, 0, "", false
	}
	colStr := rest[:colonIdx]
	message := rest[colonIdx+2:]

	l, errL := strconv.Atoi(lineStr)
	c, errC := strconv.Atoi(colStr)
	if errL != nil || errC != nil {
		return 0, 0, "", false
	}
	return l, c, message, true
}
