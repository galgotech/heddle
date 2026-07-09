package ast

// TypeInfo representa as informações de tipagem resolvidas/propagadas durante a análise semântica
type TypeInfo struct {
	Kind    string // "scalar", "slice", "map", "struct", "generic", "error"
	Name    string // ex: "float64", "string", "User"
	Package string // ex: "net/http" ou "entities" (opcional)
}
