package main

import "fmt"

var (
	// git rev-parse --short HEAD
	gitHash string

	// git describe --tags
	version string

	// go version
	goVersion string
)

func main() {
	fmt.Println(gitHash, version, goVersion)
}
