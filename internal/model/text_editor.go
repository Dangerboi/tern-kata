package model

import (
	"strings"
	"unicode/utf16"
)

type TextSelection struct {
	Text           string
	Cursor, Anchor int
}
type TextEditor struct {
	TextSelection
	History []TextSelection
}

func len16(text string) int {
	length := 0
	for _, r := range text {
		length++
		if r >= 0x10000 {
			length++
		}
	}
	return length
}

func NewTextEditor(text string) TextEditor {
	end := len16(text)
	return TextEditor{TextSelection: TextSelection{Text: text, Cursor: end, Anchor: end}}
}

func boundary(text []uint16, offset int) bool {
	return offset >= 0 && offset <= len(text) && (offset == 0 || offset == len(text) || !(text[offset-1] >= 0xd800 && text[offset-1] <= 0xdbff && text[offset] >= 0xdc00 && text[offset] <= 0xdfff))
}

// ApplyEdit rejects stale lengths and offsets inside a UTF-16 surrogate pair.
func (e TextEditor) ApplyEdit(from, to int, text string, cursor, length int) TextEditor {
	units := utf16.Encode([]rune(e.Text))
	if length != len(units) || from > to || !boundary(units, from) || !boundary(units, to) {
		return e
	}
	if from == to && text == "" {
		if !boundary(units, cursor) {
			return e
		}
		e.Cursor = cursor
		e.Anchor = cursor
		return e
	}
	inserted := utf16.Encode([]rune(text))
	next := make([]uint16, 0, len(units)-(to-from)+len(inserted))
	next = append(next, units[:from]...)
	next = append(next, inserted...)
	next = append(next, units[to:]...)
	if !boundary(next, cursor) {
		return e
	}
	value := string(utf16.Decode(next))
	if value != e.Text {
		e.History = append(e.History[:len(e.History):len(e.History)], e.TextSelection)
	}
	e.TextSelection = TextSelection{Text: value, Cursor: cursor, Anchor: cursor}
	return e
}

func (e TextEditor) Undo() TextEditor {
	if len(e.History) != 0 {
		e.TextSelection = e.History[len(e.History)-1]
		e.History = e.History[:len(e.History)-1]
	}
	return e
}

func (e TextEditor) Insert(text string) TextEditor {
	from, to := min(e.Cursor, e.Anchor), max(e.Cursor, e.Anchor)
	return e.ApplyEdit(from, to, text, from+len16(text), len16(e.Text))
}

func (e TextEditor) Key(action Action, selectText bool) TextEditor {
	switch action {
	case EditorUndo:
		return e.Undo()
	case EditorNewline:
		return e.Insert("\n")
	case EditorSelectAll:
		e.Anchor = 0
		e.Cursor = len16(e.Text)
		return e
	}
	units := utf16.Encode([]rune(e.Text))
	pos := e.Cursor
	previous := func(p int) int {
		p = max(0, p-1)
		if !boundary(units, p) {
			p--
		}
		return p
	}
	next := func(p int) int {
		p = min(len(units), p+1)
		if !boundary(units, p) {
			p++
		}
		return p
	}
	switch action {
	case EditorBackspace, EditorDelete:
		from, to := min(e.Cursor, e.Anchor), max(e.Cursor, e.Anchor)
		if from == to {
			if action == EditorBackspace {
				from = previous(from)
			} else {
				to = next(to)
			}
		}
		return e.ApplyEdit(from, to, "", from, len(units))
	case EditorLeft:
		if !selectText && e.Anchor != pos {
			pos = min(pos, e.Anchor)
		} else {
			pos = previous(pos)
		}
	case EditorRight:
		if !selectText && e.Anchor != pos {
			pos = max(pos, e.Anchor)
		} else {
			pos = next(pos)
		}
	case EditorHome:
		for pos > 0 && units[pos-1] != '\n' {
			pos--
		}
	case EditorEnd:
		for pos < len(units) && units[pos] != '\n' {
			pos++
		}
	case EditorUp, EditorDown:
		start := pos
		for start > 0 && units[start-1] != '\n' {
			start--
		}
		column := pos - start
		if action == EditorUp {
			if start == 0 {
				pos = 0
			} else {
				end := start - 1
				start = end
				for start > 0 && units[start-1] != '\n' {
					start--
				}
				pos = min(start+column, end)
			}
		} else {
			for start < len(units) && units[start] != '\n' {
				start++
			}
			if start == len(units) {
				pos = start
			} else {
				start++
				end := start
				for end < len(units) && units[end] != '\n' {
					end++
				}
				pos = min(start+column, end)
			}
		}
		if !boundary(units, pos) {
			pos--
		}
	default:
		return e
	}
	e.Cursor = pos
	if !selectText {
		e.Anchor = pos
	}
	return e
}

func editorSelectionChord(chord string) bool { return strings.HasPrefix(chord, "shift+") }
