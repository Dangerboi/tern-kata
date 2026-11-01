//go:build !windows

package main

import "os"

// swapBinary renames fresh over target. The running process keeps its old inode; a Pane or Worker started later gets the new
// file.
func swapBinary(target, fresh string) error { return os.Rename(fresh, target) }
