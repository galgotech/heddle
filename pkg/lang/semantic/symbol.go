package semantic

import "heddle/pkg/lang/ast"

type SymbolKind string

const (
	KindVar     SymbolKind = "variable"
	KindParam   SymbolKind = "parameter"
	KindFlow    SymbolKind = "flow"
	KindHandler SymbolKind = "handler"
	KindPackage SymbolKind = "package"
)

type Symbol struct {
	Name string
	Kind SymbolKind
	Node ast.Node
}

type SymbolTable struct {
	outer   *SymbolTable
	symbols map[string]*Symbol
}

func NewSymbolTable(outer *SymbolTable) *SymbolTable {
	return &SymbolTable{
		outer:   outer,
		symbols: make(map[string]*Symbol),
	}
}

func (st *SymbolTable) Define(name string, kind SymbolKind, node ast.Node) bool {
	if _, exists := st.symbols[name]; exists {
		return false
	}
	st.symbols[name] = &Symbol{Name: name, Kind: kind, Node: node}
	return true
}

func (st *SymbolTable) Resolve(name string) (*Symbol, bool) {
	sym, exists := st.symbols[name]
	if exists {
		return sym, true
	}
	if st.outer != nil {
		return st.outer.Resolve(name)
	}
	return nil, false
}
