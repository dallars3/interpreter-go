package interp

import (
	"bytes"
	"strings"
	"testing"

	"interpreter/lexer"
	"interpreter/parser"
)

// runSrc 走完整流水线并返回 print 输出。
func runSrc(t *testing.T, src string) (string, error) {
	t.Helper()
	toks, err := lexer.Tokenize(src)
	if err != nil {
		return "", err
	}
	prog, err := parser.Parse(toks)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = New(&buf).Run(prog)
	return buf.String(), err
}

func mustRun(t *testing.T, src, want string) {
	t.Helper()
	got, err := runSrc(t, src)
	if err != nil {
		t.Fatalf("unexpected error: %v\nsrc:\n%s", err, src)
	}
	if got != want {
		t.Fatalf("output = %q, want %q\nsrc:\n%s", got, want, src)
	}
}

func mustFail(t *testing.T, src, contains string) {
	t.Helper()
	_, err := runSrc(t, src)
	if err == nil {
		t.Fatalf("expected error containing %q, got nil\nsrc:\n%s", contains, src)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("error = %q, want substring %q", err.Error(), contains)
	}
}

// Bug #1：原版 int a,b 会让两个变量共享同一个 Var 对象，改 a 等于改 b。
func TestVariableIndependence(t *testing.T) {
	mustRun(t, `int a, b;
a = 1;
print(a);
print(b);`, "1\n0\n")
}

// Bug #2/#3：表达式不再用字符串传值、不再靠异常判 int/float 类型。
func TestArithmeticAndPrecedence(t *testing.T) {
	mustRun(t, `print(2 + 3 * 4);`, "14\n")
	mustRun(t, `print((2 + 3) * 4);`, "20\n")
	mustRun(t, `print(10 / 3);`, "3\n")     // 整数除法
	mustRun(t, `print(10.0 / 4);`, "2.5\n") // 浮点除法
	mustRun(t, `print(7 % 3);`, "1\n")
	mustRun(t, `print(2.5 + 1.5);`, "4.0\n")
	mustRun(t, `print(-5 + 2);`, "-3\n")
	mustRun(t, `print(~5);`, "-6\n")
	mustRun(t, `print(6 & 3);`, "2\n")
	mustRun(t, `print(5 | 2);`, "7\n")
}

// Bug：原版 print 不求值表达式，print (1+2)*(10-4) 会输出 12104。
func TestPrintEvaluatesExpression(t *testing.T) {
	mustRun(t, `print((1 + 2) * (10 - 4));`, "18\n")
	mustRun(t, `print 1 + 2;`, "3\n")
	mustRun(t, `print(1, 2, 3);`, "1 2 3\n")
}

// Bug #5：比较运算符原版无求值逻辑，遇到即 NPE。
func TestComparisons(t *testing.T) {
	mustRun(t, `print(3 > 2);`, "true\n")
	mustRun(t, `print(3 < 2);`, "false\n")
	mustRun(t, `print(2 >= 2);`, "true\n")
	mustRun(t, `print(2 != 3);`, "true\n")
	mustRun(t, `print(2 == 2.0);`, "true\n") // int/float 数值比较
}

func TestLogical(t *testing.T) {
	mustRun(t, `print(true && false);`, "false\n")
	mustRun(t, `print(!false || false);`, "true\n")
	// 短路求值：右侧是未声明变量也不应报错
	mustRun(t, `print(false && undef);`, "false\n")
	mustRun(t, `print(true || undef);`, "true\n")
}

// Bug #4：数组原版只有半成品声明，无法按下标读写，也没有越界检查。
func TestArrays(t *testing.T) {
	src := `int a[3];
a[0] = 10;
a[2] = a[0] + 5;
print(a[0]);
print(a[1]);
print(a[2]);
print(a);`
	mustRun(t, src, "10\n0\n15\n[10, 0, 15]\n")
}

// Bug：原版 if/while 只能"跳到第一个 }"，不支持 else、嵌套和回跳。
func TestIfElseChains(t *testing.T) {
	src := `int score;
score = 75;
if (score >= 90) {
	print("A");
} else if (score >= 60) {
	print("B");
} else {
	print("C");
}`
	mustRun(t, src, "B\n")
}

func TestWhileContinueSum(t *testing.T) {
	src := `int sum, i;
i = 1;
while (i <= 10) {
	if (i % 2 != 0) {
		i = i + 1;
		continue;
	}
	sum = sum + i;
	i = i + 1;
}
print(sum);`
	mustRun(t, src, "30\n")
}

// break 只跳出最内层循环
func TestNestedLoopsBreak(t *testing.T) {
	src := `int i, j, hits;
i = 0;
while (i < 3) {
	j = 0;
	while (j < 3) {
		if (j == 1) { break; }
		hits = hits + 1;
		j = j + 1;
	}
	i = i + 1;
}
print(hits);`
	mustRun(t, src, "3\n")
}

func TestBlockScope(t *testing.T) {
	src := `int a;
a = 1;
{
	int a;
	a = 9;
	print(a);
}
print(a);`
	mustRun(t, src, "9\n1\n")
}

func TestStringsAndChars(t *testing.T) {
	mustRun(t, `string s;
s = "hello" + " " + "world";
print(s);`, "hello world\n")
	mustRun(t, `char c;
c = 'A';
print(c);`, "A\n")
}

// ---------- 错误处理：带行号，不静默吞掉 ----------

func TestRuntimeErrors(t *testing.T) {
	mustFail(t, `print(x);`, "not declared")
	mustFail(t, `int x;
x = true;`, "cannot assign")
	mustFail(t, `print(1 / 0);`, "divide by zero")
	mustFail(t, `int a[2];
a[5] = 1;`, "out of bounds")
	mustFail(t, `int x;
x[0] = 1;`, "not an array")
	mustFail(t, `print(1 + true);`, "numeric operands")
	mustFail(t, `if (1) { print(1); }`, "must be boolean")
	mustFail(t, `break;`, "break outside loop")
	mustFail(t, `int a;
int a;`, "already declared")
}

func TestSyntaxErrors(t *testing.T) {
	mustFail(t, `int ;`, "syntax error")
	mustFail(t, `a = ;`, "syntax error")
	mustFail(t, `if (true) print(1);`, "syntax error") // 缺语句块
}
