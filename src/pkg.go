package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Package manifest (format "tbwkr-1").
//
// Every object edit is described relative to the *original* game object so
// that the package never needs to carry copies of the game's own data, except
// for the Korean font assets, which are generated from free fonts.
type Manifest struct {
	Format  string              `json:"format"`
	Name    string              `json:"name"`
	Version string              `json:"version"`
	Notes   []string            `json:"notes"`
	Files   []FileSpec          `json:"files"`
	Foreign map[string][]string `json:"foreign"` // outputs of the old patcher
	DLL     *DLLSpec            `json:"dll"`
}

type FileSpec struct {
	Path    string    `json:"path"`
	Vanilla string    `json:"vanilla_sha256"` // whole-file hash of the supported game build
	Objects []ObjSpec `json:"objects"`
}

type ObjSpec struct {
	ID      int64     `json:"id"`
	Class   int32     `json:"class"`
	Vanilla string    `json:"v"`    // sha256 of original object
	Patched string    `json:"p"`    // sha256 of object after this patch
	Bytes   [][3]any  `json:"b"`    // [offset, oldHex, newHex] same-size patches
	Strs    [][3]any  `json:"s"`    // [offset, old, new] length-prefixed string fields
	Subs    []SubSpec `json:"t"`    // substring edits inside one string field
	Blob    string    `json:"blob"` // full replacement (font assets only)
	Label   string    `json:"label"`
}

type SubSpec struct {
	Off   int         `json:"off"`
	Edits [][2]string `json:"edits"`
}

type DLLSpec struct {
	Path         string       `json:"path"`
	Vanilla      string       `json:"vanilla_sha256"`
	Replacements [][2]string  `json:"replacements"`
	MethodEdits  []MethodEdit `json:"method_edits"`
}

// MethodEdit changes instructions inside one method ("Type::Method"):
//   - "ldstr":  every `ldstr Orig` (optionally only right after `ldstr After`)
//     is pointed at New, overriding the global replacements in that method;
//   - "ldnull": the ldnull right after `ldstr After` becomes `ldstr New`
//     (method body is rebuilt; only for methods without branches);
//   - "bytes":  the IL byte sequence Orig (hex) is replaced by Repl (hex, same
//     length), where "{str}" stands for the token of New.
type MethodEdit struct {
	Method string `json:"method"`
	After  string `json:"after"`
	Kind   string `json:"kind"`
	Orig   string `json:"orig"`
	New    string `json:"new"`
	Repl   string `json:"repl"`
}

type Package struct {
	M     *Manifest
	files map[string]*zip.File
}

func OpenPackage(data []byte) (*Package, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("패키지를 열 수 없습니다: %w", err)
	}
	p := &Package{files: map[string]*zip.File{}}
	for _, f := range zr.File {
		p.files[f.Name] = f
	}
	mb, err := p.Read("manifest.json")
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(mb, &m); err != nil {
		return nil, fmt.Errorf("manifest.json 형식 오류: %w", err)
	}
	if m.Format != "tbwkr-1" {
		return nil, fmt.Errorf("지원하지 않는 패키지 형식: %q", m.Format)
	}
	p.M = &m
	return p, nil
}

func (p *Package) Read(name string) ([]byte, error) {
	f, ok := p.files[name]
	if !ok {
		return nil, fmt.Errorf("패키지에 %s 파일이 없습니다", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
