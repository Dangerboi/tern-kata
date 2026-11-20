package model

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NoticeKind says how the Notice line is drawn: a failure in the error tone, anything else in default text.
type NoticeKind uint8

const (
	NoticeInfo NoticeKind = iota
	NoticeFail
)

// ErrDisconnected is the one refusal for a write asked while the stream to kata is down.
var ErrDisconnected = errors.New("disconnected from kata")

// say sets the Notice to text, drawn in default text. It is one sentence in the past tense that ends with a period.
func (m Model) say(text string) Model {
	m.Notice, m.NoticeKind = text, NoticeInfo
	return m
}

// fail sets the Notice to a failure or refusal, drawn in the error tone. text is built with Cannot.
func (m Model) fail(text string) Model {
	m.Notice, m.NoticeKind = text, NoticeFail
	return m
}

// clearNotice takes the Notice line away.
func (m Model) clearNotice() Model {
	m.Notice, m.NoticeKind = "", NoticeInfo
	return m
}

// Cannot is the one sentence for a failure or refusal: "Cannot <verb> <thing>: <reason>." thing may be empty.
func Cannot(verb, thing string, err error) string {
	if thing != "" {
		verb += " " + thing
	}
	return "Cannot " + verb + ": " + Reason(err) + "."
}

// Refused is the Notice of a refusal whose reason is held as text, not an error: `Cannot <verb> <thing>: <reason>.`. The reason
// keeps its case, so a name it starts with ("Tern draws only …", "Trust must be Yes …") stays a name.
func Refused(verb, thing, reason string) string { return Cannot(verb, thing, errors.New(reason)) }

// Clause is a sentence a button's tooltip shows ("The Issue is closed.") as the reason of a Refused Notice: its first letter goes
// to lower case unless the word is an acronym.
func Clause(sentence string) string {
	first, size := utf8.DecodeRuneInString(sentence)
	if next, _ := utf8.DecodeRuneInString(sentence[size:]); size > 0 && unicode.IsUpper(first) && !unicode.IsUpper(next) {
		return string(unicode.ToLower(first)) + sentence[size:]
	}
	return sentence
}

// Reason is the first line of err's text without its closing period, so it ends a sentence; "unknown error" when there is none.
func Reason(err error) string {
	if err == nil {
		return "unknown error"
	}
	line, _, _ := strings.Cut(err.Error(), "\n")
	line = strings.TrimSuffix(strings.TrimSpace(line), ".")
	if line == "" {
		return "unknown error"
	}
	return line
}

// Progress is the line for work in flight: "Loading jobs…", or "Saving…" when thing is empty. It is the only builder of such
// lines, so every one ends in the same ellipsis.
func Progress(gerund, thing string) string {
	if thing != "" {
		gerund += " " + thing
	}
	return gerund + "…"
}

// KeyOpen and KeyClose wrap a key inside a model string. They are private-use runes, which no name, path or error text carries,
// so the view draws a keycap only where the model put one and draws every other character as typed, braces included.
const (
	KeyOpen  = "\uE000"
	KeyClose = "\uE001"
)

// Key names a key inside a model string as a keycap marker, which the view draws as a keycap.
func Key(k string) string { return KeyOpen + k + KeyClose }
