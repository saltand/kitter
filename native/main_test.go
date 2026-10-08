package main

import "testing"

// fix-path-env parity: parseLoginPath is what survives an interactive
// shell printing arbitrary text around our marker-wrapped PATH output.

func TestParseLoginPathReadsMarkedValue(t *testing.T) {
	out := "welcome banner\n" + pathMarkStart + "/usr/bin:/bin" + pathMarkEnd + "more noise\n"
	if got := parseLoginPath(out); got != "/usr/bin:/bin" {
		t.Fatalf("got %q", got)
	}
}

func TestParseLoginPathIgnoresNoiseBeforeAndAfter(t *testing.T) {
	out := "motd\nmotd2\n" + pathMarkStart + " /a:/b " + pathMarkEnd + "\nprompt$ "
	if got := parseLoginPath(out); got != "/a:/b" {
		t.Fatalf("got %q", got)
	}
}

func TestParseLoginPathWithoutMarkersFails(t *testing.T) {
	for _, out := range []string{
		"",
		"/usr/bin:/bin",
		"start-only " + pathMarkStart + "/a:/b",
		pathMarkStart + pathMarkEnd, // empty value
		pathMarkEnd + "/x" + pathMarkStart,
	} {
		if got := parseLoginPath(out); got != "" {
			t.Fatalf("parseLoginPath(%q) = %q, want empty", out, got)
		}
	}
}
