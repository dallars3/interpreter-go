// Package token 定义词法单元的类型与结构。
package token

// Type 是 token 的种类。
type Type int

const (
	ILLEGAL Type = iota
	EOF

	// 字面量
	IDENT  // 标识符
	INT    // 整数字面量
	FLOAT  // 浮点字面量
	STRING // 字符串字面量
	CHAR   // 字符字面量
	TRUE   // true
	FALSE  // false

	// 类型关键字（字面值为 int/float/boolean/char/string）
	TYPE

	// 语句关键字
	IF
	ELSE
	WHILE
	BREAK
	CONTINUE
	PRINT

	// 运算符与分隔符
	ASSIGN  // =
	PLUS    // +
	MINUS   // -
	STAR    // *
	SLASH   // /
	PERCENT // %
	AMP     // &
	PIPE    // |
	TILDE   // ~
	BANG    // !
	ANDAND  // &&
	OROR    // ||
	EQ      // ==
	NEQ     // !=
	LT      // <
	GT      // >
	LE      // <=
	GE      // >=

	SEM    // ;
	COMMA  // ,
	LPAREN // (
	RPAREN // )
	LBRACK // [
	RBRACK // ]
	LBRACE // {
	RBRACE // }
)

// Token 是一个词法单元，Line 记录其在源文件中的行号（从 1 开始），便于错误定位。
type Token struct {
	Type Type
	Lit  string
	Line int
}

func (t Type) String() string {
	return typeNames[t]
}

var typeNames = map[Type]string{
	ILLEGAL: "ILLEGAL", EOF: "EOF",
	IDENT: "IDENT", INT: "INT", FLOAT: "FLOAT", STRING: "STRING", CHAR: "CHAR",
	TRUE: "true", FALSE: "false", TYPE: "TYPE",
	IF: "if", ELSE: "else", WHILE: "while", BREAK: "break", CONTINUE: "continue", PRINT: "print",
	ASSIGN: "=", PLUS: "+", MINUS: "-", STAR: "*", SLASH: "/", PERCENT: "%",
	AMP: "&", PIPE: "|", TILDE: "~", BANG: "!",
	ANDAND: "&&", OROR: "||", EQ: "==", NEQ: "!=", LT: "<", GT: ">", LE: "<=", GE: ">=",
	SEM: ";", COMMA: ",", LPAREN: "(", RPAREN: ")",
	LBRACK: "[", RBRACK: "]", LBRACE: "{", RBRACE: "}",
}
