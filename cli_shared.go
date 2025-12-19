package main

import (
	"app/backend"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

//go:embed build/windows/info.json
var versionInfo embed.FS

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	commandStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	flagStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	argStyle     = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("245"))
	exampleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// getAppVersion returns version from embedded info.json
func getAppVersion() string {
	return backend.GetVersion(versionInfo)
}

func isCLICommand(arg string) bool {
	supportedCommands := []string{
		"upload",
		"download",
		"thumbnail", "thumb", // Get thumbnail at various sizes
		"list", "ls", // List media items
		"albums", // List albums
		"autowash", // Start auto-wash service
		"credentials", "creds", // Support both full and short form
		"help", "--help", "-h",
		"version", "--version", "-v",
	}

	return slices.Contains(supportedCommands, arg)
}

func runCLI() {
	if len(os.Args) < 2 {
		printCLIHelp()
		os.Exit(1)
		return
	}

	command := os.Args[1]

	switch command {
	case "upload":
		// Check for help flag first
		if len(os.Args) > 2 && (os.Args[2] == "--help" || os.Args[2] == "-h") {
			printUploadHelp()
			return
		}

		if len(os.Args) < 3 {
			fmt.Println("Error: filepath required")
			printUploadHelp()
			os.Exit(1)
		}

		// Parse arguments
		filePath := os.Args[2]

		// Validate that filepath exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: file or directory does not exist: %s\n", filePath)
			os.Exit(1)
		}

		filePaths := []string{filePath}
		config := cliConfig{
			threads:  3,
			logLevel: "info", // Default to info for CLI
		}

		// Parse flags
		for i := 3; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--recursive", "-r":
				config.recursive = true
			case "--force", "-f":
				config.forceUpload = true
			case "--delete", "-d":
				config.deleteFromHost = true
			case "--disable-filter", "-df":
				config.disableUnsupportedFilesFilter = true
			case "--threads", "-t":
				if i+1 < len(os.Args) {
					fmt.Sscanf(os.Args[i+1], "%d", &config.threads)
					i++
				}
			case "--log-level", "-l":
				if i+1 < len(os.Args) {
					config.logLevel = os.Args[i+1]
					i++
				}
			case "--config", "-c":
				if i+1 < len(os.Args) {
					config.configPath = os.Args[i+1]
					i++
				}
			}
		}

		// Run upload
		err := runCLIUpload(filePaths, config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Upload failed: %v\n", err)
			os.Exit(1)
		}

	case "download":
		// Check for help flag first
		if len(os.Args) > 2 && (os.Args[2] == "--help" || os.Args[2] == "-h") {
			printDownloadHelp()
			return
		}

		if len(os.Args) < 3 {
			fmt.Println("Error: media-key required")
			printDownloadHelp()
			os.Exit(1)
		}

		mediaKey := os.Args[2]
		outputPath := ""
		original := false
		configPath := ""

		// Parse flags
		for i := 3; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--output", "-o":
				if i+1 < len(os.Args) {
					outputPath = os.Args[i+1]
					i++
				}
			case "--original":
				original = true
			case "--config", "-c":
				if i+1 < len(os.Args) {
					configPath = os.Args[i+1]
					i++
				}
			}
		}

		// Set custom config path if provided
		if configPath != "" {
			backend.ConfigPath = configPath
		}

		// Run download
		err := runCLIDownload(mediaKey, outputPath, original)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Download failed: %v\n", err)
			os.Exit(1)
		}

	case "thumbnail", "thumb":
		// Check for help flag first
		if len(os.Args) > 2 && (os.Args[2] == "--help" || os.Args[2] == "-h") {
			printThumbnailHelp()
			return
		}

		if len(os.Args) < 3 {
			fmt.Println("Error: media-key required")
			printThumbnailHelp()
			os.Exit(1)
		}

		mediaKey := os.Args[2]
		outputPath := ""
		width := 0
		height := 0
		size := ""
		noOverlay := true
		forceJPEG := true
		configPath := ""

		// Parse flags
		for i := 3; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--output", "-o":
				if i+1 < len(os.Args) {
					outputPath = os.Args[i+1]
					i++
				}
			case "--width", "-w":
				if i+1 < len(os.Args) {
					if _, err := fmt.Sscanf(os.Args[i+1], "%d", &width); err != nil || width <= 0 {
						fmt.Fprintf(os.Stderr, "Warning: invalid width '%s', ignoring\n", os.Args[i+1])
						width = 0
					}
					i++
				}
			case "--height", "-h":
				if i+1 < len(os.Args) {
					if _, err := fmt.Sscanf(os.Args[i+1], "%d", &height); err != nil || height <= 0 {
						fmt.Fprintf(os.Stderr, "Warning: invalid height '%s', ignoring\n", os.Args[i+1])
						height = 0
					}
					i++
				}
			case "--size", "-s":
				if i+1 < len(os.Args) {
					size = os.Args[i+1]
					i++
				}
			case "--overlay":
				noOverlay = false
			case "--png":
				forceJPEG = false
			case "--config", "-c":
				if i+1 < len(os.Args) {
					configPath = os.Args[i+1]
					i++
				}
			}
		}

		// Set custom config path if provided
		if configPath != "" {
			backend.ConfigPath = configPath
		}

		// Run thumbnail download
		err := runCLIThumbnail(mediaKey, outputPath, width, height, size, noOverlay, forceJPEG)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Thumbnail download failed: %v\n", err)
			os.Exit(1)
		}

		case "list", "ls":
		// Check for help flag first
		if len(os.Args) > 2 && (os.Args[2] == "--help" || os.Args[2] == "-h") {
			printListHelp()
			return
		}

			// Parse flags
			configPath := ""
			pages := 1          // Default to 1 page
			maxEmptyPages := 10 // Default max empty page retries
			pageToken := ""
			jsonOutput := false

		for i := 2; i < len(os.Args); i++ {
			switch os.Args[i] {
				case "--config", "-c":
					if i+1 < len(os.Args) {
						configPath = os.Args[i+1]
						i++
					}
				case "--pages":
					if i+1 < len(os.Args) {
						_, err := fmt.Sscanf(os.Args[i+1], "%d", &pages)
						if err != nil || pages < 1 {
						fmt.Fprintf(os.Stderr, "Warning: invalid pages value '%s', using 1\n", os.Args[i+1])
						pages = 1
					}
					i++
				}
			case "--max-empty-pages":
				if i+1 < len(os.Args) {
					_, err := fmt.Sscanf(os.Args[i+1], "%d", &maxEmptyPages)
					if err != nil || maxEmptyPages < 1 {
						fmt.Fprintf(os.Stderr, "Warning: invalid max-empty-pages value '%s', using 10\n", os.Args[i+1])
						maxEmptyPages = 10
					}
					i++
				}
			case "--page-token", "-p":
				if i+1 < len(os.Args) {
					pageToken = os.Args[i+1]
					i++
				}
			case "--json", "-j":
				jsonOutput = true
			}
		}

		// Set custom config path if provided
		if configPath != "" {
			backend.ConfigPath = configPath
			}

			// Run list
			err := runCLIList(pageToken, pages, maxEmptyPages, jsonOutput)
			if err != nil {
				fmt.Fprintf(os.Stderr, "List failed: %v\n", err)
				os.Exit(1)
			}

	case "albums":
		// Check for help flag first
		if len(os.Args) > 2 && (os.Args[2] == "--help" || os.Args[2] == "-h") {
			printAlbumsHelp()
			return
		}

		// Parse flags
		configPath := ""
		pages := 1 // Default to 1 page
		pageToken := ""
		jsonOutput := false

		for i := 2; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--config", "-c":
				if i+1 < len(os.Args) {
					configPath = os.Args[i+1]
					i++
				}
			case "--pages":
				if i+1 < len(os.Args) {
					_, err := fmt.Sscanf(os.Args[i+1], "%d", &pages)
					if err != nil || pages < 1 {
						fmt.Fprintf(os.Stderr, "Warning: invalid pages value '%s', using 1\n", os.Args[i+1])
						pages = 1
					}
					i++
				}
			case "--page-token":
				if i+1 < len(os.Args) {
					pageToken = os.Args[i+1]
					i++
				}
			case "--json", "-j":
				jsonOutput = true
			default:
				fmt.Fprintf(os.Stderr, "Warning: unknown flag '%s'\n", os.Args[i])
			}
		}

		if configPath != "" {
			backend.ConfigPath = configPath
		}

		err := backend.LoadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
			os.Exit(1)
		}

		mediaBrowser := &backend.MediaBrowser{}
		currentPageToken := pageToken

		for page := 0; page < pages; page++ {
			result, err := mediaBrowser.GetAlbumList(currentPageToken)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to get album list: %v\n", err)
				os.Exit(1)
			}

			if jsonOutput {
				fmt.Printf("%+v\n", result)
			} else {
				if page == 0 {
					fmt.Printf("Albums (Page %d):\n", page+1)
				} else {
					fmt.Printf("\nAlbums (Page %d):\n", page+1)
				}
				for i, album := range result.Albums {
					fmt.Printf("%d. %s\n", i+1, album.Title)
					fmt.Printf("   Key: %s\n", album.AlbumKey)
					if album.MediaCount > 0 {
						fmt.Printf("   Media Count: %d\n", album.MediaCount)
					}
				}
				fmt.Printf("\nTotal albums on this page: %d\n", len(result.Albums))
				if result.NextPageToken != "" {
					fmt.Printf("Next page token: %s\n", result.NextPageToken)
				}
			}

			// Check if there are more pages
			if result.NextPageToken == "" {
				if !jsonOutput && page+1 < pages {
					fmt.Println("\nNo more pages available.")
				}
				break
			}

			currentPageToken = result.NextPageToken
		}

	case "autowash":
		// Check for help
		if len(os.Args) > 2 && (os.Args[2] == "--help" || os.Args[2] == "-h") {
			printAutoWashHelp()
			return
		}

		// Defaults
		config := backend.AutoWashConfig{
			Interval:      1 * time.Hour,
			DbPath:        "media_db.json",
			BackupDir:     "Downloads/gotohp_backup",
			RetentionDays: 7,
		}
		
		if home, err := os.UserHomeDir(); err == nil {
			config.BackupDir = filepath.Join(home, "Downloads", "gotohp_backup")
		}

		configPath := ""

		// Parse flags
		for i := 2; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--interval", "-i":
				if i+1 < len(os.Args) {
					d, err := time.ParseDuration(os.Args[i+1])
					if err == nil {
						config.Interval = d
					} else {
						fmt.Fprintf(os.Stderr, "Invalid interval '%s', using default 1h\n", os.Args[i+1])
					}
					i++
				}
			case "--db":
				if i+1 < len(os.Args) {
					config.DbPath = os.Args[i+1]
					i++
				}
			case "--backup-dir":
				if i+1 < len(os.Args) {
					config.BackupDir = os.Args[i+1]
					i++
				}
			case "--retention", "-r":
				if i+1 < len(os.Args) {
					fmt.Sscanf(os.Args[i+1], "%d", &config.RetentionDays)
					i++
				}
			case "--config", "-c":
				if i+1 < len(os.Args) {
					configPath = os.Args[i+1]
					i++
				}
			}
		}

		if configPath != "" {
			backend.ConfigPath = configPath
		}
		
		// Load config (credentials needed)
		if err := backend.LoadConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
			os.Exit(1)
		}

		if err := backend.RunAutoWash(config); err != nil {
			fmt.Fprintf(os.Stderr, "Auto-wash service error: %v\n", err)
			os.Exit(1)
		}

	case "credentials", "creds":
		if len(os.Args) < 3 {
			fmt.Println("Error: subcommand required")
			printCredentialsHelp()
			os.Exit(1)
		}
		// Parse config flag before handling credentials
		var configPath string
		args := os.Args[2:]
		for i := 0; i < len(args); i++ {
			if args[i] == "--config" || args[i] == "-c" {
				if i+1 < len(args) {
					configPath = args[i+1]
					// Remove config flag from args
					args = append(args[:i], args[i+2:]...)
					break
				}
			}
		}
		if configPath != "" {
			backend.ConfigPath = configPath
		}
		handleCredentialsCommand(args)

	case "help", "--help", "-h":
		printCLIHelp()
	case "version", "--version", "-v":
		fmt.Printf("gotohp v%s\n", getAppVersion())
	default:
		fmt.Printf("Error: unknown command '%s'\n\n", command)
		printCLIHelp()
		os.Exit(1)
	}
}

func containsSubstring(str, substr string) bool {
	// Case-insensitive substring search
	strLower := strings.ToLower(str)
	substrLower := strings.ToLower(substr)
	return strings.Contains(strLower, substrLower)
}

func printCLIHelp() {
	fmt.Println(titleStyle.Render("gotohp - Unofficial Google Photos CLI & GUI"))
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s              Launch GUI application\n", commandStyle.Render("gotohp"))
	fmt.Printf("  %s %s    Run CLI command\n", commandStyle.Render("gotohp"), argStyle.Render("<command>"))
	fmt.Println()
	fmt.Println("Core Commands:")
	fmt.Printf("  %s          Upload files or directories to Google Photos\n", commandStyle.Render("upload"))
	fmt.Printf("  %s        Download a file from Google Photos by media key\n", commandStyle.Render("download"))
	fmt.Printf("  %s       List media items in your library\n", commandStyle.Render("list, ls"))
	fmt.Printf("  %s          List your albums\n", commandStyle.Render("albums"))
	fmt.Println()
	fmt.Println("Advanced Commands:")
	fmt.Printf("  %s       Manage Google Photos credentials/accounts\n", commandStyle.Render("creds"))
	fmt.Printf("  %s       Start auto-sync and backup service\n", commandStyle.Render("autowash"))
	fmt.Printf("  %s   Download a thumbnail (various sizes available)\n", commandStyle.Render("thumbnail"))
	fmt.Println()
	fmt.Println("System Commands:")
	fmt.Printf("  %s            Show this help message\n", commandStyle.Render("help, -h"))
	fmt.Printf("  %s         Show version information\n", commandStyle.Render("version, -v"))
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  Use 'gotohp <command> --help' for detailed information on any command.")
}

func printFlag(short, long, arg, description string) {
	var flagPart string
	if short != "" {
		flagPart = fmt.Sprintf("  %s, %s", flagStyle.Render(short), flagStyle.Render(long))
	} else {
		flagPart = fmt.Sprintf("      %s", flagStyle.Render(long))
	}
	
	if arg != "" {
		flagPart += " " + argStyle.Render(arg)
	}
	
	// Pad flagPart to a consistent width
	plainFlagPart := lipgloss.ClearString(flagPart)
	padding := 30 - len(plainFlagPart)
	if padding < 1 {
		padding = 1
	}
	
	fmt.Printf("%s%s%s\n", flagPart, strings.Repeat(" ", padding), description)
}

func printUploadHelp() {
	fmt.Printf("Usage: %s %s %s %s\n", commandStyle.Render("gotohp"), commandStyle.Render("upload"), argStyle.Render("<filepath>"), flagStyle.Render("[flags]"))
	fmt.Println()
	fmt.Println("Upload files or directories to Google Photos.")
	fmt.Println()
	fmt.Println("Flags:")
	printFlag("-r", "--recursive", "", "Include subdirectories")
	printFlag("-t", "--threads", "<n>", "Number of upload threads (default: 3)")
	printFlag("-f", "--force", "", "Force upload even if file exists")
	printFlag("-d", "--delete", "", "Delete from host after upload")
	printFlag("-df", "--disable-filter", "", "Disable file type filtering")
	printFlag("-l", "--log-level", "<level>", "Set log level: debug, info, warn, error (default: info)")
	printFlag("-c", "--config", "<path>", "Path to config file")
}

func printAutoWashHelp() {
	fmt.Printf("Usage: %s %s %s\n", commandStyle.Render("gotohp"), commandStyle.Render("autowash"), flagStyle.Render("[flags]"))
	fmt.Println()
	fmt.Println("Start auto-wash service to automatically sync library and backup media.")
	fmt.Println()
	fmt.Println("Flags:")
	printFlag("-i", "--interval", "<duration>", "Check interval (default: 1h, e.g. 30m, 2h)")
	printFlag("", "--db", "<path>", "Database file path (default: media_db.json)")
	printFlag("", "--backup-dir", "<path>", "Directory for temporary downloads (default: Downloads/gotohp_backup)")
	printFlag("-r", "--retention", "<days>", "Days to keep downloaded files (default: 7)")
	printFlag("-c", "--config", "<path>", "Path to config file")
}

func printDownloadHelp() {
	fmt.Printf("Usage: %s %s %s %s\n", commandStyle.Render("gotohp"), commandStyle.Render("download"), argStyle.Render("<media-key>"), flagStyle.Render("[flags]"))
	fmt.Println()
	fmt.Println("Download a file from Google Photos using its media key.")
	fmt.Println()
	fmt.Println("Flags:")
	printFlag("-o", "--output", "<path>", "Output file path (default: original filename)")
	printFlag("", "--original", "", "Download the original file instead of edited")
	printFlag("-c", "--config", "<path>", "Path to config file")
}

func printThumbnailHelp() {
	fmt.Printf("Usage: %s %s %s %s\n", commandStyle.Render("gotohp"), commandStyle.Render("thumbnail"), argStyle.Render("<media-key>"), flagStyle.Render("[flags]"))
	fmt.Println()
	fmt.Println("Download a thumbnail of a media item at various sizes.")
	fmt.Println()
	fmt.Println("Size Presets (--size, -s):")
	fmt.Printf("  %-10s - 50x50 pixels\n", argStyle.Render("small"))
	fmt.Printf("  %-10s - 800x800 pixels\n", argStyle.Render("medium"))
	fmt.Printf("  %-10s - 1600x1600 pixels\n", argStyle.Render("large"))
	fmt.Println()
	fmt.Println("Flags:")
	printFlag("-o", "--output", "<path>", "Output file path")
	printFlag("-s", "--size", "<preset>", "Use a size preset (small, medium, large)")
	printFlag("-w", "--width", "<pixels>", "Custom thumbnail width in pixels")
	printFlag("-h", "--height", "<pixels>", "Custom thumbnail height in pixels")
	printFlag("", "--overlay", "", "Include overlay (e.g., play symbol for videos)")
	printFlag("", "--png", "", "Get PNG format instead of JPEG")
	printFlag("-c", "--config", "<path>", "Path to config file")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s\n", exampleStyle.Render("gotohp thumbnail ABC123 --size medium"))
	fmt.Printf("  %s\n", exampleStyle.Render("gotohp thumbnail ABC123 -w 640 -h 480"))
}

func printListHelp() {
	fmt.Printf("Usage: %s %s %s\n", commandStyle.Render("gotohp"), commandStyle.Render("list"), flagStyle.Render("[flags]"))
	fmt.Println()
	fmt.Println("List media items in your Google Photos library.")
	fmt.Println()
	fmt.Println("Flags:")
	printFlag("", "--pages", "<n>", "Number of pages to fetch (default: 1)")
	printFlag("", "--max-empty-pages", "<n>", "Max consecutive empty pages to skip (default: 10)")
	printFlag("-p", "--page-token", "<t>", "Page token for pagination")
	printFlag("-j", "--json", "", "Output in JSON format")
	printFlag("-c", "--config", "<path>", "Path to config file")
	fmt.Println()
	fmt.Println("Note: If a page returns 0 items, the next page will be fetched automatically.")
}

func printAlbumsHelp() {
	fmt.Printf("Usage: %s %s %s\n", commandStyle.Render("gotohp"), commandStyle.Render("albums"), flagStyle.Render("[flags]"))
	fmt.Println()
	fmt.Println("List albums in your Google Photos library.")
	fmt.Println()
	fmt.Println("Flags:")
	printFlag("", "--pages", "<n>", "Number of pages to fetch (default: 1)")
	printFlag("", "--page-token", "<t>", "Page token for pagination")
	printFlag("-j", "--json", "", "Output in JSON format")
	printFlag("-c", "--config", "<path>", "Path to config file")
}

func printCredentialsHelp() {
	fmt.Printf("Usage: %s %s %s %s\n", commandStyle.Render("gotohp"), commandStyle.Render("creds"), argStyle.Render("<subcommand>"), flagStyle.Render("[args]"))
	fmt.Println()
	fmt.Println("Manage Google Photos credentials and accounts.")
	fmt.Println()
	fmt.Println("Subcommands:")
	
	printSubcommand := func(cmd, arg, desc string) {
		fullCmd := commandStyle.Render(cmd)
		if arg != "" {
			fullCmd += " " + argStyle.Render(arg)
		}
		padding := 30 - len(lipgloss.ClearString(fullCmd))
		if padding < 1 {
			padding = 1
		}
		fmt.Printf("  %s%s%s\n", fullCmd, strings.Repeat(" ", padding), desc)
	}

	printSubcommand("add", "<auth-string>", "Add a new credential")
	printSubcommand("remove, rm", "<email>", "Remove a credential by email")
	printSubcommand("list, ls", "", "List all stored credentials")
	printSubcommand("set, select", "<email>", "Set active credential")
	
	fmt.Println()
	fmt.Println("Flags:")
	printFlag("-c", "--config", "<path>", "Path to config file")
}

func handleCredentialsCommand(args []string) {
	if len(args) == 0 {
		printCredentialsHelp()
		os.Exit(1)
	}

	// Load config
	err := backend.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	configManager := &backend.ConfigManager{}
	subcommand := args[0]

	switch subcommand {
	case "add":
		if len(args) < 2 {
			fmt.Println("Error: auth-string required")
			fmt.Println("Usage: gotohp credentials add <auth-string>")
			os.Exit(1)
		}
		authString := args[1]
		err := configManager.AddCredentials(authString)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding credentials: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Credentials added successfully")

	case "remove", "rm":
		if len(args) < 2 {
			fmt.Println("Error: email required")
			fmt.Println("Usage: gotohp credentials remove <email>")
			os.Exit(1)
		}
		email := args[1]
		err := configManager.RemoveCredentials(email)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error removing credentials: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Credentials for %s removed successfully\n", email)

	case "list", "ls":
		config := configManager.GetConfig()
		if len(config.Credentials) == 0 {
			fmt.Println("No credentials found")
			return
		}
		fmt.Println("Credentials:")
		for i, cred := range config.Credentials {
			params, err := backend.ParseAuthString(cred)
			if err != nil {
				fmt.Printf("  %d. [Invalid credential]\n", i+1)
				continue
			}
			email := params.Get("Email")
			marker := " "
			if email == config.Selected {
				marker = "*"
			}
			fmt.Printf("  %s %s\n", marker, email)
		}
		if config.Selected != "" {
			fmt.Printf("\n* = active\n")
		}
		fmt.Printf("\nUse 'gotohp creds set <email>' to change active account (supports partial matching)\n")

	case "set", "select":
		if len(args) < 2 {
			fmt.Println("Error: email required")
			fmt.Println("Usage: gotohp creds set <email>")
			os.Exit(1)
		}
		query := args[1]
		config := configManager.GetConfig()

		// Try to find exact match first
		var matchedEmail string
		for _, cred := range config.Credentials {
			params, err := backend.ParseAuthString(cred)
			if err != nil {
				continue
			}
			email := params.Get("Email")
			if email == query {
				matchedEmail = email
				break
			}
		}

		// If no exact match, try fuzzy matching (substring match)
		if matchedEmail == "" {
			var candidates []string
			for _, cred := range config.Credentials {
				params, err := backend.ParseAuthString(cred)
				if err != nil {
					continue
				}
				email := params.Get("Email")
				// Check if query is a substring of the email
				if containsSubstring(email, query) {
					candidates = append(candidates, email)
				}
			}

			if len(candidates) == 0 {
				fmt.Fprintf(os.Stderr, "Error: no credentials found matching '%s'\n", query)
				os.Exit(1)
			} else if len(candidates) == 1 {
				matchedEmail = candidates[0]
			} else {
				fmt.Fprintf(os.Stderr, "Error: multiple credentials match '%s':\n", query)
				for _, email := range candidates {
					fmt.Fprintf(os.Stderr, "  - %s\n", email)
				}
				fmt.Fprintf(os.Stderr, "Please be more specific\n")
				os.Exit(1)
			}
		}

		configManager.SetSelected(matchedEmail)
		fmt.Printf("✓ Active credential set to %s\n", matchedEmail)

	default:
		fmt.Printf("Error: unknown subcommand '%s'\n\n", subcommand)
		printCredentialsHelp()
		os.Exit(1)
	}
}
