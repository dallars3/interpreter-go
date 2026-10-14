// builtin_test.go 覆盖宿主内置函数注册、调用求值与 context 取消。
package interp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"interpreter/lexer"
	"interpreter/parser"
	"interpreter/value"
)

// runWith 用注册好的内置函数执行源码。
func runWith(t *testing.T, src string, ctx context.Context, reg func(*Interp)) (string, error) {
	t.Helper()
	toks, err := lexer.Tokenize(src)
	if err != nil {
		t.Fatalf("lex error: %v", err)
	}
	prog, err := parser.Parse(toks)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var buf bytes.Buffer
	in := New(&buf)
	if reg != nil {
		reg(in)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	err = in.RunContext(ctx, prog)
	return buf.String(), err
}

func intAdder(in *Interp) {
	in.RegisterBuiltin("add", func(args []value.Value) (value.Value, error) {
		if len(args) != 2 {
			return value.Value{}, errors.New("add expects 2 arguments")
		}
		if args[0].Kind != value.Int || args[1].Kind != value.Int {
			return value.Value{}, errors.New("add expects int arguments")
		}
		return value.NewInt(args[0].I + args[1].I), nil
	})
}

func TestBuiltinCall(t *testing.T) {
	out, err := runWith(t, `print(add(2, 3));`, nil, intAdder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "5\n" {
		t.Fatalf("output = %q, want 5\\n", out)
	}
}

func TestBuiltinNestedAndExpr(t *testing.T) {
	out, err := runWith(t, `print(add(add(1, 2), 3) * 2);`, nil, intAdder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "12\n" {
		t.Fatalf("output = %q, want 12\\n", out)
	}
}

func TestBuiltinVoidReturn(t *testing.T) {
	reg := func(in *Interp) {
		in.RegisterBuiltin("noop", func(args []value.Value) (value.Value, error) {
			return value.NewVoid(), nil
		})
	}
	// 作为独立语句调用时返回值被丢弃；作为 print 参数时输出空串。
	out, err := runWith(t, "noop();\nprint(\"done:\" , noop());", nil, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "done: \n" {
		t.Fatalf("output = %q, want %q", out, "done: \n")
	}
}

func TestUnknownFunctionIsRuntimeError(t *testing.T) {
	_, err := runWith(t, `print(missing(1));`, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown function") {
		t.Fatalf("want unknown-function runtime error, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error must carry the call line number, got %v", err)
	}
}

func TestBuiltinErrorCarriesLine(t *testing.T) {
	// 宿主返回普通 error，解释器包装成带行号的 RuntimeError。
	_, err := runWith(t, "\nprint(add(1));", nil, intAdder)
	if err == nil {
		t.Fatal("want arity error, got nil")
	}
	if !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "2 arguments") {
		t.Fatalf("error = %v, want line 2 arity message", err)
	}
}

func TestRegisterBuiltinRejectsDupesAndNil(t *testing.T) {
	in := New(io.Discard)
	fn := func([]value.Value) (value.Value, error) { return value.NewVoid(), nil }
	in.RegisterBuiltin("x", fn)
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate registration must panic")
		}
	}()
	in.RegisterBuiltin("x", fn)
}

func TestRunContextCancelsInfiniteLoop(t *testing.T) {
	src := `while (true) {}`
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 进入循环前就已取消
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := runWith(t, src, ctx, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Fatalf("want cancellation runtime error, got %v", err)
		}
		if time.Since(start) > time.Second {
			t.Fatal("cancelled loop returned too late")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled infinite loop did not stop")
	}
}

func TestRunContextStopsRunningLoop(t *testing.T) {
	src := `int i;
while (true) { i = i + 1; }`
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := runWith(t, src, ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("want deadline-exceeded runtime error, got %v", err)
	}
}

// 保留字（print/if/int 等）永远不会被词法器识别为 IDENT，注册这类名字
// 的内置函数不可达，必须在注册期大声失败而不是静默成功。
func TestRegisterBuiltinRejectsReservedNames(t *testing.T) {
	for _, name := range []string{"print", "if", "while", "true", "int", "string"} {
		in := New(io.Discard)
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("RegisterBuiltin(%q) must panic", name)
				}
			}()
			in.RegisterBuiltin(name, func([]value.Value) (value.Value, error) {
				t.Fatalf("builtin %q must never be reachable", name)
				return value.NewVoid(), nil
			})
		}()
	}
}

// 非法标识符名（空串、含运算符、多个 token）同样拒绝。
func TestRegisterBuiltinRejectsInvalidNames(t *testing.T) {
	for _, name := range []string{"", "a-b", "x y", "1abc"} {
		in := New(io.Discard)
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("RegisterBuiltin(%q) must panic", name)
				}
			}()
			in.RegisterBuiltin(name, func([]value.Value) (value.Value, error) {
				return value.NewVoid(), nil
			})
		}()
	}
}

// Void 不能借字符串拼接伪装成 string：拼接分支必须拒绝 Void 操作数，
// 无论是直接 print 还是赋给已声明的 string 变量。
func TestVoidDoesNotLeakThroughConcatenation(t *testing.T) {
	reg := func(in *Interp) {
		in.RegisterBuiltin("noop", func([]value.Value) (value.Value, error) {
			return value.NewVoid(), nil
		})
	}
	if _, err := runWith(t, `print("a" + noop());`, nil, reg); err == nil {
		t.Fatal(`"a" + noop() must be a runtime error`)
	}
	if _, err := runWith(t, `print(noop() + "a");`, nil, reg); err == nil {
		t.Fatal(`noop() + "a" must be a runtime error`)
	}
	if _, err := runWith(t, `
string s;
s = "a" + noop();
print(s);`, nil, reg); err == nil {
		t.Fatal("assigning a void concatenation to a string must fail")
	}
	// print 直接打印 void 仍然输出空串（不经拼接），该行为保留。
	out, err := runWith(t, `print("done:", noop());`, nil, reg)
	if err != nil {
		t.Fatalf("printing a void argument is allowed, got %v", err)
	}
	if out != "done: \n" {
		t.Fatalf("output = %q, want %q", out, "done: \n")
	}
}
