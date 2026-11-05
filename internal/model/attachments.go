package model

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// attachmentKeyPrefix starts the kata meta key of every Attachment: "attachment." + its id. The value is kata's reference to the
// file's bytes in Docbank.
const attachmentKeyPrefix = "attachment."

// DocbankOffMessage is what the Pane and the commands say when the kata daemon has no Docbank configured.
const DocbankOffMessage = "This kata daemon has no Docbank configured, so it cannot store attachments. Add a [docbank] section to kata's config.toml and restart the daemon; the Attachments guide says how."

// MaxAttachmentBytes is the largest file kata accepts as an Attachment.
const MaxAttachmentBytes = 256 << 20

// Attachment is one file attached to an Issue, as kata's native reference to it: the id in its meta key, the file's name and media
// type, its size in bytes and its Docbank version. kata can point the same id at another version, so the id alone does not name
// one set of bytes.
type Attachment struct {
	ID, Name, MediaType, Version string
	Size                         int64
}

// attachmentID is the shape of an id this program reads: it ends up in a cache path and a request path.
var attachmentID = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)

// AttachmentKey is the kata meta key of the Attachment with id.
func AttachmentKey(id string) string { return attachmentKeyPrefix + id }

// validAttachmentName is kata's rule for the name of an Attachment: 1 to 255 bytes of UTF-8 with no slash, backslash or control
// character, and neither "." nor "..".
func validAttachmentName(name string) bool {
	return utf8.ValidString(name) && name != "" && len(name) <= 255 && name != "." && name != ".." &&
		!strings.ContainsAny(name, `/\`) && !strings.ContainsFunc(name, unicode.IsControl)
}

// attachmentRef reads one reference object, a meta value or the copy a close froze into its evidence: the Attachment it names
// as id. ok is false for a value kata would not accept as a reference (no valid name, media type, size
// or Docbank version). A size is a float64 or, in a decoded event payload, a json.Number.
func attachmentRef(id string, value any) (Attachment, bool) {
	ref, isObject := value.(map[string]any)
	if !isObject {
		return Attachment{}, false
	}
	name, nameOK := ref["name"].(string)
	mediaType, typeOK := ref["media_type"].(string)
	version, versionOK := ref["docbank_version"].(string)
	var size float64
	var sizeOK bool
	switch n := ref["size"].(type) {
	case float64:
		size, sizeOK = n, true
	case json.Number:
		var err error
		size, err = n.Float64()
		sizeOK = err == nil
	}
	if nameOK && typeOK && sizeOK && versionOK && validAttachmentName(name) && mediaType != "" && version != "" && size >= 0 && size == float64(int64(size)) {
		return Attachment{ID: id, Name: name, MediaType: mediaType, Version: version, Size: int64(size)}, true
	}
	return Attachment{}, false
}

// Attachments are the Issue's Attachments read from its attachment.* meta keys, sorted by name, then id. A key whose value kata
// would not accept as a reference (no valid name, media type, size or Docbank version) is not an Attachment.
func Attachments(metadata map[string]any) []Attachment {
	var list []Attachment
	for key, value := range metadata {
		id, ok := strings.CutPrefix(key, attachmentKeyPrefix)
		if !ok || !attachmentID.MatchString(id) {
			continue
		}
		if a, ok := attachmentRef(id, value); ok {
			list = append(list, a)
		}
	}
	slices.SortFunc(list, func(a, b Attachment) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return list
}

// AttachmentLabels are the names the remove picker lists, in the order of list: a name that more than one Attachment has is
// followed by the last six characters of its id in brackets, so each label names one Attachment.
func AttachmentLabels(list []Attachment) []string {
	count := map[string]int{}
	for _, a := range list {
		count[a.Name]++
	}
	labels := make([]string, len(list))
	for i, a := range list {
		labels[i] = a.Name
		if count[a.Name] > 1 {
			labels[i] += " [" + a.ID[max(len(a.ID)-6, 0):] + "]"
		}
	}
	return labels
}

// AttachmentRowKey is the Issue detail row key of an Attachment: hex of its id, which has no ".", is unique per id and sits under
// a prefix no fixed Issue detail key uses.
func AttachmentRowKey(id string) string { return "attachment-" + hex.EncodeToString([]byte(id)) }

// FormatSize is a byte count as people read it: 512 B, 1.5 KB, 12.0 MB.
func FormatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	size, suffix := float64(n)/unit, "KB"
	for _, next := range []string{"MB", "GB"} {
		if size < unit {
			break
		}
		size, suffix = size/unit, next
	}
	return fmt.Sprintf("%.1f %s", size, suffix)
}

// Description is the Attachment as its row shows it after the name: media type and size.
func (a Attachment) Description() string { return a.MediaType + ", " + FormatSize(a.Size) }
