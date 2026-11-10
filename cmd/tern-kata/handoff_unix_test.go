//go:build !windows

package main

import (
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// openPty returns a pty's two ends: the terminal side is the file a Pane would read.
func openPty(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Skipf("unlock pty: %v", err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Skipf("pty number: %v", err)
	}
	slave, err = os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("open pty slave: %v", err)
	}
	t.Cleanup(func() { master.Close(); slave.Close() })
	return master, slave
}

func TestTTYInputCancelEndsBlockedRead(t *testing.T) {
	master, slave := openPty(t)
	input := &ttyInput{f: slave}
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		buf := make([]byte, 16)
		n, err := input.Read(buf)
		done <- result{n, err}
	}()
	select {
	case r := <-done:
		t.Fatalf("Read returned before input or cancel: %+v", r)
	case <-time.After(100 * time.Millisecond):
	}
	start := time.Now()
	_ = input.cancel()
	select {
	case r := <-done:
		if !errors.Is(r.err, os.ErrClosed) {
			t.Fatalf("cancelled Read = %+v, want os.ErrClosed", r)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("Read took %v to end after cancel", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read still blocked 2 s after cancel")
	}
	_ = master
}

func TestTTYInputReadsTypedBytes(t *testing.T) {
	master, slave := openPty(t)
	input := &ttyInput{f: slave}
	if _, err := master.Write([]byte("q\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	got := make(chan string, 1)
	go func() {
		n, _ := input.Read(buf)
		got <- string(buf[:n])
	}()
	select {
	case s := <-got:
		if s != "q" && s != "q\n" {
			t.Fatalf("Read = %q, want q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not return typed input")
	}
}
