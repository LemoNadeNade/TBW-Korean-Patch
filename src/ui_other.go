//go:build !windows

package main

import "os"

func enableVT() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0 || os.Getenv("TBWKR_VT") == "1"
}

func setTitle(string) {}

func gameRunning() bool { return os.Getenv("TBWKR_FAKE_RUNNING") == "1" }
