# Identifying Trashed Images from Google Photos Internal API

This document summarizes the method for distinguishing between normal and trashed (recycled) images within the Google Photos internal API responses, as implemented in the software.

## API Response Structure

The internal Google Photos API represents media items as complex, deeply nested objects. The key to identifying a trashed image lies within a specific field of this structure.

**Key Fields for Identification:**

1.  **Main Media Item Object (Field `2`)**: The overall metadata for a single media item is typically encapsulated within a protobuf field identified by the number `2`.
2.  **Status Information (Field `16` within Field `2`)**: Inside this main media item object (Field `2`), there is a nested message or object identified by the number `16`. This field contains crucial status information about the media item.
3.  **Trash Indicator (Field `1` within Field `16`)**: Within the "Status Information" (Field `16`), there is a sub-field identified by the number `1`. The integer value of this sub-field determines whether an image is in the trash.

**Identification Logic:**

*   **Image is in Trash**: If the value of `item[2][16][1]` (i.e., Field `1` nested within Field `16`, which is itself nested within Field `2` of the media item) is **`2`**, the image is considered to be in the trash.
*   **Image is Normal**: If the value of `item[2][16][1]` is **`1`**, the image is a normal, non-trashed item.

## Software Implementation

The software's backend logic, specifically within functions responsible for parsing API responses (e.g., `extractField2Metadata` and `parseStatusField` in `backend/api.go`), was modified to incorporate this logic:

1.  The parsing process extracts the integer value from `item[2][16][1]`.
2.  This extracted value is assigned to an internal `status` variable.
3.  A conditional check is performed: if `status` is `2`, the `IsTrash` boolean property of the `MediaItem` object is set to `true`.
4.  The frontend (e.g., `Gallery.vue`) then utilizes this `IsTrash` flag to correctly categorize and display images, often filtering trashed items into a separate "Recycle Bin" view or excluding them from the main gallery.

By implementing this logic, the software can now accurately identify and handle images that have been moved to the trash within Google Photos.