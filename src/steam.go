package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const gameDirName = "Tactical Breach Wizards"
const dataDirName = "Tactical Breach Wizards_Data"

func isGameRoot(p string) bool {
	st, err := os.Stat(filepath.Join(p, dataDirName, "sharedassets0.assets"))
	return err == nil && !st.IsDir()
}

// normalizeGameRoot accepts the game folder, its _Data folder or a file in it.
func normalizeGameRoot(p string) string {
	p = strings.TrimSpace(strings.Trim(strings.TrimSpace(p), `"`))
	if p == "" {
		return ""
	}
	for i := 0; i < 4 && p != ""; i++ {
		if isGameRoot(p) {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return ""
}

var vdfPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)

func findGameRoot() string {
	if exe, err := os.Executable(); err == nil {
		if r := normalizeGameRoot(filepath.Dir(exe)); r != "" {
			return r
		}
	}
	if wd, err := os.Getwd(); err == nil {
		if r := normalizeGameRoot(wd); r != "" {
			return r
		}
	}
	if runtime.GOOS != "windows" {
		return ""
	}
	var libs []string
	for _, base := range []string{`C:\Program Files (x86)\Steam`, `C:\Program Files\Steam`} {
		libs = append(libs, base)
		if b, err := os.ReadFile(filepath.Join(base, "steamapps", "libraryfolders.vdf")); err == nil {
			for _, m := range vdfPath.FindAllStringSubmatch(string(b), -1) {
				libs = append(libs, strings.ReplaceAll(m[1], `\\`, `\`))
			}
		}
	}
	for d := 'C'; d <= 'Z'; d++ {
		drive := string(d) + `:\`
		if _, err := os.Stat(drive); err != nil {
			continue
		}
		for _, sub := range []string{"Steam", "SteamLibrary", `Program Files (x86)\Steam`, `Program Files\Steam`, `Games\Steam`, `Games\SteamLibrary`} {
			libs = append(libs, drive+sub)
		}
	}
	for _, l := range libs {
		p := filepath.Join(l, "steamapps", "common", gameDirName)
		if isGameRoot(p) {
			return p
		}
	}
	return ""
}
