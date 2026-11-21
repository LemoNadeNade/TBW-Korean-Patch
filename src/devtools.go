package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// devRoundtrip parses and re-serialises every serialized file in dir and
// reports whether the output is byte-identical.
func devRoundtrip(dir string) int {
	ents, _ := os.ReadDir(dir)
	bad := 0
	n := 0
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !(strings.HasPrefix(name, "level") || strings.HasSuffix(name, ".assets") || strings.HasPrefix(name, "globalgamemanagers")) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			fmt.Println("read", name, err)
			bad++
			continue
		}
		f, err := ParseSFile(b)
		if err != nil {
			fmt.Println("parse", name, err)
			bad++
			continue
		}
		n++
		if !bytes.Equal(f.Bytes(), b) {
			fmt.Println("DIFF", name)
			bad++
		}
	}
	fmt.Printf("roundtrip files=%d bad=%d\n", n, bad)
	return bad
}

func devDLL(in, replJSON, out string) int {
	b, err := os.ReadFile(in)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	jb, err := os.ReadFile(replJSON)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	var repl [][2]string
	if err := json.Unmarshal(jb, &repl); err != nil {
		fmt.Println(err)
		return 1
	}
	nb, r, err := PatchDLL(b, repl, nil)
	if err != nil {
		fmt.Println("ERR", err)
		return 1
	}
	fmt.Printf("replaced=%d strings=%d already=%d missing=%d size %d->%d\n", r.Replaced, r.Strings, r.Already, len(r.Missing), len(b), len(nb))
	os.WriteFile(out, nb, 0644)
	return 0
}
