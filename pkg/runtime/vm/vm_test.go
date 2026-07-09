package vm

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"heddle/pkg/lang/compiler"
	"heddle/pkg/lang/lexer"
	"heddle/pkg/lang/parser"
	"heddle/pkg/lang/semantic"
	"heddle/pkg/runtime"
	"heddle/pkg/runtime/bridge/reflection"
)

type MockTestPkg struct {
	mu      sync.Mutex
	LastVal any
}

func (m *MockTestPkg) Store(val any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastVal = val
	return nil
}

func (m *MockTestPkg) Pow(x float64, y float64) float64 {
	return math.Pow(x, y)
}

func (m *MockTestPkg) Divide(x float64, y float64) (float64, error) {
	if y == 0 {
		return 0, errors.New("division by zero")
	}
	return x / y, nil
}

func (m *MockTestPkg) PanicFunc() (string, error) {
	panic("native panic test trigger")
}

func compileAndRun(t *testing.T, code string, mockPkg *MockTestPkg) (*runtime.Frame, error) {
	l := lexer.New(code)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("syntax errors: %v", p.Errors())
	}

	// Registrar o mock nas dependências semânticas
	available := make(map[string]any)
	for k, v := range runtime.PackagesRegistry {
		available[k] = v
		parts := strings.Split(k, "/")
		alias := parts[len(parts)-1]
		available[alias] = v
	}
	available["test_pkg"] = mockPkg

	analyzer := semantic.New(available)
	if !analyzer.Analyze(prog) {
		t.Fatalf("semantic errors: %v", analyzer.Errors())
	}

	comp := compiler.NewCompiler()
	irProg, err := comp.Compile(prog)
	if err != nil {
		t.Fatalf("compiler error: %v", err)
	}

	// Criar dispatcher personalizado com o mockPkg registrado
	metadata := make(map[string]runtime.FuncMetadata)
	for k, v := range runtime.StdlibMetadataRegistry {
		metadata[k] = v
	}
	metadata["test_pkg.store"] = runtime.FuncMetadata{ParamNames: []string{"val"}}
	metadata["test_pkg.pow"] = runtime.FuncMetadata{ParamNames: []string{"x", "y"}}
	metadata["test_pkg.divide"] = runtime.FuncMetadata{ParamNames: []string{"x", "y"}}
	metadata["test_pkg.panic_func"] = runtime.FuncMetadata{ParamNames: []string{}}

	disp := reflection.NewReflectionBridge(available, metadata)
	interpreter := NewVM(irProg, disp)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return interpreter.Run(ctx)
}

func TestInterpreterSimplePipeline(t *testing.T) {
	code := `
	import "strings"
	import "test_pkg"

	flow main {
		"heddle language" | strings.to_upper() | test_pkg.store()
	}
	`
	mock := &MockTestPkg{}
	_, err := compileAndRun(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.LastVal != "HEDDLE LANGUAGE" {
		t.Errorf("expected LastVal to be 'HEDDLE LANGUAGE', got '%v'", mock.LastVal)
	}
}

func TestInterpreterNamedAndOmittedParams(t *testing.T) {
	// Testar mapeamento dinâmico de parâmetros omitidos obtidos da estrutura da linha de entrada
	code := `
	import "test_pkg"

	flow main {
		// x: 3 será omitido e resolvido a partir do frame de entrada {x: 3}
		input = { x: 3 }
		input | test_pkg.pow(y: 2) | test_pkg.store()
	}
	`
	mock := &MockTestPkg{}
	_, err := compileAndRun(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.LastVal != 9.0 {
		t.Errorf("expected LastVal to be 9.0, got '%v'", mock.LastVal)
	}
}

func TestInterpreterFullOmittedParams(t *testing.T) {
	// Testar mapeamento onde ambos parâmetros são omitidos e resolvidos da linha
	code := `
	import "test_pkg"

	flow main {
		input = { x: 3, y: 3 }
		input | test_pkg.pow() | test_pkg.store()
	}
	`
	mock := &MockTestPkg{}
	_, err := compileAndRun(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.LastVal != 27.0 {
		t.Errorf("expected LastVal to be 27.0, got '%v'", mock.LastVal)
	}
}

func TestInterpreterFunctionErrorHandling(t *testing.T) {
	code := `
	import "test_pkg"

	handler division_error (err) {
		return 999.0
	}

	flow main {
		// 10 dividido por 0 gera erro, que deve ser capturado pelo handler local
		10 | test_pkg.divide(y: 0) ? division_error() | test_pkg.store()
	}
	`
	mock := &MockTestPkg{}
	_, err := compileAndRun(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.LastVal != 999.0 {
		t.Errorf("expected error handler to recover to 999.0, got '%v'", mock.LastVal)
	}
}

func TestInterpreterPatternMatching(t *testing.T) {
	code := `
	import "cmp"
	import "test_pkg"

	flow main {
		// Comparar 10 com 20 deve retornar a tag 'less'
		10 | cmp.compare(20) {
			less(val) {
				"LESS_TAG" | test_pkg.store()
				return val
			}
			equals(val) {
				"EQUALS_TAG" | test_pkg.store()
				return val
			}
			greater(val) {
				"GREATER_TAG" | test_pkg.store()
				return val
			}
		}
	}
	`
	mock := &MockTestPkg{}
	_, err := compileAndRun(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.LastVal != "LESS_TAG" {
		t.Errorf("expected match tag output to be 'LESS_TAG', got '%v'", mock.LastVal)
	}
}

func TestInterpreterDAGConcurrency(t *testing.T) {
	// Verificar que passos sem dependências de dados podem ser executados
	code := `
	import "test_pkg"

	flow main {
		a = 5
		b = 10
		c = a | test_pkg.pow(y: 2) // depende de a
		d = b | test_pkg.pow(y: 2) // depende de b
		// c e d podem ser calculados em paralelo
		result = (c, d) | test_pkg.store()
	}
	`
	mock := &MockTestPkg{}
	_, err := compileAndRun(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	// O mock.LastVal deve ser o resultado consolidado da tupla paralela:
	// c = 25.0, d = 100.0. A consolidação de tuplas gera uma linha contendo val0 e val1
	m, ok := mock.LastVal.(map[string]any)
	if !ok {
		t.Fatalf("expected LastVal to be consolidated tuple map, got %T: %v", mock.LastVal, mock.LastVal)
	}

	val0List, ok0 := m["val0"].([]any)
	val1List, ok1 := m["val1"].([]any)
	if !ok0 || !ok1 || len(val0List) != 1 || len(val1List) != 1 || val0List[0] != 25.0 || val1List[0] != 100.0 {
		t.Errorf("expected consolidated values val0:[25.0], val1:[100.0], got val0:%v, val1:%v", m["val0"], m["val1"])
	}
}

func TestInterpreterFlowErrorHandling(t *testing.T) {
	code := `
	import "test_pkg"

	handler flow_error (err) {
		return 888.0
	}

	flow main ? flow_error {
		10 | test_pkg.divide(y: 0) | test_pkg.store()
	}
	`
	mock := &MockTestPkg{}
	res, err := compileAndRun(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	if len(res.Rows) == 0 || res.Rows[0] != 888.0 {
		t.Errorf("expected flow handler recovery result to be 888.0, got %v", res)
	}
}

func TestInterpreterUnhandledErrorPanic(t *testing.T) {
	code := `
	import "test_pkg"

	flow main {
		10 | test_pkg.divide(y: 0)
	}
	`
	mock := &MockTestPkg{}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected execution to panic, but it did not")
		} else {
			t.Logf("Recovered panic as expected: %v", r)
		}
	}()

	_, _ = compileAndRun(t, code, mock)
}

func TestInterpreterAutonomousMapper(t *testing.T) {
	code := `
	import "test_pkg"

	flow main {
		payload = { cart: { items: { price: 10.0 } } }
		res = payload.cart.items.price (p) {
			p | test_pkg.divide(y: 2.0)
		}
		res | test_pkg.store()
	}
	`
	mock := &MockTestPkg{}
	_, err := compileAndRun(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	resMap, ok := mock.LastVal.(map[string]any)
	if !ok {
		t.Fatalf("expected LastVal to be map, got %T: %v", mock.LastVal, mock.LastVal)
	}
	cartList, ok := resMap["cart"].([]any)
	if !ok || len(cartList) != 1 {
		t.Fatalf("expected cart to be list of 1 element, got %T", resMap["cart"])
	}
	cart, ok := cartList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected cart element to be map, got %T", cartList[0])
	}
	itemsList, ok := cart["items"].([]any)
	if !ok || len(itemsList) != 1 {
		t.Fatalf("expected items to be list of 1 element, got %T", cart["items"])
	}
	items, ok := itemsList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected items element to be map, got %T", itemsList[0])
	}
	priceList, ok := items["price"].([]any)
	if !ok || len(priceList) != 1 || priceList[0] != 5.0 {
		t.Errorf("expected price to be [5.0], got %v", items["price"])
	}
}

func TestInterpreterPipedMapper(t *testing.T) {
	code := `
	import "test_pkg"

	flow main {
		payload = { cart: { items: { price: 10.0 } } }
		res = payload | .cart.items.price (p) {
			p | test_pkg.divide(y: 2.0)
		}
		res | test_pkg.store()
	}
	`
	mock := &MockTestPkg{}
	_, err := compileAndRun(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	resMap, ok := mock.LastVal.(map[string]any)
	if !ok {
		t.Fatalf("expected LastVal to be map, got %T: %v", mock.LastVal, mock.LastVal)
	}
	cartList, ok := resMap["cart"].([]any)
	if !ok || len(cartList) != 1 {
		t.Fatalf("expected cart to be list of 1 element, got %T", resMap["cart"])
	}
	cart, ok := cartList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected cart element to be map, got %T", cartList[0])
	}
	itemsList, ok := cart["items"].([]any)
	if !ok || len(itemsList) != 1 {
		t.Fatalf("expected items to be list of 1 element, got %T", cart["items"])
	}
	items, ok := itemsList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected items element to be map, got %T", itemsList[0])
	}
	priceList, ok := items["price"].([]any)
	if !ok || len(priceList) != 1 || priceList[0] != 5.0 {
		t.Errorf("expected price to be [5.0], got %v", items["price"])
	}
}

func TestInterpreterTupleJoinDeepMerge(t *testing.T) {
	code := `
	import "test_pkg"

	flow main {
		user_base = {
			id: 101.0,
			profile: {
				name: "John Doe",
				role: "user",
				location: { city: "New York", country: "US" }
			},
			settings: { theme: "light", notifications: "true" }
		}

		user_updates = {
			profile: {
				name: "Johnathan Doe",
				location: { city: "Boston" }
			},
			settings: { theme: "dark" },
			active: "true"
		}

		res = (user_base, user_updates)
		res | test_pkg.store()
	}
	`
	mock := &MockTestPkg{}
	_, err := compileAndRun(t, code, mock)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	resMap, ok := mock.LastVal.(map[string]any)
	if !ok {
		t.Fatalf("expected LastVal to be map, got %T: %v", mock.LastVal, mock.LastVal)
	}

	// Verify deep merged properties
	idVal, ok := resMap["id"].([]any)
	if !ok || len(idVal) != 1 || idVal[0] != 101.0 {
		t.Errorf("expected id to be [101.0], got %v", resMap["id"])
	}
	activeVal, ok := resMap["active"].([]any)
	if !ok || len(activeVal) != 1 || activeVal[0] != "true" {
		t.Errorf("expected active to be ['true'], got %v", resMap["active"])
	}

	profileList, ok := resMap["profile"].([]any)
	if !ok || len(profileList) != 1 {
		t.Fatalf("expected profile to be list of 1 element, got %T", resMap["profile"])
	}
	profile, ok := profileList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected profile element to be map, got %T", profileList[0])
	}

	nameVal, ok := profile["name"].([]any)
	if !ok || len(nameVal) != 1 || nameVal[0] != "Johnathan Doe" {
		t.Errorf("expected profile.name to be ['Johnathan Doe'], got %v", profile["name"])
	}
	roleVal, ok := profile["role"].([]any)
	if !ok || len(roleVal) != 1 || roleVal[0] != "user" {
		t.Errorf("expected profile.role to be ['user'], got %v", profile["role"])
	}

	locationList, ok := profile["location"].([]any)
	if !ok || len(locationList) != 1 {
		t.Fatalf("expected profile.location to be list of 1 element, got %T", profile["location"])
	}
	location, ok := locationList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected profile.location element to be map, got %T", locationList[0])
	}

	cityVal, ok := location["city"].([]any)
	if !ok || len(cityVal) != 1 || cityVal[0] != "Boston" {
		t.Errorf("expected profile.location.city to be ['Boston'], got %v", location["city"])
	}
	countryVal, ok := location["country"].([]any)
	if !ok || len(countryVal) != 1 || countryVal[0] != "US" {
		t.Errorf("expected profile.location.country to be ['US'], got %v", location["country"])
	}

	settingsList, ok := resMap["settings"].([]any)
	if !ok || len(settingsList) != 1 {
		t.Fatalf("expected settings to be list of 1 element, got %T", resMap["settings"])
	}
	settings, ok := settingsList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected settings element to be map, got %T", settingsList[0])
	}

	themeVal, ok := settings["theme"].([]any)
	if !ok || len(themeVal) != 1 || themeVal[0] != "dark" {
		t.Errorf("expected settings.theme to be ['dark'], got %v", settings["theme"])
	}
	notificationsVal, ok := settings["notifications"].([]any)
	if !ok || len(notificationsVal) != 1 || notificationsVal[0] != "true" {
		t.Errorf("expected settings.notifications to be ['true'], got %v", settings["notifications"])
	}
}

func TestInterpreterImportAliases(t *testing.T) {
	mockPkg := &MockTestPkg{}
	code := `
	import "test_pkg" my_pkg

	flow main {
		12.5 | my_pkg.store()
	}
	`
	_, err := compileAndRun(t, code, mockPkg)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	mockPkg.mu.Lock()
	defer mockPkg.mu.Unlock()
	if mockPkg.LastVal != 12.5 {
		t.Errorf("expected mockPkg.LastVal to be 12.5, got %v", mockPkg.LastVal)
	}
}

