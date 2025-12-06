package main

import (
	"app/backend"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CLI flags and config
type cliConfig struct {
	recursive                     bool
	threads                       int
	forceUpload                   bool
	deleteFromHost                bool
	disableUnsupportedFilesFilter bool
	logLevel                      string
	configPath                    string
}

// Messages for bubbletea
type uploadStartMsg struct {
	total int
}

type fileProgressMsg struct {
	workerID int
	status   string
	fileName string
	message  string
}

type fileCompleteMsg struct {
	success  bool
	fileName string
	mediaKey string
	err      error
}

type uploadCompleteMsg struct{}

// Bubbletea model
type uploadModel struct {
	progress     progress.Model
	totalFiles   int
	completed    int
	failed       int
	currentFiles map[int]string // workerID -> current file
	workers      map[int]string // workerID -> status message
	results      []uploadResult // Track all upload results
	width        int
	quitting     bool
}

type uploadResult struct {
	Path     string `json:"path"`
	Success  bool   `json:"success"`
	MediaKey string `json:"mediaKey,omitempty"`
	Error    string `json:"error,omitempty"`
}

type uploadSummary struct {
	Total     int            `json:"total"`
	Succeeded int            `json:"succeeded"`
	Failed    int            `json:"failed"`
	Results   []uploadResult `json:"results"`
}

func initialModel() uploadModel {
	return uploadModel{
		progress:     progress.New(progress.WithDefaultGradient()),
		currentFiles: make(map[int]string),
		workers:      make(map[int]string),
		results:      []uploadResult{},
		width:        80,
	}
}

func (m uploadModel) Init() tea.Cmd {
	return nil
}

func (m uploadModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.progress.Width = msg.Width - 4
		return m, nil

	case uploadStartMsg:
		m.totalFiles = msg.total
		return m, nil

	case fileProgressMsg:
		m.workers[msg.workerID] = fmt.Sprintf("[%d] %s: %s", msg.workerID, msg.status, msg.fileName)
		if msg.fileName != "" {
			m.currentFiles[msg.workerID] = msg.fileName
		}
		return m, nil

	case fileCompleteMsg:
		result := uploadResult{
			Path:     msg.fileName,
			Success:  msg.success,
			MediaKey: msg.mediaKey,
		}
		if msg.success {
			m.completed++
		} else {
			m.failed++
			if msg.err != nil {
				result.Error = msg.err.Error()
			}
		}
		m.results = append(m.results, result)
		return m, nil

	case uploadCompleteMsg:
		m.quitting = true
		return m, tea.Quit

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m uploadModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	b.WriteString(titleStyle.Render("Uploading to Google Photos"))
	b.WriteString("\n\n")

	// Progress bar
	if m.totalFiles > 0 {
		percent := float64(m.completed+m.failed) / float64(m.totalFiles)
		b.WriteString(m.progress.ViewAs(percent))
		b.WriteString(fmt.Sprintf("\n%d/%d files", m.completed+m.failed, m.totalFiles))
		b.WriteString(fmt.Sprintf(" (✓ %d success, ✗ %d failed)\n\n", m.completed, m.failed))
	}

	// Worker status
	for i := 0; i < len(m.workers); i++ {
		if status, ok := m.workers[i]; ok {
			b.WriteString(status)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\nPress Ctrl+C to cancel\n")

	return b.String()
}

// parseLogLevel converts a string log level to slog.Level
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		// Default to info for CLI
		return slog.LevelInfo
	}
}

// CLI upload implementation
func runCLIUpload(filePaths []string, config cliConfig) error {
	// Set custom config path if provided
	if config.configPath != "" {
		backend.ConfigPath = config.configPath
	}

	// Load backend config
	err := backend.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with CLI flags
	backend.AppConfig.Recursive = config.recursive
	backend.AppConfig.UploadThreads = config.threads
	backend.AppConfig.ForceUpload = config.forceUpload
	backend.AppConfig.DeleteFromHost = config.deleteFromHost
	backend.AppConfig.DisableUnsupportedFilesFilter = config.disableUnsupportedFilesFilter

	// Parse log level
	logLevel := parseLogLevel(config.logLevel)

	// Start bubbletea program
	model := initialModel()
	p := tea.NewProgram(model)

	// Create CLI app with event callback to bubbletea
	eventCallback := func(event string, data any) {
		switch event {
		case "uploadStart":
			if start, ok := data.(backend.UploadBatchStart); ok {
				p.Send(uploadStartMsg{total: start.Total})
			}
		case "ThreadStatus":
			if status, ok := data.(backend.ThreadStatus); ok {
				fileName := status.FileName
				// No truncation - show full filename
				p.Send(fileProgressMsg{
					workerID: status.WorkerID,
					status:   status.Status,
					fileName: fileName,
					message:  status.Message,
				})
			}
		case "FileStatus":
			if result, ok := data.(backend.FileUploadResult); ok {
				p.Send(fileCompleteMsg{
					success:  !result.IsError,
					fileName: result.Path,
					mediaKey: result.MediaKey,
					err:      result.Error,
				})
			}
		case "uploadStop":
			p.Send(uploadCompleteMsg{})
		}
	}

	cliApp := backend.NewCLIApp(eventCallback, logLevel)
	uploadManager := backend.NewUploadManager(cliApp)

	// Run upload in background
	go func() {
		uploadManager.Upload(cliApp, filePaths)
	}()

	// Run the TUI
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	// Print JSON summary after TUI completes
	if m, ok := finalModel.(uploadModel); ok {
		summary := uploadSummary{
			Total:     m.totalFiles,
			Succeeded: m.completed,
			Failed:    m.failed,
			Results:   m.results,
		}

		jsonOutput, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return fmt.Errorf("error generating JSON: %w", err)
		}

		fmt.Println(string(jsonOutput))
	}

	return nil
}

// CLI download implementation
func runCLIDownload(mediaKey, outputPath string, original bool) error {
	// Load backend config
	err := backend.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create API client
	api, err := backend.NewApi()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Get download URLs
	fmt.Printf("Getting download URLs for media key: %s\n", mediaKey)
	urls, err := api.GetDownloadURLs(mediaKey)
	if err != nil {
		return fmt.Errorf("failed to get download URLs: %w", err)
	}

	// Select the appropriate URL
	downloadURL := urls.EditedURL
	if original && urls.OriginalURL != "" {
		downloadURL = urls.OriginalURL
	}

	if downloadURL == "" {
		return fmt.Errorf("no download URL available")
	}

	// Determine output path if not specified
	if outputPath == "" {
		// Try to extract extension from URL, otherwise default to .bin
		ext := ".bin"
		if idx := strings.LastIndex(downloadURL, "."); idx != -1 {
			possibleExt := downloadURL[idx:]
			// Only use if it looks like a file extension (e.g., .jpg, .mp4)
			if len(possibleExt) <= 5 && len(possibleExt) > 1 {
				// Extract just the extension without query params
				if qIdx := strings.Index(possibleExt, "?"); qIdx != -1 {
					possibleExt = possibleExt[:qIdx]
				}
				if len(possibleExt) > 1 {
					ext = possibleExt
				}
			}
		}
		outputPath = mediaKey + ext
	}

	// Download the file
	fmt.Printf("Downloading to: %s\n", outputPath)
	err = api.DownloadFile(downloadURL, outputPath)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	fmt.Printf("✓ Downloaded successfully: %s\n", outputPath)
	return nil
}

// CLI list implementation
func runCLIList(pageToken string, limit int, pages int, maxEmptyPages int, jsonOutput bool) error {
	// Load backend config
	err := backend.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create API client
	api, err := backend.NewApi()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Collect all items across pages
	var allItems []backend.MediaItem
	currentPageToken := pageToken
	lastNextPageToken := ""        // Track the next page token from the last API response
	pagesRequested := 0
	emptyPageRetries := 0          // Count consecutive empty pages

	for pagesRequested < pages {
		// Get media list
		if !jsonOutput {
			if pagesRequested == 0 {
				fmt.Println("Fetching media list...")
			} else {
				fmt.Printf("Fetching page %d...\n", pagesRequested+1)
			}
		}

		result, err := api.GetMediaList(currentPageToken, limit)
		if err != nil {
			return fmt.Errorf("failed to get media list: %w", err)
		}

		// Always track the latest next page token from API response
		lastNextPageToken = result.NextPageToken

		// If page has 0 items but has a next page token, auto-skip to next page
		if len(result.Items) == 0 && result.NextPageToken != "" {
			if !jsonOutput {
				fmt.Println("Page has 0 items, automatically fetching next page...")
			}
			currentPageToken = result.NextPageToken
			emptyPageRetries++
			if emptyPageRetries >= maxEmptyPages {
				if !jsonOutput {
					fmt.Printf("Reached max empty pages limit (%d), stopping.\n", maxEmptyPages)
				}
				break
			}
			continue
		}

		// Reset empty page counter after finding items
		emptyPageRetries = 0

		allItems = append(allItems, result.Items...)
		pagesRequested++

		// If there's no next page token, stop
		if result.NextPageToken == "" {
			if !jsonOutput && pagesRequested < pages {
				fmt.Println("No more pages available.")
			}
			break
		}

		// Update page token for next iteration
		currentPageToken = result.NextPageToken
	}

	// Create final result
	finalResult := &backend.MediaListResult{
		Items:         allItems,
		NextPageToken: lastNextPageToken,
	}

	if jsonOutput {
		// Output as JSON
		jsonBytes, err := json.MarshalIndent(finalResult, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		// Human-readable output
		fmt.Printf("\nFound %d media items:\n\n", len(finalResult.Items))

		for i, item := range finalResult.Items {
			fmt.Printf("%d. %s\n", i+1, item.MediaKey)
			if item.Filename != "" {
				fmt.Printf("   Filename: %s\n", item.Filename)
			}
			if item.MediaType != "" {
				fmt.Printf("   Type: %s\n", item.MediaType)
			}
			if item.DedupKey != "" {
				fmt.Printf("   Dedup Key: %s\n", item.DedupKey)
			}
			fmt.Println()
		}

		if finalResult.NextPageToken != "" {
			fmt.Printf("Next page token: %s\n", finalResult.NextPageToken)
			fmt.Printf("Use: gotohp list --page-token \"%s\" to get the next page\n", finalResult.NextPageToken)
		}
	}

	return nil
}
