package main

import (
	"reflect"
	"testing"
)

// Only sy term panes with nothing but a shell running close with Symbolizer.
func TestIdleSyTerms(t *testing.T) {
	ls := []byte(`[{"tabs":[{"windows":[
		{"id":1,"user_vars":{},"foreground_processes":[{"cmdline":["/home/u/sy"]}]},
		{"id":2,"user_vars":{"sy_term":"1"},"foreground_processes":[{"cmdline":["/usr/bin/zsh"]}]},
		{"id":3,"user_vars":{"sy_term":"1"},"foreground_processes":[{"cmdline":["claude"]},{"cmdline":["bun","x"]}]},
		{"id":4,"user_vars":{"sy_term":"1"},"foreground_processes":[{"cmdline":["-bash"]}]},
		{"id":5,"user_vars":{},"foreground_processes":[{"cmdline":["zsh"]}]}
	]}]}]`)
	if got := idleSyTerms(ls, "1"); !reflect.DeepEqual(got, []string{"2", "4"}) {
		t.Fatalf("idle sy terms: %v, want [2 4]", got)
	}
}
