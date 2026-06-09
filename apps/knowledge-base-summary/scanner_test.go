package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRGPatternEscapesTags(t *testing.T) {
	pattern := buildRGPattern([]string{"#project/foo", "[[ops+prod]]"})
	if !strings.Contains(pattern, `project/foo`) {
		t.Fatalf("expected namespaced tag in pattern, got %q", pattern)
	}
	if !strings.Contains(pattern, `ops\+prod`) {
		t.Fatalf("expected regex escaping in pattern, got %q", pattern)
	}
}

func TestCompileTagPatternsMatchHashAndRefs(t *testing.T) {
	patterns, err := compileTagPatterns([]string{"#project/foo", "[[ops]]"})
	if err != nil {
		t.Fatalf("compileTagPatterns returned error: %v", err)
	}

	cases := []struct {
		tag   string
		line  string
		match bool
	}{
		{tag: "project/foo", line: "- TODO review #project/foo", match: true},
		{tag: "project/foo", line: "- TODO review [[project/foo]]", match: true},
		{tag: "project/foo", line: "- TODO review #project/foobar", match: false},
		{tag: "ops", line: "- note #ops-work", match: false},
		{tag: "ops", line: "- note #ops", match: true},
	}

	for _, tc := range cases {
		var patternFound *tagPattern
		for i := range patterns {
			if patterns[i].Name == tc.tag {
				patternFound = &patterns[i]
				break
			}
		}
		if patternFound == nil {
			t.Fatalf("missing pattern for tag %q", tc.tag)
		}
		if got := patternFound.Regex.MatchString(tc.line); got != tc.match {
			t.Fatalf("tag %q line %q: got %v want %v", tc.tag, tc.line, got, tc.match)
		}
	}
}

func TestBuildSummaryUsesRipgrepOutput(t *testing.T) {
	originalLookPath := rgLookPath
	originalNowUTC := nowUTC
	defer func() {
		rgLookPath = originalLookPath
		nowUTC = originalNowUTC
	}()

	rgLookPath = func(file string) (string, error) {
		return filepath.Join("testdata", "fake-rg.sh"), nil
	}
	nowUTC = func() string {
		return "2026-06-09T00:00:00Z"
	}

	root := t.TempDir()
	personalRoot := filepath.Join(root, "personal")
	workRoot := filepath.Join(root, "work")
	for _, dir := range []string{
		filepath.Join(personalRoot, "journals"),
		filepath.Join(personalRoot, "pages"),
		filepath.Join(workRoot, "pages"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}
	personalJournal := filepath.Join(personalRoot, "journals", "2026_06_09.md")
	if err := os.WriteFile(personalJournal, []byte("unused"), 0o644); err != nil {
		t.Fatalf("failed to write journal: %v", err)
	}
	personalPage := filepath.Join(personalRoot, "pages", "Project.md")
	if err := os.WriteFile(personalPage, []byte("unused"), 0o644); err != nil {
		t.Fatalf("failed to write page: %v", err)
	}
	workPage := filepath.Join(workRoot, "pages", "Ops.md")
	if err := os.WriteFile(workPage, []byte("unused"), 0o644); err != nil {
		t.Fatalf("failed to write work page: %v", err)
	}

	cfg := Config{
		PersonalPath:   personalRoot,
		WorkPath:       workRoot,
		CountOnlyTags:  []string{"#private"},
		DigestTags:     []string{"[[project/foo]]", "#ops+prod"},
		MaxDigestItems: 2,
		ExcerptLength:  80,
	}

	summary, err := BuildSummary(cfg)
	if err != nil {
		t.Fatalf("BuildSummary returned error: %v", err)
	}

	expectedParts := []string{
		"Knowledge base tag summary",
		"personal",
		"- private: 1 matches",
		"- project/foo: 2 matches",
		"pages/Project.md:4 summary [[project/foo]]",
		"journals/2026_06_09.md:12 note #private #project/foo",
		"work",
		"- ops+prod: 1 matches",
		"pages/Ops.md:3 rollout #ops+prod",
	}
	for _, part := range expectedParts {
		if !strings.Contains(summary, part) {
			t.Fatalf("expected summary to contain %q, got:\n%s", part, summary)
		}
	}
	if strings.Contains(summary, "private #project/foo") && strings.Contains(summary, "private: 1 matches\n  ") {
		t.Fatalf("count-only tag should not render excerpts:\n%s", summary)
	}
}

func TestNormalizeTag(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "inbox", want: "inbox"},
		{input: "#inbox", want: "inbox"},
		{input: "[[inbox]]", want: "inbox"},
		{input: " #project/foo ", want: "project/foo"},
	}

	for _, tc := range cases {
		got, err := normalizeTag(tc.input)
		if err != nil {
			t.Fatalf("normalizeTag(%q) returned error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("normalizeTag(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestWriteAtomicallyReplacesFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "summary.txt")

	if err := WriteAtomically(path, []byte("first")); err != nil {
		t.Fatalf("WriteAtomically returned error on first write: %v", err)
	}
	if err := WriteAtomically(path, []byte("second")); err != nil {
		t.Fatalf("WriteAtomically returned error on second write: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read cache file: %v", err)
	}
	if string(contents) != "second" {
		t.Fatalf("unexpected cache contents %q", string(contents))
	}
}
