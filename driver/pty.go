package main

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/charmbracelet/x/xpty"
)

// endMark is written at the slave after the program exits (Unix), so the reader knows it has read everything.
const endMark = "\x1b]9999;bt1898-end\a"

// term is a program at a pty whose output is fed to a terminal emulator; the emulator's replies to the
// program's queries (device attributes, background colour) are written back, as a real terminal would.
type term struct {
	pty  xpty.Pty
	cmd  *exec.Cmd
	ctx  context.Context
	mu   sync.Mutex
	em   *vt.Emulator
	read chan struct{}
	wmu  sync.Mutex
	log  *os.File
}

func start(ctx context.Context, cmd *exec.Cmd, cols, rows int, logPath string) (*term, error) {
	p, err := xpty.NewPty(cols, rows)
	if err != nil {
		return nil, err
	}
	t := &term{pty: p, cmd: cmd, ctx: ctx, em: vt.NewEmulator(200, rows), read: make(chan struct{})}
	t.em.SetBackgroundColor(color.Black)
	if logPath != "" {
		t.log, _ = os.Create(logPath)
	}
	out, err := output(p)
	if err != nil {
		_ = p.Close()
		return nil, err
	}
	if err := p.Start(cmd); err != nil {
		_ = out.Close()
		_ = p.Close()
		return nil, err
	}
	go func() { // the emulator's replies go back to the program
		buf := make([]byte, 512)
		for {
			n, err := t.em.Read(buf)
			if n > 0 {
				_ = t.write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer close(t.read)
		defer out.Close()
		buf, tail := make([]byte, 4096), []byte{}
		for {
			n, err := out.Read(buf)
			t.trace("read", buf[:n])
			t.mu.Lock()
			_, _ = t.em.Write(buf[:n])
			t.mu.Unlock()
			tail = append(tail, buf[:n]...)
			if err != nil || bytes.Contains(tail, []byte(endMark)) {
				return
			}
			tail = tail[max(0, len(tail)-len(endMark)):]
		}
	}()
	return t, nil
}

func (t *term) trace(dir string, b []byte) {
	if t.log != nil && len(b) > 0 {
		fmt.Fprintf(t.log, "%s %s %q\n", time.Now().UTC().Format("15:04:05.000000"), dir, b)
	}
}

func (t *term) write(b []byte) error {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	t.trace("write", b)
	_, err := t.pty.Write(b)
	return err
}

// screen is what the terminal shows, trailing spaces and blank rows trimmed.
func (t *term) screen() string {
	t.mu.Lock()
	s := t.em.String()
	t.mu.Unlock()
	rows := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return strings.TrimRight(strings.Join(rows, "\n"), "\n")
}

func (t *term) waitFor(timeout time.Duration, ok func(string) bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok(t.screen()) {
			return true
		}
		select {
		case <-t.read:
			return ok(t.screen())
		case <-time.After(10 * time.Millisecond):
		}
	}
	return false
}

// wait waits for the exit and for all the program wrote to be read, and returns the exit code.
func (t *term) wait() int {
	err := xpty.WaitProcess(t.ctx, t.cmd)
	if u, ok := t.pty.(*xpty.UnixPty); ok {
		_, _ = u.Slave().WriteString(endMark)
	} else {
		_ = t.pty.Close() // a ConPTY flushes its last frame when closed; the reader has its own pipe handle
	}
	select {
	case <-t.read:
	case <-time.After(10 * time.Second):
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

func (t *term) close() {
	if t.cmd.ProcessState == nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	_ = t.pty.Close()
	if c, ok := t.em.InputPipe().(io.Closer); ok {
		_ = c.Close()
	}
	if t.log != nil {
		_ = t.log.Close()
	}
}
