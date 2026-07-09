package vm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"heddle/pkg/lang/compiler"
	"heddle/pkg/lang/lexer"
	"heddle/pkg/lang/parser"
	"heddle/pkg/lang/semantic"
	"heddle/pkg/runtime"
	"heddle/pkg/runtime/bridge/reflection"
	"heddle/pkg/runtime/interaction"
)

type RangeTag string

const (
	TagEach RangeTag = "each"
)

type MockGenPkg struct{}

func (m *MockGenPkg) IntRange(start float64, end float64) (interaction.SeqAccum[RangeTag, float64], error) {
	return func(yield func(interaction.Match[RangeTag, float64]) bool) {
		for current := start; current <= end; current++ {
			matchVal := interaction.Match[RangeTag, float64]{
				Tag: TagEach,
				Val: current,
			}
			if !yield(matchVal) {
				break
			}
		}
	}, nil
}

func compileAndRunTrigger(t *testing.T, code string, mockPkg any) (*runtime.Frame, error) {
	l := lexer.New(code)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("syntax errors: %v", p.Errors())
	}

	available := make(map[string]any)
	for k, v := range runtime.PackagesRegistry {
		available[k] = v
		parts := strings.Split(k, "/")
		alias := parts[len(parts)-1]
		available[alias] = v
	}
	if mockPkg != nil {
		available["test_pkg"] = mockPkg
	}

	analyzer := semantic.New(available)
	if !analyzer.Analyze(prog) {
		t.Fatalf("semantic errors: %v", analyzer.Errors())
	}

	comp := compiler.NewCompiler()
	irProg, err := comp.Compile(prog)
	if err != nil {
		t.Fatalf("compiler error: %v", err)
	}

	metadata := make(map[string]runtime.FuncMetadata)
	for k, v := range runtime.StdlibMetadataRegistry {
		metadata[k] = v
	}
	if mockPkg != nil {
		metadata["test_pkg.int_range"] = runtime.FuncMetadata{ParamNames: []string{"start", "end"}}
	}

	disp := reflection.NewReflectionBridge(available, metadata)
	interpreter := NewVM(irProg, disp)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return interpreter.Run(ctx)
}

func TestInterpreterSeqAccumulation(t *testing.T) {
	code := `
	import "test_pkg"

	flow main {
		res = test_pkg.int_range(1, 3) {
			each(x) {
				return x
			}
		}
		return res
	}
	`
	mock := &MockGenPkg{}
	res, err := compileAndRunTrigger(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	if res == nil {
		t.Fatalf("expected non-nil result frame")
	}

	expected := []any{1.0, 2.0, 3.0}
	if len(res.Rows) != len(expected) {
		t.Fatalf("expected %d rows, got %d", len(expected), len(res.Rows))
	}

	for idx, val := range res.Rows {
		if val != expected[idx] {
			t.Errorf("at index %d: expected %v, got %v", idx, expected[idx], val)
		}
	}
}

func TestInterpreterHTTPCallbackReqReply(t *testing.T) {
	code := `
	import "net/http"

	http_server = http.server {
		host: "localhost",
		port: 0
	}

	flow main {
		http_server.post(path: "/test-cb") {
			ok(request) {
				return { status: 200, body: request.body }
			}
		}
	}
	`

	l := lexer.New(code)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("syntax errors: %v", p.Errors())
	}

	available := make(map[string]any)
	for k, v := range runtime.PackagesRegistry {
		available[k] = v
		parts := strings.Split(k, "/")
		alias := parts[len(parts)-1]
		available[alias] = v
	}

	analyzer := semantic.New(available)
	if !analyzer.Analyze(prog) {
		t.Fatalf("semantic errors: %v", analyzer.Errors())
	}

	comp := compiler.NewCompiler()
	irProg, err := comp.Compile(prog)
	if err != nil {
		t.Fatalf("compiler error: %v", err)
	}

	metadata := make(map[string]runtime.FuncMetadata)
	for k, v := range runtime.StdlibMetadataRegistry {
		metadata[k] = v
	}

	disp := reflection.NewReflectionBridge(available, metadata)
	interpreter := NewVM(irProg, disp)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		_, err := interpreter.Run(ctx)
		errChan <- err
	}()

	time.Sleep(500 * time.Millisecond)

	srvVal, exists := interpreter.globals.Get("http_server")
	if !exists {
		t.Fatalf("http_server not found in globals")
	}

	var port int
	importVal := srvVal
	if f, ok := srvVal.(*runtime.Frame); ok && len(f.Rows) > 0 {
		importVal = f.Rows[0]
	}

	pVal := reflect.ValueOf(importVal)
	for pVal.Kind() == reflect.Ptr {
		pVal = pVal.Elem()
	}
	if pVal.Kind() == reflect.Struct {
		field := pVal.FieldByName("Port")
		if field.IsValid() {
			port = int(field.Int())
		}
	}

	if port <= 0 {
		t.Fatalf("failed to retrieve server port: got %d", port)
	}

	url := fmt.Sprintf("http://localhost:%d/test-cb", port)
	reqBody := "Heddle"
	resp, err := http.Post(url, "text/plain", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", resp.StatusCode)
	}

	expectedBody := "Heddle"
	if string(bodyBytes) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, string(bodyBytes))
	}
}
