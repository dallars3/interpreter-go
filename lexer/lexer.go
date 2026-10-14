// Package lexer 是手写词法分析器：把源代码字符流切分为 token 序列。
//
// 相比 Java 原版的改进：
//   - 自动跳过 UTF-8 BOM（原版靠 Main 里 removeFirst() 吃掉 BOM 产生的 ERR token）；
//   - 支持 // 行注释与 /* 块注释 */；
//   - 每个 token 记录行号，错误可以定位到具体行；
//   - 补全 !=、<=、>= 运算符；
//   - 不再依赖 (char)read()==65535 判断 EOF，也不依赖 mark/reset 回退。
package lexer

import (
	"fmt"
	"strings"
	"unicode"

	"interpreter/token"
)

// Lexer 持有源代码与扫描位置。
type Lexer struct {
	src  []rune
	pos  int
	line int
}

// Tokenize 对源代码做一次完整词法分析。
func Tokenize(source string) ([]token.Token, error) {
	s := &Lexer{src: []rune(strings.TrimPrefix(source, "\uFEFF")), pos: 0, line: 1}
	return s.run()
}

func (l *Lexer) run() (tokens []token.Token, err error) {
	for {
		if err = l.skipWhitespaceAndComments(); err != nil {
			return nil, err
		}
		if l.pos >= len(l.src) {
			tokens = append(tokens, token.Token{Type: token.EOF, Line: l.line})
			return tokens, nil
		}
		tt, err := l.nextToken()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tt)
	}
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekAt(offset int) rune {
	if l.pos+offset >= len(l.src) {
		return 0
	}
	return l.src[l.pos+offset]
}

func (l *Lexer) advance() rune {
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
	}
	return ch
}

func (l *Lexer) skipWhitespaceAndComments() error {
	for l.pos < len(l.src) {
		ch := l.peek()
		if unicode.IsSpace(ch) {
			l.advance()
			continue
		}
		// 行注释
		if ch == '/' && l.peekAt(1) == '/' {
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
			continue
		}
		// 块注释
		if ch == '/' && l.peekAt(1) == '*' {
			startLine := l.line
			l.advance()
			l.advance()
			closed := false
			for l.pos < len(l.src) {
				if l.peek() == '*' && l.peekAt(1) == '/' {
					l.advance()
					l.advance()
					closed = true
					break
				}
				l.advance()
			}
			if !closed {
				// 未闭合的块注释按错误处理，避免静默吞掉后续代码
				return lexError(startLine, "unterminated block comment")
			}
			continue
		}
		return nil
	}
	return nil
}

func (l *Lexer) nextToken() (token.Token, error) {
	startLine := l.line
	ch := l.peek()

	switch {
	case isIdentStart(ch):
		return l.readWord(), nil
	case unicode.IsDigit(ch):
		return l.readNumber()
	case ch == '"':
		return l.readString()
	case ch == '\'':
		return l.readChar()
	}

	return l.readOperator(startLine)
}

func (l *Lexer) readWord() token.Token {
	line := l.line
	var sb strings.Builder
	for l.pos < len(l.src) && isIdentPart(l.peek()) {
		sb.WriteRune(l.advance())
	}
	word := sb.String()
	if t, ok := keywords[word]; ok {
		return token.Token{Type: t, Lit: word, Line: line}
	}
	return token.Token{Type: token.IDENT, Lit: word, Line: line}
}

var keywords = map[string]token.Type{
	"int": token.TYPE, "float": token.TYPE, "boolean": token.TYPE,
	"char": token.TYPE, "string": token.TYPE,
	"if": token.IF, "else": token.ELSE, "while": token.WHILE,
	"break": token.BREAK, "continue": token.CONTINUE,
	"true": token.TRUE, "false": token.FALSE, "print": token.PRINT,
}

func (l *Lexer) readNumber() (token.Token, error) {
	line := l.line
	var sb strings.Builder
	dots := 0
	for l.pos < len(l.src) && (unicode.IsDigit(l.peek()) || l.peek() == '.') {
		if l.peek() == '.' {
			dots++
			if dots > 1 {
				return token.Token{}, lexError(line, "malformed number literal")
			}
		}
		sb.WriteRune(l.advance())
	}
	lit := sb.String()
	if dots == 1 {
		return token.Token{Type: token.FLOAT, Lit: lit, Line: line}, nil
	}
	return token.Token{Type: token.INT, Lit: lit, Line: line}, nil
}

func (l *Lexer) readString() (token.Token, error) {
	line := l.line
	l.advance() // 吃掉左引号
	var sb strings.Builder
	for l.pos < len(l.src) && l.peek() != '"' {
		ch := l.advance()
		if ch == '\n' {
			return token.Token{}, lexError(line, "unterminated string literal")
		}
		if ch == '\\' {
			esc, err := l.readEscape(line)
			if err != nil {
				return token.Token{}, err
			}
			sb.WriteRune(esc)
			continue
		}
		sb.WriteRune(ch)
	}
	if l.pos >= len(l.src) {
		return token.Token{}, lexError(line, "unterminated string literal")
	}
	l.advance() // 吃掉右引号
	return token.Token{Type: token.STRING, Lit: sb.String(), Line: line}, nil
}

func (l *Lexer) readChar() (token.Token, error) {
	line := l.line
	l.advance() // 吃掉左单引号
	if l.pos >= len(l.src) {
		return token.Token{}, lexError(line, "unterminated char literal")
	}
	var r rune
	if l.peek() == '\\' {
		l.advance()
		esc, err := l.readEscape(line)
		if err != nil {
			return token.Token{}, err
		}
		r = esc
	} else {
		r = l.advance()
	}
	if l.pos >= len(l.src) || l.peek() != '\'' {
		return token.Token{}, lexError(line, "unterminated char literal")
	}
	l.advance() // 吃掉右单引号
	return token.Token{Type: token.CHAR, Lit: string(r), Line: line}, nil
}

func (l *Lexer) readEscape(line int) (rune, error) {
	if l.pos >= len(l.src) {
		return 0, lexError(line, "dangling escape character")
	}
	switch ch := l.advance(); ch {
	case 'n':
		return '\n', nil
	case 't':
		return '\t', nil
	case 'r':
		return '\r', nil
	case '0':
		return 0, nil
	case '\\':
		return '\\', nil
	case '"':
		return '"', nil
	case '\'':
		return '\'', nil
	default:
		return 0, lexError(line, fmt.Sprintf("unknown escape sequence \\%c", ch))
	}
}

func (l *Lexer) readOperator(line int) (token.Token, error) {
	ch := l.advance()
	two := func(second rune, yes token.Type, no token.Type) token.Token {
		if l.peek() == second {
			l.advance()
			return token.Token{Type: yes, Line: line}
		}
		return token.Token{Type: no, Line: line}
	}
	switch ch {
	case '+':
		return token.Token{Type: token.PLUS, Lit: "+", Line: line}, nil
	case '-':
		return token.Token{Type: token.MINUS, Lit: "-", Line: line}, nil
	case '*':
		return token.Token{Type: token.STAR, Lit: "*", Line: line}, nil
	case '/':
		return token.Token{Type: token.SLASH, Lit: "/", Line: line}, nil
	case '%':
		return token.Token{Type: token.PERCENT, Lit: "%", Line: line}, nil
	case '~':
		return token.Token{Type: token.TILDE, Lit: "~", Line: line}, nil
	case ';':
		return token.Token{Type: token.SEM, Line: line}, nil
	case ',':
		return token.Token{Type: token.COMMA, Line: line}, nil
	case '(':
		return token.Token{Type: token.LPAREN, Line: line}, nil
	case ')':
		return token.Token{Type: token.RPAREN, Line: line}, nil
	case '[':
		return token.Token{Type: token.LBRACK, Line: line}, nil
	case ']':
		return token.Token{Type: token.RBRACK, Line: line}, nil
	case '{':
		return token.Token{Type: token.LBRACE, Line: line}, nil
	case '}':
		return token.Token{Type: token.RBRACE, Line: line}, nil
	case '=':
		t := two('=', token.EQ, token.ASSIGN)
		return t, nil
	case '!':
		t := two('=', token.NEQ, token.BANG)
		return t, nil
	case '<':
		t := two('=', token.LE, token.LT)
		return t, nil
	case '>':
		t := two('=', token.GE, token.GT)
		return t, nil
	case '&':
		t := two('&', token.ANDAND, token.AMP)
		return t, nil
	case '|':
		t := two('|', token.OROR, token.PIPE)
		return t, nil
	default:
		return token.Token{}, lexError(line, fmt.Sprintf("unexpected character %q", ch))
	}
}

// IsIdentifier 报告 word 是否为一个可作为普通标识符的词：非空、首字符为
// 字母或下划线、其余为字母/数字/下划线，且不是语言关键字（print/if/int 等）。
// 宿主用它校验内置函数名——只有普通标识符才能在脚本里写成 name(...)。
func IsIdentifier(word string) bool {
	if word == "" {
		return false
	}
	for i, ch := range word {
		if i == 0 {
			if !isIdentStart(ch) {
				return false
			}
			continue
		}
		if !isIdentPart(ch) {
			return false
		}
	}
	if _, isKeyword := keywords[word]; isKeyword {
		return false
	}
	return true
}

func isIdentStart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func isIdentPart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

type LexError struct {
	Line int
	Msg  string
}

func (e *LexError) Error() string {
	return fmt.Sprintf("line %d: lexical error: %s", e.Line, e.Msg)
}

func lexError(line int, msg string) error {
	return &LexError{Line: line, Msg: msg}
}
