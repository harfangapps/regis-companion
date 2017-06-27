package server

import (
	"log"
	"net"

	"golang.org/x/crypto/ssh"
)

// Tunnel defines an SSH tunnel. The client connects to the Local
// address, the server connects via SSH to the Server address,
// and from there to the Remote address. Config specifies the
// configuration for the SSH connection.
//
// The bytes are transferred using the SSH tunnel from the Local
// address to the Remote address.
type Tunnel struct {
	Local  net.Addr
	Server net.Addr
	Remote net.Addr

	Config *ssh.ClientConfig
	// TODO: context.Context
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
		// TODO: wrap error with context
		return err
	}
	defer l.Close()

	for {
		local, err := l.Accept()
		if err != nil {
			// TODO: check if temporary, wrap with context
			return err
		}
		go t.forward(local)
	}
}

func (t *Tunnel) forward(local net.Conn) {
	// SSH connect to the server
	server, err := ssh.Dial(t.Server.Network(), t.Server.String(), t.Config)
	if err != nil {
		log.Printf("ssh server dial error: %s", err)
		return
	}
	defer server.Close()

	// connect to the remote address via the SSH server
	remote, err := server.Dial(t.Remote.Network(), t.Remote.String())
	if err != nil {
		log.Printf("ssh remote dial error: %s", err)
		return
	}
	defer remote.Close()

	go copyBytes(local, remote)
	go copyBytes(remote, local)
}

func (t *Tunnel) copyBytes(from, to net.Conn) {

}
