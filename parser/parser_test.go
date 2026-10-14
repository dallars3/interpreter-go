// parser_test.go 覆盖函数调用语法：name(args)。
package parser

import (
	"testing"

	"interpreter/ast"
	"interpreter/lexer"
)

func parseSrc(t *testing.T, src string) *ast.Program {
	t.Helper()
	toks, err := lexer.Tokenize(src)
	if err != nil {
		t.Fatalf("lex error: %v\nsrc: %s", err, src)
	}
	prog, err := Parse(toks)
	if err != nil {
		t.Fatalf("parse error: %v\nsrc: %s", err, src)
	}
	return prog
}

func firstPrintArg(t *testing.T, prog *ast.Program) ast.Expr {
	t.Helper()
	pr, ok := prog.Stmts[0].(*ast.Print)
	if !ok {
		t.Fatalf("first statement is %T, want *ast.Print", prog.Stmts[0])
	}
	if len(pr.Args) != 1 {
		t.Fatalf("print has %d args, want 1", len(pr.Args))
	}
	return pr.Args[0]
}

func TestParseCall(t *testing.T) {
	prog := parseSrc(t, `print(f(1, 2));`)
	call, ok := firstPrintArg(t, prog).(*ast.Call)
	if !ok {
		t.Fatalf("arg is %T, want *ast.Call", firstPrintArg(t, prog))
	}
	if call.Name != "f" {
		t.Fatalf("call name = %q, want f", call.Name)
	}
	if len(call.Args) != 2 {
		t.Fatalf("arg count = %d, want 2", len(call.Args))
	}
}

func TestParseCallNoArgsAndNested(t *testing.T) {
	prog := parseSrc(t, `print(g());`)
	call := firstPrintArg(t, prog).(*ast.Call)
	if call.Name != "g" || len(call.Args) != 0 {
		t.Fatalf("got %q with %d args, want g()", call.Name, len(call.Args))
	}

	prog = parseSrc(t, `print(add(add(1, 2), 3));`)
	outer := firstPrintArg(t, prog).(*ast.Call)
	if outer.Name != "add" || len(outer.Args) != 2 {
		t.Fatalf("outer call malformed: %q/%d", outer.Name, len(outer.Args))
	}
	inner, ok := outer.Args[0].(*ast.Call)
	if !ok || inner.Name != "add" || len(inner.Args) != 2 {
		t.Fatalf("first arg must be nested add(1,2), got %#v", outer.Args[0])
	}
}

func TestParseCallPrecedence(t *testing.T) {
	// 调用是后缀原子：f(1) + 2 的顶层必须是 Binary，Call 在左侧。
	prog := parseSrc(t, `print(f(1) + 2);`)
	bin, ok := firstPrintArg(t, prog).(*ast.Binary)
	if !ok {
		t.Fatalf("top expr is %T, want *ast.Binary", firstPrintArg(t, prog))
	}
	if _, ok := bin.X.(*ast.Call); !ok {
		t.Fatalf("left side is %T, want *ast.Call", bin.X)
	}
}

func TestParseCallStatement(t *testing.T) {
	// 无返回值（或返回值被丢弃）的调用是一条独立语句：setHeader("a", "b");
	prog := parseSrc(t, `setHeader("Content-Type", "text/html");`)
	es, ok := prog.Stmts[0].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("statement is %T, want *ast.ExprStmt", prog.Stmts[0])
	}
	call, ok := es.X.(*ast.Call)
	if !ok || call.Name != "setHeader" || len(call.Args) != 2 {
		t.Fatalf("expr stmt malformed: %#v", es.X)
	}
}

func TestParseRejectsCallingNonIdentifier(t *testing.T) {
	// 语言只支持具名函数调用：(1)(2) 非法。
	toks, err := lexer.Tokenize(`print((1)(2));`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(toks); err == nil {
		t.Fatal("expected syntax error for calling a parenthesized literal")
	}
}
