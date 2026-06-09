package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var rgLookPath = exec.LookPath

type Config struct {
	PersonalPath   string
	WorkPath       string
	CountOnlyTags  []string
	DigestTags     []string
	CachePath      string
	MaxDigestItems int
	ExcerptLength  int
}

type summaryData struct {
	GeneratedAt string
	Graphs      []graphSummary
}

type graphSummary struct {
	Name string
	Tags []tagSummary
}

type tagSummary struct {
	Name        string
	Count       int
	CountOnly   bool
	Occurrences []occurrence
}

type occurrence struct {
	Path    string
	Line    int
	Content string
}

type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

type tagPattern struct {
	Name  string
	Regex *regexp.Regexp
}

func BuildSummary(cfg Config) (string, error) {
	tagSet := orderedUniqueTags(cfg.CountOnlyTags, cfg.DigestTags)
	if len(tagSet) == 0 {
		return renderSummary(summaryData{}), nil
	}

	patterns, err := compileTagPatterns(tagSet)
	if err != nil {
		return "", err
	}

	combinedPattern := buildRGPattern(tagSet)
	graphs := []struct {
		name string
		root string
	}{
		{name: "personal", root: cfg.PersonalPath},
		{name: "work", root: cfg.WorkPath},
	}

	data := summaryData{
		GeneratedAt: nowUTC(),
		Graphs:      make([]graphSummary, 0, len(graphs)),
	}

	countOnlySet := make(map[string]struct{}, len(cfg.CountOnlyTags))
	for _, tag := range cfg.CountOnlyTags {
		countOnlySet[tag] = struct{}{}
	}

	for _, graph := range graphs {
		summary, err := scanGraph(graph.name, graph.root, combinedPattern, patterns, countOnlySet)
		if err != nil {
			return "", err
		}
		for _, tag := range tagSet {
			if _, ok := summary[tag]; !ok {
				summary[tag] = &tagSummary{Name: tag, CountOnly: isCountOnly(tag, countOnlySet)}
			}
		}
		graphSummary := graphSummary{Name: graph.name, Tags: make([]tagSummary, 0, len(summary))}
		for _, tag := range tagSet {
			entry := summary[tag]
			sortOccurrences(entry.Occurrences)
			if len(entry.Occurrences) > cfg.MaxDigestItems && !entry.CountOnly {
				entry.Occurrences = entry.Occurrences[:cfg.MaxDigestItems]
			}
			for i := range entry.Occurrences {
				entry.Occurrences[i].Content = truncate(entry.Occurrences[i].Content, cfg.ExcerptLength)
			}
			graphSummary.Tags = append(graphSummary.Tags, *entry)
		}
		data.Graphs = append(data.Graphs, graphSummary)
	}

	return renderSummary(data), nil
}

func scanGraph(graphName, root, combinedPattern string, patterns []tagPattern, countOnlySet map[string]struct{}) (map[string]*tagSummary, error) {
	rgPath, err := rgLookPath("rg")
	if err != nil {
		return nil, fmt.Errorf("ripgrep is required but was not found in PATH: %w", err)
	}

	args := []string{
		"--json",
		"--line-number",
		"--with-filename",
		"--glob", "journals/**/*.md",
		"--glob", "pages/**/*.md",
		"--regexp", combinedPattern,
		root,
	}

	cmd := exec.Command(rgPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok && exitErr.ExitCode() == 1 {
			output = nil
		} else {
			return nil, fmt.Errorf("ripgrep failed for %s graph: %w", graphName, err)
		}
	}

	summary := make(map[string]*tagSummary, len(patterns))
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		var event rgEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("failed to parse ripgrep output: %w", err)
		}
		if event.Type != "match" {
			continue
		}
		relativePath, err := filepath.Rel(root, event.Data.Path.Text)
		if err != nil {
			relativePath = event.Data.Path.Text
		}
		line := strings.TrimSpace(event.Data.Lines.Text)
		for _, pattern := range patterns {
			if !pattern.Regex.MatchString(line) {
				continue
			}
			entry := summary[pattern.Name]
			if entry == nil {
				entry = &tagSummary{Name: pattern.Name, CountOnly: isCountOnly(pattern.Name, countOnlySet)}
				summary[pattern.Name] = entry
			}
			key := fmt.Sprintf("%s|%d|%s", relativePath, event.Data.LineNumber, pattern.Name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			entry.Count++
			if entry.CountOnly {
				continue
			}
			entry.Occurrences = append(entry.Occurrences, occurrence{
				Path:    relativePath,
				Line:    event.Data.LineNumber,
				Content: line,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read ripgrep output: %w", err)
	}

	return summary, nil
}

func compileTagPatterns(tags []string) ([]tagPattern, error) {
	patterns := make([]tagPattern, 0, len(tags))
	for _, tag := range tags {
		if strings.TrimSpace(tag) == "" {
			return nil, fmt.Errorf("tag names must not be empty")
		}
		quoted := regexp.QuoteMeta(tag)
		pattern, err := regexp.Compile(`(^|[^A-Za-z0-9_/\-])#` + quoted + `($|[^A-Za-z0-9_/\-])|\[\[` + quoted + `\]\]`)
		if err != nil {
			return nil, fmt.Errorf("failed to compile pattern for tag %q: %w", tag, err)
		}
		patterns = append(patterns, tagPattern{Name: tag, Regex: pattern})
	}
	return patterns, nil
}

func buildRGPattern(tags []string) string {
	parts := make([]string, 0, len(tags))
	for _, tag := range tags {
		parts = append(parts, regexp.QuoteMeta(tag))
	}
	return `#(?:` + strings.Join(parts, `|`) + `)|\[\[(?:` + strings.Join(parts, `|`) + `)\]\]`
}

func orderedUniqueTags(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var tags []string
	for _, group := range groups {
		for _, tag := range group {
			trimmed := strings.TrimSpace(tag)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			tags = append(tags, trimmed)
		}
	}
	return tags
}

func isCountOnly(tag string, countOnlySet map[string]struct{}) bool {
	_, ok := countOnlySet[tag]
	return ok
}

func sortOccurrences(items []occurrence) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Path == items[j].Path {
			return items[i].Line > items[j].Line
		}
		return items[i].Path > items[j].Path
	})
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func asExitError(err error, target **exec.ExitError) bool {
	if err == nil {
		return false
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}
