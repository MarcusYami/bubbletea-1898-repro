//go:build !windows

package main

import (
	"io"

	"github.com/charmbracelet/x/xpty"
)

// output is the pty itself on Unix; it ends at endMark.
func output(p xpty.Pty) (io.ReadCloser, error) { return io.NopCloser(p), nil }
