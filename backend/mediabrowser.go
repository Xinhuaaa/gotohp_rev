package backend

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

const (
	// mediaKeyPrefixLength is the maximum length used when generating filenames from media keys
	mediaKeyPrefixLength = 10
)

// MediaBrowser handles media browsing operations
type MediaBrowser struct {
	api *Api
	mu  sync.Mutex
}

// getAPI lazily initializes and caches the API client so repeated calls (such as
// fetching thumbnails) can reuse the same auth token instead of requesting a
// fresh one each time.
func (m *MediaBrowser) getAPI() (*Api, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Chek if we have a cached API instance and if it matches the currently selected account
	if m.api != nil && m.api.Email == AppConfig.Selected {
		return m.api, nil
	}

	api, err := NewApi()
	if err != nil {
		return nil, err
	}

	m.api = api
	return m.api, nil
}

// GetMediaList retrieves a paginated list of media items
func (m *MediaBrowser) GetMediaList(pageToken string, syncToken string, triggerMode int, limit int) (*MediaListResult, error) {
	api, err := m.getAPI()
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := api.GetMediaList(pageToken, syncToken, triggerMode, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get media list: %w", err)
	}

	return result, nil
}

// GetAlbumList retrieves a paginated list of albums
func (m *MediaBrowser) GetAlbumList(pageToken string) (*AlbumListResult, error) {
	api, err := m.getAPI()
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := api.GetAlbumList(pageToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get album list: %w", err)
	}

	return result, nil
}

// GetThumbnail retrieves a thumbnail for a media item and returns it as base64
func (m *MediaBrowser) GetThumbnail(mediaKey string, size string) (string, error) {
	api, err := m.getAPI()
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

func validateDebugURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "https" {
		return nil, errors.New("only https URLs are allowed")
	}
	// Avoid leaking the bearer token by accidentally sending it to a non-Google endpoint.
	if u.Host != "photosdata-pa.googleapis.com" {
		return nil, fmt.Errorf("unsupported host %q (only photosdata-pa.googleapis.com allowed)", u.Host)
	}
	return u, nil
}

// DebugProtobufRequest sends an authenticated protobuf POST request built from a numeric-key JSON structure
// and returns a best-effort JSON dump of the protobuf response for inspection.
func (m *MediaBrowser) DebugProtobufRequest(endpoint string, requestJSON string) (string, error) {
	u, err := validateDebugURL(endpoint)
	if err != nil {
		return "", err
	}

	api, err := m.getAPI()
	if err != nil {
		return "", fmt.Errorf("failed to create API client: %w", err)
	}

	requestData, err := buildProtobufFromJSONText(requestJSON)
	if err != nil {
		return "", err
	}

	respBytes, err := api.doProtobufPOST(u.String(), requestData)
	if err != nil {
		return "", err
	}

	decoded, ok := decodeProtobufMessage(respBytes, 0)
	if !ok {
		// Fallback to raw bytes info if we cannot decode.
		decoded = map[string]any{
			"raw": bufferObject(respBytes),
		}
	}

	out, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}
	return string(out), nil
}

// DownloadMedia downloads a media item to the user's Downloads folder
func (m *MediaBrowser) DownloadMedia(mediaKey string) (string, error) {
	api, err := m.getAPI()
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

// DeleteMedia moves a media item to trash (soft delete).
func (m *MediaBrowser) DeleteMedia(mediaKey string) error {
	if len(mediaKey) < minMediaKeyLength {
		return fmt.Errorf("invalid media key")
	}

	api, err := m.getAPI()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := api.MoveToTrash([]string{mediaKey}); err != nil {
		return fmt.Errorf("failed to move to trash: %w", err)
	}

	return nil
}

// PermanentlyDeleteMedia permanently deletes a media item by its dedup key.
func (m *MediaBrowser) PermanentlyDeleteMedia(dedupKey string) error {
	if dedupKey == "" {
		return fmt.Errorf("invalid dedup key")
	}

	api, err := m.getAPI()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := api.PermanentlyDelete([]string{dedupKey}); err != nil {
		return fmt.Errorf("failed to permanently delete: %w", err)
	}

	return nil
}
