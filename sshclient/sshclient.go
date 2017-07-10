package sshclient

import (
	"context"
	"expvar"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"

	"bitbucket.org/harfangapps/regis-companion/addr"
	"bitbucket.org/harfangapps/regis-companion/common"
	"bitbucket.org/harfangapps/regis-companion/tunnel"

	"golang.org/x/crypto/ssh"
)

const (
	none = iota
	started
	stopped
)

var localhostNoPort = &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}

type Client struct {
	User    string
	Addr    net.Addr
	ErrChan chan<- error
	Config  *ssh.ClientConfig
	Stats   *expvar.Map

	mu      sync.Mutex
	client  *ssh.Client
	state   int
	wg      sync.WaitGroup
	tunnels map[net.Addr]*tunnel.Tunnel
}

func (c *Client) Tunnel(ctx context.Context, remote net.Addr, idleTimeout time.Duration) (net.Addr, error) {
	var client *ssh.Client
	var tun *tunnel.Tunnel

	c.mu.Lock()
	switch c.state {
	case none:
		// start the client
	case stopped:
		c.mu.Unlock()
		return nil, errors.New("ssh client stopped")
	case started:
		// immediately increment the WaitGroup to prevent client termination
		c.wg.Add(1)
		defer c.wg.Done()
		client = c.client
		tun = c.tunnels[remote]
	}
	c.mu.Unlock()

	if tun != nil && tun.Touch() {
		return tun.Local, nil
	}

	// create the listener and get the port
	l, port, err := addr.Listen(localhostNoPort)
	if err != nil {
		return nil, errors.Wrap(err, "listen failed")
	}
	localAddr := &net.TCPAddr{IP: localhostNoPort.IP, Port: port}
	tun = &tunnel.Tunnel{
		Dialer:      client,
		Local:       localAddr,
		Remote:      remote,
		IdleTimeout: idleTimeout,
		Stats:       c.Stats,
	}

	// TODO: must be locked
	c.tunnels[remote] = tun
	c.wg.Add(1)
	go c.runTunnel(ctx, l, tun)

	return tun.Local, nil
}

func (c *Client) runTunnel(ctx context.Context, l net.Listener, t *tunnel.Tunnel) {
	defer c.wg.Done()

	if err := t.Serve(ctx, l); err != nil {
		common.HandleError(err, c.ErrChan)
		return
	}
}
