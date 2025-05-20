package main

import (
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

type reader struct {
	rd    io.Reader
	buf   []byte
	begin int
	end   int

	value    []rune // token beginning
	pos      int    // position within input
	line     int    // line within input
	col      int    // column within input
	indented bool   // true if the only whitespace so far on this line
}

func newReader(rd io.Reader) reader {
	return reader{rd: rd, buf: make([]byte, 1024), line: 1, indented: true}
}

// Return the nth character without advancing.
func (l *reader) peekN(n int) rune {
	if !l.ensure(n + 1) {
		return eof
	}
	win := l.window()
	for range n {
		_, w := utf8.DecodeRune(win)
		win = win[w:]
	}
	c, _ := utf8.DecodeRune(win)
	return c
}

// Return the next character without advancing.
func (l *reader) peek() rune {
	return l.peekN(0)
}

// Consume and return the next character in the lexer input.
func (l *reader) next() rune {
	if !l.ensure(1) {
		return eof
	}
	c, w := utf8.DecodeRune(l.window())
	l.begin += w

	l.pos++
	l.value = append(l.value, c)

	if c == '\n' {
		l.col = 0
		l.line++
		l.indented = true
	} else {
		l.col++
		if !strings.ContainsRune(" \t", c) {
			l.indented = false
		}
	}

	return c
}

func (l *reader) window() []byte {
	return l.buf[l.begin:l.end]
}

func (l *reader) runecount() int {
	return utf8.RuneCount(l.window())
}

/* ensures at least n runes in the window, returns if it were possible to fill the buffer */
func (l *reader) ensure(count int) bool {
	/* if the buffer is big enough, that will do */
	for l.runecount() < count && l.end-l.begin < len(l.buf) {
		if l.begin > 0 {
			copy(l.buf, l.window())
			l.end -= l.begin
			l.begin = 0
		}
		n, err := l.rd.Read(l.buf[l.end:])
		if errors.Is(err, io.EOF) && l.buf[l.end] != '\n' {
			l.buf[l.end] = '\n'
			n = 1
		} else if err != nil {
			break
		}
		l.end += n
	}
	return l.runecount() >= count
}
