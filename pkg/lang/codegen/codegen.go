package codegen

import (
	"heddle/pkg/lang/ir"
)

// CodeGenerator define o contrato para transpiladores de código nativo das linguagens alvo
type CodeGenerator interface {
	// Generate converte o programa em IR linearizada para arquivos de código da linguagem destino
	// Retorna um mapa contendo [caminho_do_arquivo]conteudo_do_arquivo
	Generate(program *ir.Program) (map[string][]byte, error)
}
