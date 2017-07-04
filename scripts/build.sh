#!/bin/bash

set -euo pipefail

gitHash=$(git rev-parse --short HEAD)
version=$(git describe --tags)
goVersion=$(go version | cut -d\  -f 3)

go build -o bin/regis-companion -ldflags "-X main.gitHash=${gitHash} -X main.version=${version} -X main.goVersion=${goVersion}" main.go

