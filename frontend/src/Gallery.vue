<script setup lang="ts">
import { ref, onMounted, computed, onUnmounted } from 'vue'
import { MediaBrowser, ConfigManager, type MediaItem } from '../bindings/app/backend'
import Button from "./components/ui/button/Button.vue"
import MediaItemComponent from './components/MediaItem.vue'
import { Events } from '@wailsio/runtime'
import { toast } from "vue-sonner"
import { RefreshCw, Trash2 } from 'lucide-vue-next'
import { callByAnyName } from '@/utils/wailsCall'

const mediaItems = ref<MediaItem[]>([])
const loading = ref(false)
const pageToken = ref('')
const hasMore = ref(true)
const reachedEnd = ref(false)
const thumbnailSize = ref('medium')
const downloadingItems = ref<Set<string>>(new Set())
const deletingItems = ref<Set<string>>(new Set())
const seenMediaKeys = ref<Set<string>>(new Set())
const syncToken = ref('')
const updateCheckIntervalSeconds = ref(0)
const autoWashQuotaItems = ref(false)
const requestTrashItems = ref(true)
const washingAllQuotaItems = ref(false)
const washProgress = ref({ total: 0, done: 0, failed: 0 })
const DEBUG = false // Set to true to enable debug logging
let autoUpdateTimer: ReturnType<typeof setInterval> | null = null
let unsubscribeConfigChanged: (() => void) | null = null

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

async function reloadFromStart() {
  if (loading.value || washingAllQuotaItems.value) return
  mediaItems.value = []
  pageToken.value = ''
  hasMore.value = true
  reachedEnd.value = false
  seenMediaKeys.value = new Set()
  syncToken.value = ''
  await loadMediaList()
}

// Load thumbnail size from config
onMounted(async () => {
  try {
    const config = (await ConfigManager.GetConfig()) as any
    if (config.thumbnailSize) {
      thumbnailSize.value = config.thumbnailSize
    }
    updateCheckIntervalSeconds.value = config.updateCheckIntervalSeconds || 0
    autoWashQuotaItems.value = config.autoWashQuotaItems || false
    requestTrashItems.value = typeof config.requestTrashItems === 'boolean' ? config.requestTrashItems : true
    setupAutoUpdateTimer()
  } catch (error) {
    console.error('Failed to load config:', error)
  }

  unsubscribeConfigChanged = Events.On('frontend:configChanged', (event) => {
    const changed = event.data?.[0] || event.data || {}
    if (typeof changed.updateCheckIntervalSeconds === 'number') {
      updateCheckIntervalSeconds.value = changed.updateCheckIntervalSeconds
      setupAutoUpdateTimer()
    }
    if (typeof changed.autoWashQuotaItems === 'boolean') {
      autoWashQuotaItems.value = changed.autoWashQuotaItems
    }
    if (typeof changed.requestTrashItems === 'boolean') {
      requestTrashItems.value = changed.requestTrashItems
      reloadFromStart()
    }
  })

  loadMediaList()
})

onUnmounted(() => {
  if (autoUpdateTimer) {
    clearInterval(autoUpdateTimer)
    autoUpdateTimer = null
  }
  if (unsubscribeConfigChanged) {
    unsubscribeConfigChanged()
    unsubscribeConfigChanged = null
  }
})

function setupAutoUpdateTimer() {
  if (autoUpdateTimer) {
    clearInterval(autoUpdateTimer)
    autoUpdateTimer = null
  }
  const intervalMs = Math.max(0, updateCheckIntervalSeconds.value) * 1000
  if (intervalMs > 0) {
    autoUpdateTimer = setInterval(() => {
      checkUpdates({ silentNoChanges: true })
    }, intervalMs)
  }
}

async function loadMediaList() {
  if (loading.value || reachedEnd.value) return
  
  loading.value = true
  try {
    debugLog('Loading media list with pageToken:', pageToken.value)
    // Pass empty syncToken and passive triggerMode (2)
    const result = await MediaBrowser.GetMediaList(pageToken.value, "", 2, 0)
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
        console.log('Loaded items:', newItems) // Debug: Check for isTrash
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
      
      // Capture sync token if present (usually at end of list)
      if (result.syncToken) {
        syncToken.value = result.syncToken
        debugLog('Updated sync token:', syncToken.value)
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

async function checkUpdates(options?: { silentNoChanges?: boolean }) {
  if (loading.value || washingAllQuotaItems.value) return

  loading.value = true
  try {
    const useIncrementalSync = !!syncToken.value
    debugLog('Checking for updates. Incremental:', useIncrementalSync, 'syncToken:', syncToken.value)
    // If we don't have a sync token yet, fall back to polling the first page and merging unseen items.
    // Once we obtain a sync token (usually after reaching end of list), we switch to incremental updates.
    const result = useIncrementalSync
      ? await MediaBrowser.GetMediaList("", syncToken.value, 1, 0) // active triggerMode (1)
      : await MediaBrowser.GetMediaList("", "", 2, 0) // passive triggerMode (2)
    debugLog('Update result:', result)

    if (result && result.items) {
      let addedCount = 0
      let deletedCount = 0
      const newItems: MediaItem[] = []
      const toWash: MediaItem[] = []
      let washedCount = 0
      let washFailedCount = 0

      result.items.forEach(item => {
        // Status 2 means delete/remove
        if (useIncrementalSync && item.status === 2) {
           debugLog('Processing deletion for:', item.mediaKey)
           // Remove from list
           const initialLen = mediaItems.value.length
           mediaItems.value = mediaItems.value.filter(existing => existing.mediaKey !== item.mediaKey)
           if (mediaItems.value.length < initialLen) {
             deletedCount++
             // Also remove from seen set
             seenMediaKeys.value.delete(item.mediaKey)
           }
           return
        }

        // If item is already seen, it might be an update
        if (seenMediaKeys.value.has(item.mediaKey)) {
          const existingItem = mediaItems.value.find(existing => existing.mediaKey === item.mediaKey)
          if (existingItem) {
            // Update mutable properties
            existingItem.countsTowardsQuota = item.countsTowardsQuota
            ;(existingItem as any).isTrash = (item as any).isTrash
            
            // If it now consumes quota and auto-wash is enabled, handle it
            if (autoWashQuotaItems.value && item.countsTowardsQuota) {
               toWash.push(item)
            }
          }
          return
        }

        // If this item consumes quota and auto-wash is enabled, "wash" it (download+reupload)
        // and only add the washed result to the list.
        if (autoWashQuotaItems.value && item.countsTowardsQuota) {
          // Mark as seen so the original doesn't get added later during pagination in this session.
          seenMediaKeys.value.add(item.mediaKey)
          toWash.push(item)
          return
        }

        seenMediaKeys.value.add(item.mediaKey)
        newItems.push(item)
        addedCount++
      })

      if (addedCount > 0) {
        debugLog(`Found ${addedCount} new items`)
        // Prepend new items
        mediaItems.value = [...newItems, ...mediaItems.value]
      }

      // Process quota-consuming items that need washing sequentially to avoid spamming the API.
      for (const item of toWash) {
        try {
          const washedItem = await callByAnyName<MediaItem>([
            'backend.MediaBrowser.WashMedia',
            'app.backend.MediaBrowser.WashMedia',
            'app/backend.MediaBrowser.WashMedia',
          ], item.mediaKey, (item as any).dedupKey || "")
          if (washedItem && washedItem.mediaKey) {
            washedCount++
            if (!seenMediaKeys.value.has(washedItem.mediaKey)) {
              seenMediaKeys.value.add(washedItem.mediaKey)
            }
            const alreadyInList = mediaItems.value.some(existing => existing.mediaKey === washedItem.mediaKey)
            if (!alreadyInList) {
              mediaItems.value = [washedItem, ...mediaItems.value]
              addedCount++
            }
            if (washedItem.countsTowardsQuota) {
              toast.warning('Wash completed, but item still counts towards quota', {
                description: washedItem.filename || washedItem.mediaKey,
              })
            }
          } else {
            washFailedCount++
            // Fallback: add the original item so we don't lose updates.
            const alreadyInList = mediaItems.value.some(existing => existing.mediaKey === item.mediaKey)
            if (!alreadyInList) {
              mediaItems.value = [item, ...mediaItems.value]
              addedCount++
            }
          }
        } catch (error: any) {
          washFailedCount++
          console.error('Failed to wash media:', error)
          toast.error('Failed to wash quota item', { description: error?.message })
          // Fallback: add the original item so we don't lose updates.
          const alreadyInList = mediaItems.value.some(existing => existing.mediaKey === item.mediaKey)
          if (!alreadyInList) {
            mediaItems.value = [item, ...mediaItems.value]
            addedCount++
          }
        }
      }
      
      if (addedCount > 0 || deletedCount > 0) {
         const washedSummary = toWash.length > 0 ? `, ${washedCount} washed` : ''
         const washFailedSummary = washFailedCount > 0 ? ` (${washFailedCount} failed)` : ''
         toast.success(`Updated: ${addedCount} added${washedSummary}${washFailedSummary}, ${deletedCount} deleted`)
      } else {
         if (!options?.silentNoChanges) {
           toast.info('No changes')
         }
      }

      // Update sync token for next time
      if (result.syncToken) {
        syncToken.value = result.syncToken
      }
    }
  } catch (error: any) {
    console.error('Failed to check updates:', error)
    toast.error('Failed to update', { description: error?.message })
  } finally {
    loading.value = false
  }
}

async function washAllQuota() {
  if (washingAllQuotaItems.value) return

  const candidates = mediaItems.value.filter(
    (item) => !(item as any).isTrash && item.countsTowardsQuota !== false
  )
  if (candidates.length === 0) {
    toast.info('暂无占用空间的照片')
    return
  }

  washingAllQuotaItems.value = true
  washProgress.value = { total: candidates.length, done: 0, failed: 0 }
  toast.info(`开始洗白：${candidates.length} 张`)

  try {
    for (const item of candidates) {
      try {
        const washedItem = await callByAnyName<MediaItem>([
          'backend.MediaBrowser.WashMedia',
          'app.backend.MediaBrowser.WashMedia',
          'app/backend.MediaBrowser.WashMedia',
        ], item.mediaKey, (item as any).dedupKey || "")

        // Keep original marked as seen to avoid re-adding it during pagination in this session.
        if (item.mediaKey) {
          seenMediaKeys.value.add(item.mediaKey)
        }

        if (washedItem && washedItem.mediaKey) {
          seenMediaKeys.value.add(washedItem.mediaKey)

          // Remove original from list and add washed item to the top.
          mediaItems.value = mediaItems.value.filter((existing) => existing.mediaKey !== item.mediaKey)
          const alreadyInList = mediaItems.value.some((existing) => existing.mediaKey === washedItem.mediaKey)
          if (!alreadyInList) {
            mediaItems.value = [washedItem, ...mediaItems.value]
          }

          if (washedItem.countsTowardsQuota) {
            toast.warning('洗白完成但仍占用空间', {
              description: washedItem.filename || washedItem.mediaKey,
            })
          }
        } else {
          washProgress.value.failed++
        }
      } catch (error: any) {
        washProgress.value.failed++
        console.error('Failed to wash media:', error)
        toast.error('洗白失败', { description: error?.message })
      } finally {
        washProgress.value.done++
      }
    }

    toast.success(`洗白完成：${washProgress.value.done - washProgress.value.failed} 成功，${washProgress.value.failed} 失败`)
  } finally {
    washingAllQuotaItems.value = false
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

async function deleteMedia(item: MediaItem) {
  const mediaKey = item.mediaKey
  if (!mediaKey) return
  if (deletingItems.value.has(mediaKey)) return
  if (washingAllQuotaItems.value) return

  deletingItems.value.add(mediaKey)
  try {
    if ((item as any).isTrash) {
      const dedupKey = (item as any).dedupKey as string | undefined
      if (!dedupKey) {
        throw new Error('missing dedupKey for permanent delete')
      }
      if (!confirm(`永久删除？此操作不可恢复。\n\n${item.filename || mediaKey}`)) {
        return
      }
      await callByAnyName<void>([
        'backend.MediaBrowser.PermanentlyDeleteMedia',
        'app.backend.MediaBrowser.PermanentlyDeleteMedia',
        'app/backend.MediaBrowser.PermanentlyDeleteMedia',
      ], dedupKey)
      
      mediaItems.value = mediaItems.value.filter((existing) => existing.mediaKey !== mediaKey)
      toast.success('已永久删除', { description: item.filename || mediaKey })
    } else {
      await callByAnyName<void>([
        'backend.MediaBrowser.DeleteMedia',
        'app.backend.MediaBrowser.DeleteMedia',
        'app/backend.MediaBrowser.DeleteMedia',
      ], mediaKey)
      
      // Optimistic UI update: remove from list (will show under trash on next refresh if returned by API).
      mediaItems.value = mediaItems.value.filter((existing) => existing.mediaKey !== mediaKey)
      toast.success('已移入回收站', { description: item.filename || mediaKey })
    }
  } catch (error: any) {
    console.error('Failed to delete media:', error)
    toast.error((item as any).isTrash ? '永久删除失败' : '删除失败', { description: error?.message })
  } finally {
    deletingItems.value.delete(mediaKey)
  }
}

const gridCols = computed(() => {
  switch (thumbnailSize.value) {
    case 'small': return 'grid-cols-6'
    case 'large': return 'grid-cols-2'
    default: return 'grid-cols-4' // medium
  }
})

const trashItems = computed(() =>
  requestTrashItems.value ? mediaItems.value.filter((item) => (item as any).isTrash) : []
)

const quotaConsumingItems = computed(() =>
  mediaItems.value.filter((item) => !(item as any).isTrash && item.countsTowardsQuota !== false)
)

const quotaExemptItems = computed(() =>
  mediaItems.value.filter((item) => !(item as any).isTrash && item.countsTowardsQuota === false)
)
</script>

<template>
  <div class="w-full h-full flex flex-col p-4 overflow-auto">
    <div class="flex justify-between items-center mb-4">
      <h2 class="text-xl font-semibold">Photo Gallery</h2>
      <div class="flex gap-2">
        <Button
          v-if="syncToken"
          variant="outline"
          size="icon"
          @click="checkUpdates"
          :disabled="loading || washingAllQuotaItems"
          title="Check for updates"
        >
          <RefreshCw :class="['h-4 w-4', { 'animate-spin': loading }]" />
        </Button>
        <Button 
          v-if="!reachedEnd || loading" 
          variant="outline" 
          @click="loadMediaList"
          :disabled="loading || reachedEnd || washingAllQuotaItems"
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

      <div v-else class="space-y-6">
        <section class="space-y-3" v-if="trashItems.length > 0">
          <div class="flex items-center justify-between">
            <h3 class="text-lg font-medium flex items-center gap-2">
              <Trash2 class="w-5 h-5 text-red-500" />
              回收站的照片
            </h3>
            <span class="text-sm text-muted-foreground">共 {{ trashItems.length }} 张</span>
          </div>
          <div :class="['grid gap-2', gridCols]">
            <div
              v-for="item in trashItems"
              :key="item.mediaKey"
              class="relative group aspect-square bg-secondary rounded overflow-hidden border-2 border-red-200/50"
            >
              <MediaItemComponent
                :item="item"
                :thumbnail-size="thumbnailSize"
                :is-downloading="downloadingItems.has(item.mediaKey)"
                :is-deleting="deletingItems.has(item.mediaKey)"
                @download="downloadMedia(item.mediaKey, item.filename || 'photo')"
                @delete="deleteMedia(item)"
              />
            </div>
          </div>
        </section>

      <section class="space-y-3">
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-medium">占用空间的照片</h3>
          <div class="flex items-center gap-3">
            <span class="text-sm text-muted-foreground">共 {{ quotaConsumingItems.length }} 张</span>
            <Button
              variant="outline"
              size="sm"
              class="cursor-pointer"
              @click="washAllQuota"
              :disabled="loading || washingAllQuotaItems || quotaConsumingItems.length === 0"
              :title="washingAllQuotaItems ? `Washing... ${washProgress.done}/${washProgress.total}` : '一键洗白占用空间的照片'"
            >
              {{ washingAllQuotaItems ? `洗白中 ${washProgress.done}/${washProgress.total}` : '一键洗白' }}
            </Button>
          </div>
        </div>
        <div v-if="quotaConsumingItems.length === 0 && !loading" class="text-sm text-muted-foreground">
          暂无占用空间的照片
        </div>
        <div v-else-if="quotaConsumingItems.length > 0" :class="['grid gap-2', gridCols]">
          <div
            v-for="item in quotaConsumingItems"
            :key="item.mediaKey"
            class="relative group aspect-square bg-secondary rounded overflow-hidden"
          >
            <MediaItemComponent
              :item="item"
              :thumbnail-size="thumbnailSize"
              :is-downloading="downloadingItems.has(item.mediaKey)"
              :is-deleting="deletingItems.has(item.mediaKey)"
              @download="downloadMedia(item.mediaKey, item.filename || 'photo')"
              @delete="deleteMedia(item)"
            />
          </div>
        </div>
      </section>

      <section class="space-y-3">
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-medium">不占用空间的照片</h3>
          <span class="text-sm text-muted-foreground">共 {{ quotaExemptItems.length }} 张</span>
        </div>
        <div v-if="quotaExemptItems.length === 0 && !loading" class="text-sm text-muted-foreground">
          暂无不占用空间的照片
        </div>
        <div v-else-if="quotaExemptItems.length > 0" :class="['grid gap-2', gridCols]">
          <div
            v-for="item in quotaExemptItems"
            :key="item.mediaKey"
            class="relative group aspect-square bg-secondary rounded overflow-hidden"
          >
            <MediaItemComponent
              :item="item"
              :thumbnail-size="thumbnailSize"
              :is-downloading="downloadingItems.has(item.mediaKey)"
              :is-deleting="deletingItems.has(item.mediaKey)"
              @download="downloadMedia(item.mediaKey, item.filename || 'photo')"
              @delete="deleteMedia(item)"
            />
          </div>
        </div>
      </section>
      </div>

    <div v-if="loading && mediaItems.length === 0" class="flex items-center justify-center h-64">
      <div class="text-muted-foreground">Loading photos...</div>
    </div>
  </div>
</template>
