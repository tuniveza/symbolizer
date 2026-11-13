package main

// SY_3D_TRACE=file writes when things happen in the 3D view, one line
// each, to measure its frame rate and how far the picture lags the hand:
//
//	<microseconds> in     a mouse move or key came in
//	<microseconds> draw   <ms drawing> <ms since the oldest input in it>
//	<microseconds> sent   <frame> <ms writing the file>
//	<microseconds> ack    kitty says it has the picture
//	<microseconds> slow   <what> <ms>: a message, View or write that took long
//	<microseconds> framemsg, sharpmsg: the room asked to be drawn again
//
// Off (nothing written, nothing measured) without it.

import (
	"fmt"
	"os"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

var trace3D struct {
	mu   sync.Mutex
	f    *os.File
	t0   time.Time
	in   time.Time // the oldest input not drawn yet
	once sync.Once
}

// traceOn opens the trace file the first time; it says whether tracing.
func traceOn() bool {
	t := &trace3D
	t.once.Do(func() {
		if p := os.Getenv("SY_3D_TRACE"); p != "" {
			t.f, _ = os.Create(p)
			t.t0 = time.Now()
		}
	})
	return t.f != nil
}

func traceLine(format string, a ...any) {
	t := &trace3D
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprintf(t.f, "%d "+format+"\n", append([]any{time.Since(t.t0).Microseconds()}, a...)...)
}

// traceIn notes an input.
func traceIn() {
	if !traceOn() {
		return
	}
	t := &trace3D
	t.mu.Lock()
	if t.in.IsZero() {
		t.in = time.Now()
	}
	t.mu.Unlock()
	traceLine("in")
}

// traceDraw notes a drawing that took cost.
func traceDraw(cost time.Duration) {
	if !traceOn() {
		return
	}
	t := &trace3D
	t.mu.Lock()
	lag := time.Duration(0)
	if !t.in.IsZero() {
		lag = time.Since(t.in)
		t.in = time.Time{}
	}
	t.mu.Unlock()
	traceLine("draw %.2f %.2f", ms(cost), ms(lag))
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// traceMsg notes a message that came in: input, or kitty's answer.
func traceMsg(msg any) {
	if !traceOn() {
		return
	}
	switch msg.(type) {
	case room3DFrameMsg:
		traceLine("framemsg")
		return
	case room3DSharpMsg:
		traceLine("sharpmsg")
		return
	case tea.MouseMotionMsg, tea.KeyPressMsg:
		traceIn()
		return
	}
	if g, ok := msg.(uv.KittyGraphicsEvent); ok {
		traceLine("ack %s", g.Payload)
	}
}

// traceSlow notes a message that took long to take in.
func traceSlow(msg any, start time.Time) {
	if d := time.Since(start); d > 5*time.Millisecond {
		if s, ok := msg.(string); ok {
			traceLine("slow %s %.1f", s, ms(d))
		} else {
			traceLine("slow %T %.1f", msg, ms(d))
		}
	}
}
