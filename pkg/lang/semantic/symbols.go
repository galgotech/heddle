package semantic

// FuncSymbol guarda a assinatura e informações semânticas de uma função da linguagem target
type FuncSymbol struct {
	Name           string
	Receiver       string // nome da struct a qual pertence, se for um método (opcional)
	ParamNames     []string
	ParamTypes     []string
	ReturnTypes    []string
	IsSliceFn      bool
	IsScalarFn     bool
	IsGenericFn    bool
	HasErrorReturn bool
	FilePath       string
	Line           int
	Col            int
}

// TypeSymbol representa uma estrutura/classe declarada na linguagem target
type TypeSymbol struct {
	Name     string
	Fields   map[string]string // Nome do campo -> Tipo do campo como string
	FilePath string
	Line     int
	Col      int
}

// PackageSymbols reúne todas as funções e tipos exportados de um pacote/módulo
type PackageSymbols struct {
	Path  string
	Funcs map[string]*FuncSymbol
	Types map[string]*TypeSymbol
}

// TargetParser define a interface comum para os parsers de qualquer linguagem (Go, Python, NodeJS, etc.)
type TargetParser interface {
	// ParsePackage analisa o pacote na pasta indicada ou na biblioteca padrão da linguagem,
	// retornando as assinaturas dos símbolos exportados que são necessários.
	ParsePackage(pkgPath string, targetDir string, symbolsNeeded []string) (*PackageSymbols, error)
}
