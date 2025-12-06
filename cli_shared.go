package main

import (
	"app/backend"
	"embed"
	"fmt"
	"os"
	"slices"
	"strings"
)

//go:embed build/windows/info.json
var versionInfo embed.FS

// getAppVersion returns version from embedded info.json
func getAppVersion() string {
	return backend.GetVersion(versionInfo)
}

func isCLICommand(arg string) bool {
	supportedCommands := []string{
		"upload",
		"download",
		"thumbnail", "thumb", // Get thumbnail at various sizes
		"list", "ls",         // List media items
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
			fmt.Println("Usage: gotohp upload <filepath> [flags]")
			fmt.Println("\nFlags:")
			fmt.Println("  -r, --recursive              Include subdirectories")
			fmt.Println("  -t, --threads <n>            Number of upload threads (default: 3)")
			fmt.Println("  -f, --force                  Force upload even if file exists")
			fmt.Println("  -d, --delete                 Delete from host after upload")
			fmt.Println("  -df, --disable-filter        Disable file type filtering")
			fmt.Println("  -l, --log-level <level>      Set log level: debug, info, warn, error (default: info)")
			fmt.Println("  -c, --config <path>          Path to config file")
			return
		}

		if len(os.Args) < 3 {
			fmt.Println("Error: filepath required")
			fmt.Println("Usage: gotohp upload <filepath> [flags]")
			fmt.Println("\nRun 'gotohp upload --help' for more information")
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
		limit := 0           // 0 means no limit
		pages := 1           // Default to 1 page
		maxEmptyPages := 10  // Default max empty page retries
		pageToken := ""
		jsonOutput := false

		for i := 2; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--config", "-c":
				if i+1 < len(os.Args) {
					configPath = os.Args[i+1]
					i++
				}
			case "--limit", "-n":
				if i+1 < len(os.Args) {
					_, err := fmt.Sscanf(os.Args[i+1], "%d", &limit)
					if err != nil || limit < 0 {
						fmt.Fprintf(os.Stderr, "Warning: invalid limit value '%s', using no limit\n", os.Args[i+1])
						limit = 0
					}
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
		err := runCLIList(pageToken, limit, pages, maxEmptyPages, jsonOutput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "List failed: %v\n", err)
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
	fmt.Println("gotohp - Google Photos unofficial client")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  gotohp              Launch GUI application")
	fmt.Println("  gotohp <command>    Run CLI command")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  upload <filepath>   Upload a file to Google Photos")
	fmt.Println("  download <media-key> Download a file from Google Photos")
	fmt.Println("  thumbnail <media-key> Download a thumbnail at various sizes")
	fmt.Println("  list, ls            List media items in Google Photos")
	fmt.Println("  creds               Manage Google Photos credentials")
	fmt.Println("  help                Show this help message")
	fmt.Println("  version             Show version information")
	fmt.Println()
	fmt.Println("Run 'gotohp <command> --help' for more information on a command")
}

func printDownloadHelp() {
	fmt.Println("Usage: gotohp download <media-key> [flags]")
	fmt.Println()
	fmt.Println("Download a file from Google Photos using its media key.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -o, --output <path>  Output file path (default: current directory with original filename)")
	fmt.Println("  --original           Download the original file instead of the edited version")
	fmt.Println("  -c, --config <path>  Path to config file")
}

func printThumbnailHelp() {
	fmt.Println("Usage: gotohp thumbnail <media-key> [flags]")
	fmt.Println()
	fmt.Println("Download a thumbnail of a media item at various sizes.")
	fmt.Println()
	fmt.Println("Size Presets (--size, -s):")
	fmt.Println("  small    - 128x128 pixels")
	fmt.Println("  medium   - 256x256 pixels")
	fmt.Println("  large    - 512x512 pixels")
	fmt.Println("  xlarge   - 1024x1024 pixels")
	fmt.Println("  xxlarge  - 2048x2048 pixels")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -o, --output <path>   Output file path (default: <media-key>_thumb.jpg)")
	fmt.Println("  -s, --size <preset>   Use a size preset (small, medium, large, xlarge, xxlarge)")
	fmt.Println("  -w, --width <pixels>  Custom thumbnail width in pixels")
	fmt.Println("  -h, --height <pixels> Custom thumbnail height in pixels (note: use after -w to avoid conflict with --help)")
	fmt.Println("  --overlay             Include overlay (e.g., play symbol for videos)")
	fmt.Println("  --png                 Get PNG format instead of JPEG")
	fmt.Println("  -c, --config <path>   Path to config file")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  gotohp thumbnail ABC123 --size medium")
	fmt.Println("  gotohp thumbnail ABC123 -w 640 -h 480")
	fmt.Println("  gotohp thumbnail ABC123 --size large -o photo_thumb.jpg")
}

func printListHelp() {
	fmt.Println("Usage: gotohp list [flags]")
	fmt.Println()
	fmt.Println("List media items in your Google Photos library.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -n, --limit <n>          Maximum number of items to return per page")
	fmt.Println("  --pages <n>              Number of pages to fetch (default: 1)")
	fmt.Println("  --max-empty-pages <n>    Maximum consecutive empty pages to skip (default: 10)")
	fmt.Println("  -p, --page-token <t>     Page token for pagination")
	fmt.Println("  -j, --json               Output in JSON format")
	fmt.Println("  -c, --config <path>      Path to config file")
	fmt.Println()
	fmt.Println("If a page returns 0 items, the next page will be fetched automatically.")
}

func printCredentialsHelp() {
	fmt.Println("Usage: gotohp creds <subcommand> [args]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  add <auth-string>       Add a new credential")
	fmt.Println("  remove, rm <email>      Remove a credential by email")
	fmt.Println("  list, ls                List all credentials")
	fmt.Println("  set, select <email>     Set active credential (supports partial matching)")
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
