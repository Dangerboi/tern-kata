package model

import (
	"slices"
	"strings"
)

// attachmentKeyPrefix starts the kata meta key of every Attachment: "attachment." + its stored name.
const attachmentKeyPrefix = "attachment."

// Attachment is one file attached to an Issue: its stored name and the Attachment pointer its meta key holds, the host that
// holds the copy and the copy's absolute native path there.
type Attachment struct {
	Name, Host, Path string
}

// AttachmentKey is the kata meta key of the Attachment stored under name.
func AttachmentKey(name string) string { return attachmentKeyPrefix + name }

// Pointer is the meta value of the Attachment: exactly {"host", "path"}.
func (a Attachment) Pointer() map[string]any { return map[string]any{"host": a.Host, "path": a.Path} }

// OnHost is whether the copy is on the machine named host. Host names compare without regard to case, and an empty host on
// either side is never this machine.
func (a Attachment) OnHost(host string) bool {
	return host != "" && a.Host != "" && strings.EqualFold(a.Host, host)
}

// Attachments are the Issue's Attachments read from its attachment.* meta keys, sorted by name. A value that is not a
// {"host", "path"} object of strings is not an Attachment.
func Attachments(metadata map[string]any) []Attachment {
	var list []Attachment
	for key, value := range metadata {
		name, ok := strings.CutPrefix(key, attachmentKeyPrefix)
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
