package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"bitbucket.org/harfangapps/regis-companion/server"
)

var (
	localAddrFlag      = flag.String("local-addr", "127.0.0.1:7000", "Local `address`.")
	serverAddrFlag     = flag.String("server-addr", "", "SSH server `address`.")
	remoteAddrFlag     = flag.String("remote-addr", "", "Remote server `address`.")
	sshUserFlag        = flag.String("ssh-user", "", "SSH `user` to connect with.")
	sshDialTimeoutFlag = flag.Duration("ssh-dial-timeout", 5*time.Second, "SSH dial `timeout`.")
)

func main() {
	flag.Parse()

	local, err := parseAddr(*localAddrFlag, 0)
	if err != nil {
		log.Fatalf("local address: %v", err)
	}
	svr, err := parseAddr(*serverAddrFlag, 22)
	if err != nil {
		log.Fatalf("server address: %v", err)
	}
	remote, err := parseAddr(*remoteAddrFlag, 0)
	if err != nil {
		log.Fatalf("remote address: %v", err)
	}

	fmt.Println("local address:", local.Network(), local.String())
	fmt.Println("server address:", svr.Network(), svr.String())
	fmt.Println("remote address:", remote.Network(), remote.String())
	fmt.Println("(make sure the required SSH keys have been `ssh-add`ed to the agent)")

	auth, err := sshAgentAuthMethod()
	if err != nil {
		log.Fatalf("ssh agent failed: %v", err)
	}

	// properly stop the Tunnel on SIGINT
	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		cancel()
	}()

	config := &ssh.ClientConfig{
		User:            *sshUserFlag,
		Timeout:         *sshDialTimeoutFlag,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	tun := &server.Tunnel{
		Local:  local,
		Server: svr,
		Remote: remote,
		Config: config,
	}
	if err := tun.ListenAndServe(ctx); err != nil {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}

func sshAgentAuthMethod() (ssh.AuthMethod, error) {
	a, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeysCallback(agent.NewClient(a).Signers), nil
}

func parseAddr(s string, defaultPort int) (net.Addr, error) {
	if s == "" {
		return nil, errors.New("missing address")
	}
	if host, port, err := net.SplitHostPort(s); err != nil {
		// not host:port, try host only
		if ip := net.ParseIP(s); ip == nil {
			// not ip, must be a unix path
			return &net.UnixAddr{Name: s, Net: "unix"}, nil
		} else {
			return &net.TCPAddr{IP: ip, Port: defaultPort}, nil
		}
	} else if ip := net.ParseIP(host); ip == nil {
		return nil, fmt.Errorf("invalid address: %v", s)
	} else if nPort, err := strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("invalid port: %v: %v", port, err)
	} else {
		if nPort == 0 {
			nPort = defaultPort
		}
		return &net.TCPAddr{IP: ip, Port: nPort}, nil
	}
}
