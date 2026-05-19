package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tj/go-naturaldate"
)

var (
	tagRegex = regexp.MustCompile(`(^|\s)#\S+`)
	urlRegex = regexp.MustCompile(`https?://[^\s]+`)
)

type MessageParser struct {
	rootDir      string
	titleFetcher func(ctx context.Context, url string) (string, error)
	rules        *RuleEngine
}

func NewMessageParser(rootDir string, titleFetcher func(ctx context.Context, url string) (string, error)) *MessageParser {
	return &MessageParser{
		rootDir:      rootDir,
		titleFetcher: titleFetcher,
		rules:        NewRuleEngine(rootDir),
	}
}

func (p *MessageParser) Analyze(ctx context.Context, item CaptureItem, state SessionState) (LogseqBlock, string, error) {
	msg := strings.TrimSpace(item.Text)
	if msg == "" && item.Caption != "" {
		msg = strings.TrimSpace(item.Caption)
	}

	// 1. Determine Nesting
	isAlso := p.determineNesting(item, state, msg)
	indentLevel := 0
	if isAlso {
		indentLevel = 1
	}

	// 2. Determine Profile and Clean Message
	profile, cleanMsg := p.determineProfile(msg, isAlso, state)

	// 3. Handle Location
	if item.HasLocation {
		cleanMsg = p.formatLocation(item, cleanMsg)
	}

	// 4. Handle Media
	// Note: Media link formatting moved here for now, but will eventually move to formatter
	// or stay as part of the "message content" construction.
	if item.HasMedia {
		mediaLink := p.getMediaLinkStub(item)
		if cleanMsg != "" {
			cleanMsg = mediaLink + " " + cleanMsg
		} else {
			cleanMsg = mediaLink
		}
	}

	if cleanMsg == "" {
		return LogseqBlock{}, "", nil
	}

	// 5. Parse TODO and Priority
	isTodo, priority, cleanMsg := p.parseTodoAndPriority(cleanMsg)

	// 6. URL Scraping
	cleanMsg = p.scrapeURLs(ctx, cleanMsg)

	// 7. Dynamic Tag Mapping
	cleanMsg = p.rules.ApplyTags(cleanMsg)

	// 8. Natural Language Scheduling
	scheduleMarker, cleanMsg := p.parseScheduling(cleanMsg, isTodo, item.Timestamp)

	return LogseqBlock{
		Text:           cleanMsg,
		Priority:       priority,
		IsTodo:         isTodo,
		ScheduleMarker: scheduleMarker,
		Timestamp:      item.Timestamp,
		IndentLevel:    indentLevel,
	}, profile, nil
}

func (p *MessageParser) getMediaLinkStub(item CaptureItem) string {
	extension := ".jpg"
	format := "![Image](assets/%s)"
	if item.IsVoice {
		extension = ".ogg"
		format = "[Voice Note](assets/%s)"
	}
	assetName := fmt.Sprintf("capture_%s%s", item.Timestamp.Format("20060102_150405"), extension)
	return fmt.Sprintf(format, assetName)
}

func (p *MessageParser) determineNesting(item CaptureItem, state SessionState, msg string) bool {
	// Handle toggle timeout
	isToggledAlso := state.IsToggledAlso
	if isToggledAlso && item.Timestamp.Sub(state.LastInteractionTime) > 5*time.Minute {
		isToggledAlso = false
	}

	if strings.HasPrefix(strings.ToLower(msg), "also ") {
		return true
	}
	if isToggledAlso {
		hasPrefix := strings.HasPrefix(msg, "/w ") || strings.HasPrefix(msg, "/work ") ||
			strings.HasPrefix(msg, "/p ") || strings.HasPrefix(msg, "/personal ")
		if !hasPrefix {
			return true
		}
	}
	if item.HasLocation && item.Timestamp.Sub(state.LastInteractionTime) < 1*time.Minute {
		return true
	}
	return false
}

func (p *MessageParser) determineProfile(msg string, isAlso bool, state SessionState) (string, string) {
	if isAlso && state.LastJournalFile != "" {
		cleanMsg := msg
		if strings.HasPrefix(strings.ToLower(msg), "also ") {
			cleanMsg = strings.TrimSpace(msg[len("also "):])
		}
		return state.LastProfile, cleanMsg
	}

	profile := "personal"
	cleanMsg := msg

	if strings.HasPrefix(msg, "/w ") || strings.HasPrefix(msg, "/work ") {
		profile = "work"
		cleanMsg = strings.TrimPrefix(msg, "/w ")
		cleanMsg = strings.TrimPrefix(cleanMsg, "/work ")
	} else if strings.HasPrefix(msg, "/p ") || strings.HasPrefix(msg, "/personal ") {
		profile = "personal"
		cleanMsg = strings.TrimPrefix(msg, "/p ")
		cleanMsg = strings.TrimPrefix(cleanMsg, "/personal ")
	}

	return profile, strings.TrimSpace(cleanMsg)
}

func (p *MessageParser) formatLocation(item CaptureItem, cleanMsg string) string {
	var locText string
	u := fmt.Sprintf("https://www.google.com/maps?q=%f,%f", item.Latitude, item.Longitude)

	if item.VenueTitle != "" {
		addr := strings.Split(item.VenueAddr, ",")[0]
		locText = fmt.Sprintf("%s: %s (%s)", item.VenueTitle, addr, u)
	} else {
		locText = u
	}

	if cleanMsg != "" {
		return locText + " " + cleanMsg
	}
	return locText
}

func (p *MessageParser) parseTodoAndPriority(msg string) (bool, string, string) {
	isTodo := false
	cleanMsg := msg
	if strings.HasPrefix(strings.ToLower(cleanMsg), "todo ") {
		isTodo = true
		cleanMsg = strings.TrimSpace(cleanMsg[len("todo "):])
	}

	priority := ""
	if len(cleanMsg) >= 2 {
		firstTwo := strings.ToUpper(cleanMsg[:2])
		if firstTwo == "A " || firstTwo == "B " || firstTwo == "C " {
			priority = "[#" + string(firstTwo[0]) + "] "
			cleanMsg = strings.TrimSpace(cleanMsg[2:])
		}
	}
	return isTodo, priority, cleanMsg
}

func (p *MessageParser) scrapeURLs(ctx context.Context, msg string) string {
	urls := urlRegex.FindAllString(msg, -1)
	cleanMsg := msg
	for _, u := range urls {
		title, err := p.titleFetcher(ctx, u)
		if err == nil && title != "" {
			cleanMsg = strings.ReplaceAll(cleanMsg, u, fmt.Sprintf("[%s](%s)", title, u))
		}
	}
	return cleanMsg
}

func (p *MessageParser) parseScheduling(msg string, isTodo bool, now time.Time) (string, string) {
	triggers := []struct {
		prefix string
		marker string
	}{
		{"scheduled for ", "SCHEDULED"},
		{"deadline ", "DEADLINE"},
	}

	if isTodo {
		for _, word := range []string{"today", "tomorrow", "next "} {
			triggers = append(triggers, struct {
				prefix string
				marker string
			}{word, "DEADLINE"})
		}
	}

	cleanMsg := msg
	for _, trigger := range triggers {
		lower := strings.ToLower(cleanMsg)
		idx := strings.Index(lower, trigger.prefix)
		if idx != -1 {
			datePart := cleanMsg[idx+len(trigger.prefix):]
			if trigger.prefix == "today" || trigger.prefix == "tomorrow" {
				datePart = trigger.prefix
			}

			parsedDate, err := naturaldate.Parse(datePart, now)
			if err == nil {
				if trigger.prefix == "next " && parsedDate.Before(now) {
					parsedDate = parsedDate.AddDate(0, 0, 7)
				}

				marker := fmt.Sprintf(" %s: <%s %s>",
					trigger.marker,
					parsedDate.Format("2006-01-02"),
					parsedDate.Format("Mon"))

				return marker, strings.TrimSpace(cleanMsg[:idx])
			}
		}
	}
	return "", cleanMsg
}
