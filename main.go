// InterpreterDemo 的 Go 实现：一个类 C 小型语言的解释器。
//
// 流水线：源代码 → lexer（词法分析）→ parser（语法分析，生成 AST）
//
//	→ interp（树遍历解释执行）
//
// 用法：
//
//	interpreter path/to/code.txt   # 不带参数时读取当前目录下的 code.txt
package main

import (
	"fmt"
	"os"

	"interpreter/interp"
	"interpreter/lexer"
	"interpreter/parser"
)

func main() {
	path := "code.txt"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read %s: %v\n", path, err)
		os.Exit(1)
	}

	tokens, err := lexer.Tokenize(string(source))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	program, err := parser.Parse(tokens)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := interp.New(os.Stdout).Run(program); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
