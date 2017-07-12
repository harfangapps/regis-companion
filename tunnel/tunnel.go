package tunnel

import (
	"context"
	"expvar"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"bitbucket.org/harfangapps/regis-companion/common"

	"github.com/pkg/errors"
)

// SSHDialFunc is a variable that references the SSH dial function to
// use so that it can be mocked for tests.
var SSHDialFunc = DefaultSSHDial

// DefaultSSHDial is the default implementation to use for SSH Dial.
func DefaultSSHDial(n, addr string, config *ssh.ClientConfig) (DialCloser, error) {
	return ssh.Dial(n, addr, config)
}

// DialCloser defines the required functions implemented by an SSH Client.
type DialCloser interface {
	Dial(n, addr string) (net.Conn, error)
	Close() error
}

// various states of the Tunnel
const (
	none = iota
	started
	closed
)

// Tunnel represents an SSH tunnel that connects to Remote via the
// Dialer (an SSH connection) and forwards the data between Remote
// and Local addresses.
type Tunnel struct {
	// The address of the SSH server.
	SSH net.Addr
	// Config is the configuration to use to dial to the SSH server.
	Config *ssh.ClientConfig

	// The local address on which the tunnel is exposed.
	Local net.Addr
	// The remote address to connect to via the SSH connection.
	Remote net.Addr

	// The duration after which the tunnel is closed if there is no
	// activity.
	IdleTimeout time.Duration

	// The expvar tunnel statistics.
	Stats *expvar.Map

	// The channel to send errors to. If nil, the errors are logged.
	// If the send would block, the error is dropped. It is the responsibility
	// of the caller to close the channel once the Tunnel is stopped.
	ErrChan chan<- error

	// The function to cancel the context of the Tunnel.
	KillFunc func()

	server common.RetryServer
	client DialCloser

	// protects the following private fields
	mu     sync.Mutex
	killed chan struct{} // closed when tunnel is closed
	state  int
}

// KillAndWait stops the tunnel by cancelling its context using KillFunc
// and waits for a clean termination to complete before returning.
func (t *Tunnel) KillAndWait() {
	if t == nil || t.KillFunc == nil {
		return
	}
	t.KillFunc()
	<-t.killed
}

// Touch generates activity on the tunnel to prevent it from closing
// due to inactivity. It returns true if the tunnel was active when
// this was called, false otherwise.
func (t *Tunnel) Touch() bool {
	if t == nil {
		return false
	}

	t.mu.Lock()
	// Touch could be called before the Tunnel.serve goroutine was launched,
	// in which case it would not be started yet. So just check that it is
	// not closed.
	if t.state == closed {
		t.mu.Unlock()
		return false
	}
	t.server.IdleTracker.Touch()
	t.mu.Unlock()

	return true
}

// Serve starts the tunnel's server on the local address. It is a blocking
// call that always returns an error.
func (t *Tunnel) Serve(ctx context.Context, l net.Listener) error {
	t.mu.Lock()
	switch t.state {
	case none:
		// all good, keep going
	case started:
		t.mu.Unlock()
		return errors.New("tunnel already started")
	case closed:
		t.mu.Unlock()
		return errors.New("tunnel closed")
	}

	t.server.ErrChan = t.ErrChan
	t.server.Listener = l
	t.server.IdleTracker.IdleTimeout = t.IdleTimeout
	t.server.Dispatch = t.forward
	t.state = started
	t.killed = make(chan struct{})
	t.mu.Unlock()

	if t.Stats != nil {
		t.Stats.Add("active_tunnels", 1)
		t.Stats.Add("total_tunnels", 1)
	}

	defer func() {
		if t.Stats != nil {
			t.Stats.Add("active_tunnels", -1)
		}

		t.mu.Lock()
		t.state = closed
		close(t.killed)
		t.mu.Unlock()
	}()

	// connect to the SSH server and store the dialCloser
	client, err := SSHDialFunc(t.SSH.Network(), t.SSH.String(), t.Config)
	if err != nil {
		return err
	}
	t.client = client
	defer client.Close()

	return t.server.Serve(ctx)
}

func (t *Tunnel) forward(ctx context.Context, d common.Doner, local net.Conn) {
	copyBytesWg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(ctx)
	done := ctx.Done()

	if t.Stats != nil {
		t.Stats.Add("active_tunnel_conns", 1)
		t.Stats.Add("total_tunnel_conns", 1)
	}

	defer func() {
		local.Close()      // the connection must be closed on exit
		cancel()           // required to release context resources
		copyBytesWg.Wait() // wait for copyBytes goroutines

		if t.Stats != nil {
			t.Stats.Add("active_tunnel_conns", -1)
		}

		d.Done() // notify parent that this connection is done
	}()

	// connect to the remote address via the Dialer
	remote, err := t.client.Dial(t.Remote.Network(), t.Remote.String())
	if err != nil {
		common.HandleError(errors.Wrap(err, "remote dial error"), t.ErrChan)
		return
	}
	defer remote.Close()

	select {
	case <-done:
		// was stopped while connecting, will exit
	default:
		// keep track of sub-goroutines
		copyBytesWg.Add(2)
		go t.copyBytes(cancel, copyBytesWg, local, remote)
		go t.copyBytes(cancel, copyBytesWg, remote, local)
	}

	// block waiting for the stop signal
	<-done
}

func (t *Tunnel) copyBytes(cancel func(), d common.Doner, dst io.Writer, src io.Reader) {
	defer func() {
		cancel() // if one end can't forward bytes, must cancel the connection
		d.Done()
	}()

	if _, err := io.Copy(dst, src); err != nil {
		err = errors.Wrap(err, "copy bytes error")
		common.HandleError(err, t.ErrChan)
		return
	}
}
