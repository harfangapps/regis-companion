package common

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/harfangapps/regis-companion/internal/testutils"
)

func TestIdleTrackerNoActivity(t *testing.T) {
	cases := []time.Duration{
		0,
		10 * time.Millisecond,
		200 * time.Millisecond, // should be cancelled before the idle is reached
	}

	for _, c := range cases {
		timeout := 100 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		tracker := &IdleTracker{IdleTimeout: c}
		wg := &sync.WaitGroup{}

		wg.Add(1)
		start := time.Now()
		tracker.Start(ctx, cancel, wg)

		wg.Wait()
		duration := time.Since(start)

		want := c
		if want > timeout {
			want = timeout
		}

		if duration < want || duration > (want+(10*time.Millisecond)) {
			t.Errorf("want duration of %v, got %v", want, duration)
		}
	}
}

func TestIdleTrackerTouch(t *testing.T) {
	idle := 50 * time.Millisecond
	tracker := &IdleTracker{IdleTimeout: idle}
	timeout := 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	wg := &sync.WaitGroup{}

	wg.Add(1)
	start := time.Now()
	tracker.Start(ctx, cancel, wg)

	go func() {
		<-time.After(40 * time.Millisecond)
		tracker.Touch()
	}()

	wg.Wait()
	duration := time.Since(start)

	want := 2 * idle // first check detects activity
	if duration < want || duration > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, duration)
	}
}

func TestIdleTrackerConn(t *testing.T) {
	idle := 50 * time.Millisecond
	tracker := &IdleTracker{IdleTimeout: idle}
	timeout := 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	wg := &sync.WaitGroup{}

	wg.Add(1)
	start := time.Now()
	tracker.Start(ctx, cancel, wg)
	conn := tracker.TrackConn(&testutils.MockConn{})

	go func() {
		<-time.After(40 * time.Millisecond)
		conn.Read(nil)
		<-time.After(40 * time.Millisecond)
		conn.Write(nil)
	}()

	wg.Wait()
	duration := time.Since(start)

	want := 3 * idle // first 2 checks detects activity
	if duration < want || duration > (want+(20*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, duration)
	}
}
