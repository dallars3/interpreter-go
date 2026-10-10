// Package parser 实现递归下降语法分析，把 token 序列解析成 AST。
//
// Java 原版为"变量声明"手写了一张 17 状态的 LR(1) 分析表（理解和维护成本高，
// 且归约弹栈逻辑被注释、语法错误被 catch 静默吞掉）。Go 版改用递归下降：
// 文法简单直观，天然输出 AST，语法错误会携带行号与期望 token 信息。
package parser

import (
	"fmt"
	"strconv"

	"interpreter/ast"
	"interpreter/token"
)

type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d: syntax error: %s", e.Line, e.Msg)
}

type Parser struct {
	tokens []token.Token
	pos    int
}

func Parse(tokens []token.Token) (*ast.Program, error) {
	p := &Parser{tokens: tokens}
	prog := &ast.Program{}
	for p.cur().Type != token.EOF {
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		prog.Stmts = append(prog.Stmts, stmt)
	}
	return prog, nil
}

func (p *Parser) cur() token.Token  { return p.tokens[p.pos] }
func (p *Parser) next() token.Token { t := p.cur(); p.pos++; return t }

func (p *Parser) check(t token.Type) bool { return p.cur().Type == t }

func (p *Parser) match(t token.Type) bool {
	if p.check(t) {
		p.pos++
		return true
	}
	return false
}

func (p *Parser) expect(t token.Type) (token.Token, error) {
	if p.cur().Type != t {
		return token.Token{}, &ParseError{
			Line: p.cur().Line,
			Msg:  fmt.Sprintf("expected %s, got %s (%q)", t, p.cur().Type, p.cur().Lit),
		}
	}
	return p.next(), nil
}

func (p *Parser) errf(line int, format string, args ...any) error {
	return &ParseError{Line: line, Msg: fmt.Sprintf(format, args...)}
}

// ---------- 语句 ----------

func (p *Parser) parseStmt() (ast.Stmt, error) {
	switch p.cur().Type {
	case token.TYPE:
		return p.parseVarDecl()
	case token.IDENT:
		return p.parseAssign()
	case token.PRINT:
		return p.parsePrint()
	case token.IF:
		return p.parseIf()
	case token.WHILE:
		return p.parseWhile()
	case token.LBRACE:
		return p.parseBlock()
	case token.BREAK:
		t := p.next()
		if _, err := p.expect(token.SEM); err != nil {
			return nil, err
		}
		return &ast.Break{L: t.Line}, nil
	case token.CONTINUE:
		t := p.next()
		if _, err := p.expect(token.SEM); err != nil {
			return nil, err
		}
		return &ast.Continue{L: t.Line}, nil
	default:
		return nil, p.errf(p.cur().Line, "unexpected token %s (%q)", p.cur().Type, p.cur().Lit)
	}
}

// int a, b[10];
func (p *Parser) parseVarDecl() (ast.Stmt, error) {
	typeTok := p.next() // TYPE
	decl := &ast.VarDecl{TypeName: typeTok.Lit, L: typeTok.Line}
	for {
		name, err := p.expect(token.IDENT)
		if err != nil {
			return nil, err
		}
		item := ast.DeclItem{Name: name.Lit}
		if p.match(token.LBRACK) {
			size, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(token.RBRACK); err != nil {
				return nil, err
			}
			item.Size = size
		}
		decl.Items = append(decl.Items, item)
		if !p.match(token.COMMA) {
			break
		}
	}
	if _, err := p.expect(token.SEM); err != nil {
		return nil, err
	}
	return decl, nil
}

// a = expr;  或  a[i] = expr;
func (p *Parser) parseAssign() (ast.Stmt, error) {
	target, err := p.parseLValue()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(token.ASSIGN); err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(token.SEM); err != nil {
		return nil, err
	}
	return &ast.Assign{Target: target, Value: value, L: target.Line()}, nil
}

// parseLValue 解析赋值左值：IDENT ('[' expr ']')*
func (p *Parser) parseLValue() (ast.Expr, error) {
	name, err := p.expect(token.IDENT)
	if err != nil {
		return nil, err
	}
	var target ast.Expr = &ast.Ident{Name: name.Lit, L: name.Line}
	for p.match(token.LBRACK) {
		idx, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		rb, err := p.expect(token.RBRACK)
		if err != nil {
			return nil, err
		}
		target = &ast.IndexExpr{Array: target, Index: idx, L: rb.Line}
	}
	return target, nil
}

// print expr, expr;
// 兼容三种写法：print a, b;  print(a, b);  print (1+2)*3;
// 区分规则：若开头的左括号存在与之配对的右括号，且右括号后面紧跟 ; 或 ,，
// 则视为函数调用风格的括号；否则括号只是普通的表达式分组。
func (p *Parser) parsePrint() (ast.Stmt, error) {
	printTok := p.next() // PRINT
	callParen := p.looksLikeCallParen()
	if callParen {
		p.next() // 吃掉作为调用括号的 LPAREN
	}
	args := []ast.Expr{}
	for {
		arg, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if !p.match(token.COMMA) {
			break
		}
	}
	if callParen {
		if _, err := p.expect(token.RPAREN); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(token.SEM); err != nil {
		return nil, err
	}
	return &ast.Print{Args: args, L: printTok.Line}, nil
}

// looksLikeCallParen 判断当前位置的 LPAREN 是否为函数调用风格的括号。
// 通过括号配平找到对应的 RPAREN，检查其后是否为语句结束或参数分隔。
func (p *Parser) looksLikeCallParen() bool {
	if !p.check(token.LPAREN) {
		return false
	}
	depth := 0
	for i := p.pos; i < len(p.tokens); i++ {
		switch p.tokens[i].Type {
		case token.LPAREN:
			depth++
		case token.RPAREN:
			depth--
			if depth == 0 {
				if i+1 >= len(p.tokens) {
					return false
				}
				switch p.tokens[i+1].Type {
				case token.SEM, token.COMMA:
					return true
				}
				return false
			}
		}
	}
	return false
}

func (p *Parser) parseBlock() (*ast.Block, error) {
	lb := p.next() // LBRACE
	block := &ast.Block{L: lb.Line}
	for !p.check(token.RBRACE) && !p.check(token.EOF) {
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		block.Stmts = append(block.Stmts, stmt)
	}
	if _, err := p.expect(token.RBRACE); err != nil {
		return nil, err
	}
	return block, nil
}

func (p *Parser) parseIf() (ast.Stmt, error) {
	ifTok := p.next() // IF
	if _, err := p.expect(token.LPAREN); err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(token.RPAREN); err != nil {
		return nil, err
	}
	then, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	stmt := &ast.If{Cond: cond, Then: then, L: ifTok.Line}
	if p.match(token.ELSE) {
		if p.check(token.IF) {
			elseIf, err := p.parseIf()
			if err != nil {
				return nil, err
			}
			stmt.Else = elseIf
		} else {
			elseBlock, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			stmt.Else = elseBlock
		}
	}
	return stmt, nil
}

func (p *Parser) parseWhile() (ast.Stmt, error) {
	whileTok := p.next() // WHILE
	if _, err := p.expect(token.LPAREN); err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(token.RPAREN); err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.While{Cond: cond, Body: body, L: whileTok.Line}, nil
}

// ---------- 表达式（优先级由低到高） ----------

func (p *Parser) parseExpr() (ast.Expr, error) {
	return p.parseLogicOr()
}

func (p *Parser) parseLogicOr() (ast.Expr, error) {
	left, err := p.parseLogicAnd()
	if err != nil {
		return nil, err
	}
	for p.check(token.OROR) {
		op := p.next()
		right, err := p.parseLogicAnd()
		if err != nil {
			return nil, err
		}
		left = &ast.Binary{X: left, Op: op.Type, Y: right, L: op.Line}
	}
	return left, nil
}

func (p *Parser) parseLogicAnd() (ast.Expr, error) {
	left, err := p.parseBitOr()
	if err != nil {
		return nil, err
	}
	for p.check(token.ANDAND) {
		op := p.next()
		right, err := p.parseBitOr()
		if err != nil {
			return nil, err
		}
		left = &ast.Binary{X: left, Op: op.Type, Y: right, L: op.Line}
	}
	return left, nil
}

func (p *Parser) parseEquality() (ast.Expr, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.check(token.EQ) || p.check(token.NEQ) {
		op := p.next()
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = &ast.Binary{X: left, Op: op.Type, Y: right, L: op.Line}
	}
	return left, nil
}

func (p *Parser) parseComparison() (ast.Expr, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for p.check(token.LT) || p.check(token.GT) || p.check(token.LE) || p.check(token.GE) {
		op := p.next()
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		left = &ast.Binary{X: left, Op: op.Type, Y: right, L: op.Line}
	}
	return left, nil
}

func (p *Parser) parseBitOr() (ast.Expr, error) {
	left, err := p.parseBitAnd()
	if err != nil {
		return nil, err
	}
	for p.check(token.PIPE) {
		op := p.next()
		right, err := p.parseBitAnd()
		if err != nil {
			return nil, err
		}
		left = &ast.Binary{X: left, Op: op.Type, Y: right, L: op.Line}
	}
	return left, nil
}

func (p *Parser) parseBitAnd() (ast.Expr, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.check(token.AMP) {
		op := p.next()
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = &ast.Binary{X: left, Op: op.Type, Y: right, L: op.Line}
	}
	return left, nil
}

func (p *Parser) parseAdditive() (ast.Expr, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.check(token.PLUS) || p.check(token.MINUS) {
		op := p.next()
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &ast.Binary{X: left, Op: op.Type, Y: right, L: op.Line}
	}
	return left, nil
}

func (p *Parser) parseMultiplicative() (ast.Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.check(token.STAR) || p.check(token.SLASH) || p.check(token.PERCENT) {
		op := p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &ast.Binary{X: left, Op: op.Type, Y: right, L: op.Line}
	}
	return left, nil
}

func (p *Parser) parseUnary() (ast.Expr, error) {
	if p.check(token.MINUS) || p.check(token.BANG) || p.check(token.TILDE) {
		op := p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &ast.Unary{Op: op.Type, X: x, L: op.Line}, nil
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() (ast.Expr, error) {
	expr, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.match(token.LBRACK) {
		idx, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		rb, err := p.expect(token.RBRACK)
		if err != nil {
			return nil, err
		}
		expr = &ast.IndexExpr{Array: expr, Index: idx, L: rb.Line}
	}
	return expr, nil
}

func (p *Parser) parsePrimary() (ast.Expr, error) {
	t := p.cur()
	switch t.Type {
	case token.INT:
		p.next()
		n, err := strconv.ParseInt(t.Lit, 10, 64)
		if err != nil {
			return nil, p.errf(t.Line, "invalid integer literal %q", t.Lit)
		}
		return &ast.IntLit{Value: n, L: t.Line}, nil
	case token.FLOAT:
		p.next()
		f, err := strconv.ParseFloat(t.Lit, 64)
		if err != nil {
			return nil, p.errf(t.Line, "invalid float literal %q", t.Lit)
		}
		return &ast.FloatLit{Value: f, L: t.Line}, nil
	case token.STRING:
		p.next()
		return &ast.StringLit{Value: t.Lit, L: t.Line}, nil
	case token.CHAR:
		p.next()
		return &ast.CharLit{Value: t.Lit, L: t.Line}, nil
	case token.TRUE:
		p.next()
		return &ast.BoolLit{Value: true, L: t.Line}, nil
	case token.FALSE:
		p.next()
		return &ast.BoolLit{Value: false, L: t.Line}, nil
	case token.IDENT:
		p.next()
		return &ast.Ident{Name: t.Lit, L: t.Line}, nil
	case token.LPAREN:
		p.next()
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(token.RPAREN); err != nil {
			return nil, err
		}
		return expr, nil
	default:
		return nil, p.errf(t.Line, "expected expression, got %s (%q)", t.Type, t.Lit)
	}
}
