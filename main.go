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

	"github.com/harfangapps/regis-companion/server"
)

var (
	versionFlag              = flag.Bool("version", false, "Print the version.")
	generateLaunchdPlistFlag = flag.Bool("generate-launchd-plist", false, "Generate a skeleton launchd `plist` file.")

	addrFlag              = flag.String("addr", "127.0.0.1", "The `address` to bind to.")
	portFlag              = flag.Int("port", 7070, "Port `number` to listen on.")
	tunnelIdleTimeoutFlag = flag.Duration("tunnel-idle-timeout", 30*time.Minute, "Idle `timeout` for inactive SSH tunnels.")
	writeTimeoutFlag      = flag.Duration("write-timeout", 30*time.Second, "Write `timeout`.")
	sshDialTimeoutFlag    = flag.Duration("ssh-dial-timeout", 30*time.Second, "SSH dial `timeout`.")
	knownHostsFileFlag    = flag.String("known-hosts-file", "${HOME}/.ssh/known_hosts", "Known hosts `file`.")
)

var plistTemplate = `
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
	<dict>
		<key>KeepAlive</key>
		<true/>
		<key>Label</key>
		<string>com.harfangapps.regis-companion</string>
		<key>ProgramArguments</key>
		<array>
			<string>${EXECUTABLE}</string>
		</array>
		<key>RunAtLoad</key>
		<true/>
		<key>WorkingDirectory</key>
		<string>${VARDIR}</string>
		<key>StandardErrorPath</key>
		<string>${VARDIR}/log/regis-companion.log</string>
		<key>StandardOutPath</key>
		<string>${VARDIR}/log/regis-companion.log</string>
	</dict>
</plist>
`

const defaultVarDir = "/usr/local/var"

func replaceVar(v string) string {
	switch v {
	case "EXECUTABLE":
		exe, err := os.Executable()
		if err == nil {
			return exe
		}
	case "VARDIR":
		fi, err := os.Stat(defaultVarDir)
		if err != nil || !fi.IsDir() {
			// use temp dir if var dir does not exist
			return os.TempDir()
		}
		return defaultVarDir
	}
	return ""
}

func main() {
	flag.Parse()

	if *versionFlag {
		fmt.Printf("%s (git:%s go:%s)\n", server.Version, server.GitHash, runtime.Version())
		return
	}
	if *generateLaunchdPlistFlag {
		fmt.Println(os.Expand(plistTemplate, replaceVar))
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
	meta := &server.MetaConfig{
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
