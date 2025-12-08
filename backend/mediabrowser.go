package backend

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// mediaKeyPrefixLength is the maximum length used when generating filenames from media keys
	mediaKeyPrefixLength = 10
)

// MediaBrowser handles media browsing operations
type MediaBrowser struct{}

// GetMediaList retrieves a paginated list of media items
func (m *MediaBrowser) GetMediaList(pageToken string, limit int) (*MediaListResult, error) {
	// Create a new API client for each request. While this has some overhead,
	// it ensures we always have fresh authentication and simplifies error handling.
	// The API client internally caches auth tokens to minimize redundant auth requests.
	api, err := NewApi()
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := api.GetMediaList(pageToken, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get media list: %w", err)
	}

	return result, nil
}

// GetThumbnail retrieves a thumbnail for a media item and returns it as base64
func (m *MediaBrowser) GetThumbnail(mediaKey string, size string) (string, error) {
	api, err := NewApi()
	if err != nil {
		return "", fmt.Errorf("failed to create API client: %w", err)
	}

	// Parse size to width/height
	var width, height int
	switch size {
	case "small":
		width, height = 200, 200
	case "medium":
		width, height = 400, 400
	case "large":
		width, height = 800, 800
	default:
		width, height = 400, 400 // default to medium
	}

	thumbnailData, err := api.GetThumbnail(mediaKey, width, height, false, 0, false)
	if err != nil {
		return "", fmt.Errorf("failed to get thumbnail: %w", err)
	}

	// Convert to base64
	base64Data := base64.StdEncoding.EncodeToString(thumbnailData)
	return base64Data, nil
}

// DownloadMedia downloads a media item to the user's Downloads folder
func (m *MediaBrowser) DownloadMedia(mediaKey string) (string, error) {
	api, err := NewApi()
	if err != nil {
		return "", fmt.Errorf("failed to create API client: %w", err)
	}

	// Get download URLs (this also returns filename for videos)
	downloadURLs, err := api.GetDownloadURLs(mediaKey)
	if err != nil {
		return "", fmt.Errorf("failed to get download URLs: %w", err)
	}

	// Use original URL if available, otherwise use edited URL
	downloadURL := downloadURLs.EditedURL
	if downloadURLs.OriginalURL != "" {
		downloadURL = downloadURLs.OriginalURL
	}

	if downloadURL == "" {
		return "", fmt.Errorf("no download URL available for media key: %s", mediaKey)
	}

	// Get user's Downloads folder
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	downloadsDir := filepath.Join(homeDir, "Downloads", "gotohp")
	err = os.MkdirAll(downloadsDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create downloads directory: %w", err)
	}

	// Determine filename - prefer filename from download response
	filename := downloadURLs.Filename
	if filename == "" {
		// Fallback: try to get filename from media info
		mediaInfo, err := api.GetMediaInfo(mediaKey)
		if err == nil && mediaInfo.Filename != "" {
			filename = mediaInfo.Filename
		} else {
			// Last resort: generate a filename based on media key
			// Use media type to determine extension if available
			ext := ".unknown"
			if err == nil {
				if mediaInfo.MediaType == "video" {
					ext = ".mp4"
				} else if mediaInfo.MediaType == "photo" {
					ext = ".jpg"
				}
			}
			// Safely slice mediaKey to avoid panic
			keyPrefix := mediaKey
			if len(mediaKey) > mediaKeyPrefixLength {
				keyPrefix = mediaKey[:mediaKeyPrefixLength]
			}
			filename = fmt.Sprintf("%s%s", keyPrefix, ext)
		}
	}
	outputPath := filepath.Join(downloadsDir, filename)

	// Download the file
	err = api.DownloadFile(downloadURL, outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}

	return outputPath, nil
}
