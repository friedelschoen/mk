package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"
)

type Target struct {
	pat      string
	match    func(string) (string, []string, bool)
	constant bool
}

func CompileTarget(pat string, isregex bool) (Target, bool, error) {
	t := Target{pat: pat}
	if isregex {
		matcher, err := regexp.Compile(pat)
		if err != nil {
			return t, false, fmt.Errorf("invalid regular expression: `%s`: %w", pat, err)
		}

		t.match = func(input string) (string, []string, bool) {
			m := matcher.FindStringSubmatch(input)
			return "", m, m != nil
		}

		return t, false, nil
	}

	pre, post, ok := strings.Cut(pat, "%")
	if ok {
		prelen, postlen := len(pre), len(post)
		t.match = func(input string) (string, []string, bool) {
			if strings.HasPrefix(input, pre) && strings.HasSuffix(input, post) {
				return input[prelen : len(input)-postlen], nil, true
			}
			return "", nil, false
		}
		return t, false, nil
	}

	pre, post, ok = strings.Cut(pat, "&")
	if ok {
		prelen, postlen := len(pre), len(post)
		t.match = func(input string) (string, []string, bool) {
			if strings.HasPrefix(input, pre) && strings.HasSuffix(input, post) {
				return input[prelen : len(input)-postlen], nil, true
			}
			return "", nil, false
		}
		return t, false, nil
	}

	t.constant = true
	t.match = func(input string) (string, []string, bool) {
		return "", nil, input == pat
	}
	return t, true, nil

}

type Rule struct {
	filename string
	linenr   int
	attrs    RuleAttr
	program  string
	targets  []Target
	prereqs  []string
	recipe   string
}

func (g *Graph) DefaultTarget() *Rule {
	for _, r := range g.rules {
		if r.attrs&RuleMeta == 0 {
			return r
		}
	}
	return nil
}

func (g *Graph) FindRule(target string) (*Rule, string, []string) {
	for _, r := range g.rules {
		for _, t := range r.targets {
			stem, subm, ok := t.match(target)
			if ok {
				return r, stem, subm
			}
		}
	}
	return nil, "", nil
}

func (g *Graph) Build(history []string, target string) error {
	r, stem, subm := g.FindRule(target)
	if r != nil {
		return g.BuildRule(r, history, target, stem, subm)
	}
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	return fmt.Errorf("don't know how to make `%s`", target)
}

func isOutdated(target string, prereqs []string) bool {
	tstat, err := os.Stat(target)
	if err != nil {
		// Target doesn't exist — must rebuild
		return true
	}
	ttime := tstat.ModTime()
	for _, p := range prereqs {
		pstat, err := os.Stat(p)
		if err != nil {
			// Missing prereq — assume outdated
			return true
		}
		if pstat.ModTime().After(ttime) {
			return true // Prereq is newer
		}
	}
	return false
}

func (g *Graph) BuildRule(r *Rule, history []string, target string, stem string, subm []string) error {
	if slices.Contains(history, target) {
		history = append(history, target)
		return fmt.Errorf("circular dependency: %s", strings.Join(history, "->"))
	}
	history = append(history, target)
	var prereqs []string
	var errs []error
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, in := range r.prereqs {
		if stem != "" {
			in = strings.ReplaceAll(in, "%", stem)
		}
		for i, s := range subm {
			in = strings.ReplaceAll(in, fmt.Sprintf("\\%d", i), s)
		}

		prereqs = append(prereqs, in)

		wg.Add(1)
		go func() {
			err := g.Build(history, in)

			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
			wg.Done()
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if !isOutdated(target, prereqs) {
		return nil
	}

	if len(r.recipe) == 0 {
		return nil
	}

	vars := maps.Clone(g.vars)
	vars["target"] = target
	vars["prereq"] = strings.Join(prereqs, " ")
	if stem != "" {
		vars["stem"] = stem
	}
	for i, s := range subm {
		vars[fmt.Sprintf("subm%d", i)] = s
	}
	var env []string
	for k, v := range vars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	fmt.Println(strings.TrimSpace(r.recipe))
	cmd := exec.Command(*shell, "-c", r.recipe)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s:%d: unable to build: %w", r.filename, r.linenr, err)
	}
	return nil
}
