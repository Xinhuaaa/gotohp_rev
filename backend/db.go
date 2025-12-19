package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MediaDB represents the persistent database of media items
type MediaDB struct {
	Items         map[string]MediaItem `json:"items"`         // Keyed by MediaKey
	SyncToken     string               `json:"syncToken"`     // Token for incremental updates
	NextPageToken string               `json:"nextPageToken"` // Token for resuming interrupted scans
	mu            sync.RWMutex
	path          string
}

// NewMediaDB creates or loads a MediaDB from the specified file path
func NewMediaDB(path string) (*MediaDB, error) {
	db := &MediaDB{
		Items: make(map[string]MediaItem),
		path:  path,
	}

	// Try to load existing DB
	if _, err := os.Stat(path); err == nil {
		if err := db.Load(); err != nil {
			return nil, fmt.Errorf("failed to load database: %w", err)
		}
	}

	return db, nil
}

// Load reads the database from disk
func (db *MediaDB) Load() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	data, err := os.ReadFile(db.path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, db)
}

// Save writes the database to disk
func (db *MediaDB) Save() error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(db.path, data, 0644)
}

// UpdateOrAdd adds or updates a media item. Returns true if the item was new or changed.
func (db *MediaDB) UpdateOrAdd(item MediaItem) bool {
	db.mu.Lock()
	defer db.mu.Unlock()

	existing, exists := db.Items[item.MediaKey]
	if !exists {
		db.Items[item.MediaKey] = item
		return true
	}

	// Check for changes we care about (Quota, Trash status)
	changed := false
	if existing.CountsTowardsQuota != item.CountsTowardsQuota {
		existing.CountsTowardsQuota = item.CountsTowardsQuota
		changed = true
	}
	if existing.IsTrash != item.IsTrash {
		existing.IsTrash = item.IsTrash
		changed = true
	}
	// Also update basic info if missing
	if existing.DedupKey == "" && item.DedupKey != "" {
		existing.DedupKey = item.DedupKey
		changed = true
	}
    if existing.Filename == "" && item.Filename != "" {
        existing.Filename = item.Filename
        changed = true
    }

	if changed {
		db.Items[item.MediaKey] = existing
	}
	return changed
}

// GetItem retrieves an item by MediaKey
func (db *MediaDB) GetItem(mediaKey string) (MediaItem, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	item, ok := db.Items[mediaKey]
	return item, ok
}

// GetAllItems returns all items as a slice
func (db *MediaDB) GetAllItems() []MediaItem {
	db.mu.RLock()
	defer db.mu.RUnlock()
	items := make([]MediaItem, 0, len(db.Items))
	for _, item := range db.Items {
		items = append(items, item)
	}
	return items
}

// CleanupOldFiles deletes local washed files older than retentionDays
func CleanupOldFiles(dir string, retentionDays int) error {
    if retentionDays <= 0 {
        return nil
    }
    entries, err := os.ReadDir(dir)
    if err != nil {
        return err
    }
    
    cutoff := time.Now().AddDate(0, 0, -retentionDays)
    
    for _, entry := range entries {
        if entry.IsDir() {
            continue
        }
        info, err := entry.Info()
        if err != nil {
            continue
        }
        if info.ModTime().Before(cutoff) {
            os.Remove(filepath.Join(dir, entry.Name()))
        }
    }
    return nil
}
