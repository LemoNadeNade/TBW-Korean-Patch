package main

// Minimal reader/writer for Unity SerializedFile format version 21
// (Unity 2019.4, no type trees, little-endian metadata).

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

type sObject struct {
	PathID   int64
	Start    uint32 // relative to data offset
	Size     uint32
	TypeIdx  int32
	ClassID  int32
	entryPos int // position of byteStart field in file
	Data     []byte
}

type SFile struct {
	raw     []byte
	dataOff uint32
	Objects []*sObject
	byID    map[int64]*sObject
}

type rd struct {
	b []byte
	o int
}

func (r *rd) need(n int) error {
	if r.o+n > len(r.b) || r.o < 0 {
		return errors.New("unexpected end of metadata")
	}
	return nil
}
func (r *rd) u8() (byte, error) {
	if err := r.need(1); err != nil {
		return 0, err
	}
	v := r.b[r.o]
	r.o++
	return v, nil
}
func (r *rd) i16() (int16, error) {
	if err := r.need(2); err != nil {
		return 0, err
	}
	v := int16(binary.LittleEndian.Uint16(r.b[r.o:]))
	r.o += 2
	return v, nil
}
func (r *rd) i32() (int32, error) {
	if err := r.need(4); err != nil {
		return 0, err
	}
	v := int32(binary.LittleEndian.Uint32(r.b[r.o:]))
	r.o += 4
	return v, nil
}
func (r *rd) i64() (int64, error) {
	if err := r.need(8); err != nil {
		return 0, err
	}
	v := int64(binary.LittleEndian.Uint64(r.b[r.o:]))
	r.o += 8
	return v, nil
}
func (r *rd) cstr() (string, error) {
	for i := r.o; i < len(r.b); i++ {
		if r.b[i] == 0 {
			s := string(r.b[r.o:i])
			r.o = i + 1
			return s, nil
		}
	}
	return "", errors.New("unterminated string")
}

func ParseSFile(b []byte) (*SFile, error) {
	if len(b) < 20 {
		return nil, errors.New("file too small")
	}
	ver := binary.BigEndian.Uint32(b[8:])
	if ver != 21 {
		return nil, fmt.Errorf("unsupported serialized file version %d", ver)
	}
	fileSize := binary.BigEndian.Uint32(b[4:])
	dataOff := binary.BigEndian.Uint32(b[12:])
	if int(fileSize) != len(b) {
		return nil, fmt.Errorf("file size mismatch (%d != %d)", fileSize, len(b))
	}
	if b[16] != 0 {
		return nil, errors.New("big-endian metadata not supported")
	}
	r := &rd{b: b, o: 20}
	if _, err := r.cstr(); err != nil {
		return nil, err
	}
	if _, err := r.i32(); err != nil {
		return nil, err
	}
	tt, err := r.u8()
	if err != nil {
		return nil, err
	}
	if tt != 0 {
		return nil, errors.New("type trees not supported")
	}
	nt, err := r.i32()
	if err != nil {
		return nil, err
	}
	if nt < 0 || nt > 100000 {
		return nil, errors.New("bad type count")
	}
	classIDs := make([]int32, nt)
	for i := 0; i < int(nt); i++ {
		cid, err := r.i32()
		if err != nil {
			return nil, err
		}
		if _, err := r.u8(); err != nil {
			return nil, err
		}
		sidx, err := r.i16()
		if err != nil {
			return nil, err
		}
		if cid == 114 || sidx >= 0 {
			r.o += 16
		}
		r.o += 16
		if err := r.need(0); err != nil {
			return nil, err
		}
		classIDs[i] = cid
	}
	no, err := r.i32()
	if err != nil {
		return nil, err
	}
	if no < 0 || no > 10000000 {
		return nil, errors.New("bad object count")
	}
	f := &SFile{raw: b, dataOff: dataOff, byID: map[int64]*sObject{}}
	for i := 0; i < int(no); i++ {
		r.o = (r.o + 3) &^ 3
		pid, err := r.i64()
		if err != nil {
			return nil, err
		}
		pos := r.o
		st, err := r.i32()
		if err != nil {
			return nil, err
		}
		sz, err := r.i32()
		if err != nil {
			return nil, err
		}
		ti, err := r.i32()
		if err != nil {
			return nil, err
		}
		if ti < 0 || int(ti) >= len(classIDs) {
			return nil, errors.New("bad type index")
		}
		s, e := uint64(dataOff)+uint64(uint32(st)), uint64(dataOff)+uint64(uint32(st))+uint64(uint32(sz))
		if e > uint64(len(b)) {
			return nil, errors.New("object outside file")
		}
		o := &sObject{PathID: pid, Start: uint32(st), Size: uint32(sz), TypeIdx: ti,
			ClassID: classIDs[ti], entryPos: pos, Data: b[s:e]}
		f.Objects = append(f.Objects, o)
		f.byID[pid] = o
	}
	return f, nil
}

func (f *SFile) Get(pid int64) *sObject { return f.byID[pid] }

// Bytes rebuilds the file with the (possibly replaced) object data.
func (f *SFile) Bytes() []byte {
	order := make([]*sObject, len(f.Objects))
	copy(order, f.Objects)
	sort.SliceStable(order, func(i, j int) bool { return order[i].Start < order[j].Start })
	total := 0
	for _, o := range order {
		total = (total+7)&^7 + len(o.Data)
	}
	out := make([]byte, int(f.dataOff)+total)
	copy(out, f.raw[:f.dataOff])
	cur := 0
	for _, o := range order {
		cur = (cur + 7) &^ 7
		copy(out[int(f.dataOff)+cur:], o.Data)
		binary.LittleEndian.PutUint32(out[o.entryPos:], uint32(cur))
		binary.LittleEndian.PutUint32(out[o.entryPos+4:], uint32(len(o.Data)))
		cur += len(o.Data)
	}
	binary.BigEndian.PutUint32(out[4:], uint32(len(out)))
	return out
}
