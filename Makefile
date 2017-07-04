.PHONY: build
build:
	go build -ldflags "-X main.gitHash=`git rev-parse --short HEAD` -X main.version=`git describe --tags` -X main.goVersion=`go version`"
