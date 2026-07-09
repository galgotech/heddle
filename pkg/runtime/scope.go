package runtime

import (
	"sync"
)

// Scope representa um escopo de variáveis locais seguro para concorrência
type Scope struct {
	mu     sync.RWMutex
	parent *Scope
	values map[string]any
}

func NewScope(parent *Scope) *Scope {
	return &Scope{
		parent: parent,
		values: make(map[string]any),
	}
}

// Get resolve recursivamente uma variável subindo a árvore de escopos
func (s *Scope) Get(name string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, exists := s.values[name]
	if exists {
		return val, true
	}
	if s.parent != nil {
		return s.parent.Get(name)
	}
	return nil, false
}

// Set define ou atualiza uma variável no escopo atual
func (s *Scope) Set(name string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[name] = val
}
