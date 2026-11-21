package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
)

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func pad4(n int) int { return (n + 3) &^ 3 }

// encStr returns a Unity serialized string (int32 length + UTF-8 bytes),
// padded with zeros to a multiple of 4 bytes.
func encStr(s string) []byte {
	out := make([]byte, pad4(4+len(s)))
	binary.LittleEndian.PutUint32(out, uint32(len(s)))
	copy(out[4:], s)
	return out
}

type objResult int

const (
	objApplied objResult = iota // applied on an original object, verified
	objAlready                  // object already in patched state
	objPartial                  // object differed from original; applied what matched
	objSkipped                  // could not apply anything
)

type objStats struct {
	strTotal, strMissed int
}

type edit struct {
	off    int
	oldLen int // bytes consumed in the source
	repl   []byte
}

func toInt(v any) (int, error) {
	f, ok := v.(float64)
	if !ok {
		return 0, errors.New("bad offset")
	}
	return int(f), nil
}
func toStr(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", errors.New("bad string")
	}
	return s, nil
}

// PatchObject applies spec to the current object bytes. blob is the
// replacement for Blob specs (may be nil otherwise).
func PatchObject(cur []byte, spec *ObjSpec, blob []byte) ([]byte, objResult, objStats, error) {
	var st objStats
	h := sha(cur)
	if spec.Patched != "" && h == spec.Patched {
		return cur, objAlready, st, nil
	}
	exact := h == spec.Vanilla
	if spec.Blob != "" {
		if !exact {
			return cur, objSkipped, st, nil
		}
		if sha(blob) != spec.Patched {
			return nil, objSkipped, st, errors.New("패키지 데이터 손상 (폰트)")
		}
		return blob, objApplied, st, nil
	}

	src := append([]byte(nil), cur...)
	// 1) same-size byte patches
	for _, bp := range spec.Bytes {
		off, err := toInt(bp[0])
		if err != nil {
			return nil, objSkipped, st, err
		}
		oh, _ := toStr(bp[1])
		nh, _ := toStr(bp[2])
		ob, err1 := hex.DecodeString(oh)
		nb, err2 := hex.DecodeString(nh)
		if err1 != nil || err2 != nil || len(ob) != len(nb) {
			return nil, objSkipped, st, errors.New("bad byte patch")
		}
		if off < 0 || off+len(ob) > len(src) || !bytes.Equal(src[off:off+len(ob)], ob) {
			if exact {
				return nil, objSkipped, st, fmt.Errorf("byte patch mismatch at %d", off)
			}
			continue
		}
		copy(src[off:], nb)
	}

	// 2) string field replacements and substring edits, in source order
	var edits []edit
	cursor := 0
	for _, sp := range spec.Strs {
		off, err := toInt(sp[0])
		if err != nil {
			return nil, objSkipped, st, err
		}
		old, _ := toStr(sp[1])
		nw, _ := toStr(sp[2])
		st.strTotal++
		eo := encStr(old)
		pos := -1
		if exact {
			if off >= 0 && off+len(eo) <= len(src) && bytes.Equal(src[off:off+len(eo)], eo) {
				pos = off
			} else {
				return nil, objSkipped, st, fmt.Errorf("string mismatch at %d", off)
			}
		} else if len(old) >= 2 {
			pos = findAligned(src, eo, cursor)
		}
		if pos < 0 {
			st.strMissed++
			continue
		}
		edits = append(edits, edit{off: pos, oldLen: len(eo), repl: encStr(nw)})
		cursor = pos + len(eo)
	}
	for _, sb := range spec.Subs {
		off := sb.Off
		if !exact {
			off = textAssetScriptOffset(src, spec.Class)
		}
		if off < 0 || off+4 > len(src) {
			st.strTotal += len(sb.Edits)
			st.strMissed += len(sb.Edits)
			continue
		}
		n := int(binary.LittleEndian.Uint32(src[off:]))
		if n < 0 || off+4+n > len(src) {
			return nil, objSkipped, st, errors.New("bad string field")
		}
		body := src[off+4 : off+4+n]
		var out bytes.Buffer
		c := 0
		for _, e := range sb.Edits {
			st.strTotal++
			i := bytes.Index(body[c:], []byte(e[0]))
			if i < 0 {
				if exact {
					return nil, objSkipped, st, errors.New("substring not found")
				}
				st.strMissed++
				continue
			}
			out.Write(body[c : c+i])
			out.WriteString(e[1])
			c += i + len(e[0])
		}
		out.Write(body[c:])
		nb := out.Bytes()
		repl := make([]byte, pad4(4+len(nb)))
		binary.LittleEndian.PutUint32(repl, uint32(len(nb)))
		copy(repl[4:], nb)
		edits = append(edits, edit{off: off, oldLen: pad4(4 + n), repl: repl})
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].off < edits[j].off })
	var out bytes.Buffer
	c := 0
	for _, e := range edits {
		if e.off < c {
			return nil, objSkipped, st, errors.New("overlapping edits")
		}
		out.Write(src[c:e.off])
		out.Write(e.repl)
		c = e.off + e.oldLen
	}
	if c > len(src) {
		return nil, objSkipped, st, errors.New("edit past end")
	}
	out.Write(src[c:])
	res := out.Bytes()
	if exact {
		if spec.Patched != "" && sha(res) != spec.Patched {
			return nil, objSkipped, st, errors.New("결과 검증 실패")
		}
		return res, objApplied, st, nil
	}
	if bytes.Equal(res, cur) {
		return cur, objSkipped, st, nil
	}
	return res, objPartial, st, nil
}

func findAligned(b, pat []byte, from int) int {
	from = pad4(from)
	for from <= len(b)-len(pat) {
		i := bytes.Index(b[from:], pat)
		if i < 0 {
			return -1
		}
		p := from + i
		if p%4 == 0 {
			return p
		}
		from = p + 1
	}
	return -1
}

// textAssetScriptOffset locates m_Script in a TextAsset (m_Name, m_Script).
func textAssetScriptOffset(b []byte, class int32) int {
	if class != 49 || len(b) < 4 {
		return -1
	}
	n := int(binary.LittleEndian.Uint32(b))
	if n < 0 || 4+n > len(b) {
		return -1
	}
	return pad4(4 + n)
}
