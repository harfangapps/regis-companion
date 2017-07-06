#!/bin/bash

set -euo pipefail

importPath="bitbucket.org/harfangapps/regis-companion/server"
gitHash=$(git rev-parse --short HEAD)
version=$(git describe --tags)

go build -o bin/regis-companion -ldflags "-X ${importPath}.GitHash=${gitHash} -X ${importPath}.Version=${version}" main.go

