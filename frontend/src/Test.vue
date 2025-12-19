<script setup lang="ts">
import { ref } from 'vue'
import Button from './components/ui/button/Button.vue'
import { Input } from '@/components/ui/input'
import { toast } from 'vue-sonner'
import { callByAnyName } from '@/utils/wailsCall'

const endpoint = ref('https://photosdata-pa.googleapis.com/6439526531001121323/17490284929287180316')
const requestJSON = ref<string>('{\n  "2": 2,\n  "3": "",\n  "4": 2,\n  "8": {\n    "4": {\n      "2": "",\n      "3": {\n        "1": ""\n      },\n      "4": "",\n      "5": {\n        "1": ""\n      }\n    }\n  },\n  "9": ""\n}\n')
const responseJSON = ref<string>('')
const loading = ref(false)

async function send() {
  if (loading.value) return
  if (!endpoint.value.trim()) return

  loading.value = true
  responseJSON.value = ''
  try {
    const res = await callByAnyName<string>([
      'backend.MediaBrowser.DebugProtobufRequest',
      'app.backend.MediaBrowser.DebugProtobufRequest',
      'app/backend.MediaBrowser.DebugProtobufRequest',
    ], endpoint.value.trim(), requestJSON.value)
    responseJSON.value = res
  } catch (err: any) {
    console.error('Debug request failed:', err)
    toast.error('Request failed', { description: err?.message })
  } finally {
    loading.value = false
  }
}

async function copyResponse() {
  if (!responseJSON.value) return
  try {
    await navigator.clipboard.writeText(responseJSON.value)
    toast.success('Copied')
  } catch (err: any) {
    toast.error('Copy failed', { description: err?.message })
  }
}
</script>

<template>
  <div class="p-4 space-y-3">
    <div class="flex items-center gap-2">
      <Input v-model="endpoint" placeholder="https://photosdata-pa.googleapis.com/..." class="flex-1" />
      <Button :disabled="loading" @click="send" class="cursor-pointer select-none">
        {{ loading ? 'Sending...' : 'Send' }}
      </Button>
      <Button variant="outline" :disabled="!responseJSON" @click="copyResponse" class="cursor-pointer select-none">
        Copy
      </Button>
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div class="space-y-1">
        <div class="text-sm text-muted-foreground">Request JSON</div>
        <textarea
          v-model="requestJSON"
          class="w-full h-[70vh] rounded-md border bg-background p-2 font-mono text-xs"
          spellcheck="false"
        />
      </div>

      <div class="space-y-1">
        <div class="text-sm text-muted-foreground">Response JSON</div>
        <textarea
          v-model="responseJSON"
          readonly
          class="w-full h-[70vh] rounded-md border bg-background p-2 font-mono text-xs"
          spellcheck="false"
        />
      </div>
    </div>
  </div>
</template>

