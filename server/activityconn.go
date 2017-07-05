package server

import (
	"net"
	"sync/atomic"
)

var _ net.Conn = activityConn{}

type activityConn struct {
	net.Conn
	i *int64
}

func (c activityConn) Read(b []byte) (int, error) {
	atomic.AddInt64(c.i, 1)
	return c.Conn.Read(b)
}

func (c activityConn) Write(b []byte) (int, error) {
	atomic.AddInt64(c.i, 1)
	return c.Conn.Write(b)
}
