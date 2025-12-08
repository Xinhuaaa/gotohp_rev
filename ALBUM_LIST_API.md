# Album List API Documentation

## Overview

This document describes the `GetAlbumList` API function that retrieves a paginated list of albums from Google Photos.

## API Function

### `GetAlbumList(pageToken string) (*AlbumListResult, error)`

Retrieves a list of albums from Google Photos with pagination support.

**Parameters:**
- `pageToken` (string): Pagination token from the previous response. Use empty string `""` for the first request.

**Returns:**
- `*AlbumListResult`: Contains the list of albums and next page token
- `error`: Error if the request fails

**Example Usage:**

```go
// Get first page of albums
api, err := NewApi()
if err != nil {
    log.Fatal(err)
}

result, err := api.GetAlbumList("")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d albums\n", len(result.Albums))
for _, album := range result.Albums {
    fmt.Printf("Album: %s (Key: %s, Media Count: %d)\n", 
        album.Title, album.AlbumKey, album.MediaCount)
}

// Get next page if available
if result.NextPageToken != "" {
    nextResult, err := api.GetAlbumList(result.NextPageToken)
    if err != nil {
        log.Fatal(err)
    }
    // Process next page...
}
```

## Data Types

### AlbumItem

Represents a single album in Google Photos.

```go
type AlbumItem struct {
    AlbumKey   string `json:"albumKey"`    // Unique identifier for the album
    Title      string `json:"title,omitempty"` // Album name/title
    MediaCount int    `json:"mediaCount,omitempty"` // Number of media items in album
}
```

### AlbumListResult

Contains the result of an album list request.

```go
type AlbumListResult struct {
    Albums        []AlbumItem `json:"albums"`
    NextPageToken string      `json:"nextPageToken,omitempty"` // Token for next page
}
```

## Request Format

The album list request uses a complex protobuf structure. According to the specification:

**Important:** Only field `1.4` (pageToken) changes between requests. All other fields remain constant.

### Pagination Flow

1. **First Request**: Send request with `pageToken = ""`
2. **Response**: Receive list of albums and a `NextPageToken`
3. **Subsequent Requests**: Use the `NextPageToken` from the previous response as `pageToken`
4. **Last Page**: Response will have empty `NextPageToken` indicating no more pages

### Request Structure

The request follows this structure:
- **Field 1**: Main request data (complex nested structure)
  - **Field 1.1**: Media/album metadata options
  - **Field 1.2**: Various options and settings
  - **Field 1.3**: Collection and album options
  - **Field 1.4**: **Pagination token** (THE ONLY CHANGING FIELD)
  - **Field 1.7**: Type identifier (always 2)
  - **Field 1.9 through 1.26**: Various configuration fields
- **Field 2**: Additional options

## MediaBrowser Integration

The `MediaBrowser` service provides a convenient wrapper:

```go
mediaBrowser := &MediaBrowser{}

// Get first page of albums
result, err := mediaBrowser.GetAlbumList("")
if err != nil {
    log.Fatal(err)
}

// Get next page
if result.NextPageToken != "" {
    nextResult, err := mediaBrowser.GetAlbumList(result.NextPageToken)
    // ...
}
```

## Implementation Details

### Protobuf Structure

The implementation uses manual protobuf encoding to construct the exact request format required by Google Photos API. The structure is based on reverse-engineered API calls and includes:

- Multiple levels of nested messages
- Empty field placeholders for metadata options
- Specific integer arrays for configuration
- A complex field numbering scheme (including field 169945741 in field 1.18)

### Response Parsing

The response parser extracts:
1. **Albums**: Array of album items with their metadata
2. **Pagination Token**: Token for fetching the next page (found in field 4 of the response)

The parser uses low-level protobuf wire format reading to handle the complex nested structure.

## Notes

- The exact response structure may vary and the parser attempts to extract album information from multiple possible field locations
- The pagination token from the response should be used in field 1.4 of the next request
- An empty or missing pagination token indicates the last page has been reached
- The protobuf structure is based on the format specification provided in the issue

## Related Functions

- `GetMediaList(pageToken, limit)` - Retrieves media items (photos/videos)
- `GetAlbumInfo(albumKey)` - Get detailed information about a specific album (if implemented)
- `GetAlbumMedia(albumKey, pageToken)` - Get media items within a specific album (if implemented)

## Future Enhancements

Potential improvements:
- Add support for filtering albums
- Add support for sorting albums
- Implement album details retrieval
- Implement listing media items within an album
- Add album creation/modification support
