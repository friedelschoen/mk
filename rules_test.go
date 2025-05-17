package main

import (
	"regexp"
	"testing"
)

func TestMatchMetaRule(t *testing.T) {
	regex, err := regexp.Compile(`^data/processed/(\d+)/mapping_k10.bam.bai$`)
	if err != nil {
		t.Error("Failure to compile regex pattern")
	}
	pat := pattern{false, "data/processed/(\\d+)/mapping_k10.bam.bai", regex}
	matches := pat.match("data/processed/12345/mapping_k10.bam.bai")
	want := []string{"data/processed/12345/mapping_k10.bam.bai", "12345"}
	if matches[0] != want[0] || matches[1] != want[1] {
		t.Error("failed to match regular expression")
	}
}
