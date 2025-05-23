// Mkfiles are parsed into ruleSets, which as the name suggests, are sets of
// rules with accompanying recipes, as well as assigned variables which are
// expanding when evaluating rules and recipes.

package main

import (
	"fmt"
	"regexp"
	"unicode"
	"unicode/utf8"
)

type attribSet struct {
	delFailed       bool // delete targets when the recipe fails
	nonstop         bool // don't stop if the recipe fails
	forcedTimestamp bool // update timestamp whether the recipe does or not
	nonvirtual      bool // a meta-rule that will only match files
	quiet           bool // don't print the recipe
	regex           bool // regular expression meta-rule
	update          bool // treat the targets as if they were updated
	virtual         bool // rule is virtual (does not match files)
	exclusive       bool // don't execute concurrently with any other rule
}

// Error parsing an attribute
type attribError struct {
	found rune
}

// target and rereq patterns
type pattern struct {
	issuffix bool           // is a suffix '%' rule, so we should define $stem.
	spat     string         // simple string pattern
	rpat     *regexp.Regexp // non-nil if this is a regexp pattern
}

// Match a pattern, returning an array of submatches,
// or nil if it doesn't match.
func (p *pattern) match(target string) []string {
	if p.rpat != nil {
		return p.rpat.FindStringSubmatch(target)
	}

	if target == p.spat {
		return make([]string, 0)
	}

	return nil
}

// A single rule.
type rule struct {
	targets    []pattern // non-empty array of targets
	attributes attribSet // rule attributes
	prereqs    []string  // possibly empty prerequesites
	shell      []string  // command used to execute the recipe
	recipe     string    // recipe source
	command    []string  // command attribute
	ismeta     bool      // is this a meta rule
	file       string    // file where the rule is defined
	line       int       // line number on which the rule is defined
}

// Equivalent recipes.
func (r *rule) equivRecipe(r2 *rule) bool {
	if r.recipe != r2.recipe {
		return false
	}

	if len(r.shell) != len(r2.shell) {
		return false
	}

	for i := range r.shell {
		if r.shell[i] != r2.shell[i] {
			return false
		}
	}

	return true
}

// A set of rules.
type ruleSet struct {
	vars  map[string][]string
	rules []rule
	// map a target to an array of indexes into rules
	targetrules map[string][]int
}

// Read attributes for an array of strings, updating the rule.
func (r *rule) parseAttribs(inputs []string) *attribError {
	for i, input := range inputs {
		for pos, c := range input {
			w := utf8.RuneLen(c)
			switch c {
			case 'D':
				r.attributes.delFailed = true
			case 'E':
				r.attributes.nonstop = true
			case 'N':
				r.attributes.forcedTimestamp = true
			case 'n':
				r.attributes.nonvirtual = true
			case 'Q':
				r.attributes.quiet = true
			case 'R':
				r.attributes.regex = true
			case 'U':
				r.attributes.update = true
			case 'V':
				r.attributes.virtual = true
			case 'X':
				r.attributes.exclusive = true
			case 'P':
				if pos+w < len(input) {
					r.command = append(r.command, input[pos+w:])
				}
				r.command = append(r.command, inputs[i+1:]...)
				return nil

			case 'S':
				if pos+w < len(input) {
					r.shell = append(r.shell, input[pos+w:])
				}
				r.shell = append(r.shell, inputs[i+1:]...)
				return nil

			default:
				return &attribError{c}
			}
		}
	}

	return nil
}

// Add a rule to the rule set.
func (rs *ruleSet) add(r rule) {
	rs.rules = append(rs.rules, r)
	k := len(rs.rules) - 1
	for i := range r.targets {
		if r.targets[i].rpat == nil {
			rs.targetrules[r.targets[i].spat] =
				append(rs.targetrules[r.targets[i].spat], k)
		}
	}
}

func isValidVarName(v string) bool {
	for i, c := range v {
		if i == 0 && !(unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_') {
			return false
		} else if !(unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_') {
			return false
		}
	}
	return true
}

type assignmentError struct {
	what  string
	where token
}

// Parse and execute assignment operation.
func (rs *ruleSet) executeAssignment(ts []token) *assignmentError {
	assignee := ts[0].val
	if !isValidVarName(assignee) {
		return &assignmentError{
			fmt.Sprintf("target of assignment is not a valid variable name: \"%s\"", assignee),
			ts[0]}
	}

	// interpret tokens in assignment context
	var input []string
	for i := 1; i < len(ts); i++ {
		if ts[i].typ != tokenWord || (i > 1 && ts[i-1].typ != tokenWord) {
			if len(input) == 0 {
				input = append(input, ts[i].val)
			} else {
				input[len(input)-1] += ts[i].val
			}
		} else {
			input = append(input, ts[i].val)
		}
	}

	// expanded variables
	var vals []string
	for _, str := range input {
		vals = append(vals, expand(str, rs.vars, true)...)
	}

	rs.vars[assignee] = vals

	return nil
}
