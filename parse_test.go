package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func ruleAttributesNotSet(t *testing.T, r *rule) {
	noAttributes := attribSet{
		delFailed:       false,
		nonstop:         false,
		forcedTimestamp: false,
		nonvirtual:      false,
		quiet:           false,
		regex:           false,
		update:          false,
		virtual:         false,
		exclusive:       false,
	}
	if r.attributes != noAttributes {
		t.Error("rule attributes are not all false", r.attributes)
	}
}

// Test a mkfile with a single rule. The target has a single
// prerequesite; both are local files.
func TestParseOneRuleLocalFiles(t *testing.T) {
	mkfileAsString := "somefile.txt: a_prereq.csv\n\techo $target"
	env := make(map[string][]string)
	ruleSet := parse(strings.NewReader(mkfileAsString), "mkfile", "/mkfile", env)
	if len(ruleSet.rules) != 1 {
		t.Errorf("There should be 1 rule")
	}
	rule := ruleSet.rules[0]
	if len(rule.prereqs) != 1 {
		t.Errorf("There should be 1 prerequesite")
	}
	if rule.prereqs[0] != "a_prereq.csv" {
		t.Errorf("The prerequesites are not a_prereq.csv")
	}

	ruleAttributesNotSet(t, &rule)
}

// Test a mkfile with a single rule. The target has a two
// prerequesite; both are local files.
func TestParseOneRuleMultiPrereqLocalFiles(t *testing.T) {
	mkfileAsString := "somefile.txt: a_prereq.csv b_prereq.tsv\n\techo $target"
	env := make(map[string][]string)
	ruleSet := parse(strings.NewReader(mkfileAsString), "mkfile", "/mkfile", env)
	if len(ruleSet.rules) != 1 {
		t.Errorf("There should be 1 rule")
	}
	rule := ruleSet.rules[0]
	if len(rule.prereqs) != 2 {
		t.Errorf("There should be 2 prerequesite")
	}
	if rule.prereqs[0] != "a_prereq.csv" || rule.prereqs[1] != "b_prereq.tsv" {
		t.Errorf("The prerequesites are not 'a_prereq.csv b_prereq.tsv'")
	}

	ruleAttributesNotSet(t, &rule)
}

// Test a mkfile with a single rule. The target has a single
// prerequesite; both are local files. The rule has attributes
// set. Possible attributes are D, E, N, n, Q, R, U, V, X
func TestParseOneRuleWithAttributeLocalFiles(t *testing.T) {
	var attribMap = map[string]string{
		"D": "delFailed",
		"E": "nonstop",
		"n": "nonvirtual",
		"N": "forcedTimestamp",
		"Q": "quiet",
		"R": "regex",
		"U": "update",
		"V": "virtual",
		"X": "exclusive",
	}

	for k, v := range attribMap {
		mkfileAsString := fmt.Sprintf("somefile.txt:%s: a_prereq.csv\n\techo $target", k)
		env := make(map[string][]string)
		ruleSet := parse(strings.NewReader(mkfileAsString), "mkfile", "/mkfile", env)
		if len(ruleSet.rules) != 1 {
			t.Errorf("There should be 1 rule")
		}
		rule := ruleSet.rules[0]
		if len(rule.prereqs) != 1 {
			t.Errorf("There should be 1 prerequesite")
		}
		if rule.prereqs[0] != "a_prereq.csv" {
			t.Errorf("The prerequesites are not a_prereq.csv")
		}
		//t.Log(rule.attributes)
		ps := reflect.ValueOf(rule.attributes)
		f := ps.FieldByName(v)

		if f.Bool() != true {
			t.Errorf("The %s attribute has not been set", k)
		}
	}
}

// Test a mkfile with a single rule. The target has a single
// prerequesite; both are local files. Uses an alternative
// method to determine if the target is up to date.
func TestParseOneRuleAltCompareLocalFiles(t *testing.T) {
	mkfileAsString := "somefile.txt:Pcmp -s: a_prereq.csv\n\techo $target"
	env := make(map[string][]string)
	ruleSet := parse(strings.NewReader(mkfileAsString), "mkfile", "/mkfile", env)
	if len(ruleSet.rules) != 1 {
		t.Errorf("There should be 1 rule")
	}
	rule := ruleSet.rules[0]
	if len(rule.prereqs) != 1 {
		t.Errorf("There should be 1 prerequesite")
	}
	if rule.prereqs[0] != "a_prereq.csv" {
		t.Errorf("The prerequesites are not a_prereq.csv")
	}
	if rule.command[0] != "cmp" || rule.command[1] != "-s" {
		t.Error("The rule's comparison command ", rule.command, " is not cmp -s")
	}
	ruleAttributesNotSet(t, &rule)
}

// Test a mkfile with a single rule. The target has a single
// prerequesite; both are local files. Uses an alternative shell
// to run the recipie
func TestParseOneRuleAltShellLocalFiles(t *testing.T) {
	mkfileAsString := "somefile.txt:Scmp -s: a_prereq.csv\n\techo $target"
	env := make(map[string][]string)
	ruleSet := parse(strings.NewReader(mkfileAsString), "mkfile", "/mkfile", env)
	if len(ruleSet.rules) != 1 {
		t.Errorf("There should be 1 rule")
	}
	rule := ruleSet.rules[0]
	if len(rule.prereqs) != 1 {
		t.Errorf("There should be 1 prerequesite")
	}
	if rule.prereqs[0] != "a_prereq.csv" {
		t.Errorf("The prerequesites are not a_prereq.csv")
	}
	if rule.shell[0] != "cmp" || rule.shell[1] != "-s" {
		t.Error("The rule's comparison command ", rule.shell, " is not cmp -s")
	}
	ruleAttributesNotSet(t, &rule)
}

// Test a mkfile with a single rule. The target has a single
// prerequesite; both are local files. Tries to set both an
// alternative shell and an alternative comparion - this will
// fail as S and P attributes are mutually exclusive
// func TestParseOneRuleAltShellAltCmpLocalFiles(t *testing.T) {
// 	mkfileAsString := "somefile.txt:Scmp -s Pcmp -s: a_prereq.csv\n\techo $target"
// 	ruleSet := parse(strings.NewReader(mkfileAsString), "mkfile", "/mkfile")
// 	if len(ruleSet.rules) != 1 {
// 		t.Errorf("There should be 1 rule")
// 	}
// 	rule := ruleSet.rules[0]
// 	if len(rule.prereqs) != 1 {
// 		t.Errorf("There should be 1 prerequesite")
// 	}
// 	if rule.prereqs[0] != "a_prereq.csv" {
// 		t.Errorf("The prerequesites are not a_prereq.csv")
// 	}
// 	if len(rule.shell) != 2 {
// 		t.Error("there are extra elements in the shell", rule.shell)
// 	}
// 	if rule.shell[0] != "cmp" || rule.shell[1] != "-s" {
// 		t.Error("The rule's comparison command ", rule.shell, " is not cmp -s")
// 	}
// 	ruleAttributesNotSet(t, &rule)
// }

// Test a mkfile with a single rule. The target has a single
// prerequesite; both are local files.
func TestParseOneRuleHTTPPrereq(t *testing.T) {
	mkfileAsString := "somefile.txt: \"http://golang.org/a_prereq.csv\"\n\techo $target"
	env := make(map[string][]string)
	ruleSet := parse(strings.NewReader(mkfileAsString), "mkfile", "/mkfile", env)
	if len(ruleSet.rules) != 1 {
		t.Errorf("There should be 1 rule")
	}
	rule := ruleSet.rules[0]
	if len(rule.prereqs) != 1 {
		t.Errorf("There should be 1 prerequesite")
	}
	if rule.prereqs[0] != "http://golang.org/a_prereq.csv" {
		t.Errorf("The prerequesites are not http://golang.org/a_prereq.csv")
	}

	ruleAttributesNotSet(t, &rule)
}

// Make sure that we can parse assignments that are across multiple lines
// like:
//
//	OFILES = a.o\
//	         b.o
//	         c.o
//
// prog: $OFILES
//
//	cc -o $target $prereqs
func TestParseAssignmentNewLine(t *testing.T) {
	mkfileAsString := "OFILES=9p1.o\\\n9p1lib.o\nprog: $OFILES\n\techo $target"

	env := make(map[string][]string)
	ruleSet := parse(strings.NewReader(mkfileAsString), "mkfile", "/mkfile", env)
	if len(ruleSet.rules) != 1 {
		t.Errorf("There should be 1 rule")
	}
	rule := ruleSet.rules[0]
	t.Log(rule.prereqs)
	if len(rule.prereqs) != 2 {
		t.Errorf("There should be 2 prerequesites")
	}
}

func TestParseLocalRegexPrereq(t *testing.T) {
	mkfileAsString := "data/processed/(\\d+)/mapping_k10.bam.bai:R: \"/runs/contition_${stem1}_bowtie_k10/mapping.bam.bai\"\n\techo $prereq $target"
	env := make(map[string][]string)
	ruleSet := parse(strings.NewReader(mkfileAsString), "mkfile", "/mkfile", env)
	if len(ruleSet.rules) != 1 {
		t.Errorf("There should be 1 rule")
	}
	rule := ruleSet.rules[0]
	if !rule.attributes.regex || !rule.ismeta {
		t.Error("The rule has not been recognized as a regex")
	}
	if !(rule.prereqs[0] == "/runs/contition_${stem1}_bowtie_k10/mapping.bam.bai") {
		t.Error("The rule does not have the right prerequisite")
	}
}

func TestParseS3Prereq(t *testing.T) {
	mkfileAsString := "data/processed/(\\d+)/mapping_k10.bam.bai:R: \"s3://runs/contition_${stem1}_bowtie_k10/mapping.bam.bai\"\n\taws s3 cp $prereq $target"
	env := make(map[string][]string)
	ruleSet := parse(strings.NewReader(mkfileAsString), "mkfile", "/mkfile", env)
	if len(ruleSet.rules) != 1 {
		t.Errorf("There should be 1 rule")
	}
	rule := ruleSet.rules[0]
	if !rule.attributes.regex || !rule.ismeta {
		t.Error("The rule has not been recognized as a regex")
	}
	if !(rule.prereqs[0] == "s3://runs/contition_${stem1}_bowtie_k10/mapping.bam.bai") {
		t.Error("The rule does not have the right prerequisite")
	}
}
