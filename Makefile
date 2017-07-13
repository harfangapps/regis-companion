# get the version from the latest git tag if not set, removing the leading "v"
# If set, VERSION should look like "1.2.3", that is, without the leading "v".
VERSION ?= $(shell git describe --tags | cut -d v -f 2)

# The release target creates a release for the command.
# The version must be tagged first in git (using `git tag vM.m.p`) and
# must be pushed to github.
.PHONY: release
release:
	@./misc/scripts/build.sh
	@./bin/regis-companion -version
	@tar -czf ./bin/regis-companion_${VERSION}_macOS-64bit.tar.gz ./bin/regis-companion
	@echo "Upload the archive to the v${VERSION} release on github and publish it, then run 'make brew' and upate the tap:"
	@echo "https://github.com/harfangapps/regis-companion/releases/tag/v${VERSION}"

.PHONY: brew
brew:
	@./misc/scripts/version_sha.sh
