package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type AssetDownloader interface {
	DownloadAsset(ctx context.Context, item *CaptureItem, profile string) (string, error)
}

type noopAssetDownloader struct{}

func (noopAssetDownloader) DownloadAsset(ctx context.Context, item *CaptureItem, profile string) (string, error) {
	return "", nil
}

type CaptureService struct {
	rootDir      string
	parser       *MessageParser
	journal      *JournalStore
	formatter    *LogseqFormatter
	commands     *CommandDispatcher
	titleFetcher func(ctx context.Context, url string) (string, error)

	mu          sync.Mutex
	states      map[string]SessionState
	messageMaps map[string]map[int64]MessageMetadata
}

func NewCaptureService(rootDir string) *CaptureService {
	fetcher := defaultTitleFetcher
	svc := &CaptureService{
		rootDir:      rootDir,
		parser:       NewMessageParser(rootDir, fetcher),
		journal:      NewJournalStore(rootDir),
		formatter:    &LogseqFormatter{},
		titleFetcher: fetcher,
		states:       make(map[string]SessionState),
		messageMaps:  make(map[string]map[int64]MessageMetadata),
	}
	svc.commands = NewCommandDispatcher()
	return svc
}

func (s *CaptureService) SetRootDir(rootDir string) {
	s.rootDir = rootDir
	s.parser.rootDir = rootDir
	s.parser.rules.rootDir = rootDir
	s.journal.rootDir = rootDir
}

func (s *CaptureService) SetTitleFetcher(fetcher func(ctx context.Context, url string) (string, error)) {
	if fetcher == nil {
		fetcher = defaultTitleFetcher
	}
	s.titleFetcher = fetcher
	s.parser.titleFetcher = fetcher
}

func (s *CaptureService) LoadMessageMap(clientID, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageMaps[clientID] = loadMessageMap(path)
}

func (s *CaptureService) SaveMessageMap(clientID, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saveMessageMap(path, s.messageMaps[clientID])
}

func (s *CaptureService) Process(ctx context.Context, req CaptureRequest, downloader AssetDownloader) (CaptureResponse, error) {
	if req.ClientID == "" {
		req.ClientID = "default"
	}
	if downloader == nil {
		downloader = noopAssetDownloader{}
	}

	now := time.Now()
	if req.Timestamp != nil {
		now = *req.Timestamp
	}

	switch req.Kind {
	case RequestKindCapture:
		return s.handleCapture(ctx, req, downloader, now)
	case RequestKindCommand:
		return s.handleCommand(ctx, req, now)
	case RequestKindReview:
		return s.handleReview(req.ReviewDay, now)
	default:
		return CaptureResponse{}, fmt.Errorf("unsupported request kind %q", req.Kind)
	}
}

func (s *CaptureService) sessionState(clientID string) SessionState {
	return s.states[clientID]
}

func (s *CaptureService) setSessionState(clientID string, state SessionState) {
	s.states[clientID] = state
}

func (s *CaptureService) clientMessageMap(clientID string) map[int64]MessageMetadata {
	if s.messageMaps[clientID] == nil {
		s.messageMaps[clientID] = make(map[int64]MessageMetadata)
	}
	return s.messageMaps[clientID]
}

func (s *CaptureService) handleCapture(ctx context.Context, req CaptureRequest, downloader AssetDownloader, now time.Time) (CaptureResponse, error) {
	item := req.Item
	if item == nil {
		item = &CaptureItem{
			Text:      req.Text,
			Timestamp: now,
		}
	}
	if item.Timestamp.IsZero() {
		item.Timestamp = now
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.sessionState(req.ClientID)
	if state.IsToggledAlso && now.Sub(state.LastInteractionTime) > 5*time.Minute {
		state.IsToggledAlso = false
	}

	messageMap := s.clientMessageMap(req.ClientID)
	if item.IsEdit {
		return s.handleEditLocked(ctx, req.ClientID, item, downloader, state, messageMap, now)
	}

	resp, nextState, meta, err := s.captureLocked(ctx, req.ClientID, item, downloader, state)
	if err != nil {
		return CaptureResponse{}, err
	}

	s.setSessionState(req.ClientID, nextState)
	if item.ID != 0 && meta != nil {
		messageMap[item.ID] = *meta
	}

	return resp, nil
}

func (s *CaptureService) captureLocked(ctx context.Context, clientID string, item *CaptureItem, downloader AssetDownloader, state SessionState) (CaptureResponse, SessionState, *MessageMetadata, error) {
	block, profile, err := s.parser.Analyze(ctx, *item, state)
	if err != nil {
		return CaptureResponse{}, state, nil, err
	}
	if block.Text == "" {
		return CaptureResponse{OK: true}, state, nil, nil
	}

	if item.HasMedia {
		if _, err := downloader.DownloadAsset(ctx, item, profile); err != nil {
			return CaptureResponse{}, state, nil, err
		}
	}

	entry := s.formatter.Format(block, state)
	if item.IsEdit {
		return CaptureResponse{
			OK:      true,
			Reply:   "🔄 Updated entry in journal.",
			Profile: profile,
			Entry:   entry,
		}, state, nil, nil
	}

	filePath, err := s.journal.Append(profile, entry, item.Timestamp)
	if err != nil {
		return CaptureResponse{}, state, nil, err
	}

	if block.IndentLevel == 0 {
		state.LastProfile = profile
		state.LastJournalFile = filePath
	}
	state.LastEntryTime = item.Timestamp
	state.LastInteractionTime = item.Timestamp

	meta := &MessageMetadata{
		FilePath:  filePath,
		Content:   entry,
		Timestamp: item.Timestamp,
	}

	return CaptureResponse{
		OK:          true,
		Reply:       fmt.Sprintf("✅ Captured to %s journal:\n%s", profile, strings.TrimSuffix(entry, "\n")),
		Profile:     profile,
		Entry:       entry,
		JournalPath: filePath,
	}, state, meta, nil
}

func (s *CaptureService) handleEditLocked(ctx context.Context, clientID string, item *CaptureItem, downloader AssetDownloader, state SessionState, messageMap map[int64]MessageMetadata, now time.Time) (CaptureResponse, error) {
	meta, ok := messageMap[item.ID]
	if !ok {
		return CaptureResponse{}, fmt.Errorf("no metadata found for message %d", item.ID)
	}
	if now.Sub(meta.Timestamp) > time.Hour {
		return CaptureResponse{}, fmt.Errorf("edit window expired for message %d", item.ID)
	}

	item.Timestamp = meta.Timestamp
	resp, nextState, _, err := s.captureLocked(ctx, clientID, item, downloader, state)
	if err != nil {
		return CaptureResponse{}, err
	}
	if err := s.journal.Update(meta.FilePath, meta.Content, resp.Entry); err != nil {
		return CaptureResponse{}, err
	}

	if resp.Entry == "" {
		delete(messageMap, item.ID)
	} else {
		meta.Content = resp.Entry
		messageMap[item.ID] = meta
	}
	s.setSessionState(clientID, nextState)
	resp.OK = true
	resp.Reply = "🔄 Updated entry in journal."
	resp.JournalPath = meta.FilePath
	return resp, nil
}

func (s *CaptureService) handleCommand(ctx context.Context, req CaptureRequest, now time.Time) (CaptureResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.sessionState(req.ClientID)
	resp, nextState, handled, err := s.commands.Dispatch(ctx, s, req.ClientID, req.Text, state, now)
	if err != nil {
		return CaptureResponse{}, err
	}
	if !handled {
		return CaptureResponse{}, fmt.Errorf("unsupported command %q", req.Text)
	}
	s.setSessionState(req.ClientID, nextState)
	return resp, nil
}

func (s *CaptureService) handleReview(day string, now time.Time) (CaptureResponse, error) {
	targetDate := now
	switch day {
	case "today", "":
		day = "today"
	case "yesterday":
		targetDate = now.AddDate(0, 0, -1)
	default:
		return CaptureResponse{}, fmt.Errorf("unsupported review day %q", day)
	}

	responseText, err := s.journal.ReadReview(context.Background(), targetDate)
	if err != nil {
		return CaptureResponse{}, err
	}
	if responseText == "" {
		responseText = fmt.Sprintf("📭 No entries found for %s.", day)
	}
	return CaptureResponse{OK: true, Reply: responseText}, nil
}

func (s *CaptureService) helpResponse(lowerMsg string) CaptureResponse {
	topic := strings.TrimSpace(strings.TrimPrefix(lowerMsg, "help"))
	var helpText string
	switch topic {
	case "nesting":
		helpText = "*Nesting Help*\n\n" +
			"• `also [note]` - Nest under the last entry\n" +
			"• `toggle also` - Enable auto-nesting mode (5m timeout)\n" +
			"• Auto-nesting is disabled if a profile prefix (/w, /p) is used."
	case "priority":
		helpText = "*Priority Help*\n\n" +
			"• `A [note]` - High priority ([#A])\n" +
			"• `B [note]` - Medium priority ([#B])\n" +
			"• `C [note]` - Low priority ([#C])\n" +
			"• Can be combined with `todo`: `todo A fix bug`"
	case "scheduling":
		helpText = "*Scheduling Help*\n\n" +
			"• `... scheduled for tomorrow` -> SCHEDULED: <YYYY-MM-DD Day>\n" +
			"• `... deadline next friday` -> DEADLINE: <YYYY-MM-DD Day>"
	case "media":
		helpText = "*Media Help*\n\n" +
			"• Photos and voice notes are downloaded to Logseq's `assets/` folder.\n" +
			"• Captions are supported and appended to the link."
	default:
		helpText = "🤖 *Telegram Capture Help*\n\n" +
			"*Profiles:*\n" +
			"• `/w [note]` or `/work [note]` - Work journal\n" +
			"• `/p [note]` or `/personal [note]` - Personal journal\n" +
			"• `[note]` - Defaults to personal\n\n" +
			"*Review:* `/today`, `/yesterday`\n" +
			"*Topics:* `help nesting`, `help priority`, `help scheduling`, `help media`"
	}

	return CaptureResponse{OK: true, Reply: helpText}
}

func (s *CaptureService) toggleAlsoResponse(state SessionState, now time.Time) (CaptureResponse, SessionState) {
	state.IsToggledAlso = !state.IsToggledAlso
	state.LastInteractionTime = now
	reply := "❌ Also mode disabled."
	if state.IsToggledAlso {
		reply = "✅ Also mode enabled. Messages will be nested for 5 minutes of inactivity."
	}
	return CaptureResponse{OK: true, Reply: reply}, state
}

func loadMessageMap(path string) map[int64]MessageMetadata {
	result := make(map[int64]MessageMetadata)
	if path == "" {
		return result
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &result)
	}
	return result
}

func saveMessageMap(path string, messageMap map[int64]MessageMetadata) {
	if path == "" {
		return
	}
	now := time.Now()
	for id, meta := range messageMap {
		if now.Sub(meta.Timestamp) > 24*time.Hour {
			delete(messageMap, id)
		}
	}
	if data, err := json.Marshal(messageMap); err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}
