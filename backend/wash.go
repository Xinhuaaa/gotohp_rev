package backend

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WashMedia downloads a media item locally and re-uploads it using a quota-exempt client profile.
// Returns the media metadata of the uploaded (washed) item.
func (m *MediaBrowser) WashMedia(mediaKey string, dedupKey string) (*MediaItem, error) {
	if len(mediaKey) < minMediaKeyLength {
		return nil, fmt.Errorf("invalid media key")
	}
	if dedupKey == "" {
		return nil, fmt.Errorf("wash requires dedupKey (2.21.1) for permanent delete")
	}

	api, err := m.getAPI()
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	downloadURLs, err := api.GetDownloadURLs(mediaKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get download URLs: %w", err)
	}

	// We trust the dedupKey passed from the frontend which came from the list response.
	// No need to fetch info again.

	downloadURL := downloadURLs.EditedURL
	if downloadURLs.OriginalURL != "" {
		downloadURL = downloadURLs.OriginalURL
	}
	if downloadURL == "" {
		return nil, fmt.Errorf("no download URL available for media key: %s", mediaKey)
	}

	tmpDir, err := os.MkdirTemp("", "gotohp-wash-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	filename := strings.TrimSpace(downloadURLs.Filename)
	// We don't have originalInfo.Filename unless we fetch it, but downloadURLs.Filename should be sufficient.
	// If empty, we can just use a default.
	if filename == "" {
		filename = "washed"
	}
	filename = filepath.Base(filename)
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")
	outputPath := filepath.Join(tmpDir, filename)

	if err := api.DownloadFile(downloadURL, outputPath); err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	sha1HashBytes, err := CalculateSHA1(ctx, outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate sha1: %w", err)
	}

	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat downloaded file: %w", err)
	}

	sha1HashB64 := base64.StdEncoding.EncodeToString(sha1HashBytes)
	uploadToken, err := api.GetUploadToken(sha1HashB64, fileInfo.Size())
	if err != nil {
		return nil, fmt.Errorf("failed to get upload token: %w", err)
	}

	// Delete the original from Google Photos before re-uploading.
	// Step 2: Move to trash
	if err := api.MoveToTrash([]string{mediaKey}); err != nil {
		return nil, fmt.Errorf("failed to move to trash: %w", err)
	}

	// Step 3: Permanently delete
	if err := api.PermanentlyDelete([]string{dedupKey}); err != nil {
		return nil, fmt.Errorf("failed to permanently delete original (dedupKey=%s): %w", dedupKey, err)
	}

	commitToken, err := api.UploadFile(ctx, outputPath, uploadToken)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	newMediaKey, err := api.CommitUpload(commitToken, fileInfo.Name(), sha1HashBytes, fileInfo.ModTime().Unix())
	if err != nil {
		return nil, fmt.Errorf("failed to commit upload: %w", err)
	}

	item, err := api.GetMediaInfo(newMediaKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch uploaded media info: %w", err)
	}

	return item, nil
}

