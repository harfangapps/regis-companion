package server

import (
	"context"
	"io"
	"net"
	"sync"

	"github.com/pkg/errors"

	"golang.org/x/crypto/ssh"
)

type dialCloser interface {
	Close() error
	Dial(network, address string) (net.Conn, error)
}

// for tests, to be able to mock the SSH dial.
var sshDialFn func(network, address string, config *ssh.ClientConfig) (dialCloser, error) = defaultSSHDial

// the default SSH dial.
func defaultSSHDial(network, address string, config *ssh.ClientConfig) (dialCloser, error) {
	return ssh.Dial(network, address, config)
}

// Tunnel defines an SSH tunnel. The client connects to the Local
// address, the server connects via SSH to the Server address,
// and from there to the Remote address. Config specifies the
// configuration for the SSH connection.
//
// The bytes are transferred using the SSH tunnel from the Local
// address to the Remote address.
type Tunnel struct {
	// The local address the client connects too.
	Local net.Addr
	// The server address to connect to via SSH.
	Server net.Addr
	// The remote address to connect to via the server's SSH connection.
	Remote net.Addr

	// The client configuration to use to connect to Server.
	Config *ssh.ClientConfig

	// The channel to send errors to. If nil, the errors are logged.
	// If the send would block, the error is dropped. It is the responsibility
	// of the caller to close the channel once the Tunnel is stopped.
	ErrChan chan<- error
}

// ListenAndServe sets up the Tunnel by connecting via
// SSH to Server and Remote, and starts listening for
// connections on Local and transferring data between
// Local and Remote.
//
// This call is blocking, it returns only when an error
// is encountered. As such, it always returns a non-nil error.
func (t *Tunnel) ListenAndServe(ctx context.Context) error {
	l, err := net.Listen(t.Local.Network(), t.Local.String())
	if err != nil {
		return errors.Wrap(err, "listen error")
	}
	return t.serve(ctx, l)
}

// this makes it possible to test with a mock Listener.
func (t *Tunnel) serve(ctx context.Context, l net.Listener) error {
	server := retryServer{
		listener: l,
		dispatch: t.forward,
		errChan:  t.ErrChan,
	}
	return server.serve(ctx)
}

func (t *Tunnel) forward(done <-chan struct{}, serverWg *sync.WaitGroup, local net.Conn) {
	wg := &sync.WaitGroup{}
	defer func() {
		local.Close()   // close the local socket
		wg.Wait()       // wait for sub-goroutines to exit
		serverWg.Done() // signal the server that this forward goroutine is done
	}()

	// SSH connect to the server
	server, err := sshDialFn(t.Server.Network(), t.Server.String(), t.Config)
	if err != nil {
		handleError(errors.Wrap(err, "ssh server dial error"), t.ErrChan)
		return
	}
	defer server.Close()

	// connect to the remote address via the SSH server
	remote, err := server.Dial(t.Remote.Network(), t.Remote.String())
	if err != nil {
		handleError(errors.Wrap(err, "ssh remote dial error"), t.ErrChan)
		return
	}
	defer remote.Close()

	select {
	case <-done:
		// was stopped while connecting, will exit
	default:
		// use the local wait group to keep track of sub-goroutines
		wg.Add(2)
		go t.copyBytes(wg, local, remote)
		go t.copyBytes(wg, remote, local)
	}

	// block waiting for the stop signal
	<-done
}

func (t *Tunnel) copyBytes(wg *sync.WaitGroup, dst io.Writer, src io.Reader) {
	// use the local wait group, NOT t.wg
	defer wg.Done()

	if _, err := io.Copy(dst, src); err != nil {
		err = errors.Wrap(err, "copy bytes error")
		handleError(err, t.ErrChan)
		return
	}
}
