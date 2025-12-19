import { MediaBrowser } from '../../bindings/app/backend'

type ThumbnailJob = {
  key: string
  mediaKey: string
  size: string
  signal?: AbortSignal
  resolve: (value: string) => void
  reject: (reason?: any) => void
}

const BATCH_SIZE = 15
const MIN_BATCH_INTERVAL_MS = 1000
const MAX_CACHE_ENTRIES = 500

const queue: ThumbnailJob[] = []
const pending = new Map<string, Promise<string>>()
const cache = new Map<string, string>()
let processing = false

function sleep(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms))
}

function cacheSet(key: string, value: string) {
  // refresh LRU order
  if (cache.has(key)) cache.delete(key)
  cache.set(key, value)
  while (cache.size > MAX_CACHE_ENTRIES) {
    const oldestKey = cache.keys().next().value as string | undefined
    if (!oldestKey) break
    cache.delete(oldestKey)
  }
}

function abortError() {
  // DOMException exists in browsers; fall back to Error for safety.
  try {
    // eslint-disable-next-line no-undef
    return new DOMException('Aborted', 'AbortError')
  } catch {
    const err = new Error('Aborted')
    ;(err as any).name = 'AbortError'
    return err
  }
}

async function runLoop() {
  if (processing) return
  processing = true
  try {
    while (queue.length > 0) {
      const batchStart = Date.now()

      const batch: ThumbnailJob[] = []
      while (batch.length < BATCH_SIZE && queue.length > 0) {
        const job = queue.shift()!
        // If caller already gave up, skip without calling backend.
        if (job.signal?.aborted) {
          pending.delete(job.key)
          job.reject(abortError())
          continue
        }
        batch.push(job)
      }

      if (batch.length > 0) {
        await Promise.allSettled(
          batch.map(async (job) => {
            try {
              const base64 = await MediaBrowser.GetThumbnail(job.mediaKey, job.size)
              cacheSet(job.key, base64)
              job.resolve(base64)
            } catch (err) {
              job.reject(err)
            } finally {
              pending.delete(job.key)
            }
          }),
        )
      }

      const elapsed = Date.now() - batchStart
      if (elapsed < MIN_BATCH_INTERVAL_MS) {
        await sleep(MIN_BATCH_INTERVAL_MS - elapsed)
      }
    }
  } finally {
    processing = false
  }
}

export function getThumbnailBase64(mediaKey: string, size: string, signal?: AbortSignal): Promise<string> {
  const key = `${size}:${mediaKey}`

  const cached = cache.get(key)
  if (cached) return Promise.resolve(cached)

  const existing = pending.get(key)
  if (existing) return existing

  const promise = new Promise<string>((resolve, reject) => {
    queue.push({ key, mediaKey, size, signal, resolve, reject })
  })
  pending.set(key, promise)
  void runLoop()
  return promise
}

export function clearThumbnailCache() {
  cache.clear()
}
