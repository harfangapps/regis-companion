package server

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"

	"bitbucket.org/harfangapps/regis-companion/addr"
	"bitbucket.org/harfangapps/regis-companion/internal/testutils"
	"bitbucket.org/harfangapps/regis-companion/resp"
	"bitbucket.org/harfangapps/regis-companion/sshconfig"
	"bitbucket.org/harfangapps/regis-companion/tunnel"
	"golang.org/x/crypto/ssh"
)

func mockListenFunc(mockListener net.Listener) func(net.Addr) (net.Listener, int, error) {
	port := 40000
	return func(addr net.Addr) (net.Listener, int, error) {
		port++
		return mockListener, port, nil
	}
}

func setAndDeferListenFunc(fn func(addr net.Addr) (net.Listener, int, error)) func() {
	addr.ListenFunc = fn
	return func() {
		addr.ListenFunc = addr.Listen
	}
}

func mockSSHDial(dc tunnel.DialCloser) func(n, a string, conf *ssh.ClientConfig) (tunnel.DialCloser, error) {
	return func(n, a string, conf *ssh.ClientConfig) (tunnel.DialCloser, error) {
		return dc, nil
	}
}

func setAndDeferSSHDial(fn func(n, a string, conf *ssh.ClientConfig) (tunnel.DialCloser, error)) func() {
	tunnel.SSHDialFunc = fn
	return func() {
		tunnel.SSHDialFunc = tunnel.DefaultSSHDial
	}
}

func TestGetTunnelAddrTwiceReturnsSameAddr(t *testing.T) {
	// create the server listener, that returns the conn that will
	// send the gettunneladdr command twice.
	closeConn := make(chan struct{})
	cmd := bufferForResp(t, []string{"gettunneladdr", "root@127.0.0.1", "remote:7000"})
	var res testutils.SyncBuffer
	theConn := &testutils.MockConn{
		ReadFunc: func(i int, b []byte) (int, error) {
			if i < 2 {
				r := strings.NewReader(cmd.String())
				return r.Read(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		WriteFunc: func(i int, b []byte) (int, error) {
			if i < 2 {
				return res.Write(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		CloseChan: closeConn,
	}

	closeServerListener := make(chan struct{})
	serverListener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i == 0 {
				return theConn, nil
			}
			<-closeServerListener
			return nil, io.EOF
		},
		CloseChan: closeServerListener,
	}

	// create the listener for the tunnel (returned by the mocked
	// ListenFunc).
	closeTunnelListener := make(chan struct{})
	tunnelListener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeTunnelListener
			return nil, io.EOF
		},
		CloseChan: closeTunnelListener,
	}
	defer setAndDeferListenFunc(mockListenFunc(tunnelListener))()

	// create the SSH client returned by the mocked SSHDialFunc
	sshClient := &testutils.MockSSHClient{}
	defer setAndDeferSSHDial(mockSSHDial(sshClient))()

	srv := &Server{
		Addr:       tcpAddr,
		MetaConfig: &sshconfig.MetaConfig{KnownHostsFile: "/dev/null"},
	}

	timeout := 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	if err := srv.serve(ctx, serverListener); errors.Cause(err) != io.EOF {
		t.Errorf("want %v, got %v", io.EOF, err)
	}

	dur := time.Since(start)
	want := timeout
	if dur < want || dur > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, dur)
	}

	r := strings.NewReader(res.String())
	dec := resp.NewDecoder(r)
	v1, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode first response: %v", err)
	}
	v2, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode second response: %v", err)
	}
	if v1 != v2 {
		t.Errorf("want same address to be returned, got %v and %v", v1, v2)
	}

	if n := sshClient.CloseCalls(); n != 1 {
		t.Errorf("want SSHClient.Close to be called once, got %d", n)
	}
	if n := theConn.CloseCalls(); n != 1 {
		t.Errorf("want Conn.Close to be called once, got %d", n)
	}
}

func TestGetTunnelAddrTwiceAfterIdleTimeoutReturnsNewAddr(t *testing.T) {
	t.Skip()
}

func TestGetTunnelAddrKillTunnel(t *testing.T) {
	t.Skip()
}
