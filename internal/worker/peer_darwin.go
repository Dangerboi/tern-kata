package worker

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// peerAllowed reports whether the process on the other end of the control socket runs as the same user.
func peerAllowed(conn net.Conn) bool {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return false
	}
	var cred *unix.Xucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil || credErr != nil {
		return false
	}
	return int(cred.Uid) == os.Getuid()
}
