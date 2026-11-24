//go:build !windows

package main

// initConsoleUTF8 在非 Windows 平台无需处理代码页
func initConsoleUTF8() {}
