// Package value 定义解释器运行时的值模型。
//
// 相比 Java 原版（Token/Var 泛型 + 表达式链路用 String 传值 + 靠 try/catch
// 做 Integer/Float 判断），这里用一个具体的 Value 结构承载所有类型，
// 数值类型通过统一的 Number 抽象访问，彻底消除字符串往返解析和异常做控制流。
package value

import (
	"fmt"
	"strconv"
)

type Kind uint8

const (
	Int Kind = iota
	Float
	Bool
	Char
	Str
	Array
)

// Value 是运行时值。零值即 Int 的 0。
type Value struct {
	Kind Kind

	I int64   // Int
	F float64 // Float
	B bool    // Bool
	S string  // Char（单个字符）/ Str

	// Array
	Elems []Value
	ElemK Kind
}

func NewInt(i int64) Value     { return Value{Kind: Int, I: i} }
func NewFloat(f float64) Value { return Value{Kind: Float, F: f} }
func NewBool(b bool) Value     { return Value{Kind: Bool, B: b} }
func NewChar(r string) Value   { return Value{Kind: Char, S: r} }
func NewStr(s string) Value    { return Value{Kind: Str, S: s} }

// NewArray 创建长度为 length、元素类型为 elemK、元素为零值的数组。
func NewArray(elemK Kind, length int) Value {
	return Value{Kind: Array, ElemK: elemK, Elems: make([]Value, length)}
}

// Zero 返回某类型的零值。
func Zero(k Kind) Value {
	switch k {
	case Int:
		return NewInt(0)
	case Float:
		return NewFloat(0)
	case Bool:
		return NewBool(false)
	case Char:
		return NewChar("")
	case Str:
		return NewStr("")
	default:
		return Value{}
	}
}

// KindName 返回类型名，用于类型错误提示。
func (v Value) KindName() string {
	switch v.Kind {
	case Int:
		return "int"
	case Float:
		return "float"
	case Bool:
		return "boolean"
	case Char:
		return "char"
	case Str:
		return "string"
	case Array:
		return TypeName(v.ElemK) + "[]"
	default:
		return "unknown"
	}
}

func TypeName(k Kind) string {
	return Value{Kind: k}.KindName()
}

// AsFloat 是统一的数值取值入口：int 与 float 都可以无损转成 float64。
// 对应 Java 原版里 (float)/(int) 强转 + try/catch 兜底的逻辑。
func (v Value) AsFloat() (float64, bool) {
	switch v.Kind {
	case Int:
		return float64(v.I), true
	case Float:
		return v.F, true
	default:
		return 0, false
	}
}

// IsNumber 判断是否为数值类型（int/float）。
func (v Value) IsNumber() bool {
	return v.Kind == Int || v.Kind == Float
}

// String 用于 print 输出。
func (v Value) String() string {
	switch v.Kind {
	case Int:
		return strconv.FormatInt(v.I, 10)
	case Float:
		s := strconv.FormatFloat(v.F, 'f', -1, 64)
		// 与教学语言习惯保持一致：整数值浮点显示为 3.0 而不是 3
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			for _, c := range s {
				if c == '.' || c == 'e' || c == 'E' {
					return s
				}
			}
			return s + ".0"
		}
		return s
	case Bool:
		return strconv.FormatBool(v.B)
	case Char, Str:
		return v.S
	case Array:
		out := "["
		for i := range v.Elems {
			if i > 0 {
				out += ", "
			}
			out += v.Elems[i].String()
		}
		return out + "]"
	default:
		return fmt.Sprintf("<unknown %d>", v.Kind)
	}
}
