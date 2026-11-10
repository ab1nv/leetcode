package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type textField struct {
	runes  []rune
	cursor int
}

func (f *textField) value() string {
	return string(f.runes)
}

func (f *textField) insert(s string) {
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		f.runes = append(f.runes, 0)
		copy(f.runes[f.cursor+1:], f.runes[f.cursor:])
		f.runes[f.cursor] = r
		f.cursor++
	}
}

func (f *textField) handleKey(k keyEvent) bool {
	switch k.kind {
	case keyRune, keyPaste:
		if k.text == "" {
			return false
		}
		f.insert(k.text)
		return true
	case keyBackspace:
		if f.cursor == 0 {
			return false
		}
		f.runes = append(f.runes[:f.cursor-1], f.runes[f.cursor:]...)
		f.cursor--
		return true
	case keyDelete:
		if f.cursor >= len(f.runes) {
			return false
		}
		f.runes = append(f.runes[:f.cursor], f.runes[f.cursor+1:]...)
		return true
	case keyLeft:
		if f.cursor > 0 {
			f.cursor--
		}
	case keyRight:
		if f.cursor < len(f.runes) {
			f.cursor++
		}
	case keyHome, keyCtrlA:
		f.cursor = 0
	case keyEnd, keyCtrlE:
		f.cursor = len(f.runes)
	case keyCtrlU:
		if len(f.runes) == 0 {
			return false
		}
		f.runes = nil
		f.cursor = 0
		return true
	case keyCtrlW:
		return f.deleteWord()
	}
	return false
}

func (f *textField) deleteWord() bool {
	if f.cursor == 0 {
		return false
	}
	start := f.cursor
	for start > 0 && unicode.IsSpace(f.runes[start-1]) {
		start--
	}
	for start > 0 && !unicode.IsSpace(f.runes[start-1]) {
		start--
	}
	f.runes = append(f.runes[:start], f.runes[f.cursor:]...)
	f.cursor = start
	return true
}

func (f *textField) view(width int, t theme) string {
	if width <= 0 {
		return ""
	}

	offset := 0
	if f.cursor >= width {
		offset = f.cursor - width + 1
	}
	if offset > len(f.runes) {
		offset = len(f.runes)
	}
	end := offset + width
	if end > len(f.runes) {
		end = len(f.runes)
	}

	var b strings.Builder
	b.WriteString(string(f.runes[offset:end]))
	visible := end - offset

	cursorAt := f.cursor - offset
	if cursorAt >= 0 && cursorAt < visible {
		before := string(f.runes[offset : offset+cursorAt])
		cursorRune := f.runes[offset+cursorAt]
		after := string(f.runes[offset+cursorAt+1 : end])
		b.Reset()
		b.WriteString(before)
		b.WriteString(t.reverse(string(cursorRune)))
		b.WriteString(after)
	} else if cursorAt == visible {
		b.WriteString(t.reverse(" "))
		visible++
	}

	if visible < width {
		b.WriteString(strings.Repeat(" ", width-visible))
	}
	return b.String()
}

func visibleWidth(s string) int {
	width := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			if n := ansiLen(s[i:]); n > 0 {
				i += n
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if !unicode.Is(unicode.Mn, r) {
			width++
		}
		i += size
	}
	return width
}

func truncateStyled(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if visibleWidth(s) <= width {
		return s
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}

	var b strings.Builder
	used := 0
	target := width - 3
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			if n := ansiLen(s[i:]); n > 0 {
				b.WriteString(s[i : i+n])
				i += n
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		cells := 1
		if unicode.Is(unicode.Mn, r) {
			cells = 0
		}
		if used+cells > target {
			break
		}
		b.WriteString(s[i : i+size])
		used += cells
		i += size
	}
	b.WriteString("...")
	b.WriteString(ansiReset)
	return b.String()
}

func padStyled(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := visibleWidth(s)
	if w > width {
		return truncateStyled(s, width)
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

func centerStyled(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = truncateStyled(s, width)
	w := visibleWidth(s)
	left := (width - w) / 2
	return strings.Repeat(" ", left) + s
}

func truncatePlain(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	return string(runes[:width-3]) + "..."
}

func wrapPlain(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			lines = append(lines, current.String())
			current.Reset()
		}
	}

	for _, word := range words {
		wordWidth := utf8.RuneCountInString(word)
		if wordWidth > width {
			flush()
			runes := []rune(word)
			for len(runes) > width {
				lines = append(lines, string(runes[:width]))
				runes = runes[width:]
			}
			current.WriteString(string(runes))
			continue
		}
		if current.Len() == 0 {
			current.WriteString(word)
			continue
		}
		if utf8.RuneCountInString(current.String())+1+wordWidth <= width {
			current.WriteByte(' ')
			current.WriteString(word)
			continue
		}
		flush()
		current.WriteString(word)
	}
	flush()
	return lines
}

func ansiLen(s string) int {
	if len(s) < 2 || s[0] != '\x1b' {
		return 0
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
	case ']':
		for i := 2; i < len(s); i++ {
			if s[i] == '\a' {
				return i + 1
			}
			if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
	}
	return 0
}
