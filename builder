#!/usr/bin/env bash

set -euo pipefail

go test ./...

# remark: should compile as dynamic, not static!
##CGO_ENABLED=0 go build -ldflags="-extldflags=-static"
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o keyklik .
