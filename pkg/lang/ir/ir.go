package ir

// OpCode representa os códigos de operação da máquina virtual Heddle
type OpCode string

const (
	OpLoadConst   OpCode = "LOAD_CONST"   // Carrega um literal básico na pilha de avaliação
	OpLoadVar     OpCode = "LOAD_VAR"     // Carrega o valor de uma variável local ou parâmetro do escopo
	OpStoreVar    OpCode = "STORE_VAR"    // Retira o valor do topo da pilha e salva em uma variável local
	OpPropAccess  OpCode = "PROP_ACCESS"  // Resolve um caminho de atributos (ex: .cart.items.price)
	OpBuildStruct OpCode = "BUILD_STRUCT" // Instancia uma estrutura ou servidor Go
	OpBuildMap    OpCode = "BUILD_MAP"    // Cria um mapa/dicionário a partir de chaves e valores na pilha
	OpBuildArray  OpCode = "BUILD_ARRAY"  // Cria um array/lista a partir de elementos da pilha
	OpCall        OpCode = "CALL"         // Invoca uma função nativa ou fluxo
	OpMatch       OpCode = "MATCH"        // Desvia a execução avaliando tags e payloads
	OpSpawn       OpCode = "SPAWN"        // Executa múltiplos blocos concorrentemente (Split/Join)
	OpMapper      OpCode = "MAPPER"       // Executa um mapeador transformador em propriedades de containers
	OpFunctionHandler OpCode = "FUNCTION_HANDLER" // Executa um bloco monitorado por um tratador local
	OpReturn          OpCode = "RETURN"           // Encerra a execução do bloco atual retornando o topo da pilha
)

// Instruction representa o contrato comum de todas as instruções da IR linearizada
type Instruction interface {
	OpCode() OpCode
}

// LoadConst carrega um literal básico na pilha de avaliação
type LoadConst struct {
	Value any `json:"value"`
}
func (LoadConst) OpCode() OpCode { return OpLoadConst }

// LoadVar carrega o valor de uma variável local ou parâmetro do escopo
type LoadVar struct {
	Name string `json:"name"`
}
func (LoadVar) OpCode() OpCode { return OpLoadVar }

// StoreVar retira o valor do topo da pilha e salva em uma variável local
type StoreVar struct {
	Name string `json:"name"`
}
func (StoreVar) OpCode() OpCode { return OpStoreVar }

// PropAccess resolve um caminho de atributos (ex: .cart.items.price)
type PropAccess struct {
	Path []string `json:"path"`
}
func (PropAccess) OpCode() OpCode { return OpPropAccess }

// BuildStruct instancia uma estrutura ou servidor Go
type BuildStruct struct {
	Info StructInfo `json:"info"`
}
func (BuildStruct) OpCode() OpCode { return OpBuildStruct }

// BuildMap cria um mapa/dicionário a partir de chaves e valores na pilha
type BuildMap struct {
	Size int `json:"size"`
}
func (BuildMap) OpCode() OpCode { return OpBuildMap }

// BuildArray cria um array/lista a partir de elementos da pilha
type BuildArray struct {
	Size int `json:"size"`
}
func (BuildArray) OpCode() OpCode { return OpBuildArray }

// Call invoca uma função nativa ou fluxo
type Call struct {
	Info CallInfo `json:"info"`
}
func (Call) OpCode() OpCode { return OpCall }

// Match desvia a execução avaliando tags e payloads
type Match struct {
	Info MatchInfo `json:"info"`
}
func (Match) OpCode() OpCode { return OpMatch }

// Spawn executa múltiplos blocos concorrentemente (Split/Join)
type Spawn struct {
	Info SpawnInfo `json:"info"`
}
func (Spawn) OpCode() OpCode { return OpSpawn }

// Mapper executa um mapeador transformador em propriedades de containers
type Mapper struct {
	Info MapperInfo `json:"info"`
}
func (Mapper) OpCode() OpCode { return OpMapper }

// FunctionHandler executa um bloco monitorado por um tratador local
type FunctionHandler struct {
	Info FunctionHandlerInfo `json:"info"`
}
func (FunctionHandler) OpCode() OpCode { return OpFunctionHandler }

// Return encerra a execução do bloco atual retornando o topo da pilha
type Return struct{}
func (Return) OpCode() OpCode { return OpReturn }


// Function representa um grupo de instruções correspondentes a um passo/função lógico (geralmente uma declaração/statement)
type Function struct {
	ID           int           `json:"id"`
	Instructions []Instruction `json:"instructions"`
	Reads        []string      `json:"reads"`  // Variáveis lidas pelo passo
	Writes       []string      `json:"writes"` // Variáveis escritas/alteradas pelo passo
}

// Program é a representação compilada completa de um arquivo/programa Heddle
type Program struct {
	Globals  []Instruction       `json:"globals"`  // Instruções de inicialização global
	Flows    map[string]*Routine `json:"flows"`    // Fluxos declarados
	Handlers map[string]*Routine `json:"handlers"` // Tratadores de erros declarados
}

// Routine representa uma rotina executável (flow ou handler)
type Routine struct {
	Name        string     `json:"name"`
	ParamName   string     `json:"param_name,omitempty"`   // Nome do parâmetro de entrada (ex: "in", "err")
	HandlerName string     `json:"handler_name,omitempty"` // Nome do handler de erro associado ao fluxo
	Functions   []Function `json:"functions"`
}

// CallInfo contém argumentos de metadados para chamadas de funções
type CallInfo struct {
	FuncName    string   `json:"func_name"`
	ArgNames    []string `json:"arg_names,omitempty"` // Nomes dos argumentos (nil se posicional)
	NumArgs     int      `json:"num_args"`            // Quantidade de argumentos passados
	HasReceiver bool     `json:"has_receiver"`        // Indica se recebe valor do pipe anterior
}

// StructInfo contém dados para instanciação dinâmica de structs
type StructInfo struct {
	TypeName   string   `json:"type_name"`
	FieldNames []string `json:"field_names"`
}

// MatchInfo descreve branches de casamento de padrões
type MatchInfo struct {
	Cases []MatchCase `json:"cases"`
}

// MatchCase descreve um único ramo do match
type MatchCase struct {
	Tag          string        `json:"tag"`
	ParamName    string        `json:"param_name"`
	Instructions []Instruction `json:"instructions"`
}

// SpawnInfo contém caminhos paralelos para execução concorrente
type SpawnInfo struct {
	Branches [][]Instruction `json:"branches"`
}

// MapperInfo contém parâmetros de mapeadores de mutação
type MapperInfo struct {
	Path         []string      `json:"path"`
	Alias        string        `json:"alias"`
	Instructions []Instruction `json:"instructions"`
}

// FunctionHandlerInfo contém os dados de tratador de erros de passo
type FunctionHandlerInfo struct {
	Body        []Instruction `json:"body"`
	HandlerName string        `json:"handler_name"`
}
