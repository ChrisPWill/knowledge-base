package main

import (
	"fmt"
	"strings"
	"time"
)

var nowUTC = func() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func renderSummary(data summaryData) string {
	var builder strings.Builder
	if data.GeneratedAt != "" {
		builder.WriteString("Knowledge base tag summary")
		builder.WriteString("\n")
		builder.WriteString("Generated: ")
		builder.WriteString(data.GeneratedAt)
		builder.WriteString("\n")
	}

	for _, graph := range data.Graphs {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(graph.Name)
		builder.WriteString("\n")
		for _, tag := range graph.Tags {
			builder.WriteString(fmt.Sprintf("- %s: %d matches\n", tag.Name, tag.Count))
			if tag.CountOnly {
				continue
			}
			for _, occurrence := range tag.Occurrences {
				builder.WriteString(fmt.Sprintf("  %s:%d %s\n", occurrence.Path, occurrence.Line, occurrence.Content))
			}
		}
	}

	return strings.TrimSpace(builder.String()) + "\n"
}
