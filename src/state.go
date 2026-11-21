package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const backupDirName = "TBW_KR_Backup"

type fileState struct {
	Original string `json:"original"` // sha256 of the backed-up original
	Patched  string `json:"patched"`  // sha256 of the file we wrote
}

type State struct {
	Package string                `json:"package"`
	Files   map[string]*fileState `json:"files"`
	path    string
}

func loadState(root string) *State {
	p := filepath.Join(root, backupDirName, "state.json")
	st := &State{Files: map[string]*fileState{}, path: p}
	b, err := os.ReadFile(p)
	if err == nil {
		json.Unmarshal(b, st)
		if st.Files == nil {
			st.Files = map[string]*fileState{}
		}
	}
	st.path = p
	return st
}

func (s *State) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return writeAtomic(s.path, b)
}

// writeAtomic writes data to a temporary file next to path and renames it
// over path, so a failure never leaves a half-written game file behind.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tbwkr_tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func copyFileAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	tmp := dst + ".tbwkr_tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	out.Close()
	return os.Rename(tmp, dst)
}

var errBackupDamaged = errors.New("백업 파일이 손상되었습니다")
