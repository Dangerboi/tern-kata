package model

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NotificationKeyPrefix starts the metadata key of a notification slot: the prefix, then the recipient's address in unpadded
// base64url.
const NotificationKeyPrefix = "notify."

// recipientMaxBytes is kata's limit on the UTF-8 length of a recipient address.
const recipientMaxBytes = 128

var teammateHandle = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// NotificationSlot is one notification slot read the way kata reads it. Kind is empty for a request and the reply kind (reply,
// confirm, refute, supersede) for the notice a Reply left.
type NotificationSlot struct {
	Address   string // the recipient, as written (actor or actor/teammate)
	From      string
	Teammate  string
	Message   string
	Re        string // the comment handle or id the request names, if any
	Kind      string
	Broadcast bool
	Raw       string // the stored value as JSON, kept for guarding a later write
}

// NormalizeRecipient validates and trims a recipient address by kata's rules: not blank, at most 128 bytes, valid UTF-8 and no
// control or format characters.
func NormalizeRecipient(raw string) (string, error) {
	recipient := strings.TrimSpace(raw)
	if recipient == "" {
		return "", errors.New("recipient must not be blank")
	}
	if !utf8.ValidString(recipient) || len(recipient) > recipientMaxBytes {
		return "", errors.New("recipient must be valid UTF-8 and at most 128 bytes")
	}
	if strings.ContainsFunc(recipient, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) {
		return "", errors.New("recipient must not contain control characters")
	}
	return recipient, nil
}

// NotificationKey is the metadata key of the slot for a recipient address.
func NotificationKey(address string) string {
	return NotificationKeyPrefix + base64.RawURLEncoding.EncodeToString([]byte(address))
}

// NotificationAddress is the recipient a slot key names. A key that does not decode, or whose address kata would not accept or
// would spell differently, is not a slot.
func NotificationAddress(key string) (string, bool) {
	encoded, ok := strings.CutPrefix(key, NotificationKeyPrefix)
	if !ok || encoded == "" {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	address, err := NormalizeRecipient(string(raw))
	return address, err == nil && NotificationKey(address) == key
}

// ReadNotificationSlot reads the slot stored under key with the JSON value raw. The sender and message must be present and not
// blank; a malformed optional field (teammate, re, kind, broadcast) is dropped instead of failing the slot.
func ReadNotificationSlot(key, raw string) (NotificationSlot, error) {
	address, ok := NotificationAddress(key)
	if !ok {
		return NotificationSlot{}, errors.New("the key is not a notification slot")
	}
	var fields struct {
		From      string          `json:"from"`
		Message   string          `json:"message"`
		Teammate  json.RawMessage `json:"teammate"`
		Re        json.RawMessage `json:"re"`
		Kind      json.RawMessage `json:"kind"`
		Broadcast json.RawMessage `json:"broadcast"`
	}
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return NotificationSlot{}, err
	}
	if strings.TrimSpace(fields.From) == "" || strings.TrimSpace(fields.Message) == "" {
		return NotificationSlot{}, errors.New("notification requires a sender and message")
	}
	slot := NotificationSlot{Address: address, From: fields.From, Message: fields.Message, Raw: raw}
	if len(fields.Teammate) > 0 {
		if json.Unmarshal(fields.Teammate, &slot.Teammate) != nil || (slot.Teammate != "" && !teammateHandle.MatchString(slot.Teammate)) {
			slot.Teammate = ""
		}
	}
	_ = json.Unmarshal(fields.Re, &slot.Re)
	_ = json.Unmarshal(fields.Kind, &slot.Kind)
	_ = json.Unmarshal(fields.Broadcast, &slot.Broadcast)
	return slot, nil
}

// maxNamedRecipients is how many addresses a broadcast line names before it counts the rest.
const maxNamedRecipients = 3

// slotDiffChange is one notification slot in an issue.metadata_updated diff: a cleared request (Cleared) or the value now stored.
type slotDiffChange struct {
	address string
	cleared bool
	slot    NotificationSlot
	value   string
}

// slotDiff reads a metadata diff that changes nothing but readable notification slots. Any other key, or a slot value that does
// not read, gives false.
func slotDiff(diff map[string]any) ([]slotDiffChange, bool) {
	if len(diff) == 0 {
		return nil, false
	}
	var changes []slotDiffChange
	for key, change := range diff {
		address, ok := NotificationAddress(key)
		if !ok {
			return nil, false
		}
		var to any
		if fields, ok := change.(map[string]any); ok {
			to = fields["to"]
		}
		if to == nil {
			changes = append(changes, slotDiffChange{address: address, cleared: true})
			continue
		}
		raw, err := json.Marshal(to)
		if err != nil {
			return nil, false
		}
		slot, err := ReadNotificationSlot(key, string(raw))
		if err != nil {
			return nil, false
		}
		changes = append(changes, slotDiffChange{address: address, slot: slot, value: string(raw)})
	}
	slices.SortFunc(changes, func(a, b slotDiffChange) int { return strings.Compare(a.address, b.address) })
	return changes, true
}

// notificationSentence phrases a slot-only diff: one clause per distinct value, the same value on several slots naming its
// recipients together.
func notificationSentence(changes []slotDiffChange) (verb, object string) {
	var cleared []string
	type group struct {
		slot      NotificationSlot
		addresses []string
	}
	var groups []*group
	for _, c := range changes {
		if c.cleared {
			cleared = append(cleared, c.address)
			continue
		}
		if i := slices.IndexFunc(groups, func(g *group) bool { return g.slot.Raw == c.value }); i >= 0 {
			groups[i].addresses = append(groups[i].addresses, c.address)
			continue
		}
		groups = append(groups, &group{slot: c.slot, addresses: []string{c.address}})
	}
	var clauses []string
	for _, g := range groups {
		who := namedRecipients(g.addresses)
		if g.slot.Kind != "" {
			clauses = append(clauses, "told "+who+" about a "+oneLine(g.slot.Kind))
		} else {
			clauses = append(clauses, "asked "+who+": "+oneLine(g.slot.Message))
		}
	}
	for _, address := range cleared {
		clauses = append(clauses, "cleared "+address+"'s request")
	}
	verb, object, _ = strings.Cut(strings.Join(clauses, "; "), " ")
	return verb, object
}

// namedRecipients lists at most three addresses and counts the rest: "a, b, c +2".
func namedRecipients(addresses []string) string {
	if len(addresses) <= maxNamedRecipients {
		return strings.Join(addresses, ", ")
	}
	return strings.Join(addresses[:maxNamedRecipients], ", ") + " +" + strconv.Itoa(len(addresses)-maxNamedRecipients)
}
