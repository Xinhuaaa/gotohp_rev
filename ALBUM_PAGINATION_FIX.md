# Album Pagination Fix

## Problem Description (问题描述)

After the latest commit, the album list cannot be fetched completely. The pagination stops after the first request - it only fetches once and then shows "no more photos", but the software did not actually make the next photo list request.

在最新的提交后编译发现，不能正常获取完整相册列表了，只能获取一次之后显示已经显示没有更多照片了，其实软件并没有进行下一次的相片列表请求。

## Root Cause (根本原因)

The `parseAlbumResponseField1` function in `backend/api.go` was extracting the pagination token from **field 4** instead of **field 1**.

According to the protobuf documentation in "分页参数说明.md":
- Response field **1.1** contains the pagination token
- This token should be used in the next request's field 1.4

The media list implementation (`parseResponseField1`) correctly extracts the pagination token from field 1, but the album list implementation was incorrectly looking at field 4.

## Fix Applied (修复方案)

Changed line 2724-2725 in `backend/api.go`:

**Before:**
```go
// Field 4 is the pagination token (for next request's field 1.4)
if fieldNum == 4 {
    paginationToken = string(fieldData)
}
```

**After:**
```go
// Field 1 is the pagination token (for next request's field 1.4)
if fieldNum == 1 {
    paginationToken = string(fieldData)
}
```

## Verification (验证)

### Consistency Check
The fix makes the album list parsing consistent with the media list parsing:

**Media List (working):**
```go
// Field 1 is the pagination token (for next request's field 1.6)
if fieldNum == 1 {
    paginationToken = string(fieldData)
}
```

**Album List (fixed):**
```go
// Field 1 is the pagination token (for next request's field 1.4)
if fieldNum == 1 {
    paginationToken = string(fieldData)
}
```

### CLI Logic
The CLI code in `cli_shared.go` (line 402) correctly uses the pagination token:
```go
currentPageToken = result.NextPageToken
```

With the fix, `result.NextPageToken` will now be correctly populated from the response, allowing pagination to work properly.

## Testing (测试)

To test the fix, use the CLI command:
```bash
gotohp albums --pages 3
```

Expected behavior:
- First page should display albums
- Second page should display different albums with a different page token
- Third page should display more albums (or indicate end if no more albums)
- Pagination should continue until all albums are retrieved or the requested page count is reached

## Impact (影响)

This is a **minimal, surgical fix** that:
- ✅ Changes only 2 lines of code
- ✅ Fixes the pagination issue for album lists
- ✅ Makes the code consistent with the working media list implementation
- ✅ Aligns with the protobuf specification documented in "分页参数说明.md"
- ✅ Passes code review with no issues
- ✅ Passes security checks with no vulnerabilities

## Related Documentation (相关文档)

- `分页参数说明.md` - Protobuf pagination specification
- `PAGINATION_FIX.md` - Media list pagination fix (reference implementation)
- `STATE_TOKEN_FIX.md` - State token implementation details
