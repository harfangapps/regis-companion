package server

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/pkg/errors"

	"bitbucket.org/harfangapps/regis-companion/internal/testutils"

	"golang.org/x/crypto/ssh"
)

// arbitrary valid address
var tcpAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8000}
var zeroSSHConfig = &ssh.ClientConfig{}

func TestCloseUnstarted(t *testing.T) {
	tun := &Tunnel{}
	if e1 := tun.Close(); e1 != nil {
		t.Errorf("want nil, got %v", e1)
	}
	if e2 := tun.Close(); e2 != nil {
		t.Errorf("want nil, got %v", e2)
	}
}

func TestAcceptRetryTemporary(t *testing.T) {
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: zeroSSHConfig}

	var closeErr = errors.New("err")
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i < 10 {
				return nil, testutils.TempErr
			}
			return nil, io.EOF
		},
		CloseErr: closeErr,
	}

	start := time.Now()
	if err := tun.serve(listener); errors.Cause(err) != io.EOF {
		t.Errorf("want io.EOF, got %v", err)
	}

	// retried 10 times for temporary errors, so the delays should be:
	want := (5 + 10 + 20 + 40 + 80 + 160 + 320 + 640 + 1000 + 1000) * time.Millisecond
	got := time.Since(start)
	if got < want || got > (want+(100*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, got)
	}

	// closing returns the error returned by the Listener, and this
	// MockListener returns nil.
	if err := tun.Close(); err != closeErr {
		t.Errorf("want %v, got %v", closeErr, err)
	}
}

func TestAcceptRetryTemporaryReset(t *testing.T) {

}
