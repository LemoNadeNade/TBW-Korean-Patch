package main

import (
	_ "embed"
	"errors"
)

//go:embed patchdata.tbkr
var embedded []byte

func embeddedPackage() ([]byte, error) {
	if len(embedded) == 0 {
		return nil, errors.New("이 실행 파일에는 패치 데이터가 들어 있지 않습니다")
	}
	return embedded, nil
}
