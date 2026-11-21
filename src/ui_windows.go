//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	k32              = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMd = k32.NewProc("GetConsoleMode")
	procSetConsoleMd = k32.NewProc("SetConsoleMode")
	procSetTitle     = k32.NewProc("SetConsoleTitleW")
)

// enableVT turns on ANSI escape sequence support in the Windows console.
func enableVT() bool {
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return false
	}
	var mode uint32
	if r, _, _ := procGetConsoleMd.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return false
	}
	const vtProcessing = 0x0004
	r, _, _ := procSetConsoleMd.Call(uintptr(h), uintptr(mode|vtProcessing))
	return r != 0
}

func setTitle(s string) {
	p, err := syscall.UTF16PtrFromString(s)
	if err == nil {
		procSetTitle.Call(uintptr(unsafe.Pointer(p)))
	}
}

// gameRunning reports whether the game's process is currently running.
func gameRunning() bool {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(snap)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = syscall.Process32First(snap, &pe); err == nil; err = syscall.Process32Next(snap, &pe) {
		if strings.EqualFold(syscall.UTF16ToString(pe.ExeFile[:]), gameDirName+".exe") {
			return true
		}
	}
	return false
}
