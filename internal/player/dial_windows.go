//go:build windows

package player

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// mpv's IPC on Windows is a named pipe.
func dialIPC(path string) (net.Conn, error) {
	d := time.Second
	return winio.DialPipe(path, &d)
}
