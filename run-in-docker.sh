#!/bin/sh

CURRENT=$(cd "$(dirname "$0")" && pwd)
docker run --rm -it \
    -e GO111MODULE=on \
    -v "$CURRENT":/go/src/github.com/shogo82148/server-starter \
    -w /go/src/github.com/shogo82148/server-starter golang:1.11.4 "$@"
