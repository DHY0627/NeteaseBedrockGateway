//go:build !linux

package main

import "os"

// stdinIsTerminal 判断标准输入是不是终端（非交互环境不能等待人工输入）。
func stdinIsTerminal() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
