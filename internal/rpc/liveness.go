package rpc

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

// activityReader records when bytes last arrived from the server. It counts
// bytes, not whole frames: a large response arriving slowly is progress.
type activityReader struct {
	r    io.Reader
	base time.Time
	last *atomic.Int64 // time.Since(base) of the last read that returned data
}

func (a *activityReader) Read(p []byte) (int, error) {
	n, err := a.r.Read(p)
	if n > 0 {
		a.last.Store(int64(time.Since(a.base)))
	}
	return n, err
}

// Quiet returns how long nothing has arrived from the server. It uses the
// monotonic clock, so wall-clock changes do not move it.
func (c *Client) Quiet() time.Duration {
	return time.Since(c.base) - time.Duration(c.lastRead.Load())
}

// WatchLiveness runs until ctx ends or the client closes. Once nothing has
// arrived for probeAfter it sends one $/ping; any reply, or any other frame,
// counts as life. Once nothing has arrived for deadline it publishes
// ErrLinkSilent, calls onSilent once to end the transport, closes the client
// pipes and returns. Publishing termination does not wait for a stuck read.
//
// Sending never blocks the watch: on a lost network the write can wait on a
// full pipe until the transport is torn down.
func (c *Client) WatchLiveness(ctx context.Context, probeAfter, deadline time.Duration, onSilent func()) {
	tick := probeAfter / 5
	if tick <= 0 {
		tick = time.Millisecond
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-t.C:
		}
		quiet := c.Quiet()
		if quiet >= deadline {
			c.finish(ErrLinkSilent)
			onSilent()
			_ = c.Close()
			return
		}
		if quiet >= probeAfter {
			c.sendPing()
		}
	}
}

// sendPing writes one $/ping unless one is still awaiting a reply. The reply
// is matched to no pending call, so the read loop drops it after the
// activity reader has seen it.
func (c *Client) sendPing() {
	if !c.pinging.CompareAndSwap(false, true) {
		return
	}
	id := atomic.AddInt64(&c.seq, 1)
	c.pingID.Store(id)
	go func() {
		if err := c.writer.WriteFrame(Request{JSONRPC: JSONRPCVersion, ID: id, Method: MethodPing}); err != nil {
			c.pinging.Store(false)
		}
	}()
}
