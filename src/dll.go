package main

// Replaces string literals (ldstr) in a .NET assembly without any external
// library: new strings are appended to a copy of the #US heap, every ldstr
// that loads an original string is pointed at its translation, and the
// rebuilt metadata is placed at the end of the last PE section.

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

type peSection struct {
	hdr                    int // file offset of section header
	va, vsize, rawOff, raw uint32
}

type peImage struct {
	b              []byte
	opt            int
	pe32plus       bool
	salign, falign uint32
	secs           []peSection
	cliRVA         uint32
}

func le32(b []byte, o int) uint32 { return binary.LittleEndian.Uint32(b[o:]) }
func le16(b []byte, o int) uint16 { return binary.LittleEndian.Uint16(b[o:]) }

func parsePE(b []byte) (*peImage, error) {
	if len(b) < 0x40 || b[0] != 'M' || b[1] != 'Z' {
		return nil, errors.New("not a PE file")
	}
	pe := int(le32(b, 0x3c))
	if pe+24 > len(b) || string(b[pe:pe+4]) != "PE\x00\x00" {
		return nil, errors.New("bad PE header")
	}
	nsec := int(le16(b, pe+6))
	soh := int(le16(b, pe+20))
	opt := pe + 24
	magic := le16(b, opt)
	img := &peImage{b: b, opt: opt, pe32plus: magic == 0x20b}
	img.salign = le32(b, opt+32)
	img.falign = le32(b, opt+36)
	dd := opt + 96
	if img.pe32plus {
		dd = opt + 112
	}
	img.cliRVA = le32(b, dd+14*8)
	sec := opt + soh
	for i := 0; i < nsec; i++ {
		o := sec + i*40
		img.secs = append(img.secs, peSection{hdr: o, vsize: le32(b, o+8), va: le32(b, o+12),
			raw: le32(b, o+16), rawOff: le32(b, o+20)})
	}
	if img.cliRVA == 0 {
		return nil, errors.New("not a .NET assembly")
	}
	return img, nil
}

func (p *peImage) off(rva uint32) (int, error) {
	for _, s := range p.secs {
		if rva >= s.va && rva < s.va+s.raw {
			return int(rva - s.va + s.rawOff), nil
		}
	}
	return 0, fmt.Errorf("RVA %x not mapped", rva)
}

type mdStream struct {
	name    string
	off, sz int // relative to metadata root
	hdrPos  int // absolute position of the header's offset field
}

func decodeCompressed(b []byte, o int) (val, n int, err error) {
	if o >= len(b) {
		return 0, 0, errors.New("eof")
	}
	c := b[o]
	switch {
	case c&0x80 == 0:
		return int(c), 1, nil
	case c&0xC0 == 0x80:
		if o+1 >= len(b) {
			return 0, 0, errors.New("eof")
		}
		return int(c&0x3F)<<8 | int(b[o+1]), 2, nil
	case c&0xE0 == 0xC0:
		if o+3 >= len(b) {
			return 0, 0, errors.New("eof")
		}
		return int(c&0x1F)<<24 | int(b[o+1])<<16 | int(b[o+2])<<8 | int(b[o+3]), 4, nil
	}
	return 0, 0, errors.New("bad compressed int")
}

func encodeCompressed(v int) []byte {
	switch {
	case v < 0x80:
		return []byte{byte(v)}
	case v < 0x4000:
		return []byte{byte(0x80 | v>>8), byte(v)}
	default:
		return []byte{byte(0xC0 | v>>24), byte(v >> 16), byte(v >> 8), byte(v)}
	}
}

func encodeUS(s string) []byte {
	u := utf16.Encode([]rune(s))
	body := make([]byte, 2*len(u)+1)
	special := byte(0)
	for i, c := range u {
		binary.LittleEndian.PutUint16(body[2*i:], c)
		lo := c & 0xFF
		if c >= 0x100 || (lo >= 0x01 && lo <= 0x08) || (lo >= 0x0E && lo <= 0x1F) || lo == 0x27 || lo == 0x2D || lo == 0x7F {
			special = 1
		}
	}
	body[len(body)-1] = special
	return append(encodeCompressed(len(body)), body...)
}

// ECMA-335 table schema, enough to compute row sizes.
const (
	cUS = iota // placeholder
)

type col struct {
	kind int   // 0 fixed, 1 string heap, 2 guid heap, 3 blob heap, 4 table index, 5 coded index
	size int   // fixed size
	tab  int   // table for kind 4
	code []int // tables for kind 5 (-1 = unused tag)
}

func fx(n int) col      { return col{kind: 0, size: n} }
func sidx() col         { return col{kind: 1} }
func gidx() col         { return col{kind: 2} }
func bidx() col         { return col{kind: 3} }
func tidx(t int) col    { return col{kind: 4, tab: t} }
func cidx(t ...int) col { return col{kind: 5, code: t} }

var (
	ciTypeDefOrRef    = []int{0x02, 0x01, 0x1B}
	ciHasConstant     = []int{0x04, 0x08, 0x17}
	ciHasCustomAttr   = []int{0x06, 0x04, 0x01, 0x02, 0x08, 0x09, 0x0A, 0x00, 0x0E, 0x17, 0x14, 0x11, 0x1A, 0x1B, 0x20, 0x23, 0x26, 0x27, 0x28, 0x2A, 0x2C, 0x2B}
	ciHasFieldMarshal = []int{0x04, 0x08}
	ciHasDeclSecurity = []int{0x02, 0x06, 0x20}
	ciMemberRefParent = []int{0x02, 0x01, 0x1A, 0x06, 0x1B}
	ciHasSemantics    = []int{0x14, 0x17}
	ciMethodDefOrRef  = []int{0x06, 0x0A}
	ciMemberForwarded = []int{0x04, 0x06}
	ciImplementation  = []int{0x26, 0x23, 0x27}
	ciCustomAttrType  = []int{-1, -1, 0x06, 0x0A, -1}
	ciResolutionScope = []int{0x00, 0x1A, 0x23, 0x01}
	ciTypeOrMethodDef = []int{0x02, 0x06}
)

var schema = map[int][]col{
	0x00: {fx(2), sidx(), gidx(), gidx(), gidx()},
	0x01: {cidx(ciResolutionScope...), sidx(), sidx()},
	0x02: {fx(4), sidx(), sidx(), cidx(ciTypeDefOrRef...), tidx(0x04), tidx(0x06)},
	0x03: {tidx(0x04)},
	0x04: {fx(2), sidx(), bidx()},
	0x05: {tidx(0x06)},
	0x06: {fx(4), fx(2), fx(2), sidx(), bidx(), tidx(0x08)},
	0x07: {tidx(0x08)},
	0x08: {fx(2), fx(2), sidx()},
	0x09: {tidx(0x02), cidx(ciTypeDefOrRef...)},
	0x0A: {cidx(ciMemberRefParent...), sidx(), bidx()},
	0x0B: {fx(2), cidx(ciHasConstant...), bidx()},
	0x0C: {cidx(ciHasCustomAttr...), cidx(ciCustomAttrType...), bidx()},
	0x0D: {cidx(ciHasFieldMarshal...), bidx()},
	0x0E: {fx(2), cidx(ciHasDeclSecurity...), bidx()},
	0x0F: {fx(2), fx(4), tidx(0x02)},
	0x10: {fx(4), tidx(0x04)},
	0x11: {bidx()},
	0x12: {tidx(0x02), tidx(0x14)},
	0x13: {tidx(0x14)},
	0x14: {fx(2), sidx(), cidx(ciTypeDefOrRef...)},
	0x15: {tidx(0x02), tidx(0x17)},
	0x16: {tidx(0x17)},
	0x17: {fx(2), sidx(), bidx()},
	0x18: {fx(2), tidx(0x06), cidx(ciHasSemantics...)},
	0x19: {tidx(0x02), cidx(ciMethodDefOrRef...), cidx(ciMethodDefOrRef...)},
	0x1A: {sidx()},
	0x1B: {bidx()},
	0x1C: {fx(2), cidx(ciMemberForwarded...), sidx(), tidx(0x1A)},
	0x1D: {fx(4), tidx(0x04)},
	0x1E: {fx(4), fx(4)},
	0x1F: {fx(4)},
	0x20: {fx(4), fx(2), fx(2), fx(2), fx(2), fx(4), bidx(), sidx(), sidx()},
	0x21: {fx(4)},
	0x22: {fx(4), fx(4), fx(4)},
	0x23: {fx(2), fx(2), fx(2), fx(2), fx(4), bidx(), sidx(), sidx(), bidx()},
	0x24: {fx(4), tidx(0x23)},
	0x25: {fx(4), fx(4), fx(4), tidx(0x23)},
	0x26: {fx(4), sidx(), bidx()},
	0x27: {fx(4), fx(4), sidx(), sidx(), cidx(ciImplementation...)},
	0x28: {fx(4), fx(4), sidx(), cidx(ciImplementation...)},
	0x29: {tidx(0x02), tidx(0x02)},
	0x2A: {fx(2), fx(2), cidx(ciTypeOrMethodDef...), sidx()},
	0x2B: {cidx(ciMethodDefOrRef...), bidx()},
	0x2C: {tidx(0x2A), cidx(ciTypeDefOrRef...)},
}

// IL operand sizes. -1 = switch.
var op1 = func() [256]int {
	var t [256]int
	for _, o := range []int{0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13, 0x1F, 0xDE} {
		t[o] = 1
	}
	for o := 0x2B; o <= 0x37; o++ {
		t[o] = 1
	}
	for _, o := range []int{0x20, 0x22, 0x27, 0x28, 0x29, 0x6F, 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x79,
		0x7B, 0x7C, 0x7D, 0x7E, 0x7F, 0x80, 0x81, 0x8C, 0x8D, 0x8F, 0xA3, 0xA4, 0xA5, 0xC2, 0xC6, 0xD0, 0xDD} {
		t[o] = 4
	}
	for o := 0x38; o <= 0x44; o++ {
		t[o] = 4
	}
	t[0x21], t[0x23] = 8, 8
	t[0x45] = -1
	return t
}()

var op2 = map[byte]int{0x06: 4, 0x07: 4, 0x09: 2, 0x0A: 2, 0x0B: 2, 0x0C: 2, 0x0D: 2, 0x0E: 2,
	0x12: 1, 0x15: 4, 0x16: 4, 0x19: 1, 0x1C: 4}

type DLLResult struct {
	Edited   int // targeted method edits applied
	Replaced int // ldstr instructions changed
	Strings  int // distinct originals found
	Missing  []string
	Already  int // translations already present (re-run on patched file)
}

func PatchDLL(src []byte, repl [][2]string, edits []MethodEdit) ([]byte, DLLResult, error) {
	var res DLLResult
	b := append([]byte(nil), src...)
	img, err := parsePE(b)
	if err != nil {
		return nil, res, err
	}
	cli, err := img.off(img.cliRVA)
	if err != nil {
		return nil, res, err
	}
	mdRVA, mdSize := le32(b, cli+8), le32(b, cli+12)
	md, err := img.off(mdRVA)
	if err != nil {
		return nil, res, err
	}
	if le32(b, md) != 0x424A5342 {
		return nil, res, errors.New("bad metadata signature")
	}
	vlen := int(le32(b, md+12))
	p := md + 16 + vlen + 2
	nstreams := int(le16(b, p))
	p += 2
	var streams []mdStream
	for i := 0; i < nstreams; i++ {
		so, ss := int(le32(b, p)), int(le32(b, p+4))
		hp := p
		p += 8
		e := p
		for b[e] != 0 {
			e++
		}
		name := string(b[p:e])
		p = (e + 1 + 3) &^ 3
		streams = append(streams, mdStream{name: name, off: so, sz: ss, hdrPos: hp})
	}
	hdrEnd := p - md
	var us, tbl, strs *mdStream
	for i := range streams {
		switch streams[i].name {
		case "#US":
			us = &streams[i]
		case "#~", "#-":
			tbl = &streams[i]
		case "#Strings":
			strs = &streams[i]
		}
	}
	if us == nil || tbl == nil || strs == nil {
		return nil, res, errors.New("metadata streams not found")
	}

	// --- read #US heap
	heap := b[md+us.off : md+us.off+us.sz]
	byOff := map[int]string{}
	offOf := map[string]int{}
	for o := 1; o < len(heap); {
		n, k, err := decodeCompressed(heap, o)
		if err != nil {
			break
		}
		if n == 0 {
			o += k
			continue
		}
		if o+k+n > len(heap) {
			break
		}
		raw := heap[o+k : o+k+n-1]
		u := make([]uint16, len(raw)/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(raw[2*i:])
		}
		s := string(utf16.Decode(u))
		byOff[o] = s
		if _, ok := offOf[s]; !ok {
			offOf[s] = o
		}
		o += k + n
	}

	// --- new heap: old heap + appended translations
	newHeap := append([]byte(nil), heap...)
	target := map[int]int{} // old offset -> new offset
	transSet := map[string]bool{}
	for _, r := range repl {
		transSet[r[1]] = true
	}
	added := map[string]int{}
	for _, r := range repl {
		if r[0] == r[1] {
			continue
		}
		found := false
		for o, s := range byOff {
			if s != r[0] {
				continue
			}
			found = true
			no, ok := added[r[1]]
			if !ok {
				if eo, ok2 := offOf[r[1]]; ok2 {
					no = eo
				} else {
					no = len(newHeap)
					newHeap = append(newHeap, encodeUS(r[1])...)
				}
				added[r[1]] = no
			}
			target[o] = no
		}
		if found {
			res.Strings++
		} else if _, ok := offOf[r[1]]; ok {
			res.Already++
		} else {
			res.Missing = append(res.Missing, r[0])
		}
	}
	strOff := func(t string) int {
		if no, ok := added[t]; ok {
			return no
		}
		if eo, ok := offOf[t]; ok {
			added[t] = eo
			return eo
		}
		no := len(newHeap)
		newHeap = append(newHeap, encodeUS(t)...)
		added[t] = no
		byOff[no] = t
		return no
	}
	for _, e := range edits {
		strOff(e.New)
	}
	if len(newHeap) >= 1<<24 {
		return nil, res, errors.New("string heap too large")
	}

	// --- locate MethodDef rows
	t := md + tbl.off
	heapSizes := b[t+6]
	valid := binary.LittleEndian.Uint64(b[t+8:])
	rows := map[int]int{}
	q := t + 24
	for i := 0; i < 64; i++ {
		if valid&(1<<uint(i)) != 0 {
			rows[i] = int(le32(b, q))
			q += 4
		}
	}
	if heapSizes&0x40 != 0 {
		q += 4
	}
	strSz, guidSz, blobSz := 2, 2, 2
	if heapSizes&1 != 0 {
		strSz = 4
	}
	if heapSizes&2 != 0 {
		guidSz = 4
	}
	if heapSizes&4 != 0 {
		blobSz = 4
	}
	colSize := func(c col) int {
		switch c.kind {
		case 0:
			return c.size
		case 1:
			return strSz
		case 2:
			return guidSz
		case 3:
			return blobSz
		case 4:
			if rows[c.tab] < 1<<16 {
				return 2
			}
			return 4
		default:
			bits := 0
			for (1 << uint(bits)) < len(c.code) {
				bits++
			}
			mx := 0
			for _, tb := range c.code {
				if tb >= 0 && rows[tb] > mx {
					mx = rows[tb]
				}
			}
			if mx < 1<<uint(16-bits) {
				return 2
			}
			return 4
		}
	}
	rowSize := func(tab int) (int, error) {
		cols, ok := schema[tab]
		if !ok {
			return 0, fmt.Errorf("unknown table %x", tab)
		}
		n := 0
		for _, c := range cols {
			n += colSize(c)
		}
		return n, nil
	}
	pos := q
	for tab := 0; tab < 6; tab++ {
		if valid&(1<<uint(tab)) == 0 {
			continue
		}
		rs, err := rowSize(tab)
		if err != nil {
			return nil, res, err
		}
		pos += rs * rows[tab]
	}
	mrs, err := rowSize(6)
	if err != nil {
		return nil, res, err
	}

	// --- method names ("Type::Method") from TypeDef.MethodList ranges
	strHeap := b[md+strs.off : md+strs.off+strs.sz]
	strAt := func(i int) string {
		if i < 0 || i >= len(strHeap) {
			return ""
		}
		e := i
		for e < len(strHeap) && strHeap[e] != 0 {
			e++
		}
		return string(strHeap[i:e])
	}
	readIdx := func(p, sz int) int {
		if sz == 4 {
			return int(le32(b, p))
		}
		return int(le16(b, p))
	}
	tdPos := q
	for tab := 0; tab < 2; tab++ {
		if valid&(1<<uint(tab)) != 0 {
			rs, _ := rowSize(tab)
			tdPos += rs * rows[tab]
		}
	}
	tdCols := schema[0x02]
	tdSize, _ := rowSize(0x02)
	tdNameOff := colSize(tdCols[0])
	tdMLOff := 0
	for k := 0; k < 5; k++ {
		tdMLOff += colSize(tdCols[k])
	}
	mlSize := colSize(tdCols[5])
	ownerName := make([]string, rows[6]+1)
	for t := 1; t <= rows[2]; t++ {
		row := tdPos + (t-1)*tdSize
		start := readIdx(row+tdMLOff, mlSize)
		end := rows[6] + 1
		if t < rows[2] {
			end = readIdx(row+tdSize+tdMLOff, mlSize)
		}
		name := strAt(readIdx(row+tdNameOff, strSz))
		for m := start; m < end && m <= rows[6]; m++ {
			if m >= 1 {
				ownerName[m] = name
			}
		}
	}
	editUsed := make([]bool, len(edits))
	editIdx := map[string][]int{}
	for k, e := range edits {
		editIdx[e.Method] = append(editIdx[e.Method], k)
	}

	// --- rewrite ldstr tokens
	type reloc struct {
		row  int
		body []byte
	}
	var relocs []reloc
	seen := map[uint32]bool{}
	for i := 0; i < rows[6]; i++ {
		rva := le32(b, pos+i*mrs)
		if rva == 0 || seen[rva] {
			continue
		}
		seen[rva] = true
		mo, err := img.off(rva)
		if err != nil {
			return nil, res, err
		}
		h := b[mo]
		var code, size int
		if h&3 == 2 {
			code, size = mo+1, int(h>>2)
		} else if h&3 == 3 {
			code, size = mo+int(le16(b, mo)>>12)*4, int(le32(b, mo+4))
		} else {
			return nil, res, fmt.Errorf("bad method header at %x", rva)
		}
		end := code + size
		if end > len(b) {
			return nil, res, errors.New("method body out of range")
		}
		full := ownerName[i+1] + "::" + strAt(readIdx(pos+i*mrs+8, strSz))
		mine := editIdx[full]
		prevStr := ""
		for ip := code; ip < end; {
			op := b[ip]
			ip++
			var n int
			if op == 0xFE {
				n = op2[b[ip]]
				ip++
			} else {
				n = op1[op]
			}
			cur := ""
			if op == 0x72 {
				tok := le32(b, ip)
				if tok>>24 == 0x70 {
					cur = byOff[int(tok&0xFFFFFF)]
					done := false
					for _, k := range mine {
						e := edits[k]
						if e.Kind == "ldstr" && e.Orig == cur && (e.After == "" || e.After == prevStr) {
							binary.LittleEndian.PutUint32(b[ip:], 0x70000000|uint32(strOff(e.New)))
							editUsed[k], done = true, true
							break
						}
					}
					if !done {
						if no, ok := target[int(tok&0xFFFFFF)]; ok {
							binary.LittleEndian.PutUint32(b[ip:], 0x70000000|uint32(no))
							res.Replaced++
						}
					}
				}
			}
			prevStr = cur
			if n == -1 {
				cnt := int(le32(b, ip))
				ip += 4 + 4*cnt
			} else {
				ip += n
			}
		}
		// same-size byte edits
		for _, k := range mine {
			e := edits[k]
			if e.Kind != "bytes" {
				continue
			}
			find, err1 := hex.DecodeString(e.Orig)
			tmpl := strings.ReplaceAll(e.Repl, "{str}", fmt.Sprintf("%08x", 0))
			rep, err2 := hex.DecodeString(tmpl)
			if err1 != nil || err2 != nil || len(find) != len(rep) || len(find) == 0 {
				return nil, res, fmt.Errorf("%s: bad byte edit", full)
			}
			if j := strings.Index(e.Repl, "{str}"); j >= 0 {
				binary.LittleEndian.PutUint32(rep[j/2:], 0x70000000|uint32(strOff(e.New)))
			}
			if at := bytes.Index(b[code:end], find); at >= 0 && bytes.Count(b[code:end], find) == 1 {
				copy(b[code+at:], rep)
				editUsed[k] = true
			}
		}
		// ldnull -> ldstr needs a rebuilt body
		var nulls []MethodEdit
		var nullIdx []int
		for _, k := range mine {
			if edits[k].Kind == "ldnull" {
				nulls = append(nulls, edits[k])
				nullIdx = append(nullIdx, k)
			}
		}
		if len(nulls) > 0 {
			body, used, err := editMethod(b, mo, code, size, nulls, byOff, strOff)
			if err != nil {
				return nil, res, fmt.Errorf("%s: %w", full, err)
			}
			if body != nil {
				relocs = append(relocs, reloc{row: pos + i*mrs, body: body})
				for u, k := range nullIdx {
					if used[u] {
						editUsed[k] = true
					}
				}
			}
		}
	}
	for k, u := range editUsed {
		if u {
			res.Edited++
		} else {
			res.Missing = append(res.Missing, edits[k].Method+": "+edits[k].Orig+edits[k].After)
		}
	}
	if res.Replaced == 0 && res.Edited == 0 {
		return src, res, nil
	}

	// --- layout of the appended region: [relocated method bodies][metadata]
	last := img.secs[len(img.secs)-1]
	if int(last.rawOff+last.raw) != len(b) {
		return nil, res, errors.New("unexpected data after last section")
	}
	start := (last.vsize + 15) &^ 15
	if start > last.raw {
		return nil, res, errors.New("last section layout not supported")
	}
	var bodies []byte
	for _, r := range relocs {
		for len(bodies)%4 != 0 {
			bodies = append(bodies, 0)
		}
		binary.LittleEndian.PutUint32(b[r.row:], last.va+start+uint32(len(bodies)))
		bodies = append(bodies, r.body...)
	}
	for len(bodies)%4 != 0 {
		bodies = append(bodies, 0)
	}

	// --- rebuild metadata blob (same stream order, #US replaced)
	for len(newHeap)%4 != 0 {
		newHeap = append(newHeap, 0)
	}
	oldMD := b[md : md+int(mdSize)]
	nm := append([]byte(nil), oldMD[:hdrEnd]...)
	for i := range streams {
		s := &streams[i]
		data := oldMD[s.off : s.off+s.sz]
		if s == us {
			data = newHeap
		}
		for len(nm)%4 != 0 {
			nm = append(nm, 0)
		}
		no := len(nm)
		nm = append(nm, data...)
		hp := s.hdrPos - md
		binary.LittleEndian.PutUint32(nm[hp:], uint32(no))
		binary.LittleEndian.PutUint32(nm[hp+4:], uint32(len(data)))
	}

	region := append(bodies, nm...)
	mdStart := start + uint32(len(bodies))
	newV := start + uint32(len(region))
	newRaw := (newV + img.falign - 1) / img.falign * img.falign
	out := make([]byte, int(last.rawOff+newRaw))
	copy(out, b)
	copy(out[last.rawOff+start:], region)
	binary.LittleEndian.PutUint32(out[last.hdr+8:], newV)
	binary.LittleEndian.PutUint32(out[last.hdr+16:], newRaw)
	ch := le32(out, last.hdr+36)
	ch &^= 0x02000000 // not discardable
	ch |= 0x40000040  // initialized data, readable
	binary.LittleEndian.PutUint32(out[last.hdr+36:], ch)
	soi := (last.va + newV + img.salign - 1) / img.salign * img.salign
	binary.LittleEndian.PutUint32(out[img.opt+56:], soi)
	binary.LittleEndian.PutUint32(out[img.opt+64:], 0) // checksum
	binary.LittleEndian.PutUint32(out[cli+8:], last.va+mdStart)
	binary.LittleEndian.PutUint32(out[cli+12:], uint32(len(nm)))
	return out, res, nil
}

// editMethod rebuilds a fat method body that has no branches and no
// exception sections, applying the given edits. It returns the new body
// (header + code) and the number of edits applied.
func editMethod(b []byte, mo, code, size int, es []MethodEdit, byOff map[int]string, strOff func(string) int) ([]byte, []bool, error) {
	if b[mo]&3 != 3 {
		return nil, nil, errors.New("tiny method header not supported")
	}
	flags := le16(b, mo)
	if flags&0x08 != 0 {
		return nil, nil, errors.New("method has exception sections")
	}
	hdrSize := int(flags>>12) * 4
	var newCode []byte
	used := make([]bool, len(es))
	prevStr := ""
	prevIsStr := false
	for ip := code; ip < code+size; {
		op := b[ip]
		start := ip
		ip++
		var n int
		if op == 0xFE {
			n = op2[b[ip]]
			ip++
		} else {
			n = op1[op]
		}
		if (op >= 0x2B && op <= 0x45) || op == 0xDD || op == 0xDE {
			return nil, nil, errors.New("method has branches")
		}
		if n == -1 {
			return nil, nil, errors.New("method has switch")
		}
		ip += n
		ins := b[start:ip]
		curStr, curIsStr := "", false
		if op == 0x72 {
			curStr, curIsStr = byOff[int(le32(b, start+1)&0xFFFFFF)], true
		}
		replaced := false
		if prevIsStr {
			for k, e := range es {
				if used[k] || e.After != prevStr {
					continue
				}
				if (e.Kind == "ldnull" && op == 0x14) || (e.Kind == "ldstr" && curIsStr && curStr == e.Orig) {
					tok := make([]byte, 5)
					tok[0] = 0x72
					binary.LittleEndian.PutUint32(tok[1:], 0x70000000|uint32(strOff(e.New)))
					newCode = append(newCode, tok...)
					used[k], replaced = true, true
					break
				}
			}
		}
		if !replaced {
			newCode = append(newCode, ins...)
		}
		prevStr, prevIsStr = curStr, curIsStr
	}
	applied := 0
	for _, u := range used {
		if u {
			applied++
		}
	}
	if applied == 0 {
		return nil, used, nil
	}
	hdr := append([]byte(nil), b[mo:mo+hdrSize]...)
	binary.LittleEndian.PutUint32(hdr[4:], uint32(len(newCode)))
	return append(hdr, newCode...), used, nil
}

// usHangulCount counts Hangul syllables in the assembly's #US heap. The
// original game has (practically) none, so a large count means some Korean
// patch has already rewritten this file.
func usHangulCount(b []byte) int {
	img, err := parsePE(b)
	if err != nil {
		return 0
	}
	cli, err := img.off(img.cliRVA)
	if err != nil || cli+16 > len(b) {
		return 0
	}
	md, err := img.off(le32(b, cli+8))
	if err != nil || md+16 > len(b) || le32(b, md) != 0x424A5342 {
		return 0
	}
	p := md + 16 + int(le32(b, md+12)) + 2
	if p+2 > len(b) {
		return 0
	}
	n := int(le16(b, p))
	p += 2
	for i := 0; i < n && p+8 < len(b); i++ {
		so, ss := int(le32(b, p)), int(le32(b, p+4))
		p += 8
		e := p
		for e < len(b) && b[e] != 0 {
			e++
		}
		name := string(b[p:e])
		p = (e + 1 + 3) &^ 3
		if name != "#US" || md+so+ss > len(b) {
			continue
		}
		heap := b[md+so : md+so+ss]
		count := 0
		for o := 1; o < len(heap); {
			l, k, err := decodeCompressed(heap, o)
			if err != nil || o+k+l > len(heap) {
				break
			}
			for c := o + k; c+1 < o+k+l; c += 2 {
				r := rune(le16(heap, c))
				if r >= 0xAC00 && r <= 0xD7A3 {
					count++
				}
			}
			if l == 0 {
				o += k
			} else {
				o += k + l
			}
		}
		return count
	}
	return 0
}
