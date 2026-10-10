// Package interp 是树遍历解释器：遍历 AST 并执行。
//
// 修复的 Java 原版问题：
//   - 每个声明的变量都有独立的值对象（原版 int a,b 会让 a、b 指向同一个 Var）；
//   - 数值通过 value.Value 的类型方法访问，不再用 String 往返传值、不再靠异常判类型；
//   - print 会真正求值表达式（原版只打印变量/字面量，跳过运算符）；
//   - if/else、while 基于 AST 递归执行，支持嵌套；
//   - 数组下标读写完整实现，并带越界与类型检查；
//   - 比较运算符 > < >= <= == != 完整实现（原版词法识别后无求值逻辑，会 NPE）；
//   - 所有运行时错误带行号，不再静默失败。
package interp

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"interpreter/ast"
	"interpreter/token"
	"interpreter/value"
)

// RuntimeError 携带行号的运行时错误。
type RuntimeError struct {
	Line int
	Msg  string
}

func (e *RuntimeError) Error() string {
	return fmt.Sprintf("line %d: runtime error: %s", e.Line, e.Msg)
}

// signal 用于 break/continue 跨语句栈传播，只在 while 处被捕获。
type signalKind uint8

const (
	sigBreak signalKind = iota + 1
	sigContinue
)

type signalErr struct {
	kind signalKind
	line int
}

func (e signalErr) Error() string {
	if e.kind == sigBreak {
		return "break outside loop"
	}
	return "continue outside loop"
}

// Env 是词法作用域环境，parent 指向外层作用域。
type Env struct {
	vars   map[string]value.Value
	parent *Env
}

func NewEnv(parent *Env) *Env {
	return &Env{vars: make(map[string]value.Value), parent: parent}
}

// declare 在当前作用域声明变量；同名重复声明报错。
func (e *Env) declare(name string, v value.Value) error {
	if _, exists := e.vars[name]; exists {
		return fmt.Errorf("variable %q already declared in this scope", name)
	}
	e.vars[name] = v
	return nil
}

func (e *Env) get(name string) (value.Value, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if v, ok := cur.vars[name]; ok {
			return v, true
		}
	}
	return value.Value{}, false
}

func (e *Env) set(name string, v value.Value) bool {
	for cur := e; cur != nil; cur = cur.parent {
		if _, ok := cur.vars[name]; ok {
			cur.vars[name] = v
			return true
		}
	}
	return false
}

// Interp 解释器状态。
type Interp struct {
	rootEnv *Env
	out     io.Writer
}

func New(out io.Writer) *Interp {
	if out == nil {
		out = os.Stdout
	}
	return &Interp{rootEnv: NewEnv(nil), out: out}
}

// Run 执行整个程序。
func (in *Interp) Run(prog *ast.Program) (err error) {
	// 捕获逃逸出所有 while 的 break/continue，转为错误
	defer func() {
		if r := recover(); r != nil {
			if sig, ok := r.(signalErr); ok {
				err = &RuntimeError{Line: sig.line, Msg: sig.Error()}
				return
			}
			panic(r)
		}
	}()
	for _, stmt := range prog.Stmts {
		if err = in.execStmt(stmt, in.rootEnv); err != nil {
			return err
		}
	}
	return nil
}

// ---------- 语句执行 ----------

func (in *Interp) execStmt(s ast.Stmt, env *Env) error {
	switch n := s.(type) {
	case *ast.VarDecl:
		return in.execVarDecl(n, env)
	case *ast.Assign:
		return in.execAssign(n, env)
	case *ast.Print:
		return in.execPrint(n, env)
	case *ast.Block:
		return in.execBlock(n, env)
	case *ast.If:
		return in.execIf(n, env)
	case *ast.While:
		return in.execWhile(n, env)
	case *ast.Break:
		panic(signalErr{kind: sigBreak, line: n.L})
	case *ast.Continue:
		panic(signalErr{kind: sigContinue, line: n.L})
	default:
		return &RuntimeError{Line: s.Line(), Msg: "unknown statement"}
	}
}

func typeOfName(typeName string) (value.Kind, bool) {
	switch typeName {
	case "int":
		return value.Int, true
	case "float":
		return value.Float, true
	case "boolean":
		return value.Bool, true
	case "char":
		return value.Char, true
	case "string":
		return value.Str, true
	default:
		return 0, false
	}
}

func (in *Interp) execVarDecl(d *ast.VarDecl, env *Env) error {
	kind, ok := typeOfName(d.TypeName)
	if !ok {
		return &RuntimeError{Line: d.L, Msg: "unknown type " + d.TypeName}
	}
	for _, item := range d.Items {
		var v value.Value
		if item.Size == nil {
			v = value.Zero(kind) // 每个变量独立 new，修复别名共享 bug
		} else {
			sizeVal, err := in.eval(item.Size, env)
			if err != nil {
				return err
			}
			if sizeVal.Kind != value.Int {
				return &RuntimeError{Line: item.Size.Line(), Msg: "array size must be an int"}
			}
			n := int(sizeVal.I)
			if n < 0 {
				return &RuntimeError{Line: item.Size.Line(), Msg: "array size must be non-negative"}
			}
			v = value.NewArray(kind, n)
		}
		if err := env.declare(item.Name, v); err != nil {
			return &RuntimeError{Line: d.L, Msg: err.Error()}
		}
	}
	return nil
}

func (in *Interp) execAssign(a *ast.Assign, env *Env) error {
	v, err := in.eval(a.Value, env)
	if err != nil {
		return err
	}
	switch target := a.Target.(type) {
	case *ast.Ident:
		cur, ok := env.get(target.Name)
		if !ok {
			return &RuntimeError{Line: target.L, Msg: fmt.Sprintf("variable %q is not declared", target.Name)}
		}
		if err := checkAssignable(target.L, cur, v); err != nil {
			return err
		}
		if !env.set(target.Name, v) {
			return &RuntimeError{Line: target.L, Msg: fmt.Sprintf("variable %q is not declared", target.Name)}
		}
	case *ast.IndexExpr:
		return in.assignIndex(target, v, env)
	default:
		return &RuntimeError{Line: a.L, Msg: "invalid assignment target"}
	}
	return nil
}

// checkAssignable 校验表达式值能否赋给已声明的目标（按声明类型严格匹配）。
func checkAssignable(line int, dst, src value.Value) error {
	if dst.Kind == value.Array {
		if src.Kind != value.Array || src.ElemK != dst.ElemK || len(src.Elems) != len(dst.Elems) {
			return &RuntimeError{Line: line, Msg: fmt.Sprintf("cannot assign %s to %s", src.KindName(), dst.KindName())}
		}
		return nil
	}
	if dst.Kind != src.Kind {
		return &RuntimeError{Line: line, Msg: fmt.Sprintf("cannot assign %s to %s", src.KindName(), dst.KindName())}
	}
	return nil
}

func (in *Interp) assignIndex(idx *ast.IndexExpr, v value.Value, env *Env) error {
	// 解析器允许 a[i][j] 形式的链式下标，而语言只有一维数组。
	// 这里不能直接做类型断言，否则链式下标会 panic。
	base, ok := idx.Array.(*ast.Ident)
	if !ok {
		return &RuntimeError{Line: idx.L, Msg: "multi-dimensional arrays are not supported"}
	}
	name := base
	arr, ok := env.get(name.Name)
	if !ok {
		return &RuntimeError{Line: name.L, Msg: fmt.Sprintf("variable %q is not declared", name.Name)}
	}
	if arr.Kind != value.Array {
		return &RuntimeError{Line: idx.L, Msg: fmt.Sprintf("%q is not an array", name.Name)}
	}
	index, err := in.eval(idx.Index, env)
	if err != nil {
		return err
	}
	if index.Kind != value.Int {
		return &RuntimeError{Line: idx.L, Msg: "array index must be an int"}
	}
	i := int(index.I)
	if i < 0 || i >= len(arr.Elems) {
		return &RuntimeError{Line: idx.L, Msg: fmt.Sprintf("array index %d out of bounds (length %d)", i, len(arr.Elems))}
	}
	if v.Kind != arr.ElemK {
		return &RuntimeError{Line: idx.L, Msg: fmt.Sprintf("cannot assign %s to array element of type %s", v.KindName(), value.TypeName(arr.ElemK))}
	}
	arr.Elems[i] = v // 通过 slice 底层数组就地修改
	env.set(name.Name, arr)
	return nil
}

func (in *Interp) execPrint(pr *ast.Print, env *Env) error {
	parts := make([]string, 0, len(pr.Args))
	for _, arg := range pr.Args {
		v, err := in.eval(arg, env) // 关键修复：print 对表达式真正求值
		if err != nil {
			return err
		}
		parts = append(parts, v.String())
	}
	fmt.Fprintln(in.out, strings.Join(parts, " "))
	return nil
}

func (in *Interp) execBlock(b *ast.Block, parent *Env) error {
	scope := NewEnv(parent) // 块级作用域
	for _, s := range b.Stmts {
		if err := in.execStmt(s, scope); err != nil {
			return err
		}
	}
	return nil
}

func (in *Interp) execIf(f *ast.If, env *Env) error {
	cond, err := in.eval(f.Cond, env)
	if err != nil {
		return err
	}
	if cond.Kind != value.Bool {
		return &RuntimeError{Line: f.L, Msg: fmt.Sprintf("if condition must be boolean, got %s", cond.KindName())}
	}
	if cond.B {
		return in.execBlock(f.Then, env)
	}
	if f.Else != nil {
		switch e := f.Else.(type) {
		case *ast.Block:
			return in.execBlock(e, env)
		case *ast.If:
			return in.execIf(e, env)
		}
	}
	return nil
}

func (in *Interp) execWhile(w *ast.While, env *Env) error {
	for {
		cond, err := in.eval(w.Cond, env)
		if err != nil {
			return err
		}
		if cond.Kind != value.Bool {
			return &RuntimeError{Line: w.L, Msg: fmt.Sprintf("while condition must be boolean, got %s", cond.KindName())}
		}
		if !cond.B {
			return nil
		}
		stop := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					if sig, ok := r.(signalErr); ok {
						if sig.kind == sigBreak {
							stop = true
							return
						}
						// continue：什么都不做，进入下一轮
						return
					}
					panic(r) // 非控制流信号（嵌套循环/真实 panic）继续上抛
				}
			}()
			err = in.execBlock(w.Body, env)
		}()
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
}

// ---------- 表达式求值 ----------

func (in *Interp) eval(e ast.Expr, env *Env) (value.Value, error) {
	switch n := e.(type) {
	case *ast.IntLit:
		return value.NewInt(n.Value), nil
	case *ast.FloatLit:
		return value.NewFloat(n.Value), nil
	case *ast.BoolLit:
		return value.NewBool(n.Value), nil
	case *ast.StringLit:
		return value.NewStr(n.Value), nil
	case *ast.CharLit:
		return value.NewChar(n.Value), nil
	case *ast.Ident:
		v, ok := env.get(n.Name)
		if !ok {
			return value.Value{}, &RuntimeError{Line: n.L, Msg: fmt.Sprintf("variable %q is not declared", n.Name)}
		}
		return v, nil
	case *ast.IndexExpr:
		return in.evalIndex(n, env)
	case *ast.Unary:
		return in.evalUnary(n, env)
	case *ast.Binary:
		return in.evalBinary(n, env)
	default:
		return value.Value{}, &RuntimeError{Line: e.Line(), Msg: "unknown expression"}
	}
}

func (in *Interp) evalIndex(n *ast.IndexExpr, env *Env) (value.Value, error) {
	arrV, err := in.eval(n.Array, env)
	if err != nil {
		return value.Value{}, err
	}
	if arrV.Kind != value.Array {
		return value.Value{}, &RuntimeError{Line: n.L, Msg: "index applied to a non-array value"}
	}
	idx, err := in.eval(n.Index, env)
	if err != nil {
		return value.Value{}, err
	}
	if idx.Kind != value.Int {
		return value.Value{}, &RuntimeError{Line: n.L, Msg: "array index must be an int"}
	}
	i := int(idx.I)
	if i < 0 || i >= len(arrV.Elems) {
		return value.Value{}, &RuntimeError{Line: n.L, Msg: fmt.Sprintf("array index %d out of bounds (length %d)", i, len(arrV.Elems))}
	}
	return arrV.Elems[i], nil
}

func (in *Interp) evalUnary(u *ast.Unary, env *Env) (value.Value, error) {
	v, err := in.eval(u.X, env)
	if err != nil {
		return value.Value{}, err
	}
	switch u.Op {
	case token.MINUS:
		switch v.Kind {
		case value.Int:
			return value.NewInt(-v.I), nil
		case value.Float:
			return value.NewFloat(-v.F), nil
		}
		return value.Value{}, typeMismatch(u.L, "unary -", v)
	case token.BANG:
		if v.Kind != value.Bool {
			return value.Value{}, typeMismatch(u.L, "unary !", v)
		}
		return value.NewBool(!v.B), nil
	case token.TILDE:
		if v.Kind != value.Int {
			return value.Value{}, typeMismatch(u.L, "unary ~", v)
		}
		return value.NewInt(^v.I), nil
	}
	return value.Value{}, &RuntimeError{Line: u.L, Msg: "unknown unary operator"}
}

func (in *Interp) evalBinary(b *ast.Binary, env *Env) (value.Value, error) {
	// 短路逻辑运算
	if b.Op == token.ANDAND || b.Op == token.OROR {
		l, err := in.eval(b.X, env)
		if err != nil {
			return value.Value{}, err
		}
		if l.Kind != value.Bool {
			return value.Value{}, typeMismatch(b.L, b.Op.String(), l)
		}
		if b.Op == token.ANDAND && !l.B {
			return value.NewBool(false), nil
		}
		if b.Op == token.OROR && l.B {
			return value.NewBool(true), nil
		}
		r, err := in.eval(b.Y, env)
		if err != nil {
			return value.Value{}, err
		}
		if r.Kind != value.Bool {
			return value.Value{}, typeMismatch(b.L, b.Op.String(), r)
		}
		return value.NewBool(r.B), nil
	}

	left, err := in.eval(b.X, env)
	if err != nil {
		return value.Value{}, err
	}
	right, err := in.eval(b.Y, env)
	if err != nil {
		return value.Value{}, err
	}

	// 相等性：数值允许 int/float 互比，其余要求同类型
	switch b.Op {
	case token.EQ, token.NEQ:
		eq, err := looseEquals(b.L, left, right)
		if err != nil {
			return value.Value{}, err
		}
		if b.Op == token.NEQ {
			eq = !eq
		}
		return value.NewBool(eq), nil
	}

	// 大小比较：仅数值
	switch b.Op {
	case token.LT, token.GT, token.LE, token.GE:
		lf, lok := left.AsFloat()
		rf, rok := right.AsFloat()
		if !lok || !rok {
			return value.Value{}, &RuntimeError{
				Line: b.L,
				Msg:  fmt.Sprintf("cannot compare %s with %s", left.KindName(), right.KindName()),
			}
		}
		var res bool
		switch b.Op {
		case token.LT:
			res = lf < rf
		case token.GT:
			res = lf > rf
		case token.LE:
			res = lf <= rf
		case token.GE:
			res = lf >= rf
		}
		return value.NewBool(res), nil
	}

	// 字符串拼接
	if b.Op == token.PLUS && (left.Kind == value.Str || right.Kind == value.Str) {
		return value.NewStr(left.String() + right.String()), nil
	}

	// 位运算：仅整数
	if b.Op == token.AMP || b.Op == token.PIPE {
		if left.Kind != value.Int || right.Kind != value.Int {
			return value.Value{}, &RuntimeError{
				Line: b.L,
				Msg:  fmt.Sprintf("bitwise operator requires int operands, got %s, %s", left.KindName(), right.KindName()),
			}
		}
		if b.Op == token.AMP {
			return value.NewInt(left.I & right.I), nil
		}
		return value.NewInt(left.I | right.I), nil
	}

	// 算术运算
	lf, lok := left.AsFloat()
	rf, rok := right.AsFloat()
	if !lok || !rok {
		return value.Value{}, &RuntimeError{
			Line: b.L,
			Msg:  fmt.Sprintf("operator %s requires numeric operands, got %s, %s", b.Op.String(), left.KindName(), right.KindName()),
		}
	}
	bothInt := left.Kind == value.Int && right.Kind == value.Int
	switch b.Op {
	case token.PLUS:
		if bothInt {
			return value.NewInt(left.I + right.I), nil
		}
		return value.NewFloat(lf + rf), nil
	case token.MINUS:
		if bothInt {
			return value.NewInt(left.I - right.I), nil
		}
		return value.NewFloat(lf - rf), nil
	case token.STAR:
		if bothInt {
			return value.NewInt(left.I * right.I), nil
		}
		return value.NewFloat(lf * rf), nil
	case token.SLASH:
		if bothInt {
			if right.I == 0 {
				return value.Value{}, &RuntimeError{Line: b.L, Msg: "integer divide by zero"}
			}
			return value.NewInt(left.I / right.I), nil
		}
		if rf == 0 {
			return value.Value{}, &RuntimeError{Line: b.L, Msg: "float divide by zero"}
		}
		return value.NewFloat(lf / rf), nil
	case token.PERCENT:
		if bothInt {
			if right.I == 0 {
				return value.Value{}, &RuntimeError{Line: b.L, Msg: "integer modulo by zero"}
			}
			return value.NewInt(left.I % right.I), nil
		}
		if rf == 0 {
			return value.Value{}, &RuntimeError{Line: b.L, Msg: "float modulo by zero"}
		}
		return value.NewFloat(math.Mod(lf, rf)), nil
	}
	return value.Value{}, &RuntimeError{Line: b.L, Msg: "unsupported operator " + b.Op.String()}
}

func looseEquals(line int, a, b value.Value) (bool, error) {
	if af, ok := a.AsFloat(); ok {
		bf, ok2 := b.AsFloat()
		if !ok2 {
			return false, &RuntimeError{Line: line, Msg: fmt.Sprintf("cannot compare %s with %s", a.KindName(), b.KindName())}
		}
		return af == bf, nil
	}
	if a.Kind != b.Kind {
		return false, &RuntimeError{Line: line, Msg: fmt.Sprintf("cannot compare %s with %s", a.KindName(), b.KindName())}
	}
	switch a.Kind {
	case value.Bool:
		return a.B == b.B, nil
	case value.Char, value.Str:
		return a.S == b.S, nil
	default:
		return false, &RuntimeError{Line: line, Msg: fmt.Sprintf("cannot compare %s with %s", a.KindName(), b.KindName())}
	}
}

func typeMismatch(line int, op string, v value.Value) error {
	return &RuntimeError{Line: line, Msg: fmt.Sprintf("operator %s cannot be applied to %s", op, v.KindName())}
}
