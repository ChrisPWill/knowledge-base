package main

import (
	"fmt"
	"time"
)

type LogseqBlock struct {
	Text           string
	Priority       string // [#A], [#B], etc.
	IsTodo         bool
	ScheduleMarker string // SCHEDULED: <...>, DEADLINE: <...>
	Timestamp      time.Time
	IndentLevel    int
}

type LogseqFormatter struct{}

func (f *LogseqFormatter) Format(block LogseqBlock, state SessionState) string {
	timeStr := block.Timestamp.Format("15:04")
	todoPrefix := ""
	if block.IsTodo {
		todoPrefix = "TODO "
	}

	tagSuffix := ""
	if block.IndentLevel == 0 && !tagRegex.MatchString(block.Text) {
		tagSuffix = " #inbox"
	}

	if block.IndentLevel > 0 {
		indent := ""
		for i := 0; i < block.IndentLevel; i++ {
			indent += "  "
		}
		if block.Timestamp.Sub(state.LastEntryTime) > time.Hour {
			return fmt.Sprintf("%s- %s%s %s%s%s\n", indent, block.Priority, timeStr, block.Text, block.ScheduleMarker, tagSuffix)
		}
		return fmt.Sprintf("%s- %s%s%s%s\n", indent, block.Priority, block.Text, block.ScheduleMarker, tagSuffix)
	}

	return fmt.Sprintf("- %s%s%s %s%s%s\n", todoPrefix, block.Priority, timeStr, block.Text, block.ScheduleMarker, tagSuffix)
}
