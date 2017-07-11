package main

import (
	"context"
	"expvar"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"bitbucket.org/harfangapps/regis-companion/server"
	"bitbucket.org/harfangapps/regis-companion/sshconfig"
)

var (
	versionFlag           = flag.Bool("version", false, "Print the version.")
	addrFlag              = flag.String("addr", "127.0.0.1", "Server `address` to bind to.")
	portFlag              = flag.Int("port", 7070, "Port `number` to listen on.")
	tunnelIdleTimeoutFlag = flag.Duration("tunnel-idle-timeout", 30*time.Minute, "Idle `timeout` for inactive SSH tunnels.")
	writeTimeoutFlag      = flag.Duration("write-timeout", 30*time.Second, "Write `timeout`.")
	sshDialTimeoutFlag    = flag.Duration("ssh-dial-timeout", 30*time.Second, "SSH dial `timeout`.")
	knownHostsFileFlag    = flag.String("known-hosts-file", "${HOME}/.ssh/known_hosts", "Known hosts `file`.")
)

func main() {
	flag.Parse()

	if *versionFlag {
		fmt.Printf("%s (git:%s go:%s)\n", server.Version, server.GitHash, runtime.Version())
		return
	}

	ip := net.ParseIP(*addrFlag)
	if ip == nil {
		log.Fatalf("invalid address: %v", *addrFlag)
	}

	// handle SIGINT and SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		fmt.Println("received stop signal, stopping...")
		cancel()
	}()

	// configure and start the server
	meta := &sshconfig.MetaConfig{
		KnownHostsFile: os.ExpandEnv(*knownHostsFileFlag),
		SSHDialTimeout: *sshDialTimeoutFlag,
	}

	srv := &server.Server{
		Addr:              &net.TCPAddr{IP: ip, Port: *portFlag},
		MetaConfig:        meta,
		TunnelIdleTimeout: *tunnelIdleTimeoutFlag,
		WriteTimeout:      *writeTimeoutFlag,
		Stats:             expvar.NewMap("server"),
	}
	if err := srv.ListenAndServe(ctx); err != nil {
		log.Fatalf("exit with error: %v", err)
	}
}
