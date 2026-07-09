package lsp

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.lsp.dev/protocol"
)

type dummyClient struct {
	protocol.Client
}

func (c *dummyClient) PublishDiagnostics(ctx context.Context, params *protocol.PublishDiagnosticsParams) error {
	return nil
}

func TestDocumentConcurrencyStress(t *testing.T) {
	cache := NewDocumentCache()
	uriStr := "file:///workspace/stress_test.he"

	// Primeiro, inserir o documento inicial
	_ = cache.Put(uriStr, `flow main() { return "hello" }`)

	client := &dummyClient{}
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 20
	iterations := 50

	// Goroutine que reconstrói/modifica o documento continuamente no cache
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			content := fmt.Sprintf(`flow main() { return %d }`, i)
			cache.Put(uriStr, content)
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Goroutines que leem o documento concorrentemente (diagnósticos, AST)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Tenta obter o documento mais recente
				d, ok := cache.Get(uriStr)
				if ok && d != nil {
					// Chamar métodos de leitura concorrentes do Document
					d.PublishDiagnostics(ctx, client)
					_ = d.GetProgram()
					_ = d.GetContent()
					_ = d.GetParserErrors()
					_ = d.GetSemanticErrors()
				}
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
}
