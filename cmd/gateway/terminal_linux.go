//go:build linux

package main

import (
	"syscall"
	"unsafe"
)

// stdinIsTerminal 报告标准输入是否连接着真实的终端（pty 或控制台）。
//
// 不能只看 os.ModeCharDevice：/dev/null 也是字符设备，`cmd </dev/null` 会被误判成
// 交互式终端。这里用 ioctl(TCGETS) 判断——只有真的终端才会成功。
func stdinIsTerminal() bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&termios)),
		0, 0, 0,
	)
	return errno == 0
}
