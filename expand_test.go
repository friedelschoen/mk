package main

import (
	"reflect"
	"testing"

	"github.com/sanity-io/litter"
)

type expandtv struct {
	input       string
	vars        map[string][]string
	expandticks bool
	want        []string
}

func TestExpand(t *testing.T) {

	tests := []expandtv{
		{
			input:       "a",
			vars:        map[string][]string{},
			expandticks: false,
			want:        []string{"a"},
		},
		{
			input: "a",
			vars: map[string][]string{
				"a": {"glenda"},
			},
			expandticks: false,
			want:        []string{"a"},
		},
		{
			input: "$a",
			vars: map[string][]string{
				"a": {"glenda"},
			},
			expandticks: false,
			want:        []string{"glenda"},
		},
		{
			input: "${a}",
			vars: map[string][]string{
				"a": {"glenda"},
			},
			expandticks: false,
			want:        []string{"glenda"},
		},
		{
			input: "${a}",
			vars: map[string][]string{
				"a": {"glenda", "gopher"},
			},
			expandticks: false,
			want:        []string{"glenda", "gopher"},
		},
		{
			input: "ab$targetpath",
			vars: map[string][]string{
				"targetpath": {"./testdata"},
			},
			expandticks: false,
			want:        []string{"ab./testdata"},
		},
		{
			input: "$targetpathab",
			vars: map[string][]string{
				"targetpath": {"./testdata"},
			},
			expandticks: false,
			want:        []string{"$targetpathab"},
		},
		{
			input: "$targetpath/foo",
			vars: map[string][]string{
				"targetpath": {"./testdata"},
			},
			expandticks: false,
			want:        []string{"./testdata/foo"},
		},
		{
			input: "${targetpath}/foo",
			vars: map[string][]string{
				"targetpath": {"./testdata"},
			},
			expandticks: false,
			want:        []string{"./testdata/foo"},
		},
		//This one differs between p9p mk and mk.
		//Do we want this difference?
		{
			input: "\"$targetpath\"",
			vars: map[string][]string{
				"targetpath": {"./testdata"},
			},
			expandticks: false,
			want:        []string{"./testdata"},
		},
		{
			input: "\"s3://$targetpath\"",
			vars: map[string][]string{
				"targetpath": {"testdata"},
			},
			expandticks: false,
			want:        []string{"s3://testdata"},
		},
		{
			input: "'$targetpath'",
			vars: map[string][]string{
				"targetpath": {"./testdata"},
			},
			expandticks: false,
			want:        []string{"$targetpath"},
		},
		{
			input: "$prefix.$suffix",
			vars: map[string][]string{
				"prefix": {"name"},
				"suffix": {"o"},
			},
			expandticks: false,
			want:        []string{"name.o"},
		},
		{
			input: "${targets:%=$targetpath/%}",
			vars: map[string][]string{
				"targetpath": {"./testdata"},
				"targets":    {"rupert", "ruxpin"},
			},
			expandticks: false,
			want:        []string{"./testdata/rupert", "./testdata/ruxpin"},
		},
		{
			input: "${targets:%=%.$novar}",
			vars: map[string][]string{
				"suffixes": {"o", "ab", "b"},
				"targets":  {"rupert", "ruxpin"},
			},
			expandticks: false,
			want:        []string{"rupert.$novar", "ruxpin.$novar"},
		},
		{
			input: "${targets:%=%.$suffixes}",
			vars: map[string][]string{
				"suffixes": {"teddy", "ab", "b"},
				"targets":  {"rupert", "ruxpin"},
			},
			expandticks: false,
			want: []string{
				"rupert.teddy",
				"ab",
				"b",
				"ruxpin.teddy",
				"ab",
				"b",
			},
		},
		{
			input: "${targets:%=%.$suffixes}",
			vars: map[string][]string{
				"suffixes": {"adventure"},
				"targets":  {"rupert bear", "ruxpin bear"},
			},
			expandticks: false,
			want: []string{
				"rupert bear.adventure",
				"ruxpin bear.adventure",
			},
		},
	}

	//	failing := tests[11:]

	for i, tv := range tests {
		got := expand(tv.input, tv.vars, tv.expandticks)

		if !reflect.DeepEqual(got, tv.want) {
			t.Errorf("%d: input: %#v, vars: %s, ticks: %v. got %s, want %s",
				i,
				tv.input, litter.Sdump(tv.vars), tv.expandticks,
				litter.Sdump(got),
				litter.Sdump(tv.want))
		}
	}
}

func TestExpandRecipeSigils(t *testing.T) {
	tests := []expandtv{
		{
			input: "/runs/contition_${stem1}_bowtie_k10/mapping.bam.bai",
			vars: map[string][]string{
				"stem1": {"a"},
			},
			expandticks: false,
			want:        []string{"/runs/contition_a_bowtie_k10/mapping.bam.bai"},
		},
		{
			input: "s3://runs/contition_${stem1}_bowtie_k10/mapping.bam.bai",
			vars: map[string][]string{
				"stem1": {"a"},
			},
			expandticks: false,
			want:        []string{"s3://runs/contition_a_bowtie_k10/mapping.bam.bai"},
		},
		{
			input: "mkdir -p $target\necho $target",
			vars: map[string][]string{
				"target": {"a"},
			},
			expandticks: false,
			want:        []string{"mkdir -p a\necho a"},
		},
		{
			input: "mkdir -p $(dirname $target)\necho $target",
			vars: map[string][]string{
				"target": {"a"},
			},
			expandticks: false,
			want:        []string{"mkdir -p $(dirname a)\necho a"},
		},
	}

	for i, tv := range tests {
		got := expandRecipeSigils(tv.input, tv.vars)

		if !reflect.DeepEqual(got, tv.want[0]) {
			t.Errorf("%d: input: %#v, vars: %s. got %s, want %s",
				i,
				tv.input, litter.Sdump(tv.vars),
				litter.Sdump(got),
				litter.Sdump(tv.want[0]))
		}
	}
}

func TestExpandBackQuoted(t *testing.T) {
	tests := []expandtv{
		{
			input: "seq 1 5`",
			vars: map[string][]string{
				"shell": {"sh"},
			},
			expandticks: false,
			want:        []string{"1"},
		},
	}

	for i, tv := range tests {
		got, _ := expandBackQuoted(tv.input, tv.vars)

		if !reflect.DeepEqual(got, tv.want) {
			t.Errorf("%d: input: %#v, vars: %s. got %s, want %s",
				i,
				tv.input, litter.Sdump(tv.vars),
				litter.Sdump(got),
				litter.Sdump(tv.want))
		}
	}
}
