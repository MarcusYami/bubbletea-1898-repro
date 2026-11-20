package main

import (
	"errors"
	"io"
	"syscall"

	"github.com/charmbracelet/x/xpty"
)

// output gives the reader its own handle on the ConPTY's output pipe, so it reads on to the pipe's end after
// the pty is closed (closing a ConPTY flushes its last frame).
func output(p xpty.Pty) (io.ReadCloser, error) {
	c, ok := p.(*xpty.ConPty)
	if !ok {
		return io.NopCloser(p), nil
	}
	self, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, err
	}
	var dup syscall.Handle
	if err = syscall.DuplicateHandle(self, syscall.Handle(c.OutPipeReadFd()), self, &dup, 0, false, syscall.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	return pipeEnd(dup), nil
}

type pipeEnd syscall.Handle

func (h pipeEnd) Read(b []byte) (int, error) {
	var n uint32
	err := syscall.ReadFile(syscall.Handle(h), b, &n, nil)
	if errors.Is(err, syscall.ERROR_BROKEN_PIPE) {
		err = io.EOF
	}
	return int(n), err
}

func (h pipeEnd) Close() error { return syscall.CloseHandle(syscall.Handle(h)) }
