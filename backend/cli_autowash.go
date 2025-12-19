package backend

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AutoWashConfig holds configuration for the auto-wash process
type AutoWashConfig struct {
	Interval       time.Duration
	DbPath         string
	BackupDir      string
	RetentionDays  int
	MaxWashRetries int
}

// RunAutoWash starts the continuous auto-wash process
func RunAutoWash(config AutoWashConfig) error {
	fmt.Println("Starting Auto-Wash Service...")
	fmt.Printf("Config: Interval=%v, DB=%s, BackupDir=%s, Retention=%d days\n",
		config.Interval, config.DbPath, config.BackupDir, config.RetentionDays)

	// Initialize DB
	db, err := NewMediaDB(config.DbPath)
	if err != nil {
		return fmt.Errorf("failed to init DB: %w", err)
	}
	fmt.Printf("Database loaded with %d items.\n", len(db.Items))

	// Create API client
	api, err := NewApi()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Ensure backup dir exists
	if err := os.MkdirAll(config.BackupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup dir: %w", err)
	}

	// Initial full sync (if empty) or just use existing
	// We'll treat the first loop iteration as the initial sync/check
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()

	// Run once immediately
	if err := performAutoWashCycle(api, db, config); err != nil {
		fmt.Printf("Error in initial cycle: %v\n", err)
	}

	for range ticker.C {
		if err := performAutoWashCycle(api, db, config); err != nil {
			fmt.Printf("Error in cycle: %v\n", err)
		}
	}

	return nil
}

func performAutoWashCycle(api *Api, db *MediaDB, config AutoWashConfig) error {
	isInitial := db.SyncToken == ""
	if isInitial {
		fmt.Println("\n--- Starting Initial Full Scan ---")
	} else {
		fmt.Printf("\n--- Starting Incremental Update (SyncToken: %s) ---\n", db.SyncToken[:8]+"...")
	}
	
	updatedItemsCount := 0
	// Resume from saved page token if available
	pageToken := db.NextPageToken
	
	// Track the new sync token if the API returns one
	newSyncToken := ""
	
	// Page through results
	for {
		// triggerMode: 1 if we have a sync token (Active/Incremental), 2 if not (Passive/Full scan)
		mode := 2
		currentSyncToken := ""
		if !isInitial {
			mode = 1
			currentSyncToken = db.SyncToken
		}

		list, err := api.GetMediaList(pageToken, currentSyncToken, mode, 0)
		if err != nil {
			return fmt.Errorf("list fetch failed: %w", err)
		}

		for _, item := range list.Items {
			// Update DB
			changed := db.UpdateOrAdd(item)
			if changed {
				updatedItemsCount++
				// Check if it needs washing
				if shouldWash(item) {
					fmt.Printf("[Detected] Quota Item: %s (%s)\n", item.Filename, item.MediaKey)
					if err := processItemWash(api, item, config); err != nil {
						fmt.Printf("[Error] Wash failed for %s: %v\n", item.Filename, err)
					}
				}
			}
		}

		// Capture the latest sync token from the response (usually on the last page)
		if list.SyncToken != "" {
			newSyncToken = list.SyncToken
			fmt.Printf("  [Debug] Received SyncToken: %s...\n", newSyncToken[:10])
		}

		// Save resumption state
		db.NextPageToken = list.NextPageToken
		if err := db.Save(); err != nil {
			fmt.Printf("Warning: Failed to save database checkpoint: %v\n", err)
		}

		if list.NextPageToken == "" {
			break
		}
		pageToken = list.NextPageToken
	}
	
	// Cycle complete: update SyncToken and clear resume token
	if newSyncToken != "" {
		db.SyncToken = newSyncToken
		fmt.Println("  [Info] SyncToken updated and saved.")
	} else if isInitial {
		fmt.Println("  [Warning] Initial scan completed but NO SyncToken received. Next run might be full scan again.")
	}
	db.NextPageToken = ""
	
	if err := db.Save(); err != nil {
		fmt.Printf("Warning: Failed to save final database state: %v\n", err)
	}
	
	fmt.Printf("Cycle complete. Updated items: %d. Total in DB: %d\n", updatedItemsCount, len(db.Items))

	// 2. Cleanup old local files... (rest remains same)
	if config.RetentionDays > 0 {
		fmt.Printf("Cleaning up files older than %d days...\n", config.RetentionDays)
		if err := CleanupOldFiles(config.BackupDir, config.RetentionDays); err != nil {
			fmt.Printf("Error cleaning up: %v\n", err)
		}
	}

	return nil
}

func shouldWash(item MediaItem) bool {
	return !item.IsTrash && item.CountsTowardsQuota
}

func processItemWash(api *Api, item MediaItem, config AutoWashConfig) error {
	fmt.Printf(">>> Washing: %s\n", item.Filename)

	// 1. Download
	// Get URL
	urls, err := api.GetDownloadURLs(item.MediaKey)
	if err != nil {
		return fmt.Errorf("get url failed: %w", err)
	}
	
	url := urls.EditedURL
	if urls.OriginalURL != "" {
		url = urls.OriginalURL
	}
	if url == "" {
		return fmt.Errorf("no download url")
	}

	// Local path
	filename := item.Filename
	if filename == "" {
		if urls.Filename != "" {
			filename = urls.Filename
		} else {
			filename = fmt.Sprintf("%s.bin", item.MediaKey)
		}
	}
	localPath := filepath.Join(config.BackupDir, filename)

	// Check if already downloaded
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		fmt.Printf("    Downloading... ")
		if err := api.DownloadFile(url, localPath); err != nil {
			fmt.Println("Failed.")
			return err
		}
		fmt.Println("Done.")
	} else {
		fmt.Println("    File exists locally, skipping download.")
	}

	// 2. Move to Trash
	fmt.Printf("    Moving to Trash... ")
	if err := api.MoveToTrash([]string{item.MediaKey}); err != nil {
		fmt.Println("Failed.")
		return err
	}
	fmt.Println("Done.")

	// 3. Permanently Delete
	if item.DedupKey == "" {
		fmt.Println("    Warning: No DedupKey, skipping permanent delete (safety).")
		return fmt.Errorf("missing dedup key")
	}
	fmt.Printf("    Permanently Deleting... ")
	if err := api.PermanentlyDelete([]string{item.DedupKey}); err != nil {
		fmt.Println("Failed.")
		return err
	}
	fmt.Println("Done.")

	// 4. Upload
	fmt.Printf("    Uploading... ")
	ctx := context.Background()
	
	sha1Bytes, _ := CalculateSHA1(ctx, localPath)
	fileInfo, _ := os.Stat(localPath)
	sha1B64 := base64.StdEncoding.EncodeToString(sha1Bytes)

	token, err := api.GetUploadToken(sha1B64, fileInfo.Size())
	if err != nil {
		fmt.Println("Failed (GetToken).")
		return err
	}

	commitToken, err := api.UploadFile(ctx, localPath, token)
	if err != nil {
		fmt.Println("Failed (Upload).")
		return err
	}

	// Use standard CommitUpload (Pixel XL logic inside)
	_, err = api.CommitUpload(commitToken, fileInfo.Name(), sha1Bytes, fileInfo.ModTime().Unix())
	if err != nil {
		fmt.Println("Failed (Commit).")
		return err
	}
	fmt.Println("Done (Success).")

	return nil
}
