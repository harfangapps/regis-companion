package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"strconv"
)

var (
	localAddrFlag  = flag.String("local-addr", "127.0.0.1:7000", "Local `address`.")
	serverAddrFlag = flag.String("server-addr", "", "SSH server `address`.")
	remoteAddrFlag = flag.String("remote-addr", "", "Remote server `address`.")
)

func main() {
	flag.Parse()

	local, err := parseAddr(*localAddrFlag)
	if err != nil {
		log.Fatalf("local address: %v", err)
	}
	server, err := parseAddr(*serverAddrFlag)
	if err != nil {
		log.Fatalf("server address: %v", err)
	}
	remote, err := parseAddr(*remoteAddrFlag)
	if err != nil {
		log.Fatalf("remote address: %v", err)
	}

	fmt.Println(local.Network(), local.String())
	fmt.Println(server.Network(), server.String())
	fmt.Println(remote.Network(), remote.String())
}

func parseAddr(s string) (net.Addr, error) {
	if s == "" {
		return nil, errors.New("missing address")
	}
	if host, port, err := net.SplitHostPort(s); err != nil {
		return &net.UnixAddr{Name: s, Net: "unix"}, nil
	} else if ip := net.ParseIP(host); ip == nil {
		return nil, fmt.Errorf("invalid address: %v", s)
	} else if nPort, err := strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("invalid port: %v: %v", port, err)
	} else {
		return &net.TCPAddr{IP: ip, Port: nPort}, nil
	}
}
