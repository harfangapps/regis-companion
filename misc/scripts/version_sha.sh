#!/bin/bash

set -euo pipefail

version=$(git describe --tags)
srcArchivePath=$(mktemp -t regis-companion.XXXX)
curl -o ${srcArchivePath} "https://github.com/harfangapps/regis-companion/archive/${version}.tar.gz"
sha=$(shasum -a 256 ${srcArchivePath})
echo "SHA256 for Homebrew formula: ${sha}"
unlink ${srcArchivePath}

