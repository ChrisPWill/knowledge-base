package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Rule struct {
	Tag      string
	Keywords []string
}

type RuleEngine struct {
	rootDir    string
	rules      []Rule
	lastLoaded time.Time
	mu         sync.RWMutex
}

func NewRuleEngine(rootDir string) *RuleEngine {
	return &RuleEngine{rootDir: rootDir}
}

func (e *RuleEngine) ApplyTags(text string) string {
	e.maybeReload()

	e.mu.RLock()
	defer e.mu.RUnlock()

	cleanMsg := text
	for _, rule := range e.rules {
		for _, kw := range rule.Keywords {
			re := regexp.MustCompile("(?i)\\b" + regexp.QuoteMeta(kw) + "\\b")
			if re.MatchString(cleanMsg) {
				if !strings.Contains(cleanMsg, rule.Tag) {
					cleanMsg += " " + rule.Tag
				}
				break
			}
		}
	}
	return cleanMsg
}

func (e *RuleEngine) maybeReload() {
	rulesFile := filepath.Join(e.rootDir, "personal", "pages", "Telegram Rules.md")
	info, err := os.Stat(rulesFile)
	if err != nil {
		return
	}

	e.mu.RLock()
	lastLoaded := e.lastLoaded
	e.mu.RUnlock()

	if info.ModTime().After(lastLoaded) {
		e.load(rulesFile, info.ModTime())
	}
}

func (e *RuleEngine) load(path string, modTime time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var newRules []Rule
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- #") {
			continue
		}
		parts := strings.SplitN(line[2:], ":", 2)
		if len(parts) != 2 {
			continue
		}
		tag := strings.TrimSpace(parts[0])
		if !strings.HasPrefix(tag, "#") {
			tag = "#" + tag
		}
		rawKeywords := strings.Split(parts[1], ",")
		var keywords []string
		for _, kw := range rawKeywords {
			kw = strings.TrimSpace(kw)
			if kw != "" {
				keywords = append(keywords, kw)
			}
		}
		newRules = append(newRules, Rule{Tag: tag, Keywords: keywords})
	}

	e.rules = newRules
	e.lastLoaded = modTime
}
