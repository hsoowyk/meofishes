//go:build !windows

package player

import (
	"net"
	"time"
)

func dialIPC(path string) (net.Conn, error) { return net.DialTimeout("unix", path, time.Second) }
