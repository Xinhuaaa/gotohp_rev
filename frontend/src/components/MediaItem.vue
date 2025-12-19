<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, computed } from 'vue'
import { type MediaItem } from '../../bindings/app/backend'
import Button from "./ui/button/Button.vue"
import { Download, X } from 'lucide-vue-next'
import { getThumbnailBase64 } from '@/utils/thumbnailQueue'

const props = defineProps<{
  item: MediaItem
  thumbnailSize: string
  isDownloading: boolean
  isDeleting?: boolean
}>()

const emit = defineEmits<{
  (e: 'download'): void
  (e: 'delete'): void
}>()

const thumbnailData = ref<string>('')
const loading = ref(true)
const error = ref(false)
let abortController: AbortController | null = null

const isTrash = computed(() => (props.item as any).isTrash === true)
const canDelete = computed(() => {
  if (props.isDownloading || props.isDeleting) return false
  if (isTrash.value) return !!(props.item as any).dedupKey
  return true
})

onMounted(async () => {
  await loadThumbnail()
})

onUnmounted(() => {
  abortController?.abort()
  abortController = null
})

watch(
  () => [props.item.mediaKey, props.thumbnailSize] as const,
  async () => {
    abortController?.abort()
    abortController = null
    await loadThumbnail()
  },
)

async function loadThumbnail() {
  loading.value = true
  error.value = false
  try {
    abortController = new AbortController()
    const base64Data = await getThumbnailBase64(props.item.mediaKey, props.thumbnailSize, abortController.signal)
    // Backend returns thumbnails as JPEG format from Google Photos API
    thumbnailData.value = `data:image/jpeg;base64,${base64Data}`
  } catch (err) {
    if ((err as any)?.name === 'AbortError') return
    console.error('Failed to load thumbnail:', err)
    error.value = true
  } finally {
    loading.value = false
  }
}

function handleDownload() {
  emit('download')
}

function handleDelete(event: MouseEvent) {
  event.stopPropagation()
  emit('delete')
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

    <!-- Delete button (top-right) -->
    <Button
      variant="destructive"
      size="icon"
      class="absolute top-1 right-1 h-7 w-7 z-10 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto transition-opacity"
      @click="handleDelete"
      :disabled="!canDelete"
      :title="isTrash ? 'Permanent delete' : 'Delete (move to trash)'"
    >
      <X class="h-4 w-4" />
    </Button>

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
