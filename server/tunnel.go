package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

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

	// wg waits for `forward` goroutines to exit
	wg sync.WaitGroup
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
	defer l.Close()

	done := ctx.Done()
	go func() {
		<-done
		l.Close()
	}()

	var delay time.Duration
	for {
		local, err := l.Accept()
		if err != nil {
			err = errors.Wrap(err, "tunnel Accept error")

			// if the Tunnel was stopped, return immediately
			select {
			case <-done:
				t.wg.Wait()
				return err
			default:
				// go on
			}

			// if the error is temporary, retry after a delay
			if t.handleTemporary(&delay, err) {
				continue
			}
			return err
		}
		delay = 0
		t.wg.Add(1)
		go t.forward(done, local)
	}
}

func (t *Tunnel) forward(done <-chan struct{}, local net.Conn) {
	wg := &sync.WaitGroup{}
	defer func() {
		local.Close() // close the local socket
		wg.Wait()     // wait for sub-goroutines to exit
		t.wg.Done()   // signal the Tunnel that this forward goroutine is done
	}()

	// SSH connect to the server
	server, err := sshDialFn(t.Server.Network(), t.Server.String(), t.Config)
	if err != nil {
		t.handleError(errors.Wrap(err, "ssh server dial error"))
		return
	}
	defer server.Close()

	// connect to the remote address via the SSH server
	remote, err := server.Dial(t.Remote.Network(), t.Remote.String())
	if err != nil {
		t.handleError(errors.Wrap(err, "ssh remote dial error"))
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
		t.handleError(err)
		return
	}
}

// handle temporary errors by delaying a retry. Returns false if the error is
// not temporary.
func (t *Tunnel) handleTemporary(delay *time.Duration, err error) bool {
	root := errors.Cause(err)

	if te, ok := root.(interface {
		Temporary() bool
	}); ok && te.Temporary() {
		if *delay == 0 {
			*delay = 5 * time.Millisecond
		} else {
			*delay *= 2
		}

		if max := 1 * time.Second; *delay > max {
			*delay = max
		}

		t.handleError(errors.Wrap(err, fmt.Sprintf("temporary error, retrying in %v", *delay)))
		time.Sleep(*delay)
		return true
	}

	return false
}

// handle the error by sending it to the ErrChan or logging it.
func (t *Tunnel) handleError(err error) {
	select {
	case t.ErrChan <- err:
	default:
		// log if ErrChan is nil, drop otherwise
		if t.ErrChan == nil {
			log.Print(err)
		}
	}
}
