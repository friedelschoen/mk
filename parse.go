// This is a mkfile parser. It executes assignments and includes as it goes, and
// collects a set of rules, which are returned as a ruleSet object.

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type parser struct {
	l        *lexer   // underlying lexer
	name     string   // name of the file being parsed
	path     string   // full path of the file being parsed
	tokenbuf []token  // tokens consumed on the current statement
	rules    *ruleSet // current ruleSet
}

// Pretty errors.
func (p *parser) parseError(context string, expected string, found token) {
	mkPrintError(fmt.Sprintf("%s:%d: syntax error: ", p.name, found.line))
	mkPrintError(fmt.Sprintf("while %s, expected %s but found '%s'.\n",
		context, expected, found.String()))
	mkError("")
}

// More basic errors.
func (p *parser) basicErrorAtToken(what string, found token) {
	p.basicErrorAtLine(what, found.line)
}

func (p *parser) basicErrorAtLine(what string, line int) {
	mkError(fmt.Sprintf("%s:%d: syntax error: %s\n", p.name, line, what))
}

// Accept a token for use in the current statement being parsed.
func (p *parser) push(t token) {
	p.tokenbuf = append(p.tokenbuf, t)
}

// Clear all the accepted tokens. Called when a statement is finished.
func (p *parser) clear() {
	p.tokenbuf = p.tokenbuf[:0]
}

// A parser state function takes a parser and the next token and returns a new
// state function, or nil if there was a parse error.
type parserStateFun func(*parser, token) parserStateFun

// Parse a mkfile, returning a new ruleSet.
func parse(input io.Reader, name string, path string, env map[string][]string) *ruleSet {
	rules := &ruleSet{env,
		make([]rule, 0),
		make(map[string][]int)}
	parseInto(input, name, rules, path)
	return rules
}

// Parse a mkfile inserting rules and variables into a given ruleSet.
func parseInto(input io.Reader, name string, rules *ruleSet, path string) {
	l := lex(input, false)
	p := &parser{l, name, path, []token{}, rules}
	oldmkfiledir := p.rules.vars["mkfiledir"]
	p.rules.vars["mkfiledir"] = []string{filepath.Dir(path)}
	state := parseTopLevel
	for {
		t, ok := l.nextToken()
		if !ok {
			break
		}
		if t.typ == tokenError {
			p.basicErrorAtLine(l.errmsg, t.line)
			break
		}

		state = state(p, t)
	}

	// insert a dummy newline to allow parsing of any assignments or recipeless
	// rules to finish.
	state = state(p, token{tokenNewline, "\n", l.line, l.col})

	p.rules.vars["mkfiledir"] = oldmkfiledir

	// TODO: Error when state != parseTopLevel
}

// We are at the top level of a mkfile, expecting rules, assignments, or
// includes.
func parseTopLevel(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		return parseTopLevel
	case tokenPipeInclude:
		return parsePipeInclude
	case tokenRedirInclude:
		return parseRedirInclude
	case tokenWord:
		return parseAssignmentOrTarget(p, t)
	default:
		p.parseError("parsing mkfile",
			"a rule, include, or assignment", t)
	}

	return parseTopLevel
}

func prettyPipeIncludeName(args []string) string {
	nb := new(strings.Builder)
	nb.WriteString("<|")
	nb.WriteString(args[0])
	for _, a := range args[1:] {
		nb.WriteRune(' ')
		nb.WriteString(a)
	}
	return nb.String()
}

// Consumed a '<|'
func parsePipeInclude(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		if len(p.tokenbuf) == 0 {
			p.basicErrorAtToken("empty pipe include", t)
		}
		args := make([]string, 0, len(p.tokenbuf))
		for _, tk := range p.tokenbuf {
			// TODO(rjk): Do we need to expand backticks here?
			args = append(args, expand(tk.val, p.rules.vars, false)...)
		}

		// TODO(rjk): determine what env should be in comparison with p9p.

		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = nil
		cmd.Stderr = os.Stderr
		output, err := cmd.StdoutPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to create pipe: %v", err)
			return nil
		}
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "unable to start process: %v", err)
			return nil
		}

		parseInto(output, prettyPipeIncludeName(args), p.rules, p.path)
		p.clear()
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "unable to wait for process: %v", err)
			return nil
		}
		return parseTopLevel
	// Almost anything goes. Let the shell sort it out.
	case tokenPipeInclude:
		fallthrough
	case tokenRedirInclude:
		fallthrough
	case tokenColon:
		fallthrough
	case tokenAssign:
		fallthrough
	case tokenWord:
		p.tokenbuf = append(p.tokenbuf, t)

	default:
		p.parseError("parsing piped include", "a shell command", t)
	}

	return parsePipeInclude
}

// Consumed a '<'
func parseRedirInclude(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		var filenameraw strings.Builder
		for i := range p.tokenbuf {
			filenameraw.WriteString(p.tokenbuf[i].val)
		}

		// Expand variables in paths.
		parts := expand(filenameraw.String(), p.rules.vars, false)
		if len(parts) != 1 {
			mkError("filename variables need to be a single value")
		}

		// TODO(rjk): Be sure that this is the right behaviour.
		filename := parts[0]

		input, err := os.Open(filename)
		if err != nil {
			p.basicErrorAtToken(fmt.Sprintf("cannot open %s", filename), p.tokenbuf[0])
		}
		defer input.Close()

		path, err := filepath.Abs(filename)
		if err != nil {
			mkError("unable to find mkfile's absolute path")
		}

		parseInto(input, filename, p.rules, path)

		p.clear()
		return parseTopLevel

	case tokenWord:
		p.tokenbuf = append(p.tokenbuf, t)

	default:
		p.parseError("parsing include", "a file name", t)
	}

	return parseRedirInclude
}

// Encountered a bare string at the beginning of the line.
func parseAssignmentOrTarget(p *parser, t token) parserStateFun {
	p.push(t)
	return parseEqualsOrTarget
}

// Consumed one bare string ot the beginning of the line.
func parseEqualsOrTarget(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenAssign:
		return parseAssignment

	case tokenWord:
		p.push(t)
		return parseTargets

	case tokenColon:
		p.push(t)
		return parseAttributesOrPrereqs

	default:
		p.parseError("reading a target or assignment",
			"'=', ':', or another target", t)
	}

	return parseTopLevel // unreachable
}

// Consumed 'foo='. Everything else is a value being assigned to foo.
func parseAssignment(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		err := p.rules.executeAssignment(p.tokenbuf)
		if err != nil {
			p.basicErrorAtToken(err.what, err.where)
		}
		p.clear()
		return parseTopLevel

	default:
		p.push(t)
	}

	return parseAssignment
}

// Everything up to ':' must be a target.
func parseTargets(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenWord:
		p.push(t)
	case tokenColon:
		p.push(t)
		return parseAttributesOrPrereqs

	default:
		p.parseError("reading a rule's targets",
			"filename or pattern", t)
	}

	return parseTargets
}

// Consume one or more strings followed by a first ':'.
func parseAttributesOrPrereqs(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		return parseRecipe
	case tokenColon:
		p.push(t)
		return parsePrereqs
	case tokenWord:
		p.push(t)
	default:
		p.parseError("reading a rule's attributes or prerequisites",
			"an attribute, pattern, or filename", t)
	}

	return parseAttributesOrPrereqs
}

// Targets and attributes and the second ':' have been consumed.
func parsePrereqs(p *parser, t token) parserStateFun {
	switch t.typ {
	case tokenNewline:
		return parseRecipe
	case tokenWord:
		p.push(t)

	default:
		p.parseError("reading a rule's prerequisites",
			"filename or pattern", t)
	}

	return parsePrereqs
}

// An entire rule has been consumed.
func parseRecipe(p *parser, t token) parserStateFun {
	// Assemble the rule!
	r := rule{}

	// find one or two colons
	i := 0
	for ; i < len(p.tokenbuf) && p.tokenbuf[i].typ != tokenColon; i++ {
	}
	j := i + 1
	for ; j < len(p.tokenbuf) && p.tokenbuf[j].typ != tokenColon; j++ {
	}

	// rule has attributes
	if j < len(p.tokenbuf) {
		var attribs []string
		for k := i + 1; k < j; k++ {
			exparts := expand(p.tokenbuf[k].val, p.rules.vars, true)
			attribs = append(attribs, exparts...)
		}
		err := r.parseAttribs(attribs)
		if err != nil {
			msg := fmt.Sprintf("while reading a rule's attributes expected an attribute but found \"%c\".", err.found)
			p.basicErrorAtToken(msg, p.tokenbuf[i+1])
		}

		// If we don't have a shell set, check vars, check default shell
		if r.shell == nil {
			if len(p.rules.vars["shell"]) > 0 {
				r.shell = p.rules.vars["shell"]
			} else {
				r.shell = []string{defaultShell}
			}
		}

		if r.attributes.regex {
			r.ismeta = true
		}
	} else {
		j = i
	}

	// targets
	// TODO: fact-check, required to be resetted?
	r.targets = r.targets[:0]
	for k := 0; k < i; k++ {
		exparts := expand(p.tokenbuf[k].val, p.rules.vars, true)
		for i := range exparts {
			targetstr := exparts[i]
			r.targets = append(r.targets, pattern{spat: targetstr})

			if r.attributes.regex {
				rpat, err := regexp.Compile("^" + targetstr + "$")
				if err != nil {
					msg := fmt.Sprintf("invalid regular expression: %q", err)
					p.basicErrorAtToken(msg, p.tokenbuf[k])
				}
				r.targets[len(r.targets)-1].rpat = rpat
			} else {
				idx := strings.IndexRune(targetstr, '%')
				if idx >= 0 {
					var left, right string
					if idx > 0 {
						left = regexp.QuoteMeta(targetstr[:idx])
					}
					if idx < len(targetstr)-1 {
						right = regexp.QuoteMeta(targetstr[idx+1:])
					}

					patstr := fmt.Sprintf("^%s(.*)%s$", left, right)
					rpat, err := regexp.Compile(patstr)
					if err != nil {
						msg := fmt.Sprintf("error compiling suffix rule. This is a bug. Error: %s", err)
						p.basicErrorAtToken(msg, p.tokenbuf[k])
					}
					r.targets[len(r.targets)-1].rpat = rpat
					r.targets[len(r.targets)-1].issuffix = true
					r.ismeta = true
				}
			}
		}
	}

	// prereqs
	// TODO: fact-check, required to be resetted?
	r.prereqs = r.prereqs[:0]
	for k := j + 1; k < len(p.tokenbuf); k++ {
		exparts := expand(p.tokenbuf[k].val, p.rules.vars, true)
		r.prereqs = append(r.prereqs, exparts...)
	}

	if t.typ == tokenRecipe {
		r.recipe = expandRecipeSigils(stripIndentation(t.val, t.col), p.rules.vars)
	}

	p.rules.add(r)
	p.clear()

	// the current token doesn't belong to this rule
	if t.typ != tokenRecipe {
		return parseTopLevel(p, t)
	}

	return parseTopLevel
}
