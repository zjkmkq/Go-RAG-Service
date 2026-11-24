//go:build windows

package main

import "syscall"

// initConsoleUTF8 把 Windows 控制台的输入/输出代码页切到 UTF-8。
// 不切的话，中文输入在 GBK 代码页下会被按 GBK 编码读入，发给后端就是乱码。
func initConsoleUTF8() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setCP := kernel32.NewProc("SetConsoleCP")
	setOutCP := kernel32.NewProc("SetConsoleOutputCP")
	setCP.Call(65001)
	setOutCP.Call(65001)
}
