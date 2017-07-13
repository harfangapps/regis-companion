#!/bin/bash

set -euo pipefail

srcArchivePath=$(mktemp -t regis-companion.XXXX)
sha=$(shasum -a 256 ${srcArchivePath})
echo "SHA256 for Homebrew formula: ${sha}"
unlink ${srcArchivePath}

