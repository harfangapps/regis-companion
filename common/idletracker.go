package common

import (
	"context"
	"net"
	"sync/atomic"
	"time"
)

// IdleTracker tracks activity and cancels a context when there is none
// during a whole IdleTimeout duration.
type IdleTracker struct {
	IdleTimeout time.Duration

	currentCounter  uint64
	previousCounter uint64
}

// Start starts the tracker. If the IdleTimeout is less than or equal to
// 0, there is no tracking to do and the call is a no-op, calling Done
// immediately on d. Otherwise it launches the tracking goroutine.
func (t *IdleTracker) Start(ctx context.Context, cancel func(), d Doner) {
	if t.IdleTimeout <= 0 {
		d.Done()
		return
	}

	go t.track(ctx, cancel, d)
}

func (t *IdleTracker) track(ctx context.Context, cancel func(), d Doner) {
	defer d.Done()

	done := ctx.Done()
	for {
		select {
		case <-time.After(t.IdleTimeout):
			current := atomic.LoadUint64(&t.currentCounter)
			previous := atomic.LoadUint64(&t.previousCounter)

			if current == previous {
				// no activity since last check
				cancel()
				return
			}
			// there was activity, check again next time
			atomic.CompareAndSwapUint64(&t.previousCounter, previous, current)

		case <-done:
			return
		}
	}
}

// Touch notifies the tracker of activity.
func (t *IdleTracker) Touch() {
	atomic.AddUint64(&t.currentCounter, 1)
}

var _ net.Conn = activityConn{}

type activityConn struct {
	net.Conn
	i *uint64
}

func (c activityConn) Read(b []byte) (int, error) {
	atomic.AddUint64(c.i, 1)
	return c.Conn.Read(b)
}

func (c activityConn) Write(b []byte) (int, error) {
	atomic.AddUint64(c.i, 1)
	return c.Conn.Write(b)
}

// TrackConn wraps the provided connection and returns a connection
// that notifies the tracker of activity on Read and Write.
func (t *IdleTracker) TrackConn(c net.Conn) net.Conn {
	if t.IdleTimeout <= 0 {
		return c
	}
	return activityConn{c, &t.currentCounter}
}
