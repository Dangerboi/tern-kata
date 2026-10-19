package model

import (
	"net/url"
	"strings"
)

// fileURL is the file URL of an absolute native path: C:\a b\x.png is file:///C:/a%20b/x.png, /a/x.png is file:///a/x.png.
// Only a Windows path (a drive letter or a UNC share) has its backslashes turned into slashes; on Unix one is part of a name.
func fileURL(path string) string {
	if windowsPath(path) {
		path = strings.ReplaceAll(path, "\\", "/")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// windowsPath is whether path has a drive letter (C:\ or C:/) or is a UNC path.
func windowsPath(path string) bool {
	if strings.HasPrefix(path, `\\`) {
		return true
	}
	return len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') &&
		(path[0]|0x20 >= 'a' && path[0]|0x20 <= 'z')
}
