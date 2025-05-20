// String substitution and expansion.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Expand a word. This includes substituting variables and handling quotes.
func expand(input string, vars map[string][]string, expandBackticks bool) []string {
	var parts []string
	var expanded strings.Builder
	for len(input) > 0 {
		j := strings.IndexAny(input, "\"'`$\\")

		if j < 0 {
			expanded.WriteString(input)
			break
		}

		expanded.WriteString(input[:j])
		c := input[j]
		input = input[j+1:]

		var off int
		var out string
		switch c {
		case '\\':
			out, off = expandEscape(input)
			expanded.WriteString(out)

		case '"':
			out, off = expandDoubleQuoted(input, vars, expandBackticks)
			expanded.WriteString(out)

		case '\'':
			out, off = expandSingleQuoted(input)
			expanded.WriteString(out)

		case '`':
			if expandBackticks {
				var outparts []string
				outparts, off = expandBackQuoted(input, vars)
				if len(outparts) > 0 {
					outparts[0] = expanded.String() + outparts[0]
					expanded.Reset()
					expanded.WriteString(outparts[len(outparts)-1])
					parts = append(parts, outparts[:len(outparts)-1]...)
				}
			} else {
				out = input
				off = len(input)
				expanded.WriteString(out)
			}
		case '$':
			var outparts []string
			outparts, off = expandSigil(input, vars)
			if len(outparts) > 0 {
				firstpart := expanded.String() + outparts[0]
				if len(outparts) > 1 {
					parts = append(parts, firstpart)
					if len(outparts) > 2 {
						parts = append(parts, outparts[1:len(outparts)-1]...)
					}
					expanded.Reset()
					expanded.WriteString(outparts[len(outparts)-1])
				} else {
					expanded.Reset()
					expanded.WriteString(firstpart)
				}
			}
		}

		input = input[off:]
	}

	if expanded.Len() > 0 {
		parts = append(parts, expanded.String())
	}

	return parts
}

// Expand following a '\\'
func expandEscape(input string) (string, int) {
	c, w := utf8.DecodeRuneInString(input)
	if c == '\t' || c == ' ' {
		return string(c), w
	}
	if c == '\n' {
		return "", w
	}
	return "\\" + string(c), w
}

// Expand a double quoted string starting after a '\"'
func expandDoubleQuoted(input string, vars map[string][]string, expandBackticks bool) (string, int) {
	// find the first non-escaped "
	i := 0
	j := 0
	for {
		j = strings.IndexAny(input[i:], "\"\\")
		if j < 0 {
			break
		}
		j += i

		c, w := utf8.DecodeRuneInString(input[j:])
		i = j + w

		if c == '"' {
			return strings.Join(expand(input[:j], vars, expandBackticks), " "), i
		}

		if c == '\\' {
			if i < len(input) {
				_, w := utf8.DecodeRuneInString(input[i:])
				i += w
			} else {
				break
			}
		}
	}

	return input, len(input)
}

// Expand a single quoted string starting after a '\”
func expandSingleQuoted(input string) (string, int) {
	j := strings.Index(input, "'")
	if j < 0 {
		return input, len(input)
	}
	return input[:j], (j + 1)
}

var expandSigil_namelist_pattern = regexp.MustCompile(`^\s*([^:]+)\s*:\s*([^%]*)%([^=]*)\s*=\s*([^%]*)%([^%]*)\s*`)

// Expand something starting with at '$'.
func expandSigil(input string, vars map[string][]string) ([]string, int) {
	c, w := utf8.DecodeRuneInString(input)
	var offset int
	var varname string
	namelist_pattern := expandSigil_namelist_pattern

	if c == '$' { // escaping of "$" with "$$"
		return []string{"$"}, 2
	} else if c == '{' { // match bracketed expansions: ${foo}, or ${foo:a%b=c%d}
		j := strings.IndexRune(input[w:], '}')
		if j < 0 {
			return []string{"$" + input}, len(input)
		}
		varname = input[w : w+j]
		offset = w + j + 1

		// is this a namelist?
		mat := namelist_pattern.FindStringSubmatch(varname)
		if mat != nil && isValidVarName(mat[1]) {
			// ${varname:a%b=c%d}
			varname = mat[1]
			a, b, c, d := mat[2], mat[3], mat[4], mat[5]
			values, ok := vars[varname]
			if !ok {
				return []string{}, offset
			}

			pat := regexp.MustCompile(strings.Join([]string{`^\Q`, a, `\E(.*)\Q`, b, `\E$`}, ""))
			expanded_values := make([]string, 0, len(values))
			for _, value := range values {
				value_match := pat.FindStringSubmatch(value)
				if value_match != nil {
					expanded_values = append(expanded_values, expand(strings.Join([]string{c, value_match[1], d}, ""), vars, false)...)
				} else {
					// What case is this?
					expanded_values = append(expanded_values, value)
				}
			}

			return expanded_values, offset
		}
	} else { // bare variables: $foo
		// try to match a variable name
		i := 0
		j := i
		for j < len(input) {
			c, w = utf8.DecodeRuneInString(input[j:])
			if !(unicode.IsLetter(c) || c == '_' || (j > i && unicode.IsDigit(c))) {
				break
			}
			j += w
		}

		if j > i {
			varname = input[i:j]
			offset = j
		} else {
			offset = j + 1
			return []string{"$" + input[:offset]}, offset
			//return []string{"$" + input}, len(input)
		}
	}

	if isValidVarName(varname) {
		varvals, ok := vars[varname]
		if ok {
			return varvals, offset
		}

		// Find the subsitution in the environment.
		if varval, ok := os.LookupEnv(varname); ok {
			return []string{varval}, offset
		}

		return []string{"$" + input[:offset]}, offset
	}

	// Find the subsitution in the environment.
	if varval, ok := os.LookupEnv(varname); ok {
		return []string{varval}, offset
	}

	return []string{"$" + input}, len(input)
}

// Find and expand all sigils in a recipe, producing a flat string.
func expandRecipeSigils(input string, vars map[string][]string) string {
	var expanded strings.Builder
	for len(input) > 0 {
		off := strings.IndexAny(input, "$\\")
		if off < 0 {
			expanded.WriteString(input)
			break
		}
		expanded.WriteString(input[:off])
		input = input[off:]

		c, w := utf8.DecodeRuneInString(input)
		if c == '$' {
			input = input[w:]
			ex, k := expandSigil(input, vars)
			for n, s := range ex {
				if n > 0 {
					expanded.WriteByte(' ')
				}
				expanded.WriteString(s)
			}
			input = input[k:]
		} else if c == '\\' {
			input = input[w:]
			c, w := utf8.DecodeRuneInString(input)
			if c == '$' {
				expanded.WriteByte('$')
			} else {
				expanded.WriteByte('\\')
				expanded.WriteRune(c)
			}
			input = input[w:]
		}
	}

	return expanded.String()
}

// Expand all unescaped '%' characters.
func expandSuffixes(input string, stem string) string {
	var expanded []byte
	for i := 0; i < len(input); {
		j := strings.IndexAny(input[i:], "\\%")
		if j < 0 {
			expanded = append(expanded, input[i:]...)
			break
		}

		c, w := utf8.DecodeRuneInString(input[j:])
		expanded = append(expanded, input[i:j]...)
		if c == '%' {
			expanded = append(expanded, stem...)
			i = j + w
		} else {
			j += w
			c, w := utf8.DecodeRuneInString(input[j:])
			if c == '%' {
				expanded = append(expanded, '%')
				i = j + w
			}
		}
	}

	return string(expanded)
}

// Expand a backtick quoted string, by executing the contents.
func expandBackQuoted(input string, vars map[string][]string) ([]string, int) {
	// TODO: expand sigils?
	j := strings.Index(input, "`")
	if j < 0 {
		return []string{input}, len(input)
	}

	env := os.Environ()
	for key, values := range vars {
		env = append(env, key+"="+strings.Join(values, " "))
	}

	// TODO - might have $shell available by now, but maybe not?
	// It's not populated, regardless

	var shell string
	var shellargs []string
	if len(vars["shell"]) < 1 {
		shell, shellargs = expandShell(defaultShell, shellargs)
	} else {
		shell, shellargs = expandShell(vars["shell"][0], shellargs)
	}

	cmd := exec.Command(shell, shellargs...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input[:j])
	cmd.Stderr = os.Stderr
	output, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create pipe: %v", err)
		return nil, 0
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "unable to start process: %v", err)
		return nil, 0
	}

	var parts []string
	tokens := lex(output, true)
	for {
		t, ok := tokens.nextToken()
		if !ok {
			break
		}
		parts = append(parts, t.val)
	}

	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "unable to wait for process: %v", err)
		return nil, 0
	}

	return parts, (j + 1)
}

// Expand the shell command into cmd, args...
// Ex. "sh -c", "pwd" becomes sh, [-c, pwd]
func expandShell(shcmd string, args []string) (string, []string) {
	var shell string
	var shellargs []string

	fields := strings.Fields(shcmd)
	shell = fields[0]

	if len(fields) > 1 {
		shellargs = fields[1:]
	}

	switch {
	// TODO - This case logic might be shaky, works for now
	case len(shellargs) > 0 && len(args) > 0:
		args = append(shellargs, args...)

	case len(shellargs) > 0 && dontDropArgs:
		args = append(shellargs, args...)

	default:
		//fmt.Println("dropping in expand!")
	}

	if len(shellargs) > 0 && dontDropArgs {

	} else {

	}

	return shell, args
}
