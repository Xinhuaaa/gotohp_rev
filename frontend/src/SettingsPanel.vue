<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { ConfigManager } from '../bindings/app/backend'
import { Events } from '@wailsio/runtime'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select'

import {
    NumberField,
    NumberFieldContent,
    NumberFieldDecrement,
    NumberFieldIncrement,
    NumberFieldInput,
} from '@/components/ui/number-field'
import { callByAnyName } from '@/utils/wailsCall'

interface Settings {
    proxy: string
    useQuota: boolean
    saver: boolean
    recursive: boolean
    forceUpload: boolean
    deleteFromHost: boolean
    disableUnsupportedFilesFilter: boolean
    uploadThreads: number
    thumbnailSize: string
    updateCheckIntervalSeconds: number
    autoWashQuotaItems: boolean
    requestTrashItems: boolean
}

const settings = ref<Settings>({
    proxy: '',
    useQuota: false,
    saver: false,
    recursive: false,
    forceUpload: false,
    deleteFromHost: false,
    disableUnsupportedFilesFilter: false,
    uploadThreads: 0,
    thumbnailSize: 'medium',
    updateCheckIntervalSeconds: 0,
    autoWashQuotaItems: false,
    requestTrashItems: true,
})

onMounted(async () => {
    const config = (await ConfigManager.GetConfig()) as any
    settings.value = {
        proxy: config.proxy || '',
        useQuota: config.useQuota || false,
        saver: config.saver || false,
        recursive: config.recursive || false,
        forceUpload: config.forceUpload || false,
        deleteFromHost: config.deleteFromHost || false,
        disableUnsupportedFilesFilter: config.disableUnsupportedFilesFilter || false,
        uploadThreads: config.uploadThreads || 1,
        thumbnailSize: config.thumbnailSize || 'medium',
        updateCheckIntervalSeconds: config.updateCheckIntervalSeconds || 0,
        autoWashQuotaItems: config.autoWashQuotaItems || false,
        requestTrashItems: typeof config.requestTrashItems === 'boolean' ? config.requestTrashItems : true,
    }
})

// Watch for changes to proxy value and update backend
watch(() => settings.value.proxy, async (newValue) => {
    await ConfigManager.SetProxy(newValue)
})

// Create individual watchers for each boolean setting
watch(() => settings.value.useQuota, async (newValue) => {
    await ConfigManager.SetUseQuota(newValue)
})

watch(() => settings.value.saver, async (newValue) => {
    await ConfigManager.SetSaver(newValue)
})

watch(() => settings.value.recursive, async (newValue) => {
    await ConfigManager.SetRecursive(newValue)
})

watch(() => settings.value.forceUpload, async (newValue) => {
    await ConfigManager.SetForceUpload(newValue)
})

watch(() => settings.value.deleteFromHost, async (newValue) => {
    await ConfigManager.SetDeleteFromHost(newValue)
})

watch(() => settings.value.disableUnsupportedFilesFilter, async (newValue) => {
    await ConfigManager.SetDisableUnsupportedFilesFilter(newValue)
})

watch(() => settings.value.uploadThreads, async (newValue) => {
    if (newValue < 1) {
        settings.value.uploadThreads = 1
    } else {
        await ConfigManager.SetUploadThreads(newValue)
    }
})

watch(() => settings.value.thumbnailSize, async (newValue) => {
    await ConfigManager.SetThumbnailSize(newValue)
})

watch(() => settings.value.updateCheckIntervalSeconds, async (newValue) => {
    if (newValue < 0) {
        settings.value.updateCheckIntervalSeconds = 0
    } else {
        const seconds = secondsToInt(newValue)
        await callByAnyName<void>([
            'backend.ConfigManager.SetUpdateCheckIntervalSeconds',
            'app.backend.ConfigManager.SetUpdateCheckIntervalSeconds',
            'app/backend.ConfigManager.SetUpdateCheckIntervalSeconds',
        ], seconds)
        await Events.Emit('frontend:configChanged', { updateCheckIntervalSeconds: seconds })
    }
})

watch(() => settings.value.autoWashQuotaItems, async (newValue) => {
    await callByAnyName<void>([
        'backend.ConfigManager.SetAutoWashQuotaItems',
        'app.backend.ConfigManager.SetAutoWashQuotaItems',
        'app/backend.ConfigManager.SetAutoWashQuotaItems',
    ], newValue)
    await Events.Emit('frontend:configChanged', { autoWashQuotaItems: newValue })
})

watch(() => settings.value.requestTrashItems, async (newValue) => {
    await callByAnyName<void>([
        'backend.ConfigManager.SetRequestTrashItems',
        'app.backend.ConfigManager.SetRequestTrashItems',
        'app/backend.ConfigManager.SetRequestTrashItems',
    ], newValue)
    await Events.Emit('frontend:configChanged', { requestTrashItems: newValue })
})

function secondsToInt(value: number): number {
    return Number.isFinite(value) ? Math.floor(value) : 0
}
</script>

<template>
    <div class="flex flex-col gap-2.5 m-4">
        <NumberField v-model="settings.uploadThreads" class="flex items-center justify-between">
            <Label for="upload-threads" class="size-full">上传线程数</Label>
            <NumberFieldContent>
                <NumberFieldDecrement class="cursor-pointer" :disabled="settings.uploadThreads <= 1" />
                <NumberFieldInput />
                <NumberFieldIncrement class="cursor-pointer" />
            </NumberFieldContent>
        </NumberField>
        <div class="flex items-center justify-between">
            <Label for="thumbnail-size" class="size-full">缩略图大小</Label>
            <Select v-model="settings.thumbnailSize">
                <SelectTrigger class="w-[120px]">
                    <SelectValue placeholder="选择大小" />
                </SelectTrigger>
                <SelectContent>
                    <SelectItem value="small">小</SelectItem>
                    <SelectItem value="medium">中</SelectItem>
                    <SelectItem value="large">大</SelectItem>
                </SelectContent>
            </Select>
        </div>
        <div class="flex items-center justify-between">
            <Label for="use-quota" class="size-full cursor-pointer">使用配额（占用空间）</Label>
            <Switch id="use-quota" v-model="settings.useQuota" />
        </div>
        <div class="flex items-center justify-between">
            <Label for="saver-mode" class="size-full cursor-pointer">存储节省质量</Label>
            <Switch id="saver-mode" v-model="settings.saver" />
        </div>
        <div class="flex items-center justify-between">
            <Label for="recursive" class="size-full cursor-pointer">递归上传文件夹</Label>
            <Switch id="recursive" v-model="settings.recursive" />
        </div>
        <div class="flex items-center justify-between">
            <Label for="force-upload" class="size-full cursor-pointer">强制上传</Label>
            <Switch id="force-upload" v-model="settings.forceUpload" />
        </div>
        <div class="flex items-center justify-between">
            <Label for="filter-unsupported" class="size-full cursor-pointer">关闭不支持格式过滤</Label>
            <Switch id="filter-unsupported" v-model="settings.disableUnsupportedFilesFilter" />
        </div>
        <div class="flex items-center justify-between">
            <Label for="delete-host" class="size-full cursor-pointer">上传后删除本地文件</Label>
            <Switch id="delete-host" variant="destructive" v-model="settings.deleteFromHost" />
        </div>
        <NumberField v-model="settings.updateCheckIntervalSeconds" class="flex items-center justify-between">
            <Label for="update-check-interval" class="size-full">更新检查间隔（秒，0=关闭）</Label>
            <NumberFieldContent>
                <NumberFieldDecrement class="cursor-pointer" :disabled="settings.updateCheckIntervalSeconds <= 0" />
                <NumberFieldInput />
                <NumberFieldIncrement class="cursor-pointer" />
            </NumberFieldContent>
        </NumberField>
        <div class="flex items-center justify-between">
            <Label for="auto-wash" class="size-full cursor-pointer">更新时自动洗白占用空间的照片</Label>
            <Switch id="auto-wash" v-model="settings.autoWashQuotaItems" />
        </div>
        <div class="flex items-center justify-between">
            <Label for="request-trash" class="size-full cursor-pointer">请求回收站照片</Label>
            <Switch id="request-trash" v-model="settings.requestTrashItems" />
        </div>
        <div>
            <Input v-model="settings.proxy" type="text" placeholder="代理地址（可选）" />
        </div>
    </div>

</template>
