<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { MediaBrowser, type MediaItem } from '../../bindings/app/backend'
import Button from "./ui/button/Button.vue"
import { Download } from 'lucide-vue-next'

const props = defineProps<{
  item: MediaItem
  thumbnailSize: string
  isDownloading: boolean
}>()

const emit = defineEmits<{
  (e: 'download'): void
}>()

const thumbnailData = ref<string>('')
const loading = ref(true)
const error = ref(false)

onMounted(async () => {
  await loadThumbnail()
})

async function loadThumbnail() {
  loading.value = true
  error.value = false
  try {
    const base64Data = await MediaBrowser.GetThumbnail(props.item.mediaKey, props.thumbnailSize)
    // Backend returns thumbnails as JPEG format from Google Photos API
    thumbnailData.value = `data:image/jpeg;base64,${base64Data}`
  } catch (err) {
    console.error('Failed to load thumbnail:', err)
    error.value = true
  } finally {
    loading.value = false
  }
}

function handleDownload() {
  emit('download')
}
</script>

<template>
  <div class="w-full h-full relative">
    <!-- Loading state -->
    <div v-if="loading" class="absolute inset-0 flex items-center justify-center bg-secondary">
      <div class="text-xs text-muted-foreground">Loading...</div>
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="absolute inset-0 flex items-center justify-center bg-secondary">
      <div class="text-xs text-muted-foreground">Failed to load</div>
    </div>

    <!-- Thumbnail -->
    <img
      v-else
      :src="thumbnailData"
      :alt="item.filename || 'Photo'"
      class="w-full h-full object-cover"
    />

    <!-- Hover overlay with download button -->
    <div class="absolute inset-0 bg-black bg-opacity-0 group-hover:bg-opacity-50 transition-all flex items-center justify-center opacity-0 group-hover:opacity-100">
      <Button
        variant="secondary"
        size="icon"
        @click="handleDownload"
        :disabled="isDownloading"
        class="cursor-pointer"
        :title="isDownloading ? 'Downloading...' : 'Download'"
      >
        <Download v-if="!isDownloading" class="h-4 w-4" />
        <span v-else class="text-xs animate-pulse">↓</span>
      </Button>
    </div>

    <!-- Filename tooltip -->
    <div v-if="item.filename" class="absolute bottom-0 left-0 right-0 bg-black bg-opacity-70 text-white text-xs p-1 truncate opacity-0 group-hover:opacity-100 transition-opacity">
      {{ item.filename }}
    </div>
  </div>
</template>
