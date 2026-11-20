//go:build windows

package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
)

func socketPath(string) string { return fmt.Sprintf(`\\.\pipe\hoshi-eval-%d`, os.Getpid()) }

func dial(path string) (net.Conn, error) {
	timeout := time.Second
	return winio.DialPipe(path, &timeout)
}
