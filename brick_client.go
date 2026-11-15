package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// The brick client: brick (the plain-English compiler in brick/) runs as
// `python3 brick/brick.py serve` in the background, and Symbolizer asks it
// things over its stdin and stdout, one JSON line each way (brick/serve.py).
// Answers come back as brickMsg, matched to the question by id.

type brickMsg struct {
	id   int
	what string         // what the question was for (lines, watch, decisions …)
	data map[string]any // the answer
	err  string
}

type brickClient struct {
	mu      sync.Mutex
	in      io.WriteCloser
	answers chan brickMsg
	next    int
	asked   map[int]string
	dead    string
}

// brickHome is where brick lives: the brick/ folder next to Symbolizer's own
// program (it is a symlink into the repository), or SY_BRICK.
func brickHome() string {
	if d := os.Getenv("SY_BRICK"); d != "" {
		return d
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			if d := filepath.Join(filepath.Dir(real), "brick"); fileExists(filepath.Join(d, "brick.py")) {
				return d
			}
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "an", "ca", "sy", "brick")
}


// startBrick starts brick serve. A failure is kept and said on the next question.
func startBrick() *brickClient {
	c := &brickClient{answers: make(chan brickMsg, 64), asked: map[int]string{}}
	home := brickHome()
	cmd := exec.Command("python3", filepath.Join(home, "brick.py"), "serve")
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "PYTHONPATH="+home, "BRICK_TRAINER=off")
	in, err1 := cmd.StdinPipe()
	out, err2 := cmd.StdoutPipe()
	if err1 != nil || err2 != nil {
		c.dead = "brick could not start"
		return c
	}
	if err := cmd.Start(); err != nil {
		c.dead = "brick could not start: " + err.Error()
		return c
	}
	c.in = in
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var a map[string]any
			if json.Unmarshal(sc.Bytes(), &a) != nil {
				continue
			}
			id := 0
			if f, ok := a["id"].(float64); ok {
				id = int(f)
			}
			c.mu.Lock()
			what := c.asked[id]
			delete(c.asked, id)
			c.mu.Unlock()
			msg := brickMsg{id: id, what: what, data: a}
			if ok, _ := a["ok"].(bool); !ok {
				msg.err, _ = a["error"].(string)
			}
			c.answers <- msg
		}
		c.mu.Lock()
		c.dead = "brick stopped"
		c.mu.Unlock()
		c.answers <- brickMsg{what: "dead", err: "brick stopped"}
	}()
	go func() { _ = cmd.Wait() }()
	return c
}

// ask sends a question; what says what the answer is for. -> its id, or 0.
func (c *brickClient) ask(what string, fields map[string]any) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead != "" || c.in == nil {
		return 0
	}
	c.next++
	q := map[string]any{"id": c.next}
	for k, v := range fields {
		q[k] = v
	}
	if _, ok := q["do"]; !ok {
		q["do"] = what
	}
	b, _ := json.Marshal(q)
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		c.dead = "brick stopped"
		return 0
	}
	c.asked[c.next] = what
	return c.next
}

// listen waits for the next answer (re-armed after each one).
func (c *brickClient) listen() tea.Cmd {
	return func() tea.Msg { return <-c.answers }
}

// helpers for reading answers
func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func num(m map[string]any, k string) float64 { f, _ := m[k].(float64); return f }
func list(m map[string]any, k string) []any  { l, _ := m[k].([]any); return l }
func obj(v any) map[string]any               { o, _ := v.(map[string]any); return o }
func yes(m map[string]any, k string) bool    { b, _ := m[k].(bool); return b }
