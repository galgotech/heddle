package vm

import (
	"context"
	"fmt"
	"sync"

	"heddle/pkg/lang/ir"
	"heddle/pkg/runtime"
)

type functionScheduler struct {
	vm *VM
}

func (s *functionScheduler) ExecuteFunctions(ctx context.Context, functions []ir.Function, scope *runtime.Scope) (*runtime.Frame, error) {
	N := len(functions)
	if N == 0 {
		return runtime.NewFrame([]any{}), nil
	}

	// 1. Coleta de Dependências (Construir Grafo de Dependências)
	// deps[j] contém os índices de todas as funções i < j das quais j depende
	deps := make(map[int][]int)
	for j := 0; j < N; j++ {
		deps[j] = []int{}
		for k := 0; k < j; k++ {
			// Regras de dependência de dados
			hasDataDependency := false
			// Read-after-Write (RAW)
			for _, wVar := range functions[k].Writes {
				for _, rVar := range functions[j].Reads {
					if wVar == rVar {
						hasDataDependency = true
						break
					}
				}
			}
			// Write-after-Write (WAW)
			for _, wVar := range functions[k].Writes {
				for _, rVar := range functions[j].Writes {
					if wVar == rVar {
						hasDataDependency = true
						break
					}
				}
			}
			// Write-after-Read (WAR)
			for _, wVar := range functions[k].Reads {
				for _, rVar := range functions[j].Writes {
					if wVar == rVar {
						hasDataDependency = true
						break
					}
				}
			}

			// Se houver instruções de retorno, concorrência deve aguardar
			hasReturn := false
			for _, inst := range functions[k].Instructions {
				if inst.OpCode() == ir.OpReturn {
					hasReturn = true
					break
				}
			}

			if hasDataDependency || hasReturn {
				deps[j] = append(deps[j], k)
			}
		}
	}

	// 2. Execução Topológica por Camadas de DAG
	completed := make(map[int]bool)
	var completedMu sync.Mutex

	for len(completed) < N {
		var readyIndices []int
		completedMu.Lock()
		for j := 0; j < N; j++ {
			if completed[j] {
				continue
			}
			// Verificar se todas as dependências estão prontas
			allDepsDone := true
			for _, depIdx := range deps[j] {
				if !completed[depIdx] {
					allDepsDone = false
					break
				}
			}
			if allDepsDone {
				readyIndices = append(readyIndices, j)
			}
		}
		completedMu.Unlock()

		if len(readyIndices) == 0 {
			return nil, fmt.Errorf("deadlock or loop detected in flow functions execution graph")
		}

		// Executar funções da camada concorrente
		var wg sync.WaitGroup
		errChan := make(chan error, len(readyIndices))
		var resultFrame *runtime.Frame
		var resMu sync.Mutex

		for _, idx := range readyIndices {
			wg.Add(1)
			go func(funcIdx int) {
				defer wg.Done()
				function := functions[funcIdx]

				frame, err := s.vm.executor.ExecuteInstructions(ctx, function.Instructions, scope)
				if err != nil {
					errChan <- err
					return
				}

				if frame != nil {
					resMu.Lock()
					resultFrame = frame
					resMu.Unlock()
				}

				completedMu.Lock()
				completed[funcIdx] = true
				completedMu.Unlock()
			}(idx)
		}
		wg.Wait()

		// Propagar erro se houver
		select {
		case err := <-errChan:
			return nil, err
		default:
		}

		// Se alguma função executou um RETURN, interrompemos e retornamos o frame
		if resultFrame != nil {
			return resultFrame, nil
		}
	}

	return runtime.NewFrame([]any{}), nil
}
