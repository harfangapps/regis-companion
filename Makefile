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
	@./misc/scripts/version_sha.sh
	@echo "Upload the archive to the v${VERSION} release on github:"
	@echo "https://github.com/harfangapps/releases/tag/v${VERSION}"

