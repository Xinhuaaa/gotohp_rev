# State Token Pagination Fix

## Problem Summary

The pagination feature was not working correctly because the Go implementation was missing state token support. According to the Python reference implementation (`reference/PYTHON实现`), Google Photos API requires different request patterns for initial and subsequent page requests.

## Root Cause

The Go implementation only used the `get_library_page_init` pattern (field 4 for page_token only), but proper pagination requires the `get_library_page` pattern (fields 4 and 6 for page_token and state_token respectively).

### Python Reference Implementation Comparison

**Initial Page Request (`get_library_page_init`):**
```python
proto_body = {
    "1": {
        # ... metadata fields ...
        "4": page_token,  # Only page token, NO state token
        "7": 2,
        # ... other fields ...
    }
}
```

**Subsequent Page Requests (`get_library_page`):**
```python
proto_body = {
    "1": {
        # ... metadata fields ...
        "4": page_token,    # Page token
        "6": state_token,   # State token - REQUIRED for pagination
        "7": 2,
        # ... other fields ...
    }
}
```

## Solution

Added state token support across all layers of the application:

### 1. Backend API Layer (`backend/api.go`)

**Function Signature Change:**
```go
// Before
func (a *Api) GetMediaList(pageToken string, limit int) (*MediaListResult, error)

// After
func (a *Api) GetMediaList(pageToken string, stateToken string, limit int) (*MediaListResult, error)
```

**Protobuf Request Building:**
```go
func buildMediaListRequestField1(pageToken string, stateToken string, limit int) []byte {
    // ... existing fields ...
    
    // field1.4 - page token (string)
    if pageToken != "" {
        writeProtobufString(&buf, 4, pageToken)
    }

    // field1.6 - state token (string) - NEW!
    if stateToken != "" {
        writeProtobufString(&buf, 6, stateToken)
    }
    
    // ... remaining fields ...
}
```

### 2. MediaBrowser Wrapper (`backend/mediabrowser.go`)

```go
// Updated to accept and pass state token
func (m *MediaBrowser) GetMediaList(pageToken string, stateToken string, limit int) (*MediaListResult, error) {
    // ...
    result, err := api.GetMediaList(pageToken, stateToken, limit)
    // ...
}
```

### 3. CLI (`cli.go`)

```go
// Track state token across requests
var currentStateToken string = ""

for pagesRequested < pages {
    result, err := api.GetMediaList(currentPageToken, currentStateToken, limit)
    
    // Update state token from response
    if result.StateToken != "" {
        currentStateToken = result.StateToken
    }
    
    // ... handle results ...
}

// Include in final output
finalResult := &backend.MediaListResult{
    Items:         allItems,
    NextPageToken: lastNextPageToken,
    StateToken:    currentStateToken,
}
```

### 4. Frontend (`frontend/src/Gallery.vue`)

```typescript
// Track state token
const stateToken = ref('')

async function loadMediaList() {
    // Pass state token in request
    const result = await MediaBrowser.GetMediaList(
        pageToken.value, 
        stateToken.value, 
        50
    )
    
    // Update state token from response
    if (result.stateToken) {
        stateToken.value = result.stateToken
    }
    
    // ... handle results ...
}
```

### 5. TypeScript Bindings (`frontend/bindings/app/backend/mediabrowser.ts`)

```typescript
// Updated signature
export function GetMediaList(
    pageToken: string, 
    stateToken: string, 
    limit: number
): $CancellablePromise<MediaListResult>
```

## How It Works

### Request Flow

1. **Initial Request (First Page)**
   - pageToken: `""` (empty)
   - stateToken: `""` (empty)
   - Behaves like `get_library_page_init`

2. **API Response**
   - Returns items array
   - Returns `nextPageToken` for next page
   - Returns `stateToken` for state tracking

3. **Subsequent Requests**
   - pageToken: `<nextPageToken from previous response>`
   - stateToken: `<stateToken from previous response>`
   - Behaves like `get_library_page`

4. **State Token Purpose**
   - Tracks the library state (uploads, deletions, modifications)
   - Ensures consistent pagination even if library changes
   - Prevents duplicates and missing items

## Benefits

✅ **Correct Pagination**: Each request returns new, non-duplicate items
✅ **State Consistency**: Library changes don't break pagination
✅ **API Compliance**: Matches Google Photos API expectations
✅ **Reference Alignment**: Matches proven Python implementation

## Testing Checklist

When testing this fix, verify:

- [ ] First page loads correctly (empty state token)
- [ ] Subsequent pages load new items (state token is passed)
- [ ] No duplicate items appear across pages
- [ ] State token is updated from each response
- [ ] Pagination continues until end of library
- [ ] End-of-list detection still works
- [ ] CLI output includes state token
- [ ] Frontend tracks state token properly

## Files Changed

1. `backend/api.go` - Core API implementation
2. `backend/mediabrowser.go` - MediaBrowser wrapper
3. `cli.go` - CLI pagination logic
4. `frontend/src/Gallery.vue` - Frontend pagination
5. `frontend/bindings/app/backend/mediabrowser.ts` - TypeScript bindings

## References

- Python Reference Implementation: `reference/PYTHON实现/gpmc/api.py`
  - Line 704-867: `get_library_page_init` (no state token)
  - Line 869-1036: `get_library_page` (with state token)
- Python Client Implementation: `reference/PYTHON实现/gpmc/client.py`
  - Line 665-692: `_process_pages_init`
  - Line 693-720: `_process_pages`
