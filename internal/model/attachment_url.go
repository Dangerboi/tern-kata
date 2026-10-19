package model

import (
	"net/url"
	"strings"
)

// fileURL is the file URL of an absolute native path: C:\a b\x.png is file:///C:/a%20b/x.png, /a/x.png is file:///a/x.png.
func fileURL(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}
