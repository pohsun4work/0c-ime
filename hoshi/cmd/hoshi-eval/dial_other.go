//go:build !windows

package main

import (
	"net"
	"path/filepath"
)

func socketPath(dir string) string { return filepath.Join(dir, "engine.sock") }

func dial(path string) (net.Conn, error) { return net.Dial("unix", path) }
