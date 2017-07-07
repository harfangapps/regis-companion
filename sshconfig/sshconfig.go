package sshconfig

import (
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// MetaConfig holds configuration options to use to create SSH client
// configurations.
type MetaConfig struct {
	KnownHostsFile string
	SSHDialTimeout time.Duration

	mu    sync.Mutex
	agent net.Conn
}

// ErrNoKnownHostsFile is returned when the KnownHostsFile field is empty.
var ErrNoKnownHostsFile = errors.New("sshconfig: missing known hosts file")

// WithAgent returns an SSH ClientConfig that authenticates via the
// SSH agent.
func (c *MetaConfig) WithAgent(user string) (*ssh.ClientConfig, error) {
	if c.KnownHostsFile == "" {
		return nil, ErrNoKnownHostsFile
	}
	hostKeyCallback, err := knownhosts.New(c.KnownHostsFile)
	if err != nil {
		return nil, err
	}

	auth, err := c.sshAgentAuthMethod()
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User:            user,
		Timeout:         c.SSHDialTimeout,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: hostKeyCallback,
	}, nil
}

func (c *MetaConfig) sshAgentAuthMethod() (ssh.AuthMethod, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.agent != nil {
		return ssh.PublicKeysCallback(agent.NewClient(c.agent).Signers), nil
	}

	conn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return nil, err
	}
	c.agent = conn
	return ssh.PublicKeysCallback(agent.NewClient(c.agent).Signers), nil
}
