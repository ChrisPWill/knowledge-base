package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	if value == "" {
		return nil
	}
	*s = append(*s, value)
	return nil
}

func main() {
	var (
		personalPath   = flag.String("personal-path", "", "Path to the personal knowledge base")
		workPath       = flag.String("work-path", "", "Path to the work knowledge base")
		cachePath      = flag.String("cache-path", "", "Path to write the rendered summary cache")
		maxDigestItems = flag.Int("max-digest-items", 5, "Maximum excerpts to include per digest tag")
		excerptLength  = flag.Int("excerpt-length", 160, "Maximum number of characters per excerpt")
		countOnlyTags  stringList
		digestTags     stringList
	)

	flag.Var(&countOnlyTags, "count-only-tag", "Tag to report as a private count-only entry")
	flag.Var(&digestTags, "digest-tag", "Tag to report with excerpts")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	if err := validateConfig(*personalPath, *workPath, *cachePath, *maxDigestItems, *excerptLength); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	cfg := Config{
		PersonalPath:   *personalPath,
		WorkPath:       *workPath,
		CountOnlyTags:  append([]string(nil), countOnlyTags...),
		DigestTags:     append([]string(nil), digestTags...),
		CachePath:      *cachePath,
		MaxDigestItems: *maxDigestItems,
		ExcerptLength:  *excerptLength,
	}

	rendered, err := BuildSummary(cfg)
	if err != nil {
		slog.Error("failed to build summary", "error", err)
		os.Exit(1)
	}

	if err := WriteAtomically(cfg.CachePath, []byte(rendered)); err != nil {
		slog.Error("failed to write cache", "error", err)
		os.Exit(1)
	}
}

func validateConfig(personalPath, workPath, cachePath string, maxDigestItems, excerptLength int) error {
	switch {
	case personalPath == "":
		return fmt.Errorf("personal-path is required")
	case workPath == "":
		return fmt.Errorf("work-path is required")
	case cachePath == "":
		return fmt.Errorf("cache-path is required")
	case maxDigestItems < 1:
		return fmt.Errorf("max-digest-items must be at least 1")
	case excerptLength < 16:
		return fmt.Errorf("excerpt-length must be at least 16")
	default:
		return nil
	}
}
