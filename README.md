# gotohp

![demo](readme_assets/app_demo.webp)

Unofficial Google Photos Desktop GUI Client

## Features

### GUI Features
- **Upload**: Drag-and-drop file upload interface with real-time progress tracking
- **Gallery**: Browse your Google Photos library with thumbnail previews
- **Download**: Download photos directly to your computer
- **Settings**: Configurable upload and display settings

### Upload Features
- Unlimited uploads (can be disabled)
- Individual files or directories uploads, with optional recursive scanning
- Skips files already present in your account
- Configurable upload threads
- Real-time upload progress tracking

### Gallery Features
- Browse all photos from your Google Photos account
- Adjustable thumbnail sizes (small, medium, large)
- One-click photo download
- Pagination support
- Optional periodic update checks (incremental sync) with quota-item “wash” (download + re-upload)

### General Features
- Credential management for multiple accounts
- CLI mode for advanced users
- Configurable, presistent settings (stored in "%system config path%/gotohp/gotohp.config")  
    You can force local config by creating empty gotohp.config next to executable.

## [Download](https://github.com/xob0t/gotohp/releases/latest)

## CLI Usage

### Windows

Windows releases include a dedicated CLI executable (`gotohp-cli.exe`) for command-line usage:

```cmd
gotohp-cli.exe upload C:\path\to\photos --recursive --threads 5
gotohp-cli.exe creds list
gotohp-cli.exe creds add "androidId=..."
gotohp-cli.exe creds set user@gmail.com
gotohp-cli.exe thumbnail <media-key> --size large
gotohp-cli.exe version
```

**Available commands:**

- `upload <filepath>` - Upload files or directories
  - `-r, --recursive` - Include subdirectories
  - `-t, --threads <n>` - Number of upload threads (default: 3)
  - `-f, --force` - Force upload even if file exists
  - `-d, --delete` - Delete from host after upload
  - `-df, --disable-filter` - Disable file type filtering
  - `-l, --log-level <level>` - Set log level: debug, info, warn, error (default: info)
  - `-c, --config <path>` - Path to config file
- `thumbnail <media-key>` (alias: `thumb`) - Download a thumbnail at various sizes
  - `-s, --size <preset>` - Size preset: small (50px), medium (800px), large (1600px)
  - `-w, --width <pixels>` - Custom thumbnail width
  - `-h, --height <pixels>` - Custom thumbnail height
  - `-o, --output <path>` - Output file path
  - `--overlay` - Include overlay (e.g., play symbol for videos)
  - `--png` - Get PNG format instead of JPEG
  - `-c, --config <path>` - Path to config file
- `list` (alias: `ls`) - List media items in Google Photos
  - `--pages <n>` - Number of pages to fetch (default: 1)
  - `-p, --page-token <t>` - Page token for pagination
  - `-j, --json` - Output in JSON format
  - `-c, --config <path>` - Path to config file
- `albums` - List albums in Google Photos
  - `--pages <n>` - Number of pages to fetch (default: 1)
  - `--page-token <t>` - Page token for pagination
  - `-j, --json` - Output in JSON format
  - `-c, --config <path>` - Path to config file
- `creds list` (alias: `ls`) - List all credentials
- `creds add <auth-string>` - Add new credentials
- `creds remove <email>` (alias: `rm`) - Remove credentials
- `creds set <email>` (alias: `select`) - Set active credential (supports partial matching)
- `version` - Show version information
- `help` - Show help message

### macOS / Linux

The main executable supports CLI mode:

```bash
./gotohp upload /path/to/photos --recursive --threads 5
./gotohp creds list
./gotohp thumbnail <media-key> --size large
./gotohp list --pages 2
./gotohp albums --pages 1
./gotohp version
```

## Requires mobile app credentials to work

You only need to do this once.

### Option 1 - ReVanced. No root required

1. Install Google Photos ReVanced on your android device/emulator.
    - Install GmsCore [https://github.com/ReVanced/GmsCore/releases](https://github.com/ReVanced/GmsCore/releases)
    - Install patched apk [https://github.com/j-hc/revanced-magisk-module/releases](https://github.com/j-hc/revanced-magisk-module/releases) or patch it yourself
2. Connect the device to your PC via ADB.
3. Open the terminal on your PC and execute

    Windows

    ```cmd
    adb logcat | FINDSTR "auth%2Fphotos.native"
    ```

    Linux/Mac

    ```shell
    adb logcat | grep "auth%2Fphotos.native"
    ```

4. If you are already using ReVanced - remove Google Account from GmsCore.
5. Open Google Photos ReVanced on your device and log into your account.
6. One or more identical GmsCore logs should appear in the terminal.
7. Copy text from `androidId=` to the end of the line from any log.
8. That's it! 🎉

### Option 2 - Official apk. Root required

<details>
  <summary><strong>Click to expand</strong></summary>

1. Get a rooted android device or an emulator. Recommended Android versions 9-13
2. Connect the device to your PC via ADB.
3. Install [HTTP Toolkit](https://httptoolkit.com)
4. In HTTP Toolkit, select Intercept - `Android Device via ADB`. Filter traffic with

    ```text
    contains(https://www.googleapis.com/auth/photos.native)
    ```

    Or if you have an older version of Google Photos, try

    ```text
    contains(www.googleapis.com%2Fauth%2Fplus.photos.readwrite)
    ```

5. Open Google Photos app and login with your account.
6. A single request should appear.  
   Copy request body as text.

#### Troubleshooting

- **No Auth Request Intercepted**  
  1. Log out of your Google account.
  2. Log in again.
  3. Try `Android App via Frida` interception method in HTTP Toolkit.

</details>

## Build

Follow official wails3 guide
[https://v3alpha.wails.io/getting-started/installation/](https://v3alpha.wails.io/getting-started/installation/)
