package tunnel

import (
	"context"
	"expvar"
	"io"
	"net"
	"sync"
	"time"

	"bitbucket.org/harfangapps/regis-companion/common"

	"github.com/pkg/errors"
)

// Dialer defines the Dial function implemented by an SSH Client.
type Dialer interface {
	Dial(n, addr string) (net.Conn, error)
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
	// The SSH dialer.
	Dialer Dialer
	// The local address on which the tunnel is exposed.
	Local net.Addr
	// The remote address to connect to via Dialer.
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

	server common.RetryServer

	mu    sync.Mutex // protects the state
	state int
}

// Touch generates activity on the tunnel to prevent it from closing
// due to inactivity. It returns true if the tunnel was active when
// this was called, false otherwise.
func (t *Tunnel) Touch() bool {
	t.mu.Lock()
	if t.state != started {
		t.mu.Unlock()
		return false
	}
	t.mu.Unlock()

	t.server.IdleTracker.Touch()
	return true
}

// Serve starts the tunnel's server on the local address. It is a blocking
// call that always returns an error.
func (t *Tunnel) Serve(ctx context.Context, l net.Listener) error {
	t.mu.Lock()
	switch t.state {
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
	t.mu.Unlock()

	if t.Stats != nil {
		t.Stats.Add("active_tunnels", 1)
		t.Stats.Add("total_tunnels", 1)
	}

	defer func() {
		t.mu.Lock()
		t.state = closed
		t.mu.Unlock()

		if t.Stats != nil {
			t.Stats.Add("active_tunnels", -1)
		}
	}()

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

	// TODO: instead use a pool of SSH clients for a given address, and try
	// twice: once with the SSH client from the pool, and if it fails once
	// again with a new SSH client and store it back in the Pool.

	// connect to the remote address via the Dialer
	remote, err := t.Dialer.Dial(t.Remote.Network(), t.Remote.String())
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
