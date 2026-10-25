//go:build !linux && !darwin

package worker

import "net"

// peerAllowed admits every peer here: this platform has no peer-credential check wired in, so the guard is the
// state folder's ownership and the socket's mode.
func peerAllowed(net.Conn) bool { return true }
