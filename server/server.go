package server

import (
	"context"
	"expvar"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"bitbucket.org/harfangapps/regis-companion/addr"
	"bitbucket.org/harfangapps/regis-companion/common"
	"bitbucket.org/harfangapps/regis-companion/resp"
	"bitbucket.org/harfangapps/regis-companion/sshconfig"
	"bitbucket.org/harfangapps/regis-companion/tunnel"

	"github.com/pkg/errors"
)

// Build variables, set when building the binary
var (
	// git rev-parse --short HEAD
	GitHash string

	// git describe --tags
	Version string
)

var (
	errEmptyCmd      = errors.New("command is empty")
	defaultLocalAddr = &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
)

// each supported command implements this interface
type command interface {
	Execute(cmdName string, req []string, s *Server) (interface{}, error)
}

// assigned in init
var (
	supportedCommands map[string]command
	commandNames      []string
)

func init() {
	supportedCommands = map[string]command{
		"command":       commandCmd{},
		"gettunneladdr": getTunnelAddrCmd{},
		"killtunnel":    killTunnelCmd{},
		"info":          infoCmd{},
		"ping":          pingCmd{},
	}

	for k := range supportedCommands {
		commandNames = append(commandNames, k)
	}
	sort.Strings(commandNames)
}

type tunnelKey struct {
	User   string
	Server addr.HostPortAddr
	Remote addr.HostPortAddr
}

// various states of the Server
const (
	none = iota
	started
	closed
)

// Server defines the regis-companion Server that listens for incoming connections
// and manages SSH tunnels.
type Server struct {
	// The address the server listens on.
	Addr net.Addr
	// The MetaConfig to use to create SSH ClientConfig.
	MetaConfig *sshconfig.MetaConfig

	// Duration before the tunnels stop if there is no active connection.
	TunnelIdleTimeout time.Duration
	// Write timeout before returning a network error on a write attempt.
	WriteTimeout time.Duration

	// If not nil, this is an expvar map that contains statistics about the server,
	// tunnels and connections.
	Stats *expvar.Map

	// The channel to send errors to. If nil, the errors are logged.
	// If the send would block, the error is dropped. It is the responsibility
	// of the caller to close the channel once the Server is stopped.
	// If set, this ErrChain is used for all Tunnels started by this
	// Server.
	ErrChan chan<- error

	server common.RetryServer

	// mu protects the following private fields
	mu      sync.Mutex
	state   int
	tunnels map[tunnelKey]*tunnel.Tunnel
	ctx     context.Context // stored to pass along to Tunnels
}

// ListenAndServe starts the server on the specified Addr.
//
// This call is blocking, it returns only when an error is
// encountered. As such, it always returns a non-nil error.
func (s *Server) ListenAndServe(ctx context.Context) error {
	l, err := net.Listen(s.Addr.Network(), s.Addr.String())
	if err != nil {
		return errors.Wrap(err, "listen error")
	}
	return s.serve(ctx, l)
}

// getTunnelAddr returns the local address to use to access the SSH tunnel.
// If a Tunnel exists for the requested server+remote addresses, it is
// Touched to see if it is still alive, and if so its existing local address
// is used.
//
// Otherwise, a new Tunnel is started for that server+remote pair and that
// Tunnel's local address is returned.
func (s *Server) getTunnelAddr(user string, server, remote addr.HostPortAddr) (net.Addr, error) {
	key := tunnelKey{User: user, Server: server, Remote: remote}

	s.mu.Lock()
	defer s.mu.Unlock()

	tun := s.tunnels[key]

	// if the tunnel exists and is still alive (confirmed by calling
	// Touch with a return value of true), use it.
	if tun.Touch() {
		return tun.Local, nil
	}

	// otherwise launch a new Tunnel
	config, err := s.MetaConfig.WithAgent(user)
	if err != nil {
		return nil, err
	}

	// get the port for this new tunnel
	l, port, err := addr.ListenFunc(defaultLocalAddr)
	if err != nil {
		return nil, err
	}

	// context specific for this tunnel
	ctx, cancel := context.WithCancel(s.ctx)
	tun = &tunnel.Tunnel{
		SSH:         server,
		Config:      config,
		Local:       &net.TCPAddr{IP: defaultLocalAddr.IP, Port: port},
		Remote:      remote,
		IdleTimeout: s.TunnelIdleTimeout,
		Stats:       s.Stats,
		ErrChan:     s.ErrChan,
		KillFunc:    cancel,
	}

	// launch the Tunnel
	s.tunnels[key] = tun
	go s.serveTunnel(ctx, tun, l)

	return tun.Local, nil
}

func (s *Server) serveTunnel(ctx context.Context, tun *tunnel.Tunnel, l net.Listener) {
	defer tun.KillFunc() // must be called to release context resources

	if err := tun.Serve(ctx, l); err != nil {
		err = errors.Wrap(err, "tunnel serve error")
		common.HandleError(err, s.ErrChan)
		return
	}
}

func (s *Server) killTunnel(user string, server, remote addr.HostPortAddr) error {
	key := tunnelKey{User: user, Server: server, Remote: remote}

	s.mu.Lock()
	defer s.mu.Unlock()

	tun := s.tunnels[key]
	if tun == nil {
		return nil
	}
	fmt.Println(">>>>>>> killing tunnel")
	tun.KillAndWait()
	fmt.Println(">>>>>>> tunnel killed")
	return nil
}

func (s *Server) serve(ctx context.Context, l net.Listener) error {
	s.mu.Lock()
	switch s.state {
	case none:
		// all good, keep going
	case started:
		s.mu.Unlock()
		return errors.New("server already started")
	case closed:
		s.mu.Unlock()
		return errors.New("server closed")
	}

	s.tunnels = make(map[tunnelKey]*tunnel.Tunnel)
	s.ctx = ctx
	s.server.Dispatch = s.serveConn
	s.server.ErrChan = s.ErrChan
	s.server.Listener = l
	s.state = started
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		// properly terminate all tunnels
		for _, tun := range s.tunnels {
			fmt.Println(">>>>>>> killing tunnel")
			tun.KillAndWait()
			fmt.Println(">>>>>>> tunnel killed")
		}
		s.tunnels = nil
		s.state = closed
		s.mu.Unlock()
	}()

	return s.server.Serve(ctx)
}

func (s *Server) serveConn(ctx context.Context, d common.Doner, conn net.Conn) {
	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(ctx)
	done := ctx.Done()

	defer func() {
		conn.Close() // close the serviced connection
		cancel()     // required to release resources
		wg.Wait()    // wait for readWriteLoop goroutine to exit
		d.Done()     // signal the server that this connection is done
	}()

	wg.Add(1)
	go s.readWriteLoop(cancel, wg, conn)

	// block waiting for the stop signal
	<-done
}

func (s *Server) readWriteLoop(cancel func(), d common.Doner, conn net.Conn) {
	defer func() {
		cancel()
		d.Done()
	}()

	dec := resp.NewDecoder(conn)
	enc := resp.NewEncoder(conn)
	for {
		// read the request
		req, err := dec.DecodeRequest()
		if err != nil {
			err = errors.Wrap(err, "decode request error")
			common.HandleError(err, s.ErrChan)
			return
		}

		// handle the request
		res, err := s.execute(req)
		if err != nil {
			err = errors.Wrap(err, "execute request error")
			common.HandleError(err, s.ErrChan)
			return
		}

		// write the response
		if s.WriteTimeout > 0 {
			if err := conn.SetWriteDeadline(time.Now().Add(s.WriteTimeout)); err != nil {
				err = errors.Wrap(err, "set write deadline")
				common.HandleError(err, s.ErrChan)
				return
			}
		}
		if err := enc.Encode(res); err != nil {
			err = errors.Wrap(err, "encode response error")
			common.HandleError(err, s.ErrChan)
			return
		}
	}
}

func (s *Server) execute(req []string) (interface{}, error) {
	if s.Stats != nil {
		s.Stats.Add("commands_executed", 1)
		s.Stats.Add("commands_inprogress", 1)
	}

	defer func() {
		if s.Stats != nil {
			s.Stats.Add("commands_inprogress", -1)
		}
	}()

	if len(req) == 0 {
		return nil, errEmptyCmd
	}

	cmdName := strings.ToLower(req[0])
	cmd, ok := supportedCommands[cmdName]
	if !ok {
		return resp.Error(fmt.Sprintf("ERR unknown command %v", cmdName)), nil
	}
	return cmd.Execute(cmdName, req, s)
}
