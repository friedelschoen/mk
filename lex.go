package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

type tokenType int

const eof rune = '\u0000'

// Rune's that cannot be part of a bare (unquoted) string.
const nonBareRunes = " \t\n\r\\=:#'\"$"

// Return true if the string contains whitespace only.
func onlyWhitespace(s []rune) bool {
	return !slices.ContainsFunc(s, unicode.IsSpace)
}

const (
	tokenError tokenType = iota
	tokenNewline
	tokenWord
	tokenPipeInclude
	tokenRedirInclude
	tokenColon
	tokenAssign
	tokenRecipe
)

func (typ tokenType) String() string {
	switch typ {
	case tokenError:
		return "[Error]"
	case tokenNewline:
		return "[Newline]"
	case tokenWord:
		return "[Word]"
	case tokenPipeInclude:
		return "[PipeInclude]"
	case tokenRedirInclude:
		return "[RedirInclude]"
	case tokenColon:
		return "[Colon]"
	case tokenAssign:
		return "[Assign]"
	case tokenRecipe:
		return "[Recipe]"
	}
	return "[MysteryToken]"
}

type token struct {
	typ  tokenType // token type
	val  string    // token string
	line int       // line where it was found
	col  int       // column on which the token began
}

func (t *token) String() string {
	if t.typ == tokenError {
		return t.val
	} else if t.typ == tokenNewline {
		return "\\n"
	}

	return t.val
}

type lexer struct {
	input     []rune  // input string to be lexed
	output    []token // channel on which tokens are sent
	start     int     // token beginning
	startcol  int     // column on which the token begins
	pos       int     // position within input
	line      int     // line within input
	col       int     // column within input
	errmsg    string  // set to an appropriate error message when necessary
	indented  bool    // true if the only whitespace so far on this line
	barewords bool    // lex only a sequence of words
	state     lexerStateFun
}

// A lexerStateFun is simultaneously the the state of the lexer and the next
// action the lexer will perform.
type lexerStateFun func(*lexer) lexerStateFun

func (l *lexer) lexerror(what string) {
	if l.errmsg == "" {
		l.errmsg = what
	}
	l.emit(tokenError)
}

// Return the nth character without advancing.
func (l *lexer) peekN(n int) rune {
	if l.pos+n >= len(l.input) {
		return 0
	}
	return l.input[l.pos+n]
}

// Return the next character without advancing.
func (l *lexer) peek() rune {
	return l.peekN(0)
}

// Consume and return the next character in the lexer input.
func (l *lexer) next() rune {
	if l.pos >= len(l.input) {
		return eof
	}
	c := l.input[l.pos]
	l.pos++

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

// Skip and return the next character in the lexer input.
func (l *lexer) skip() {
	l.next()
	l.start = l.pos
	l.startcol = l.col
}

func (l *lexer) emit(typ tokenType) {
	l.output = append(l.output, token{typ, string(l.input[l.start:l.pos]), l.line, l.startcol})
	l.start = l.pos
	l.startcol = 0
}

// Consume the next run if it is in the given string.
func (l *lexer) accept(valid string) bool {
	if strings.ContainsRune(valid, l.peek()) {
		l.next()
		return true
	}
	return false
}

// Consume characters from the valid string until the next is not.
func (l *lexer) acceptRun(valid string) int {
	prevpos := l.pos
	for strings.ContainsRune(valid, l.peek()) {
		l.next()
	}
	return l.pos - prevpos
}

// Accept until something from the given string is encountered.
func (l *lexer) acceptUntil(invalid string) {
	for l.pos < len(l.input) && !strings.ContainsRune(invalid, l.peek()) {
		l.next()
	}

	if l.peek() == eof {
		l.lexerror(fmt.Sprintf("end of file encountered while looking for one of: %s", invalid))
	}
}

// Accept until something from the given string is encountered, or the end of th
// file
func (l *lexer) acceptUntilOrEof(invalid string) {
	for l.pos < len(l.input) && !strings.ContainsRune(invalid, l.peek()) {
		l.next()
	}
}

// Skip characters from the valid string until the next is not.
func (l *lexer) skipRun(valid string) int {
	prevpos := l.pos
	for strings.ContainsRune(valid, l.peek()) {
		l.skip()
	}
	return l.pos - prevpos
}

// Skip until something from the given string is encountered.
func (l *lexer) skipUntil(invalid string) {
	for l.pos < len(l.input) && !strings.ContainsRune(invalid, l.peek()) {
		l.skip()
	}

	if l.peek() == eof {
		l.lexerror(fmt.Sprintf("end of file encountered while looking for one of: %s", invalid))
	}
}

// Start a new lexer to lex the given input.
func lex(input string) *lexer {
	// Files without a trailing newline are considered to have one.
	if len(input) > 0 && input[len(input)-1] != '\n' {
		input = input + "\n"
	}

	return &lexer{input: []rune(input), line: 1, col: 0, indented: true, state: lexTopLevel}
}

func lexWords(input string) *lexer {
	return &lexer{input: []rune(input), line: 1, col: 0, indented: true, barewords: true, state: lexTopLevel}
}

func (l *lexer) nextToken() (token, bool) {
	for l.state != nil && len(l.output) == 0 {
		l.state = l.state(l)
	}
	if len(l.output) == 0 {
		return token{}, false
	}
	tok := l.output[0]
	l.output = l.output[1:]
	return tok, true
}

func lexTopLevel(l *lexer) lexerStateFun {
	for {
		l.skipRun(" \t\r")
		// emit a newline token if we are ending a non-empty line.
		if l.peek() == '\n' && !l.indented {
			l.next()
			if l.barewords {
				return nil
			} else {
				l.emit(tokenNewline)
			}
		}
		l.skipRun(" \t\r\n")

		if l.peek() == '\\' && l.peekN(1) == '\n' {
			l.next()
			l.next()
			l.indented = false
		} else {
			break
		}
	}

	if l.indented && l.col > 0 {
		return lexRecipe
	}

	c := l.peek()
	switch c {
	case eof:
		return nil
	case '#':
		return lexComment
	case '<':
		return lexInclude
	case ':':
		return lexColon
	case '=':
		return lexAssign
	case '"':
		return lexDoubleQuotedWord
	case '\'':
		return lexSingleQuotedWord
	case '`':
		return lexBackQuotedWord
	}

	return lexBareWord
}

func lexColon(l *lexer) lexerStateFun {
	l.next()
	l.emit(tokenColon)
	return lexTopLevel
}

func lexAssign(l *lexer) lexerStateFun {
	l.next()
	l.emit(tokenAssign)
	return lexTopLevel
}

func lexComment(l *lexer) lexerStateFun {
	l.skip() // '#'
	l.skipUntil("\n")
	return lexTopLevel
}

func lexInclude(l *lexer) lexerStateFun {
	l.next() // '<'
	if l.accept("|") {
		l.emit(tokenPipeInclude)
	} else {
		l.emit(tokenRedirInclude)
	}
	return lexTopLevel
}

func lexDoubleQuotedWord(l *lexer) lexerStateFun {
	l.next() // '"'
	for l.peek() != '"' && l.peek() != eof {
		l.acceptUntil("\\\"")
		if l.accept("\\") {
			l.accept("\"")
		}
	}

	if l.peek() == eof {
		l.lexerror("end of file encountered while parsing a quoted string.")
	}

	l.next() // '"'
	return lexBareWord
}

func lexBackQuotedWord(l *lexer) lexerStateFun {
	l.next() // '`'
	l.acceptUntil("`")
	l.next() // '`'
	return lexBareWord
}

func lexSingleQuotedWord(l *lexer) lexerStateFun {
	l.next() // '\''
	l.acceptUntil("'")
	l.next() // '\''
	return lexBareWord
}

func lexRecipe(l *lexer) lexerStateFun {
	for {
		l.acceptUntilOrEof("\n")
		l.acceptRun(" \t\n\r")
		if !l.indented || l.col == 0 {
			break
		}
	}

	if !onlyWhitespace(l.input[l.start:l.pos]) {
		l.emit(tokenRecipe)
	}
	return lexTopLevel
}

func lexBareWord(l *lexer) lexerStateFun {
	l.acceptUntil(nonBareRunes)
	c := l.peek()
	if c == '"' {
		return lexDoubleQuotedWord
	} else if c == '\'' {
		return lexSingleQuotedWord
	} else if c == '`' {
		return lexBackQuotedWord
	} else if c == '\\' {
		c1 := l.peekN(1)
		if c1 == '\n' || c1 == '\r' {
			if l.start < l.pos {
				l.emit(tokenWord)
			}
			l.skip()
			l.skip()
			return lexTopLevel
		} else {
			l.next()
			l.next()
			return lexBareWord
		}
	} else if c == '$' {
		c1 := l.peekN(1)
		if c1 == '{' {
			return lexBracketExpansion
		} else {
			l.next()
			return lexBareWord
		}
	}

	if l.start < l.pos {
		l.emit(tokenWord)
	}

	return lexTopLevel
}

func lexBracketExpansion(l *lexer) lexerStateFun {
	l.next() // '$'
	l.next() // '{'
	l.acceptUntil("}")
	l.next() // '}'
	return lexBareWord
}
