package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"unicode"
)

/*
   D      If the rule exits with a non-null status, the target is
          deleted.

   E      Continue execution if the rule draws errors.

   N      If there is no rule, the target has its time updated.

   n      The rule is a meta-rule that cannot be a target of a virtual
          rule.  Only files match the pattern in the target.

   P      The characters after the P until the terminating : are taken as
          a program name.  It will be invoked as sh -c prog 'arg1' 'arg2'
          and should return a zero exit status if and only if arg1 is up
          to date with respect to arg2.  Date stamps are still propagated
          in the normal way.

   Q      The rule is not printed prior to execution.

   R      The rule is a meta-rule using regular expressions.  In the rule,
          % has no special meaning.  The target is interpreted as a
          regular expression as defined in The prerequisites may contain
          references to subexpressions in form \n, as in the substitute
          command of

   U      The targets are considered to have been updated even if the
          rule did not do so.

   V      The targets of this rule are marked as virtual.  They are
          distinct from files of the same name.
*/

const Quotes = "\"'`"

type RuleAttr int

const (
	RuleDelete RuleAttr = 1 << iota
	RuleContinue
	RuleAlwaysUpdate
	RuleMeta
	RuleQuiet
	RuleRegex
	RuleNoValidate
	RuleVirtual
)

type Graph struct {
	vars  map[string]string
	rules []*Rule
	pos   struct {
		filename string
		linenr   int
	}
}

// findNextUnquoted finds the first unquoted character in 'chrs' in the input string.
func findNextUnquoted(text string, chrs string) (rune, int) {
	var inQuote rune
	for i, c := range text {
		switch {
		case inQuote == 0 && strings.ContainsRune(Quotes, c):
			inQuote = c
		case inQuote == c:
			inQuote = 0
		case inQuote == 0 && strings.ContainsRune(chrs, c):
			return c, i
		}
	}
	return 0, -1
}

// parseVar expands a variable reference starting from text[0], e.g., $FOO or ${FOO}.
func (p *Graph) parseExpr(text string, keep bool) (string, int) {
	if len(text) == 0 {
		return "$", 0
	}
	if text[0] == '{' {
		end := strings.IndexByte(text, '}')
		if end == -1 {
			return "$", 0
		}
		key, expr, _ := strings.Cut(text[1:end], ":")

		val, ok := p.vars[key]
		if !ok {
			if keep {
				return "", end + 1
			} else {
				return "$", 0
			}
		}
		if len(expr) > 0 {
			left, right, ok := strings.Cut(expr[1:], "%")
			if !ok {
				if keep {
					return "", end + 1
				} else {
					return "$", 0
				}
			}
			pre, post, _ := strings.Cut(left, "%")
			if strings.HasPrefix(val, pre) && strings.HasSuffix(val, post) {
				perc := val[len(pre) : len(val)-len(post)]
				val = strings.ReplaceAll(right, "%", perc)
			}
		}

		return val, end + 1
	}
	i := 0
	for _, c := range text {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
			break
		}
		i++
	}
	if i == 0 {
		return "$", 0
	}
	key := text[:i]
	val, ok := p.vars[key]
	if !ok {
		if keep {
			return "", i
		} else {
			return "$", 0
		}
	}
	return val, i
}

func (p *Graph) environ() []string {
	var res []string
	for k, v := range p.vars {
		res = append(res, fmt.Sprintf("%s=%s", k, v))
	}
	return res
}

// expand replaces $var or ${var} with their values from the parser.
func (p *Graph) expand(text string, keep bool) string {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return ""
	}
	var out strings.Builder
	if len(text) >= 2 && text[0] == '`' && text[len(text)-1] == '`' {
		text = text[1 : len(text)-1]

		cmd := exec.Command(*shell, "-c", text)
		cmd.Env = p.environ()
		cmd.Stdout = &out
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			panic(err)
		}
		return out.String()
	}

	for {
		idx := strings.IndexRune(text, '$')
		if idx == -1 {
			out.WriteString(text)
			break
		}
		out.WriteString(text[:idx])
		val, n := p.parseExpr(text[idx+1:], keep)
		out.WriteString(val)
		text = text[idx+1+n:]
	}
	return out.String()
}

func (r *Rule) parseAttributes(text string) (string, error) {
	for strings.ContainsRune(text, ':') {
		switch text[0] {
		case 'D':
			r.attrs |= RuleContinue
			text = text[1:]
		case 'E':
			r.attrs |= RuleDelete
			text = text[1:]
		case 'N':
			r.attrs |= RuleAlwaysUpdate
			text = text[1:]
		case 'n':
			r.attrs |= RuleMeta
			text = text[1:]
		case 'Q':
			r.attrs |= RuleQuiet
			text = text[1:]
		case 'R':
			r.attrs |= RuleRegex
			text = text[1:]
		case 'U':
			r.attrs |= RuleNoValidate
			text = text[1:]
		case 'V':
			r.attrs |= RuleVirtual
			text = text[1:]
		case 'P':
			end := strings.IndexByte(text, ':')
			r.program = text[:end]
			text = text[end:]
		case ':':
			text = text[1:]
		case ' ', '\t':
			return text, nil
		default:
			return "", fmt.Errorf("invalid rule-attribute '%c'", text[0])
		}
	}
	return text, nil
}

// parseRule adds or merges rules, or errors on ambiguity.
func (p *Graph) parseRule(line string, idx int, _ string) error {
	r := &Rule{}
	r.filename = p.pos.filename
	r.linenr = p.pos.linenr

	targetstr, prereqstr := line[:idx], line[idx+1:]

	prereqstr, err := r.parseAttributes(prereqstr)
	if err != nil {
		return err
	}

	targetstr = strings.TrimSpace(targetstr)
	prereqstr = strings.TrimSpace(prereqstr)

	for _, name := range strings.Fields(targetstr) {
		sname := p.expand(name, false)
		if sname == "" {
			continue
		}
		t, constant, err := CompileTarget(sname, r.attrs&RuleRegex > 0)
		if err != nil {
			return err
		}
		if !constant {
			r.attrs |= RuleMeta
		}
		r.targets = append(r.targets, t)
	}
	for _, name := range strings.Fields(prereqstr) {
		sname := p.expand(name, false)
		if sname == "" {
			continue
		}
		r.prereqs = append(r.prereqs, sname)
	}

	// Match existing rules for potential merge/override
rulescan:
	for i, existing := range p.rules {
		// Compare number and values of target patterns (string match)
		if len(r.targets) != len(existing.targets) {
			continue
		}
		for j := range r.targets {
			if r.targets[j].pat != existing.targets[j].pat {
				continue rulescan
			}
		}

		// Match found: apply Plan 9 rule semantics
		hasRecipeA := strings.TrimSpace(existing.recipe) != ""
		hasRecipeB := strings.TrimSpace(r.recipe) != ""
		samePrereqs := slices.Equal(existing.prereqs, r.prereqs)

		switch {
		case !hasRecipeA && hasRecipeB:
			// Append new prereqs to the existing rule
			existing.prereqs = append(existing.prereqs, r.prereqs...)
			return nil
		case hasRecipeA && hasRecipeB && !samePrereqs:
			return fmt.Errorf("%s:%d: ambiguous recipe for target `%s` with differing prerequisites",
				r.filename, r.linenr, r.targets[0].pat)
		case samePrereqs:
			// Override rule
			p.rules[i] = r
			return nil
		}
	}

	// No match found, add as new rule
	p.rules = append(p.rules, r)
	return nil
}

func (p *Graph) parseVar(line string, idx int, _ string) error {
	name := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])
	p.vars[p.expand(name, false)] = p.expand(value, false)
	return nil
}

func (p *Graph) parseInclude(line string, idx int, dir string) error {
	for _, c := range line[:idx] {
		if !unicode.IsSpace(c) {
			return fmt.Errorf("garbage before include: %s", strings.TrimSpace(line[:idx]))
		}
	}

	line = line[idx+1:]

	if len(line) > 0 && line[0] == '|' {
		cmd := exec.Command(*shell, "-c", line[1:])
		cmd.Stderr = os.Stderr
		output, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		p.parseFile(output, dir, "<command>")
		return nil
	}
	name := strings.TrimSpace(line)
	name = p.expand(name, false)
	if !strings.HasPrefix(name, "/") {
		name = path.Join(dir, name)
	}
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	return p.parseFile(file, path.Dir(name), name)
}

// parseLine parses a single line of input.
func (p *Graph) parseLine(line string, dir string) error {
	// remove comments
	if _, idx := findNextUnquoted(line, "#"); idx != -1 {
		line = line[:idx]
	}

	if strings.TrimSpace(line) == "" {
		return nil
	}

	if len(p.rules) > 0 && (line[0] == ' ' || line[0] == '\t') {
		r := p.rules[len(p.rules)-1]
		if len(r.recipe) > 0 {
			r.recipe += "\n"
		}
		r.recipe += line[1:]
		return nil
	}

	ch, idx := findNextUnquoted(line, ":<=")
	switch ch {
	case ':':
		return p.parseRule(line, idx, dir)
	case '=':
		return p.parseVar(line, idx, dir)
	case '<':
		return p.parseInclude(line, idx, dir)
	default:
		return fmt.Errorf("syntax error; expected one of <:=")
	}
}

// parse reads and parses lines from the given reader.
func (p *Graph) parseFile(r io.Reader, dir string, filename string) error {
	scanner := bufio.NewScanner(r)
	var buf strings.Builder

	p.pos.filename = filename
	p.pos.linenr = 0
	for scanner.Scan() {
		p.pos.linenr++
		line := scanner.Text()
		if strings.HasSuffix(line, "\\") {
			buf.WriteString(line[:len(line)-1])
			continue
		}
		buf.WriteString(line)
		if err := p.parseLine(buf.String(), dir); err != nil {
			return fmt.Errorf("%s:%d: %v", filename, p.pos.linenr, err)
		}
		buf.Reset()
	}
	return scanner.Err()
}
