#!/bin/bash

set -euo pipefail

importPath="bitbucket.org/harfangapps/regis-companion/server"
gitHash=$(git rev-parse --short HEAD)
version=$(git describe --tags)
goVersion=$(go version | cut -d\  -f 3)

go build -o bin/regis-companion -ldflags "-X ${importPath}.GitHash=${gitHash} -X ${importPath}.Version=${version} -X ${importPath}.GoVersion=${goVersion}" main.go

