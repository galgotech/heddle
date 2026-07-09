package semantic

import (
	"os"
	"path/filepath"
	"testing"

	"heddle/internal/config"
	"heddle/pkg/lang/ast"
	"heddle/pkg/lang/lexer"
	"heddle/pkg/lang/parser"
)

func parseCode(t *testing.T, code string) *parser.Parser {
	l := lexer.New(code)
	p := parser.New(l)
	return p
}

func TestStage1MissingFlowMain(t *testing.T) {
	code := `
	import "io"
	flow process(in) {
		return in
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	analyzer := New(nil)
	success := analyzer.Analyze(prog)
	if success {
		t.Fatal("expected semantic analysis to fail due to missing 'flow main'")
	}

	foundError := false
	for _, err := range analyzer.Errors() {
		if contains(err, "flow 'main' must be declared") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("expected error regarding missing flow main, got: %v", analyzer.Errors())
	}
}

func TestStage1UndefinedVariables(t *testing.T) {
	code := `
	flow main {
		x = y // y is not defined
		return x
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	analyzer := New(nil)
	globalTable := NewSymbolTable(nil)
	analyzer.runHeddleSemanticPass(prog, globalTable)

	if len(analyzer.Errors()) == 0 {
		t.Fatal("expected semantic error for undefined variable 'y'")
	}

	foundError := false
	for _, err := range analyzer.Errors() {
		if contains(err, "undefined identifier 'y'") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("expected error regarding undefined variable y, got: %v", analyzer.Errors())
	}
}

func TestStage1InvalidReturnPlacement(t *testing.T) {
	code := `
	import "io"
	return 42 // illegal return at global scope
	
	flow main {
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	analyzer := New(nil)
	globalTable := NewSymbolTable(nil)
	analyzer.runHeddleSemanticPass(prog, globalTable)

	if len(analyzer.Errors()) == 0 {
		t.Fatal("expected semantic error for illegal return placement")
	}

	foundError := false
	for _, err := range analyzer.Errors() {
		if contains(err, "return statement not allowed") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("expected error regarding return statement placement, got: %v", analyzer.Errors())
	}
}

func TestStage1ExtractImportsAndSymbols(t *testing.T) {
	code := `
	import "math"
	import "strings" str
	
	flow main {
		"test" | str.to_upper()
		10 | math.pow(2)
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	analyzer := New(nil)
	globalTable := NewSymbolTable(nil)
	importsUsed, symbolsNeeded := analyzer.runHeddleSemanticPass(prog, globalTable)

	if len(analyzer.Errors()) > 0 {
		t.Fatalf("expected no semantic errors, got: %v", analyzer.Errors())
	}

	// Verificar imports
	if importsUsed["math"] != "math" {
		t.Errorf("expected 'math' -> 'math', got: %v", importsUsed["math"])
	}
	if importsUsed["str"] != "strings" {
		t.Errorf("expected 'str' -> 'strings', got: %v", importsUsed["str"])
	}

	// Verificar símbolos requisitados
	mathNeeded := symbolsNeeded["math"]
	if len(mathNeeded) != 1 || mathNeeded[0] != "pow" {
		t.Errorf("expected 'math' to need ['pow'], got: %v", mathNeeded)
	}

	strNeeded := symbolsNeeded["strings"]
	if len(strNeeded) != 1 || strNeeded[0] != "to_upper" {
		t.Errorf("expected 'strings' to need ['to_upper'], got: %v", strNeeded)
	}
}

func TestStage2PackageNotFound(t *testing.T) {
	code := `
	import "net/http"
	flow main {
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()

	// Sem net/http no ambiente
	analyzer := New(map[string]any{})
	success := analyzer.Analyze(prog)

	if success {
		t.Fatal("expected semantic analysis to fail because package 'net/http' is missing")
	}

	foundError := false
	for _, err := range analyzer.Errors() {
		if contains(err, "package 'net/http'") && contains(err, "was not found") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("expected package missing error, got: %v", analyzer.Errors())
	}
}

type MathMock struct {
	Pow func(x, y float64) (float64, error)
}

func TestStage2SymbolNotFound(t *testing.T) {
	code := `
	import "math"
	flow main {
		10 | math.invalid_function()
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()

	available := map[string]any{
		"math": MathMock{}, // Sem a função 'invalid_function'
	}

	analyzer := New(available)
	success := analyzer.Analyze(prog)

	if success {
		t.Fatal("expected semantic analysis to fail because function 'invalid_function' is missing")
	}

	foundError := false
	for _, err := range analyzer.Errors() {
		if contains(err, "symbol 'invalid_function' not found") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("expected symbol missing error, got: %v", analyzer.Errors())
	}
}

func TestStage2ReflectionSuccess(t *testing.T) {
	code := `
	import "math"
	flow main {
		10 | math.pow(2)
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()

	mockPow := func(x, y float64) (float64, error) { return 0, nil }
	available := map[string]any{
		"math": MathMock{
			Pow: mockPow,
		},
	}

	analyzer := New(available)
	globalTable := NewSymbolTable(nil)
	importsUsed, symbolsNeeded := analyzer.runHeddleSemanticPass(prog, globalTable)

	goPkgs := make(map[string]*PackageSymbols)
	for _, path := range importsUsed {
		pkgSyms, err := analyzer.targetParser.ParsePackage(path, "", symbolsNeeded[path])
		if err != nil {
			t.Fatalf("ParsePackage error: %v", err)
		}
		goPkgs[path] = pkgSyms
	}

	mathPkg, exists := goPkgs["math"]
	if !exists {
		t.Fatal("expected 'math' package metadata to exist")
	}

	powFunc, exists := mathPkg.Funcs["Pow"]
	if !exists {
		t.Fatal("expected 'Pow' function metadata to exist (resolved from snake_case 'pow')")
	}

	if len(powFunc.ParamTypes) != 2 {
		t.Errorf("expected 2 parameters, got %d", len(powFunc.ParamTypes))
	}
	if powFunc.ParamTypes[0] != "float64" {
		t.Errorf("expected first parameter to be float64, got %v", powFunc.ParamTypes[0])
	}
	if !powFunc.IsScalarFn {
		t.Error("expected Pow to be recognized as a scalar function")
	}
}

func TestStage3ArgumentCountMismatch(t *testing.T) {
	code := `
	import "math"
	flow main {
		10 | math.pow(2, 3, 4) // Pow expects x (implicit) and y (explicit). Total 2 arguments. Here we passed 3 explicit + 1 implicit = 4.
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()

	mockPow := func(x, y float64) (float64, error) { return 0, nil }
	available := map[string]any{
		"math": MathMock{
			Pow: mockPow,
		},
	}

	analyzer := New(available)
	success := analyzer.Analyze(prog)

	if success {
		t.Fatal("expected semantic analysis to fail due to argument count mismatch")
	}

	foundError := false
	for _, err := range analyzer.Errors() {
		if contains(err, "argument count mismatch") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("expected argument count mismatch error, got: %v", analyzer.Errors())
	}
}

type ServerMock struct {
	Host string
	Port int
}

type HttpMock struct {
	NewServer func(host string, port int) (*ServerMock, error)
}

func TestStage3StructInitializerFieldMismatch(t *testing.T) {
	code := `
	import "net/http"
	flow main {
		server = http.server {
			host: "localhost",
			invalid_field: 8080
		}
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()

	available := map[string]any{
		"net/http": HttpMock{
			NewServer: func(host string, port int) (*ServerMock, error) { return nil, nil },
		},
	}

	analyzer := New(available)
	success := analyzer.Analyze(prog)

	if success {
		t.Fatal("expected semantic analysis to fail due to struct field mismatch")
	}

	foundError := false
	for _, err := range analyzer.Errors() {
		if contains(err, "unknown field") || contains(err, "not found") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("expected unknown field error, got: %v", analyzer.Errors())
	}
}

type StringsMock2 struct {
	ToUpper func(s string) string // returns string only, no error
}

func TestStage3HandlerOnNonErrorFunc(t *testing.T) {
	code := `
	import "strings"
	
	handler log_error(err) {
		return 0
	}
	
	flow main {
		"hello" | strings.to_upper() ? log_error
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()

	available := map[string]any{
		"strings": StringsMock2{
			ToUpper: func(s string) string { return s },
		},
	}

	analyzer := New(available)
	success := analyzer.Analyze(prog)

	if success {
		t.Fatal("expected semantic analysis to fail because ? handler is used on a function that does not return an error")
	}

	foundError := false
	for _, err := range analyzer.Errors() {
		if contains(err, "does not return an error") || contains(err, "cannot apply handler") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("expected error regarding handler on non-error func, got: %v", analyzer.Errors())
	}
}

func TestStage1InvalidReturnInGlobalMapper(t *testing.T) {
	code := `
	x = [1, 2, 3] | .items (item) {
		return 0 // illegal return inside mapper at global scope
	}

	flow main {
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	analyzer := New(nil)
	globalTable := NewSymbolTable(nil)
	analyzer.runHeddleSemanticPass(prog, globalTable)

	if len(analyzer.Errors()) == 0 {
		t.Fatal("expected semantic error for illegal return placement inside global mapper")
	}

	foundError := false
	for _, err := range analyzer.Errors() {
		if contains(err, "return statement not allowed") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("expected error regarding return statement placement, got: %v", analyzer.Errors())
	}
}

type ServerConfigMock struct {
	Timeout int
}

type ServerActualMock struct {
	Host string
}

type MultiStructMock struct {
	NewServer       func() (*ServerActualMock, error)
	NewServerConfig func() (*ServerConfigMock, error)
}

func TestStage2StructExactMatchResolution(t *testing.T) {
	code := `
	import "web"
	flow main {
		s = web.server {
			host: "localhost"
		}
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()

	available := map[string]any{
		"web": MultiStructMock{
			NewServer:       func() (*ServerActualMock, error) { return nil, nil },
			NewServerConfig: func() (*ServerConfigMock, error) { return nil, nil },
		},
	}

	analyzer := New(available)
	success := analyzer.Analyze(prog)

	// Se a resolução falhasse ou colidisse, teríamos erros semânticos (ex: campo inválido "host" se resolvesse para ServerConfigMock).
	if !success {
		t.Fatalf("expected semantic analysis to succeed, got errors: %v", analyzer.Errors())
	}
}

type HTTPHandlerMock struct {
	Host string
}

type CustomHTTPMock struct {
	NewHTTPHandler func() (*HTTPHandlerMock, error)
}

func TestStage2PascalSiglaConversion(t *testing.T) {
	code := `
	import "net/http"
	flow main {
		h = http.http_handler {
			host: "localhost"
		}
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()

	available := map[string]any{
		"net/http": CustomHTTPMock{
			NewHTTPHandler: func() (*HTTPHandlerMock, error) { return nil, nil },
		},
	}

	analyzer := New(available)
	success := analyzer.Analyze(prog)

	if !success {
		t.Fatalf("expected semantic analysis to succeed with HTTP sigla conversion, got errors: %v", analyzer.Errors())
	}
}

func contains(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr || find(str, substr) >= 0)
}

func find(str, substr string) int {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestHeddleTOMLParser(t *testing.T) {
	tomlContent := `
# project settings
[ project ]
name = "test_project"
version = "1.0.0"

[ languages.golang ]
path = "pkg/go"

[languages.python]
path = "pkg/python"
`
	tmpfile, err := os.CreateTemp("", "heddle_test_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(tomlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfigFromFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to parse TOML: %v", err)
	}

	if cfg.Name != "test_project" {
		t.Errorf("expected project name to be 'test_project', got '%s'", cfg.Name)
	}
	if cfg.Languages["golang"] != "pkg/go" {
		t.Errorf("expected golang path to be 'pkg/go', got '%s'", cfg.Languages["golang"])
	}
	if cfg.Languages["python"] != "pkg/python" {
		t.Errorf("expected python path to be 'pkg/python', got '%s'", cfg.Languages["python"])
	}
}

func TestDefaultStdlibResolution(t *testing.T) {
	tomlContent := `
[project]
name = "default_test_proj"
version = "0.0.1"
`
	tmpfile, err := os.CreateTemp("", "heddle_test_default_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(tomlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfigFromFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to parse TOML: %v", err)
	}

	// Verify that golang is not in the config map
	if _, exists := cfg.Languages["golang"]; exists {
		t.Errorf("expected golang to not be in the config map")
	}

	// But ResolveLocalPackagePath should still resolve it to pkg/lib
	resolved, ok := cfg.ResolveLocalPackagePath("/mock/root", "golang", "math")
	if !ok {
		t.Errorf("expected ResolveLocalPackagePath to succeed for golang")
	}
	expected := filepath.Join("/mock/root", "pkg/lib", "math")
	if resolved != expected {
		t.Errorf("expected resolved path to be %q, got %q", expected, resolved)
	}
}

func TestTypePropagation(t *testing.T) {
	code := `
	import "math"
	import "strings"
	flow main {
		x = 10 | math.pow(2)
		y = "test" | strings.to_upper()
		return 0
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	analyzer := NewWithParser(&GoASTParser{})
	success := analyzer.Analyze(prog)
	if !success {
		t.Fatalf("expected semantic analysis to succeed with GoASTParser, got errors: %v", analyzer.Errors())
	}

	var mainFlow *ast.FlowStatement
	for _, stmt := range prog.Statements {
		if f, ok := stmt.(*ast.FlowStatement); ok && f.Name.Value == "main" {
			mainFlow = f
			break
		}
	}
	if mainFlow == nil {
		t.Fatal("expected flow main to be found")
	}

	var assignX *ast.AssignExpression
	var assignY *ast.AssignExpression
	for _, s := range mainFlow.Body.Statements {
		if exprStmt, ok := s.(*ast.ExpressionStatement); ok {
			if ae, ok := exprStmt.Expression.(*ast.AssignExpression); ok {
				if ae.Name.Value == "x" {
					assignX = ae
				} else if ae.Name.Value == "y" {
					assignY = ae
				}
			}
		}
	}

	if assignX == nil {
		t.Fatal("expected assignment to 'x' to be found")
	}
	if assignY == nil {
		t.Fatal("expected assignment to 'y' to be found")
	}

	types := analyzer.InferredTypes()

	tX := types[assignX]
	if tX == nil {
		t.Fatal("expected 'x' assignment to have InferredType")
	}
	if tX.Kind != "scalar" || tX.Name != "float64" {
		t.Errorf("expected 'x' to have type scalar float64, got %s %s", tX.Kind, tX.Name)
	}

	tIdentX := types[assignX.Name]
	if tIdentX == nil {
		t.Fatal("expected 'x' identifier to have InferredType")
	}
	if tIdentX.Kind != "scalar" || tIdentX.Name != "float64" {
		t.Errorf("expected 'x' identifier to have type scalar float64, got %s %s", tIdentX.Kind, tIdentX.Name)
	}

	tY := types[assignY]
	if tY == nil {
		t.Fatal("expected 'y' assignment to have InferredType")
	}
	if tY.Kind != "scalar" || tY.Name != "string" {
		t.Errorf("expected 'y' to have type scalar string, got %s %s", tY.Kind, tY.Name)
	}
}

func TestLocalPackageValidation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "heddle_proj_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tomlContent := `
[project]
name = "local_test_proj"
version = "0.0.1"

[languages.golang]
path = "my_go_code"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "heddle.toml"), []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	goCodePath := filepath.Join(tmpDir, "my_go_code", "calc")
	if err := os.MkdirAll(goCodePath, 0755); err != nil {
		t.Fatal(err)
	}

	calcGoContent := `package calc

func Add(a, b float64) float64 {
	return a + b
}
`
	if err := os.WriteFile(filepath.Join(goCodePath, "calc.go"), []byte(calcGoContent), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	code := `
	import "calc"
	flow main {
		r = 10 | calc.add(20)
		return r
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	analyzer := NewWithParser(&GoASTParser{})
	success := analyzer.Analyze(prog)
	if !success {
		t.Fatalf("expected semantic analysis to succeed with local package calc, got errors: %v", analyzer.Errors())
	}
}

func TestStructQualifiedType(t *testing.T) {
	// Testa se isStructTypeName reconhece tipos qualificados (ex: *calc.Result ou calc.Result)
	if !isStructTypeName("calc.Result") {
		t.Error("expected calc.Result to be recognized as a struct type name")
	}
	if !isStructTypeName("*calc.Result") {
		t.Error("expected *calc.Result to be recognized as a struct type name")
	}
	if !isStructTypeName("[]*calc.Result") {
		t.Error("expected []*calc.Result to be recognized as a struct type name")
	}
	if isStructTypeName("string") {
		t.Error("expected string to not be recognized as a struct type name")
	}
}

func TestSnakeCaseTypeRegistration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "heddle_proj_snake_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tomlContent := `
[project]
name = "snake_test_proj"
version = "0.0.1"

[languages.golang]
path = "my_go_code"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "heddle.toml"), []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	goCodePath := filepath.Join(tmpDir, "my_go_code", "calc")
	if err := os.MkdirAll(goCodePath, 0755); err != nil {
		t.Fatal(err)
	}

	calcGoContent := `package calc

type UserProfile struct {
	Name string
}

func NewUserProfile(name string) *UserProfile {
	return &UserProfile{Name: name}
}
`
	if err := os.WriteFile(filepath.Join(goCodePath, "calc.go"), []byte(calcGoContent), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	// Testamos a inicialização usando a chave snake_case do struct (user_profile)
	code := `
	import "calc"
	flow main {
		u = calc.user_profile {
			name: "Alice"
		}
		return u
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	analyzer := NewWithParser(&GoASTParser{})
	success := analyzer.Analyze(prog)
	if !success {
		t.Fatalf("expected semantic analysis to succeed with snake_case type registration, got errors: %v", analyzer.Errors())
	}
}

func TestTuplePipelineReceiverSemanticError(t *testing.T) {
	code := `
	flow main {
		in = { a: "1" }
		res = in | (in, in)
		return res
	}
	`
	p := parseCode(t, code)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	analyzer := New(nil)
	success := analyzer.Analyze(prog)
	if success {
		t.Fatal("expected semantic analysis to fail when a tuple is used as a pipeline receiver")
	}

	foundError := false
	for _, err := range analyzer.Errors() {
		if contains(err, "tuple expression cannot be used as a pipeline receiver") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("expected error about tuple pipeline receiver, got: %v", analyzer.Errors())
	}
}

