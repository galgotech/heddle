package parser

import (
	"strings"
	"testing"

	"heddle/pkg/lang/ast"
	"heddle/pkg/lang/lexer"
)

func TestImportStatement(t *testing.T) {
	input := `
import "math"
import "strings" str
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 2 {
		t.Fatalf("program.Statements does not contain 2 statements. got=%d", len(program.Statements))
	}

	tests := []struct {
		expectedPath  string
		expectedAlias string
	}{
		{"math", ""},
		{"strings", "str"},
	}

	for i, tt := range tests {
		stmt := program.Statements[i]
		importStmt, ok := stmt.(*ast.ImportStatement)
		if !ok {
			t.Fatalf("statements[%d] not *ast.ImportStatement. got=%T", i, stmt)
		}

		if importStmt.Path != tt.expectedPath {
			t.Errorf("importStmt.Path not %q. got=%q", tt.expectedPath, importStmt.Path)
		}

		if tt.expectedAlias != "" {
			if importStmt.Alias == nil {
				t.Fatalf("importStmt.Alias was nil but expected %q", tt.expectedAlias)
			}
			if importStmt.Alias.Value != tt.expectedAlias {
				t.Errorf("importStmt.Alias not %q. got=%q", tt.expectedAlias, importStmt.Alias.Value)
			}
		} else {
			if importStmt.Alias != nil {
				t.Errorf("importStmt.Alias was not nil. got=%v", importStmt.Alias)
			}
		}
	}
}

func TestFlowAndHandlerStatements(t *testing.T) {
	input := `
flow main(in) {
  return in
}

flow flow2 ? handle_err {
  return
}

handler handle_err(err) {
  return err
}
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 3 {
		t.Fatalf("program.Statements does not contain 3 statements. got=%d", len(program.Statements))
	}

	// 1. flow main(in) { return in }
	stmt1 := program.Statements[0].(*ast.FlowStatement)
	if stmt1.Name.Value != "main" {
		t.Errorf("stmt1.Name.Value not 'main'. got=%q", stmt1.Name.Value)
	}
	if stmt1.Param.Value != "in" {
		t.Errorf("stmt1.Param.Value not 'in'. got=%q", stmt1.Param.Value)
	}
	if stmt1.HandlerName != nil {
		t.Errorf("stmt1.HandlerName was not nil. got=%v", stmt1.HandlerName)
	}
	if len(stmt1.Body.Statements) != 1 {
		t.Fatalf("stmt1.Body.Statements does not have 1 statement. got=%d", len(stmt1.Body.Statements))
	}
	retStmt, ok := stmt1.Body.Statements[0].(*ast.ReturnStatement)
	if !ok {
		t.Fatalf("flow body statement not *ast.ReturnStatement. got=%T", stmt1.Body.Statements[0])
	}
	if retStmt.ReturnValue.(*ast.Identifier).Value != "in" {
		t.Errorf("retStmt.ReturnValue not 'in'. got=%s", retStmt.ReturnValue.String())
	}

	// 2. flow flow2 ? handle_err { return }
	stmt2 := program.Statements[1].(*ast.FlowStatement)
	if stmt2.Name.Value != "flow2" {
		t.Errorf("stmt2.Name.Value not 'flow2'. got=%q", stmt2.Name.Value)
	}
	if stmt2.Param != nil {
		t.Errorf("stmt2.Param was not nil. got=%v", stmt2.Param)
	}
	if stmt2.HandlerName.Value != "handle_err" {
		t.Errorf("stmt2.HandlerName.Value not 'handle_err'. got=%q", stmt2.HandlerName.Value)
	}

	// 3. handler handle_err(err) { return err }
	stmt3 := program.Statements[2].(*ast.HandlerStatement)
	if stmt3.Name.Value != "handle_err" {
		t.Errorf("stmt3.Name.Value not 'handle_err'. got=%q", stmt3.Name.Value)
	}
	if stmt3.Param.Value != "err" {
		t.Errorf("stmt3.Param.Value not 'err'. got=%q", stmt3.Param.Value)
	}
}

func TestAssignAndPipeExpressions(t *testing.T) {
	input := `
tax_rate = 0.15
cart_with_taxes = clean_payload
  | .cart.items.price
  | calc.mul_scalar(scalar: tax_rate)
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 2 {
		t.Fatalf("program.Statements does not contain 2 statements. got=%d", len(program.Statements))
	}

	// 1. tax_rate = 0.15
	stmt1, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("statements[0] not *ast.ExpressionStatement. got=%T", program.Statements[0])
	}
	assign1, ok := stmt1.Expression.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("stmt1.Expression not *ast.AssignExpression. got=%T", stmt1.Expression)
	}
	if assign1.Name.Value != "tax_rate" {
		t.Errorf("assign1.Name.Value not 'tax_rate'. got=%q", assign1.Name.Value)
	}
	if assign1.Value.(*ast.FloatLiteral).Value != 0.15 {
		t.Errorf("assign1.Value not 0.15. got=%v", assign1.Value)
	}

	// 2. cart_with_taxes = clean_payload | .cart.items.price | calc.mul_scalar(scalar: tax_rate)
	stmt2, ok := program.Statements[1].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("statements[1] not *ast.ExpressionStatement. got=%T", program.Statements[1])
	}
	assign2, ok := stmt2.Expression.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("stmt2.Expression not *ast.AssignExpression. got=%T", stmt2.Expression)
	}
	if assign2.Name.Value != "cart_with_taxes" {
		t.Errorf("assign2.Name.Value not 'cart_with_taxes'. got=%q", assign2.Name.Value)
	}

	pipe, ok := assign2.Value.(*ast.PipeExpression)
	if !ok {
		t.Fatalf("assign2.Value not *ast.PipeExpression. got=%T", assign2.Value)
	}
	if len(pipe.Expressions) != 3 {
		t.Fatalf("pipe.Expressions does not have 3 expressions. got=%d", len(pipe.Expressions))
	}

	// First expression: clean_payload (Identifier)
	if pipe.Expressions[0].(*ast.Identifier).Value != "clean_payload" {
		t.Errorf("pipe.Expressions[0] not 'clean_payload'. got=%s", pipe.Expressions[0].String())
	}

	// Second expression: .cart.items.price (PathExpression, relative)
	path, ok := pipe.Expressions[1].(*ast.PathExpression)
	if !ok {
		t.Fatalf("pipe.Expressions[1] not *ast.PathExpression. got=%T", pipe.Expressions[1])
	}
	if path.Root != nil {
		t.Errorf("path.Root was not nil. got=%v", path.Root)
	}
	if len(path.Elements) != 3 || path.Elements[0] != "cart" || path.Elements[1] != "items" || path.Elements[2] != "price" {
		t.Errorf("path.Elements not ['cart', 'items', 'price']. got=%v", path.Elements)
	}

	// Third expression: calc.mul_scalar(scalar: tax_rate) (CallExpression)
	call, ok := pipe.Expressions[2].(*ast.CallExpression)
	if !ok {
		t.Fatalf("pipe.Expressions[2] not *ast.CallExpression. got=%T", pipe.Expressions[2])
	}
	callFn, ok := call.Function.(*ast.PathExpression)
	if !ok {
		t.Fatalf("call.Function not *ast.PathExpression. got=%T", call.Function)
	}
	if callFn.Root.Value != "calc" || len(callFn.Elements) != 1 || callFn.Elements[0] != "mul_scalar" {
		t.Errorf("call.Function not 'calc.mul_scalar'. got=%s", callFn.String())
	}
	if len(call.Arguments) != 1 {
		t.Fatalf("call.Arguments does not have 1 argument. got=%d", len(call.Arguments))
	}
	arg := call.Arguments[0]
	if arg.Name.Value != "scalar" {
		t.Errorf("arg.Name not 'scalar'. got=%q", arg.Name.Value)
	}
	if arg.Value.(*ast.Identifier).Value != "tax_rate" {
		t.Errorf("arg.Value not 'tax_rate'. got=%s", arg.Value.String())
	}
}

func TestTupleAndSplitJoin(t *testing.T) {
	input := `
data_merge = (data, data_merge) | {
  val1: .data.x,
  val2: .data_merge.y
}
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("program.Statements does not contain 1 statement. got=%d", len(program.Statements))
	}

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	assign, ok := stmt.Expression.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("stmt.Expression not *ast.AssignExpression. got=%T", stmt.Expression)
	}

	pipe, ok := assign.Value.(*ast.PipeExpression)
	if !ok {
		t.Fatalf("assign.Value not *ast.PipeExpression. got=%T", assign.Value)
	}

	// 1. TupleExpression
	tuple, ok := pipe.Expressions[0].(*ast.TupleExpression)
	if !ok {
		t.Fatalf("pipe.Expressions[0] not *ast.TupleExpression. got=%T", pipe.Expressions[0])
	}
	if len(tuple.Expressions) != 2 {
		t.Fatalf("tuple.Expressions does not have 2 expressions. got=%d", len(tuple.Expressions))
	}
	if tuple.Expressions[0].(*ast.Identifier).Value != "data" {
		t.Errorf("tuple.Expressions[0] not 'data'")
	}
	if tuple.Expressions[1].(*ast.Identifier).Value != "data_merge" {
		t.Errorf("tuple.Expressions[1] not 'data_merge'")
	}

	// 2. MapLiteral
	ml, ok := pipe.Expressions[1].(*ast.MapLiteral)
	if !ok {
		t.Fatalf("pipe.Expressions[1] not *ast.MapLiteral. got=%T", pipe.Expressions[1])
	}
	if len(ml.Pairs) != 2 {
		t.Fatalf("ml.Pairs does not have 2 pairs. got=%d", len(ml.Pairs))
	}

	p1 := ml.Pairs[0]
	if p1.Key.(*ast.Identifier).Value != "val1" {
		t.Errorf("p1.Key not 'val1'")
	}
	p1Val, ok := p1.Value.(*ast.PathExpression)
	if !ok || p1Val.Root != nil || len(p1Val.Elements) != 2 || p1Val.Elements[0] != "data" || p1Val.Elements[1] != "x" {
		t.Errorf("p1.Value not '.data.x'. got=%s", p1.Value.String())
	}
}

func TestTriggerBlockStatement(t *testing.T) {
	input := `
time.tick("0.5s") {
  each(now) {
    now | io.print()
  }
}
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("program.Statements does not contain 1 statement. got=%d", len(program.Statements))
	}

	tbs, ok := program.Statements[0].(*ast.TriggerBlockStatement)
	if !ok {
		t.Fatalf("program.Statements[0] not *ast.TriggerBlockStatement. got=%T", program.Statements[0])
	}

	call := tbs.Trigger.(*ast.CallExpression)
	if call.Function.(*ast.PathExpression).String() != "time.tick" {
		t.Errorf("tbs.Trigger function not 'time.tick'. got=%s", call.Function.String())
	}

	if len(tbs.Cases) != 1 {
		t.Fatalf("tbs.Cases does not have 1 case. got=%d", len(tbs.Cases))
	}

	c := tbs.Cases[0]
	if c.Tag != "each" {
		t.Errorf("case tag not 'each'. got=%q", c.Tag)
	}
	if c.Param.Value != "now" {
		t.Errorf("case param not 'now'. got=%q", c.Param.Value)
	}
}

func TestMapLiteralAssignmentAndTransformation(t *testing.T) {
	input := `
transform = {
  id: .user_id,
  full_name: .name
}

result = payload | transform
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 2 {
		t.Fatalf("program.Statements does not contain 2 statements. got=%d", len(program.Statements))
	}

	// 1. transform = { id: .user_id, full_name: .name }
	stmt1 := program.Statements[0].(*ast.ExpressionStatement)
	assign1 := stmt1.Expression.(*ast.AssignExpression)
	ml := assign1.Value.(*ast.MapLiteral)
	if len(ml.Pairs) != 2 {
		t.Fatalf("ml.Pairs count not 2. got=%d", len(ml.Pairs))
	}
	if ml.Pairs[0].Key.(*ast.Identifier).Value != "id" {
		t.Errorf("ml.Pairs[0].Key not 'id'")
	}
	if ml.Pairs[0].Value.(*ast.PathExpression).String() != ".user_id" {
		t.Errorf("ml.Pairs[0].Value not '.user_id'. got=%s", ml.Pairs[0].Value.String())
	}

	// 2. result = payload | transform
	stmt2 := program.Statements[1].(*ast.ExpressionStatement)
	assign2 := stmt2.Expression.(*ast.AssignExpression)
	pipe := assign2.Value.(*ast.PipeExpression)
	if pipe.Expressions[0].(*ast.Identifier).Value != "payload" {
		t.Errorf("pipe.Expressions[0] not 'payload'")
	}
	if pipe.Expressions[1].(*ast.Identifier).Value != "transform" {
		t.Errorf("pipe.Expressions[1] not 'transform'")
	}
}

func TestPatternMatchingExpression(t *testing.T) {
	input := `
10 | cmp.compare(20) {
  less(val) {
    return val
  }
  greater(val) {
    return val
  }
}
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("program.Statements does not contain 1 statement. got=%d", len(program.Statements))
	}

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	pipe := stmt.Expression.(*ast.PipeExpression)

	if pipe.Expressions[0].(*ast.IntegerLiteral).Value != 10 {
		t.Errorf("pipe.Expressions[0] not 10")
	}

	match := pipe.Expressions[1].(*ast.MatchExpression)
	if match.Target.(*ast.CallExpression).Function.(*ast.PathExpression).String() != "cmp.compare" {
		t.Errorf("match target function not 'cmp.compare'")
	}

	if len(match.Cases) != 2 {
		t.Fatalf("match.Cases count not 2. got=%d", len(match.Cases))
	}

	c1 := match.Cases[0]
	if c1.Tag != "less" || c1.Param.Value != "val" {
		t.Errorf("c1 tag/param incorrect. got=%s(%s)", c1.Tag, c1.Param.Value)
	}

	c2 := match.Cases[1]
	if c2.Tag != "greater" || c2.Param.Value != "val" {
		t.Errorf("c2 tag/param incorrect. got=%s(%s)", c2.Tag, c2.Param.Value)
	}
}

func TestStructInitializer(t *testing.T) {
	input := `
http_server = http.server {
  host: "localhost",
  port: 8080
}
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("program.Statements does not contain 1 statement. got=%d", len(program.Statements))
	}

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	assign := stmt.Expression.(*ast.AssignExpression)
	si, ok := assign.Value.(*ast.StructInitializer)
	if !ok {
		t.Fatalf("assign.Value not *ast.StructInitializer. got=%T", assign.Value)
	}

	if si.Type.(*ast.PathExpression).String() != "http.server" {
		t.Errorf("si.Type not 'http.server'. got=%s", si.Type.String())
	}

	if len(si.Fields) != 2 {
		t.Fatalf("si.Fields count not 2. got=%d", len(si.Fields))
	}

	f1 := si.Fields[0]
	if f1.Name.Value != "host" || f1.Value.(*ast.StringLiteral).Value != "localhost" {
		t.Errorf("field 1 incorrect: %s=%s", f1.Name.Value, f1.Value.String())
	}

	f2 := si.Fields[1]
	if f2.Name.Value != "port" || f2.Value.(*ast.IntegerLiteral).Value != 8080 {
		t.Errorf("field 2 incorrect: %s=%s", f2.Name.Value, f2.Value.String())
	}
}

func TestMapperExpression(t *testing.T) {
	input := `
cart_with_taxes = clean_payload 
  | .cart.items.price (p) {
    p | calc.mul_scalar(scalar: tax_rate)
  }
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("program.Statements does not contain 1 statement. got=%d", len(program.Statements))
	}

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	assign := stmt.Expression.(*ast.AssignExpression)
	pipe := assign.Value.(*ast.PipeExpression)

	mapper, ok := pipe.Expressions[1].(*ast.MapperExpression)
	if !ok {
		t.Fatalf("pipe.Expressions[1] not *ast.MapperExpression. got=%T", pipe.Expressions[1])
	}

	if mapper.Path.(*ast.PathExpression).String() != ".cart.items.price" {
		t.Errorf("mapper.Path not '.cart.items.price'. got=%s", mapper.Path.String())
	}

	if mapper.Alias.Value != "p" {
		t.Errorf("mapper.Alias not 'p'. got=%q", mapper.Alias.Value)
	}

	if len(mapper.Body.Statements) != 1 {
		t.Fatalf("mapper.Body.Statements count not 1. got=%d", len(mapper.Body.Statements))
	}
}

func TestArrayAndPrefixExpressions(t *testing.T) {
	input := `
x = [-10, 20]
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("program.Statements does not contain 1 statement. got=%d", len(program.Statements))
	}

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	assign := stmt.Expression.(*ast.AssignExpression)
	arr := assign.Value.(*ast.ArrayLiteral)

	if len(arr.Elements) != 2 {
		t.Fatalf("arr.Elements count not 2. got=%d", len(arr.Elements))
	}

	pref, ok := arr.Elements[0].(*ast.PrefixExpression)
	if !ok || pref.Operator != "-" || pref.Right.(*ast.IntegerLiteral).Value != 10 {
		t.Errorf("pref incorrect. got=%s", arr.Elements[0].String())
	}
}

func TestFunctionHandlerExpression(t *testing.T) {
	input := `"456" | strconv.parse_int() ? division_by_zero() | io.print()`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("program.Statements does not contain 1 statement. got=%d", len(program.Statements))
	}

	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("not *ast.ExpressionStatement. got=%T", program.Statements[0])
	}

	pipe, ok := stmt.Expression.(*ast.PipeExpression)
	if !ok {
		t.Fatalf("not *ast.PipeExpression. got=%T", stmt.Expression)
	}

	if len(pipe.Expressions) != 3 {
		t.Fatalf("pipe.Expressions count not 3. got=%d", len(pipe.Expressions))
	}

	she, ok := pipe.Expressions[1].(*ast.FunctionHandlerExpression)
	if !ok {
		t.Fatalf("second not *ast.FunctionHandlerExpression. got=%T", pipe.Expressions[1])
	}
	if she.Expr.(*ast.CallExpression).Function.String() != "strconv.parse_int" {
		t.Errorf("she.Expr incorrect: %s", she.Expr.String())
	}
	if she.Handler.(*ast.CallExpression).Function.String() != "division_by_zero" {
		t.Errorf("she.Handler incorrect: %s", she.Handler.String())
	}
}

func TestTrailingCommas(t *testing.T) {
	input := `
x = [1, 2,]
y = { a: 1, b: 2, }
`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	checkParserErrors(t, p)

	if len(program.Statements) != 2 {
		t.Fatalf("program.Statements count not 2. got=%d", len(program.Statements))
	}
}

func TestMatchAndMapperRobustness(t *testing.T) {
	// 1. Test Match on a single line (no newlines after opening brace)
	inputMatch := `10 | cmp.compare(20) { less(val) { return val } greater(val) { return val } }`
	lMatch := lexer.New(inputMatch)
	pMatch := New(lMatch)
	progMatch := pMatch.ParseProgram()
	checkParserErrors(t, pMatch)

	if len(progMatch.Statements) != 1 {
		t.Fatalf("expected 1 statement, got=%d", len(progMatch.Statements))
	}

	// 2. Test Mapper with newline before block brace
	inputMapper := `
cart_with_taxes = clean_payload 
  | .cart.items.price (p)
  {
    p | calc.mul_scalar(scalar: tax_rate)
  }
`
	lMapper := lexer.New(inputMapper)
	pMapper := New(lMapper)
	progMapper := pMapper.ParseProgram()
	checkParserErrors(t, pMapper)

	if len(progMapper.Statements) != 1 {
		t.Fatalf("expected 1 statement, got=%d", len(progMapper.Statements))
	}
}

func TestASTNilSafety(t *testing.T) {
	// Manually construct AST nodes with nil pointer components to verify they print "<nil>" instead of panicking
	pipe := &ast.PipeExpression{
		Token: lexer.Token{Type: lexer.TokenPipe, Literal: "|"},
		Expressions: []ast.Expression{
			nil,
			&ast.Identifier{Token: lexer.Token{Type: lexer.TokenIdent, Literal: "x"}, Value: "x"},
		},
	}

	if s := pipe.String(); s != "<nil> | x" {
		t.Errorf("expected string to be '<nil> | x', got=%q", s)
	}

	assign := &ast.AssignExpression{
		Token: lexer.Token{Type: lexer.TokenAssign, Literal: "="},
		Name:  nil,
		Value: nil,
	}

	if s := assign.String(); s != "<nil> = <nil>" {
		t.Errorf("expected string to be '<nil> = <nil>', got=%q", s)
	}
}

func TestParserRecursionLimit(t *testing.T) {
	// Cria uma expressão profundamente aninhada com parênteses: 1200 níveis
	var input string
	for i := 0; i < 1200; i++ {
		input += "("
	}
	input += "1"
	for i := 0; i < 1200; i++ {
		input += ")"
	}

	l := lexer.New(input)
	p := New(l)
	_ = p.ParseProgram()

	errors := p.Errors()
	if len(errors) == 0 {
		t.Fatalf("expected recursion depth limit error, but got none")
	}

	foundLimitError := false
	for _, err := range errors {
		if strings.Contains(err, "max recursion depth exceeded") {
			foundLimitError = true
			break
		}
	}

	if !foundLimitError {
		t.Errorf("expected error message to contain 'max recursion depth exceeded', got errors: %v", errors)
	}
}

func checkParserErrors(t *testing.T, p *Parser) {
	errors := p.Errors()
	if len(errors) == 0 {
		return
	}

	t.Errorf("parser has %d errors", len(errors))
	for _, msg := range errors {
		t.Errorf("parser error: %q", msg)
	}
	t.FailNow()
}
