# Testing Guide for Pagination Fix

## Prerequisites
1. Build the application using `make build`
2. Have a Google Photos account with photos uploaded
3. Configure credentials in the application

## Test Scenarios

### Scenario 1: Normal Pagination Flow
**Steps:**
1. Launch the application
2. Navigate to the Gallery view
3. Click "Load More" button
4. Verify new photos appear (no duplicates)
5. Repeat step 3-4 multiple times
6. Verify pagination continues until all photos are loaded

**Expected Result:**
- Each click loads ~50 new photos
- No duplicate photos appear
- Button shows "已到底部" when all photos are loaded
- Message "没有更多照片了" appears at the end

### Scenario 2: Small Library (< 50 photos)
**Steps:**
1. Use an account with fewer than 50 photos
2. Navigate to the Gallery view
3. Observe initial load

**Expected Result:**
- All photos load immediately
- "Load More" button does not appear (or shows "已到底部")
- No error messages

### Scenario 3: Empty Library
**Steps:**
1. Use an account with no photos
2. Navigate to the Gallery view

**Expected Result:**
- Shows "No photos found" message
- Shows "Upload some photos to see them here" hint
- No "Load More" button appears

### Scenario 4: End of Library Detection
**Steps:**
1. Load photos until near the end of library
2. Click "Load More" for the last page
3. Observe button state

**Expected Result:**
- Last page of photos loads
- Button shows "已到底部" and becomes disabled
- Toast notification appears: "已到底部 - 没有更多照片了"
- Additional text "没有更多照片了" appears next to button

### Scenario 5: Duplicate Detection
**Steps:**
1. Enable debug mode: Set `DEBUG = true` in `frontend/src/Gallery.vue`
2. Rebuild frontend: `cd frontend && npm run build:dev`
3. Rebuild app: `cd .. && make build`
4. Run application and open browser console
5. Navigate to Gallery and click "Load More"
6. Look for "Skipping duplicate item" messages in console

**Expected Result:**
- If duplicates are detected, they are logged and filtered out
- No duplicate photos appear in the gallery
- If all items in a page are duplicates, pagination stops

### Scenario 6: Network Error Handling
**Steps:**
1. Disconnect network or use airplane mode
2. Click "Load More"

**Expected Result:**
- Error toast appears: "Failed to load photos"
- Error description shows network error message
- Button remains enabled for retry

### Scenario 7: Rapid Clicking Prevention
**Steps:**
1. Navigate to Gallery
2. Rapidly click "Load More" button multiple times

**Expected Result:**
- Only one request is made (loading state prevents concurrent requests)
- Button shows "Loading..." during request
- Button is disabled during loading

### Scenario 8: Thumbnail Size Consistency
**Steps:**
1. Change thumbnail size in Settings (Small/Medium/Large)
2. Navigate to Gallery
3. Load multiple pages

**Expected Result:**
- All thumbnails maintain consistent size
- Grid adjusts correctly (6/4/2 columns)
- Pagination works correctly regardless of thumbnail size

## Debug Mode

To enable detailed logging:
1. Edit `frontend/src/Gallery.vue`
2. Change `const DEBUG = false` to `const DEBUG = true` (line 16)
3. Rebuild: `cd frontend && npm run build:dev && cd .. && make build`
4. Open browser developer console while using the app

Debug logs will show:
- Page token for each request
- Items returned in each response
- Duplicate detection
- End-of-list detection

## Performance Considerations

### Expected Behavior:
- Initial load: ~1-2 seconds (depending on network)
- Each "Load More" click: ~1-2 seconds
- Thumbnail loading: Progressive (may take a few seconds per thumbnail)

### Red Flags:
- Requests taking > 5 seconds consistently
- Memory usage growing unbounded (check browser dev tools)
- Thumbnails not loading after 10 seconds
- Duplicate photos appearing in the grid

## Known Limitations

1. **No infinite scroll**: User must click "Load More" to load additional pages
2. **No virtual scrolling**: All loaded photos remain in DOM (may impact performance with 1000+ photos)
3. **No search/filter**: Cannot search or filter photos yet
4. **No album support**: Shows all photos in library, not organized by album

## Troubleshooting

### Problem: "Load More" button appears but clicking does nothing
**Solution:** 
- Check console for errors
- Verify credentials are valid
- Check network connectivity

### Problem: Same photos appear multiple times
**Solution:**
- This should be fixed by this PR
- Enable DEBUG mode and check console for "Skipping duplicate item" messages
- If still occurring, report as a bug

### Problem: Button never shows "已到底部"
**Solution:**
- Check if you have more photos than expected
- Look for errors in console
- Try refreshing the page

### Problem: Photos not loading at all
**Solution:**
- Verify Google Photos credentials are configured
- Check if photos exist in the account
- Look for API errors in console
