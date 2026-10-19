package model

import (
	"slices"
	"strings"
)

// AttachmentKeyPrefix starts the kata meta key of every Attachment: "attachment." + its stored name.
const AttachmentKeyPrefix = "attachment."

// Attachment is one file attached to an Issue: its stored name and the pointer its meta key holds, the host that
// holds the copy and the copy's absolute native path there.
type Attachment struct {
	Name, Host, Path string
}

// Attachments are the Issue's Attachments read from its attachment.* meta keys, sorted by name. A value that is not a
// {"host", "path"} object of strings is not an Attachment.
func Attachments(metadata map[string]any) []Attachment {
	var list []Attachment
	for key, value := range metadata {
		name, ok := strings.CutPrefix(key, AttachmentKeyPrefix)
		pointer, isObject := value.(map[string]any)
		if !ok || name == "" || !isObject {
			continue
		}
		host, hostOK := pointer["host"].(string)
		path, pathOK := pointer["path"].(string)
		if hostOK && pathOK {
			list = append(list, Attachment{Name: name, Host: host, Path: path})
		}
	}
	slices.SortFunc(list, func(a, b Attachment) int { return strings.Compare(a.Name, b.Name) })
	return list
}
