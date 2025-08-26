package common

import "testing"

func TestFilter_Allow_IncludesOnly(t *testing.T) {
	f := &filter{includes: []string{"INFO"}}
	if !f.allow("[INFO] hello") {
		t.Fatalf("expected include to allow line")
	}
	if f.allow("[DEBUG] hidden") {
		t.Fatalf("expected non-matching include to block line")
	}
}

func TestFilter_Allow_ExcludesOnly(t *testing.T) {
	f := &filter{excludes: []string{"secret"}}
	if f.allow("public data") == false {
		t.Fatalf("expected line without exclude to pass")
	}
	if f.allow("this has secret token") {
		t.Fatalf("expected line with exclude substring to be blocked")
	}
}

func TestFilter_Allow_BothIncludeExclude(t *testing.T) {
	f := &filter{includes: []string{"INFO"}, excludes: []string{"drop"}}
	if !f.allow("INFO: ok") {
		t.Fatalf("expected INFO ok to pass")
	}
	if f.allow("DEBUG: ok") {
		t.Fatalf("expected DEBUG blocked by include filter")
	}
	if f.allow("INFO: drop this") {
		t.Fatalf("expected INFO with exclude substring to be blocked")
	}
}

func TestFilter_Allow_EmptyIncludesMeansAllowAllUnlessExcluded(t *testing.T) {
	f := &filter{includes: nil, excludes: []string{"bad"}}
	if !f.allow("something good") {
		t.Fatalf("expected allowed when not excluded")
	}
	if f.allow("very bad thing") {
		t.Fatalf("expected excluded substring to block")
	}
}
