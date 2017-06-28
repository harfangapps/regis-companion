package server

import (
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
	// If the send would block, the error is dropped.
	ErrChan chan<- error

	// mu protects the following private fields
	mu       sync.Mutex
	listener net.Listener
	done     chan struct{}
	closeErr error

	// wg waits for copyBytes goroutines to exit
	wg sync.WaitGroup
}

// ListenAndServe sets up the Tunnel by connecting via
// SSH to Server and Remote, and starts listening for
// connections on Local and transferring data between
// Local and Remote.
//
// This call is blocking, it returns only when an error
// is encountered. As such, it always returns a non-nil error.
func (t *Tunnel) ListenAndServe() error {
	l, err := net.Listen(t.Local.Network(), t.Local.String())
	if err != nil {
		return errors.Wrap(err, "listen error")
	}
	return t.serve(l)
}

// Close immediately closes the Tunnel's listener and
// any active connections. It returns the error returned
// from closing the Listener.
func (t *Tunnel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	select {
	case <-t.done:
		// already closed
		return t.closeErr
	default:
		if t.done == nil {
			// was never started, nothing to do
			return nil
		}

		// first close the channel
		close(t.done)
		// then close the listener, which will trigger the rest
		t.closeErr = t.listener.Close()
		// wait for goroutines to exit cleanly
		t.wg.Wait()
		// can now close the ErrChan safely
		if t.ErrChan != nil {
			close(t.ErrChan)
		}

		return t.closeErr
	}
}

// this makes it possible to test with a Listener that fails to accept.
func (t *Tunnel) serve(l net.Listener) error {
	defer l.Close()

	t.mu.Lock()
	t.done = make(chan struct{})
	t.listener = l
	t.mu.Unlock()

	var delay time.Duration
	for {
		local, err := l.Accept()
		if err != nil {
			err = errors.Wrap(err, "tunnel Accept error")

			// if the Tunnel was closed, return immediately
			select {
			case <-t.done:
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
		go t.forward(local)
	}
}

func (t *Tunnel) forward(local net.Conn) {
	defer local.Close()

	// acquire the lock, as wg.Add is racy with wg.Wait in Tunnel.Close.
	t.mu.Lock()
	select {
	case <-t.done:
		// was closed while waiting on the lock, exit
		t.mu.Unlock()
		return
	default:
		// is definitely not closed and not `wg.Wait`ing
		t.wg.Add(1)
		defer t.wg.Done()
	}
	t.mu.Unlock()

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

	// acquire the lock, as wg.Add is racy with wg.Wait in Tunnel.Close.
	t.mu.Lock()
	select {
	case <-t.done:
		// was closed while waiting on the lock, will exit
	default:
		// is definitely not closed and not `wg.Wait`ing
		t.wg.Add(2)
		go t.copyBytes(local, remote)
		go t.copyBytes(remote, local)
	}
	t.mu.Unlock()

	// block waiting for the Close signal
	<-t.done
}

func (t *Tunnel) copyBytes(dst io.Writer, src io.Reader) {
	defer t.wg.Done()

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
