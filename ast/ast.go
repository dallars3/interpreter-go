// Package ast 定义抽象语法树的节点。
//
// Java 原版没有 AST，Main 直接在 token 流上"切一句执行一句"，if/while 靠
// 移动全局迭代器"跳到 }"实现，因此无法支持嵌套和循环体回跳。
// Go 版先由 parser 把整个源文件解析成 AST，解释器再遍历语法树执行，
// 控制流天然支持嵌套，while 也只是对子树反复求值。
package ast

import "interpreter/token"

// Node 所有 AST 节点都能报告所在行号。
type Node interface {
	Line() int
}

// Expr 表达式节点。
type Expr interface {
	Node
	exprNode()
}

// Stmt 语句节点。
type Stmt interface {
	Node
	stmtNode()
}

// Program 是整个源文件。
type Program struct {
	Stmts []Stmt
}

// ---------- 表达式 ----------

type Ident struct {
	Name string
	L    int
}

type IntLit struct {
	Value int64
	L     int
}

type FloatLit struct {
	Value float64
	L     int
}

type BoolLit struct {
	Value bool
	L     int
}

type StringLit struct {
	Value string
	L     int
}

type CharLit struct {
	Value string
	L     int
}

// Unary 一元表达式：-x、!x、~x
type Unary struct {
	Op token.Type
	X  Expr
	L  int
}

// Binary 二元表达式：x + y、x && y、x < y、x & y 等
type Binary struct {
	X  Expr
	Op token.Type
	Y  Expr
	L  int
}

// IndexExpr 数组下标表达式：a[i]
type IndexExpr struct {
	Array Expr
	Index Expr
	L     int
}

// Call 函数调用表达式：name(arg1, arg2)。
// 语言不支持用户自定义函数，被调用的只能是宿主注册的内置函数，
// 因此被调用方直接存名字，不需要通用的"函数值"表达式。
type Call struct {
	Name string
	Args []Expr
	L    int
}

// ExprStmt 是表达式语句，目前只可能是一次被丢弃返回值的函数调用，
// 例如 setHeader("Content-Type", "text/html");
type ExprStmt struct {
	X Expr
	L int
}

// ---------- 语句 ----------

// DeclItem 一条声明中的单个变量：a 或 a[10]
type DeclItem struct {
	Name string
	Size Expr // nil 表示标量
}

// VarDecl 变量声明：int a, b[10];
type VarDecl struct {
	TypeName string
	Items    []DeclItem
	L        int
}

// Assign 赋值语句：a = expr; 或 a[i] = expr;
type Assign struct {
	Target Expr // *Ident 或 *IndexExpr
	Value  Expr
	L      int
}

// Print 输出语句：print expr, expr;  （也兼容 print(expr, expr);）
type Print struct {
	Args []Expr
	L    int
}

// Block 语句块：{ ... }
type Block struct {
	Stmts []Stmt
	L     int
}

type If struct {
	Cond Expr
	Then *Block
	Else Stmt // *Block、*If 或 nil
	L    int
}

type While struct {
	Cond Expr
	Body *Block
	L    int
}

type Break struct{ L int }
type Continue struct{ L int }

// ---------- 接口实现 ----------

func (n *Ident) Line() int    { return n.L }
func (n *IntLit) Line() int   { return n.L }
func (n *FloatLit) Line() int { return n.L }
func (n *BoolLit) Line() int  { return n.L }
func (n *StringLit) Line() int {
	return n.L
}
func (n *CharLit) Line() int { return n.L }
func (n *Unary) Line() int   { return n.L }
func (n *Binary) Line() int  { return n.L }
func (n *Call) Line() int    { return n.L }
func (n *IndexExpr) Line() int {
	return n.L
}

func (s *ExprStmt) Line() int { return s.L }
func (s *VarDecl) Line() int  { return s.L }
func (s *Assign) Line() int   { return s.L }
func (s *Print) Line() int    { return s.L }
func (s *Block) Line() int    { return s.L }
func (s *If) Line() int       { return s.L }
func (s *While) Line() int    { return s.L }
func (s *Break) Line() int    { return s.L }
func (s *Continue) Line() int { return s.L }

func (*Ident) exprNode()     {}
func (*IntLit) exprNode()    {}
func (*FloatLit) exprNode()  {}
func (*BoolLit) exprNode()   {}
func (*StringLit) exprNode() {}
func (*CharLit) exprNode()   {}
func (*Unary) exprNode()     {}
func (*Binary) exprNode()    {}
func (*Call) exprNode()      {}
func (*IndexExpr) exprNode() {}

func (*ExprStmt) stmtNode() {}
func (*VarDecl) stmtNode()  {}
func (*Assign) stmtNode()   {}
func (*Print) stmtNode()    {}
func (*Block) stmtNode()    {}
func (*If) stmtNode()       {}
func (*While) stmtNode()    {}
func (*Break) stmtNode()    {}
func (*Continue) stmtNode() {}
