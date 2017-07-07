package server

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"bitbucket.org/harfangapps/regis-companion/resp"
	"bitbucket.org/harfangapps/regis-companion/sshconfig"

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
		"info":          infoCmd{},
		"ping":          pingCmd{},
	}

	for k := range supportedCommands {
		commandNames = append(commandNames, k)
	}
	sort.Strings(commandNames)
}

type tunnelAddrs struct {
	Server net.Addr
	Remote net.Addr
}

// Server defines the regis-companion Server that listens for incoming connections
// and manages SSH tunnels.
type Server struct {
	// The address the server listens on.
	Addr net.Addr
	// The MetaConfig to use to create SSH ClientConfig.
	MetaConfig sshconfig.MetaConfig

	// Duration before the tunnels stop if there is no active connection.
	TunnelIdleTimeout time.Duration
	// Write timeout before returning a network error on a write attempt.
	WriteTimeout time.Duration

	// The channel to send errors to. If nil, the errors are logged.
	// If the send would block, the error is dropped. It is the responsibility
	// of the caller to close the channel once the Server is stopped.
	// If set, this ErrChain is used for all Tunnels started by this
	// Server.
	ErrChan chan<- error

	// mu protects the map of addresses-to-tunnel and the ctx
	mu      sync.Mutex
	tunnels map[tunnelAddrs]*Tunnel
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
func (s *Server) getTunnelAddr(server, remote net.Addr, user string) (net.Addr, error) {
	key := tunnelAddrs{Server: server, Remote: remote}

	s.mu.Lock()
	defer s.mu.Unlock()
	tun := s.tunnels[key]

	// if the tunnel exists and is still alive (confirmed by calling
	// Touch with a return value of true), use it.
	if tun != nil && tun.Touch() {
		return tun.Local, nil
	}

	// otherwise launch a new Tunnel
	config, err := s.MetaConfig.WithAgent(user)
	if err != nil {
		return nil, err
	}

	tun = &Tunnel{
		// Local will be overwritten once the port is known
		Local:       defaultLocalAddr,
		Server:      server,
		Remote:      remote,
		Config:      config,
		ErrChan:     s.ErrChan,
		IdleTimeout: s.TunnelIdleTimeout,
	}
	l, port, err := tun.Listen()
	if err != nil {
		return nil, err
	}
	tun.Local = &net.TCPAddr{IP: defaultLocalAddr.IP, Port: port}
	s.tunnels[key] = tun

	// launch the Tunnel
	go tun.Serve(s.ctx, l)

	return tun.Local, nil
}

func (s *Server) serve(ctx context.Context, l net.Listener) error {
	s.mu.Lock()
	s.tunnels = make(map[tunnelAddrs]*Tunnel)
	s.ctx = ctx
	s.mu.Unlock()

	server := retryServer{
		Listener: l,
		Dispatch: s.serveConn,
		ErrChan:  s.ErrChan,
	}
	return server.serve(ctx)
}

func (s *Server) serveConn(ctx context.Context, serverWg *sync.WaitGroup, conn net.Conn) {
	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(ctx)
	done := ctx.Done()

	defer func() {
		conn.Close()    // close the serviced connection
		cancel()        // required to release resources
		wg.Wait()       // wait for sub-goroutines to exit
		serverWg.Done() // signal the server that this connection is done
	}()

	wg.Add(1)
	go s.readWriteLoop(cancel, wg, conn)

	// block waiting for the stop signal
	<-done
}

func (s *Server) readWriteLoop(cancel func(), wg *sync.WaitGroup, conn net.Conn) {
	defer func() {
		cancel()
		wg.Done()
	}()

	dec := resp.NewDecoder(conn)
	enc := resp.NewEncoder(conn)
	for {
		// read the request
		req, err := dec.DecodeRequest()
		if err != nil {
			err = errors.Wrap(err, "decode request error")
			handleError(err, s.ErrChan)
			return
		}

		// handle the request
		res, err := s.execute(req)
		if err != nil {
			err = errors.Wrap(err, "execute request error")
			handleError(err, s.ErrChan)
			return
		}

		// write the response
		if s.WriteTimeout > 0 {
			if err := conn.SetWriteDeadline(time.Now().Add(s.WriteTimeout)); err != nil {
				err = errors.Wrap(err, "set write deadline")
				handleError(err, s.ErrChan)
				return
			}
		}
		if err := enc.Encode(res); err != nil {
			err = errors.Wrap(err, "encode response error")
			handleError(err, s.ErrChan)
			return
		}
	}
}

func (s *Server) execute(req []string) (interface{}, error) {
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
