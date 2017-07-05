package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	"bitbucket.org/harfangapps/regis-companion/server"
)

var (
	// git rev-parse --short HEAD
	gitHash string

	// git describe --tags
	version string

	// go version
	goVersion string
)

var (
	versionFlag           = flag.Bool("version", false, "Print the version.")
	addrFlag              = flag.String("addr", "127.0.0.1", "Server `address` to bind to.")
	portFlag              = flag.Int("port", 7070, "Port `number` to listen on.")
	tunnelIdleTimeoutFlag = flag.Duration("tunnel-idle-timeout", 30*time.Minute, "Idle `timeout` for inactive SSH tunnels.")
	writeTimeoutFlag      = flag.Duration("write-timeout", 30*time.Second, "Write `timeout`.")
)

func main() {
	flag.Parse()

	if *versionFlag {
		fmt.Printf("%s (git:%s go:%s)\n", version, gitHash, goVersion)
	}

	ip := net.ParseIP(*addrFlag)
	if ip == nil {
		log.Fatalf("invalid address: %v", *addrFlag)
	}

	// handle SIGINT
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		cancel()
	}()

	// configure and start the server
	srv := &server.Server{
		Addr:              &net.TCPAddr{IP: ip, Port: *portFlag},
		TunnelIdleTimeout: *tunnelIdleTimeoutFlag,
		WriteTimeout:      *writeTimeoutFlag,
	}
	if err := srv.ListenAndServe(ctx); err != nil {
		log.Fatalf("exit with error %v", err)
	}
}