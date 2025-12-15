<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { MediaBrowser, ConfigManager, type MediaItem } from '../bindings/app/backend'
import Button from "./components/ui/button/Button.vue"
import MediaItemComponent from './components/MediaItem.vue'
import { toast } from "vue-sonner"

const mediaItems = ref<MediaItem[]>([])
const loading = ref(false)
const pageToken = ref('')
const hasMore = ref(true)
const reachedEnd = ref(false)
const thumbnailSize = ref('medium')
const pageSize = ref(50)
const downloadingItems = ref<Set<string>>(new Set())
const seenMediaKeys = ref<Set<string>>(new Set())
const DEBUG = false // Set to true to enable debug logging

function debugLog(...args: any[]) {
  if (DEBUG) {
    console.log(...args)
  }
}

function showEndOfListMessage() {
  if (mediaItems.value.length > 0) {
    toast.info('已到底部', {
      description: '没有更多照片了',
    })
  }
}

function markAsEndOfList() {
  reachedEnd.value = true
  hasMore.value = false
}

// Load thumbnail size from config
onMounted(async () => {
  try {
    const config = await ConfigManager.GetConfig()
    if (config.thumbnailSize) {
      thumbnailSize.value = config.thumbnailSize
    }
    if (config.mediaPageSize && config.mediaPageSize > 0) {
      pageSize.value = config.mediaPageSize
    }
  } catch (error) {
    console.error('Failed to load config:', error)
  }
  loadMediaList()
})

async function loadMediaList() {
  if (loading.value || reachedEnd.value) return
  
  loading.value = true
  try {
    debugLog('Loading media list with pageToken:', pageToken.value)
    const result = await MediaBrowser.GetMediaList(pageToken.value, pageSize.value)
    debugLog('Received result:', result)
    
    if (result && result.items) {
      // Filter out duplicate items based on mediaKey
      const newItems = result.items.filter(item => {
        if (seenMediaKeys.value.has(item.mediaKey)) {
          debugLog('Skipping duplicate item:', item.mediaKey)
          return false
        }
        seenMediaKeys.value.add(item.mediaKey)
        return true
      })
      
      debugLog(`Adding ${newItems.length} new items (${result.items.length} total in response)`)
      
      // Add new items to the list
      if (newItems.length > 0) {
        mediaItems.value = [...mediaItems.value, ...newItems]
      }
      
      // Check if we've reached the end
      const hasNextPage = !!result.nextPageToken
      const allDuplicates = newItems.length === 0 && result.items.length > 0
      
      if (allDuplicates || !hasNextPage) {
        debugLog('Reached end of list:', allDuplicates ? 'all duplicates' : 'no next page token')
        markAsEndOfList()
        // Show message if: all duplicates OR (no next page AND no new items this time)
        if (allDuplicates || (!hasNextPage && newItems.length === 0)) {
          showEndOfListMessage()
        }
      } else {
        pageToken.value = result.nextPageToken || ''
        hasMore.value = true
      }
    } else {
      // No items in response
      debugLog('No items in response - reached end')
      markAsEndOfList()
      showEndOfListMessage()
    }
  } catch (error: any) {
    console.error('Failed to load media list:', error)
    toast.error('Failed to load photos', {
      description: error?.message,
    })
  } finally {
    loading.value = false
  }
}

async function downloadMedia(mediaKey: string, filename: string) {
  if (downloadingItems.value.has(mediaKey)) return
  
  downloadingItems.value.add(mediaKey)
  try {
    const savedPath = await MediaBrowser.DownloadMedia(mediaKey)
    toast.success('Download complete!', {
      description: `Saved to: ${savedPath}`,
    })
  } catch (error: any) {
    console.error('Failed to download media:', error)
    toast.error('Download failed', {
      description: error?.message || 'Unknown error',
    })
  } finally {
    downloadingItems.value.delete(mediaKey)
  }
}

const gridCols = computed(() => {
  switch (thumbnailSize.value) {
    case 'small': return 'grid-cols-6'
    case 'large': return 'grid-cols-2'
    default: return 'grid-cols-4' // medium
  }
})
</script>

<template>
  <div class="w-full h-full flex flex-col p-4 overflow-auto">
    <div class="flex justify-between items-center mb-4">
      <h2 class="text-xl font-semibold">Photo Gallery</h2>
      <div class="flex gap-2">
        <Button 
          v-if="!reachedEnd || loading" 
          variant="outline" 
          @click="loadMediaList"
          :disabled="loading || reachedEnd"
          class="cursor-pointer"
        >
          {{ loading ? 'Loading...' : (reachedEnd ? '已到底部' : 'Load More') }}
        </Button>
        <div v-if="reachedEnd && mediaItems.length > 0" class="text-sm text-muted-foreground flex items-center">
          没有更多照片了
        </div>
      </div>
    </div>

    <div v-if="mediaItems.length === 0 && !loading" class="flex flex-col items-center justify-center h-64 text-muted-foreground">
      <p>No photos found</p>
      <p class="text-sm">Upload some photos to see them here</p>
    </div>

    <div v-if="mediaItems.length > 0" :class="['grid gap-2', gridCols]">
      <div 
        v-for="item in mediaItems" 
        :key="item.mediaKey"
        class="relative group aspect-square bg-secondary rounded overflow-hidden"
      >
        <MediaItemComponent 
          :item="item" 
          :thumbnail-size="thumbnailSize"
          :is-downloading="downloadingItems.has(item.mediaKey)"
          @download="downloadMedia(item.mediaKey, item.filename || 'photo')"
        />
      </div>
    </div>

    <div v-if="loading && mediaItems.length === 0" class="flex items-center justify-center h-64">
      <div class="text-muted-foreground">Loading photos...</div>
    </div>
  </div>
</template>
