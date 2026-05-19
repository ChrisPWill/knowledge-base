package main

import (
	"context"
	"fmt"
	"path/filepath"
)

type MediaService struct {
	client  TelegramClient
	rootDir string
}

func NewMediaService(client TelegramClient, rootDir string) *MediaService {
	return &MediaService{
		client:  client,
		rootDir: rootDir,
	}
}

func (s *MediaService) DownloadAsset(ctx context.Context, item *CaptureItem, profile string) (string, error) {
	if !item.HasMedia {
		return "", nil
	}

	filePath, err := s.client.GetFile(ctx, item.MediaID)
	if err != nil {
		return "", fmt.Errorf("failed to get file path: %w", err)
	}

	extension := ".jpg"
	format := "![Image](assets/%s)"
	if item.IsVoice {
		extension = ".ogg"
		format = "[Voice Note](assets/%s)"
	}

	assetName := fmt.Sprintf("capture_%s%s", item.Timestamp.Format("20060102_150405"), extension)
	assetPath := filepath.Join(s.rootDir, profile, "assets", assetName)

	if err := s.client.DownloadFile(ctx, filePath, assetPath); err != nil {
		return "", fmt.Errorf("failed to download asset: %w", err)
	}

	return fmt.Sprintf(format, assetName), nil
}
