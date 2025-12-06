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
const thumbnailSize = ref('medium')
const downloadingItems = ref<Set<string>>(new Set())

// Load thumbnail size from config
onMounted(async () => {
  try {
    const config = await ConfigManager.GetConfig()
    if (config.thumbnailSize) {
      thumbnailSize.value = config.thumbnailSize
    }
  } catch (error) {
    console.error('Failed to load config:', error)
  }
  loadMediaList()
})

async function loadMediaList() {
  if (loading.value || !hasMore.value) return
  
  loading.value = true
  try {
    const result = await MediaBrowser.GetMediaList(pageToken.value, 50)
    if (result) {
      mediaItems.value = [...mediaItems.value, ...result.items]
      pageToken.value = result.nextPageToken || ''
      hasMore.value = !!result.nextPageToken
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
          v-if="hasMore" 
          variant="outline" 
          @click="loadMediaList"
          :disabled="loading"
          class="cursor-pointer"
        >
          {{ loading ? 'Loading...' : 'Load More' }}
        </Button>
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
