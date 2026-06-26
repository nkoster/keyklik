#!/bin/bash
# remark: should compile as dynamic, not static!
##CGO_ENABLED=0 go build -ldflags="-extldflags=-static"
CGO_ENABLED=1 go build -ldflags="-s -w"
