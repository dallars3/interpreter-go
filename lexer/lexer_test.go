package lexer

import (
	"testing"

	"interpreter/token"
)

func typesOf(tokens []token.Token) []token.Type {
	out := make([]token.Type, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, t.Type)
	}
	return out
}

func TestBasicTokens(t *testing.T) {
	src := `int a = 3 + 2 * 4;`
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatal(err)
	}
	got := typesOf(toks)
	want := []token.Type{
		token.TYPE, token.IDENT, token.ASSIGN, token.INT,
		token.PLUS, token.INT, token.STAR, token.INT, token.SEM, token.EOF,
	}
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestOperatorsAndKeywords(t *testing.T) {
	src := `!= <= >= == && || ! ~ & | % while if else break continue true false print`
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []token.Type{
		token.NEQ, token.LE, token.GE, token.EQ,
		token.ANDAND, token.OROR, token.BANG, token.TILDE,
		token.AMP, token.PIPE, token.PERCENT,
		token.WHILE, token.IF, token.ELSE, token.BREAK, token.CONTINUE,
		token.TRUE, token.FALSE, token.PRINT, token.EOF,
	}
	got := typesOf(toks)
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestLiterals(t *testing.T) {
	src := `12 12.5 "hi\n" 'A'`
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []token.Type{token.INT, token.FLOAT, token.STRING, token.CHAR, token.EOF}
	got := typesOf(toks)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestBOMAndComments(t *testing.T) {
	// 以 UTF-8 BOM 开头，含行注释与块注释
	src := "\uFEFFint a; // line comment\n/* block\ncomment */ int b;"
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatal(err)
	}
	// 去掉末尾 EOF 后，应只有 int a ; int b ; 六个 token
	if len(toks) != 7 {
		var names []string
		for _, tk := range toks {
			names = append(names, tk.Type.String())
		}
		t.Fatalf("expected 7 tokens incl EOF, got %d: %v", len(toks), names)
	}
	if toks[0].Type != token.TYPE {
		t.Fatalf("BOM not skipped, first token = %s", toks[0].Type)
	}
}

func TestLineNumbers(t *testing.T) {
	src := "int a;\n\nint b;"
	toks, err := Tokenize(src)
	if err != nil {
		t.Fatal(err)
	}
	// 第二个 int 在第 3 行
	secondTypeIdx := 3 // TYPE IDENT SEM TYPE ...
	if toks[secondTypeIdx].Line != 3 {
		t.Fatalf("line = %d, want 3", toks[secondTypeIdx].Line)
	}
}

func TestLexErrors(t *testing.T) {
	bad := []string{
		`"unterminated`,
		`'ab`,
		`/* unterminated`,
		`1.2.3`,
		`@`,
	}
	for _, s := range bad {
		if _, err := Tokenize(s); err == nil {
			t.Errorf("expected lexical error for %q, got nil", s)
		}
	}
}
