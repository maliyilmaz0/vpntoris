package main

import "testing"

func TestEscapePS(t *testing.T) {
	if got := escapePS("it's"); got != "it''s" {
		t.Fatalf("got %q", got)
	}
}
