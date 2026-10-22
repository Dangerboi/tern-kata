//go:build windows

package worker

// syncDir does nothing: Windows refuses to sync a folder ("Access is denied"), opened directly or through os.Root.
func syncDir(string) error { return nil }
