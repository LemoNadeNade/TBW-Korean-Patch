#!/bin/sh
# usage: ./build.sh path/to/package.tbkr
set -e
cp "$1" patchdata.tbkr
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o TBW_KR_Patcher.exe .
: > patchdata.tbkr
