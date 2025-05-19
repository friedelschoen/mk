// Various function for dealing with recipes.

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

// Try to unindent a recipe, so that it begins an column 0. (This is mainly for
// recipes in python, or other indentation-significant languages.)
func stripIndentation(s string, mincol int) string {
	// trim leading whitespace
	reader := bufio.NewReader(strings.NewReader(s))
	var output strings.Builder
	for {
		line, err := reader.ReadString('\n')
		col := 0
		for _, c := range line {
			if col >= mincol {
				break
			}
			if !unicode.IsSpace(c) {
				break
			}
			col++
		}
		output.WriteString(line[col:])
		if err != nil {
			break
		}
	}

	return output.String()
}

// Indent each line of a recipe.
func printIndented(out io.Writer, s string, ind int) {
	indentation := strings.Repeat(" ", ind)
	reader := bufio.NewReader(strings.NewReader(s))
	firstline := true
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if !firstline {
				io.WriteString(out, indentation)
			}
			io.WriteString(out, line)
		}
		if err != nil {
			break
		}
		firstline = false
	}
}

// Execute a recipe.
func dorecipe(target string, u *node, e *edge, dryrun bool) bool {
	vars := make(map[string][]string)
	vars["target"] = []string{target}
	if e.r.ismeta {
		if e.r.attributes.regex {
			for i := range e.matches {
				vars[fmt.Sprintf("stem%d", i)] = e.matches[i : i+1]
			}
		} else {
			vars["stem"] = []string{e.stem}
		}
	}

	// TODO: other variables to set
	// alltargets
	// newprereq

	var prereqs []string
	for i := range u.prereqs {
		if u.prereqs[i].r == e.r && u.prereqs[i].v != nil {
			prereqs = append(prereqs, u.prereqs[i].v.name)
			vars[fmt.Sprintf("prereq%d", i+1)] = []string{u.prereqs[i].v.name}
		}
	}
	vars["prereq"] = prereqs

	// Setup the shell in vars.
	sh, args := expandShell(defaultShell, []string{})
	if len(e.r.shell) > 0 {
		sh, args = expandShell(e.r.shell[0], e.r.shell[1:])
	}
	vars["shell"] = append([]string{sh}, args...)

	// Build the command.
	input := expandRecipeSigils(e.r.recipe, vars)

	mkPrintRecipe(target, input, e.r.attributes.quiet)
	if dryrun {
		return true
	}

	// Merge and construct the execution environment for this recipe.
	for k, v := range GlobalMkState {
		if _, ok := vars[k]; !ok {
			vars[k] = v
		}
	}
	// "\x01" is a magic constant that Plan9 rc uses to separate elements in an array.
	// TODO(rjk): Do the right thing for other shells that have arrays.
	env := os.Environ()
	for k, v := range vars {
		env = append(env, k+"="+strings.Join(v, "\x01"))
	}

	cmd := exec.Command(sh, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		//fmt.Fprintf(os.Stderr, "command failed: %v\n", err)
		return false
	}

	return true
}
