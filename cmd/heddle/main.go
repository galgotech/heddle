package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"heddle/pkg/lang/compiler"
	"heddle/pkg/lang/lexer"
	"heddle/pkg/lang/lsp"
	"heddle/pkg/lang/parser"
	"heddle/pkg/lang/semantic"
	"heddle/pkg/runtime"
	"heddle/pkg/runtime/bridge/reflection"
	"heddle/pkg/runtime/vm"
)

var rootCmd = &cobra.Command{
	Use:   "heddle",
	Short: "Heddle is a declarative flow orchestration language compiler",
}

var checkCmd = &cobra.Command{
	Use:   "check [file.he]",
	Short: "Run semantic analysis on a heddle program file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := args[0]
		content, err := os.ReadFile(filename)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", filename, err)
			os.Exit(1)
		}

		l := lexer.New(string(content))
		p := parser.New(l)
		program := p.ParseProgram()

		if len(p.Errors()) > 0 {
			fmt.Printf("Found %d syntax error(s):\n", len(p.Errors()))
			for i, errStr := range p.Errors() {
				fmt.Printf("  %d. %s\n", i+1, errStr)
			}
			os.Exit(1)
		}

		analyzer := semantic.NewWithParser(&semantic.GoASTParser{})
		if !analyzer.Analyze(program) {
			fmt.Printf("Found %d error(s):\n", len(analyzer.DetailedErrors()))
			for i, err := range analyzer.DetailedErrors() {
				prefix := fmt.Sprintf("  %d. ", i+1)
				indent := strings.Repeat(" ", len(prefix))

				errStr := err.Format(filename, string(content))
				lines := strings.Split(errStr, "\n")
				for j, line := range lines {
					if j == 0 {
						fmt.Printf("%s%s\n", prefix, line)
					} else {
						fmt.Printf("%s%s\n", indent, line)
					}
				}
			}
			os.Exit(1)
		}

		fmt.Println("Success: Semantic check passed!")
	},
}

var runCmd = &cobra.Command{
	Use:   "run [file.he]",
	Short: "Execute a heddle program file using the VM interpreter",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := args[0]
		content, err := os.ReadFile(filename)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", filename, err)
			os.Exit(1)
		}

		l := lexer.New(string(content))
		p := parser.New(l)
		program := p.ParseProgram()

		if len(p.Errors()) > 0 {
			fmt.Printf("Found %d syntax error(s):\n", len(p.Errors()))
			for i, errStr := range p.Errors() {
				fmt.Printf("  %d. %s\n", i+1, errStr)
			}
			os.Exit(1)
		}

		analyzer := semantic.NewWithParser(&semantic.GoASTParser{})
		if !analyzer.Analyze(program) {
			fmt.Printf("Found %d error(s):\n", len(analyzer.DetailedErrors()))
			for i, err := range analyzer.DetailedErrors() {
				prefix := fmt.Sprintf("  %d. ", i+1)
				indent := strings.Repeat(" ", len(prefix))

				errStr := err.Format(filename, string(content))
				lines := strings.Split(errStr, "\n")
				for j, line := range lines {
					if j == 0 {
						fmt.Printf("%s%s\n", prefix, line)
					} else {
						fmt.Printf("%s%s\n", indent, line)
					}
				}
			}
			os.Exit(1)
		}

		// Compilação AST -> IR
		comp := compiler.NewCompiler()
		irProg, err := comp.Compile(program)
		if err != nil {
			fmt.Printf("Compilation error: %v\n", err)
			os.Exit(1)
		}

		// Execução da IR
		bridge := reflection.NewReflectionBridge(runtime.PackagesRegistry, runtime.StdlibMetadataRegistry)
		virtualMachine := vm.NewVM(irProg, bridge)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err = virtualMachine.Run(ctx)
		if err != nil {
			fmt.Printf("Runtime error: %v\n", err)
			os.Exit(1)
		}
	},
}

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Start the Heddle Language Server (LSP) on stdio",
	Run: func(cmd *cobra.Command, args []string) {
		if err := lsp.StartServer(); err != nil {
			fmt.Printf("LSP Server error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(lspCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
