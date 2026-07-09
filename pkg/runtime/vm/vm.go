package vm

import (
	"context"
	"fmt"
	"sync"

	"heddle/pkg/lang/ir"
	"heddle/pkg/runtime"
	"heddle/pkg/runtime/bridge"
)

type VM struct {
	program      *ir.Program
	bridge       bridge.GoBridge
	globals      *runtime.Scope
	flows        map[string]*ir.Routine
	handlers     map[string]*ir.Routine
	activeEvents int
	mu           sync.Mutex

	// Decoupled sub-systems
	functions *functionScheduler
	executor  *instructionExecutor
	matcher   *patternMatcher
	mapper    *mapper
	reflector *reflectionInvoker
	merger    *tupleMerger
}

func NewVM(prog *ir.Program, b bridge.GoBridge) *VM {
	if b == nil {
		panic("NewVM: bridge is required (cannot be nil)")
	}
	v := &VM{
		program:  prog,
		bridge:   b,
		globals:  runtime.NewScope(nil),
		flows:    prog.Flows,
		handlers: prog.Handlers,
	}

	// Initialize default components
	v.functions = &functionScheduler{vm: v}
	v.executor = &instructionExecutor{vm: v}
	v.matcher = &patternMatcher{vm: v}
	v.mapper = &mapper{vm: v}
	v.reflector = &reflectionInvoker{vm: v}
	v.merger = &tupleMerger{vm: v}

	return v
}

func (v *VM) incrementActiveEvents() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.activeEvents++
}

func (v *VM) decrementActiveEvents() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.activeEvents--
}

// Run executa as declarações globais e em seguida inicia o fluxo principal 'main'
func (v *VM) Run(ctx context.Context) (*runtime.Frame, error) {
	defer v.cleanup()
	// 1. Executar as declarações e inicializações globais (como instanciar servidores HTTP)
	if len(v.program.Globals) > 0 {
		_, err := v.executor.ExecuteInstructions(ctx, v.program.Globals, v.globals)
		if err != nil {
			return nil, fmt.Errorf("global initialization error: %w", err)
		}
	}

	// 2. Localizar e executar o fluxo principal 'main'
	_, exists := v.flows["main"]
	if !exists {
		return nil, fmt.Errorf("runtime error: flow 'main' not found")
	}

	emptyInput := runtime.NewFrame([]any{})
	res, err := v.ExecuteFlow(ctx, "main", emptyInput)
	if err != nil {
		return nil, err
	}

	// 3. Se houver loops de timer/callbacks ativos, manter a VM rodando
	v.mu.Lock()
	hasEvents := v.activeEvents > 0
	v.mu.Unlock()

	if hasEvents {
		// Bloqueia aguardando cancelamento do contexto
		<-ctx.Done()
	}

	return res, nil
}

// ExecuteFlow cria um novo escopo local e executa os passos do fluxo
func (v *VM) ExecuteFlow(ctx context.Context, flowName string, input *runtime.Frame) (*runtime.Frame, error) {
	flow, exists := v.flows[flowName]
	if !exists {
		return nil, fmt.Errorf("undefined flow '%s'", flowName)
	}

	// Novo escopo local para o fluxo
	localScope := runtime.NewScope(v.globals)
	if flow.ParamName != "" {
		localScope.Set(flow.ParamName, input)
	}

	res, err := v.functions.ExecuteFunctions(ctx, flow.Functions, localScope)
	if err != nil {
		if flow.HandlerName != "" {
			return v.ExecuteHandler(ctx, flow.HandlerName, err)
		}
		panic(err)
	}
	return res, nil
}

// ExecuteHandler executa um tratador de erro passando o erro como parâmetro
func (v *VM) ExecuteHandler(ctx context.Context, handlerName string, err error) (*runtime.Frame, error) {
	handler, exists := v.handlers[handlerName]
	if !exists {
		return nil, fmt.Errorf("undefined handler '%s'", handlerName)
	}

	localScope := runtime.NewScope(v.globals)
	if handler.ParamName != "" {
		// Envelopa o erro em um Frame
		localScope.Set(handler.ParamName, runtime.NewScalarFrame(err.Error()))
	}

	return v.functions.ExecuteFunctions(ctx, handler.Functions, localScope)
}

func (v *VM) cleanup() {
}
