# Pagination Fix Documentation

## Problem Description

The gallery pagination had the following issues:
1. **Next page button could only be clicked once** - After the first click, subsequent clicks would not load more items
2. **Duplicate content** - The next page would show duplicate images from the previous page
3. **No proper end-of-list detection** - Users couldn't tell when they reached the end of their photo library

## Root Causes

### 1. Missing Page Size Limit in API Request
The `GetMediaList` function in `backend/api.go` was not sending the `limit` parameter to the Google Photos API. The protobuf request builder was missing the page size field (field 1.2), which meant the API was returning an uncontrolled number of items, potentially causing pagination issues.

### 2. No Duplicate Detection
When the API returned the same items across multiple requests (due to pagination token issues), the frontend would display them multiple times without filtering.

### 3. Insufficient End-of-List Detection
The frontend only checked for the presence of `nextPageToken` but didn't handle cases where:
- All returned items were duplicates
- The API returned an empty response
- Multiple clicks on the same page token

## Solution Implemented

### Backend Changes (`backend/api.go`)

1. **Added page size limit to protobuf request**:
   ```go
   // field1.2 - page size limit (varint)
   if limit > 0 {
       writeProtobufVarint(&buf, 2, int64(limit))
   }
   ```
   This ensures the Google Photos API respects the requested page size.

2. **Updated function signatures** to propagate the limit parameter:
   - `buildMediaListRequest(pageToken string, limit int)`
   - `buildMediaListRequestField1(pageToken string, limit int)`

### Frontend Changes (`frontend/src/Gallery.vue`)

1. **Added duplicate detection**:
   - New `seenMediaKeys` Set to track all loaded media keys
   - Filter returned items to exclude duplicates before adding to display

2. **Enhanced end-of-list detection**:
   - Track `reachedEnd` state separately from `hasMore`
   - Detect end when:
     - No `nextPageToken` in response
     - All returned items are duplicates
     - API returns empty item list

3. **Improved user feedback**:
   - Button text changes to "已到底部" (reached the end) when appropriate
   - Additional message "没有更多照片了" (no more photos) displayed
   - Toast notification when reaching the end
   - Button becomes disabled when end is reached

4. **Added console logging** for debugging:
   - Log page token on each request
   - Log duplicate detection
   - Log end-of-list detection

## Implementation Details

### Duplicate Detection Logic
```typescript
const newItems = result.items.filter(item => {
  if (seenMediaKeys.value.has(item.mediaKey)) {
    console.log('Skipping duplicate item:', item.mediaKey)
    return false
  }
  seenMediaKeys.value.add(item.mediaKey)
  return true
})
```

### End-of-List Detection Logic
The implementation handles three scenarios:

1. **All items are duplicates**:
   ```typescript
   if (newItems.length === 0 && result.items.length > 0) {
     reachedEnd.value = true
     hasMore.value = false
     toast.info('已到底部', { description: '没有更多照片了' })
   }
   ```

2. **No nextPageToken**:
   ```typescript
   if (!result.nextPageToken) {
     reachedEnd.value = true
     hasMore.value = false
   }
   ```

3. **Empty response**:
   ```typescript
   if (!result || !result.items || result.items.length === 0) {
     reachedEnd.value = true
     hasMore.value = false
   }
   ```

## Testing

The fix addresses the following test cases:

1. ✅ **First load** - Loads initial set of items correctly
2. ✅ **Load more (normal)** - Subsequent pages load without duplicates
3. ✅ **Load more (all duplicates)** - Properly detects and handles duplicate responses
4. ✅ **Load more (no items)** - Handles empty responses gracefully
5. ✅ **Reached end** - Prevents additional requests once end is reached
6. ✅ **Loading state** - Prevents concurrent requests during loading

## Benefits

1. **Correct pagination** - Each click loads a new page of unique items
2. **No duplicates** - Duplicate detection ensures each image appears only once
3. **Better UX** - Users know when they've reached the end of their library
4. **Improved reliability** - Handles edge cases like empty responses and API inconsistencies
5. **Better debugging** - Console logs help diagnose pagination issues

## Future Improvements

Potential enhancements for future versions:
- Implement infinite scroll instead of "Load More" button
- Add loading skeleton for better visual feedback
- Implement virtual scrolling for large photo libraries
- Add ability to reset/refresh the gallery
- Persist pagination state across page refreshes
