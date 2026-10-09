<template>
  <section class="card space-y-5 p-6" :aria-label="t(`${key}.title`)">
    <div>
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t(`${key}.title`) }}</h2>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t(`${key}.description`) }}</p>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t(`${key}.scope`) }}</p>
    </div>
    <p v-if="loading" role="status">{{ t(`${key}.loading`) }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <p v-if="saved" role="status" class="text-sm text-green-600">{{ t(`${key}.saved`) }}</p>
    <fieldset v-for="version in versions" :key="version.id" :disabled="saving" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <legend class="px-2 font-medium">{{ t(`${key}.clients.${version.id}`) }}</legend>
      <dl class="grid gap-2 text-sm sm:grid-cols-2">
        <div><dt class="text-gray-500">{{ t(`${key}.builtin`) }}</dt><dd class="font-mono">{{ version.builtin_version }}</dd></div>
        <div><dt class="text-gray-500">{{ t(`${key}.effective`) }}</dt><dd><span class="font-mono">{{ version.effective_version }}</span> · {{ t(`${key}.sources.${version.effective_source}`) }}</dd></div>
      </dl>
      <div class="mt-4 flex items-center justify-between gap-4">
        <label :for="`${version.id}-auto`" class="text-sm">{{ t(`${key}.auto`) }}</label>
        <Toggle :id="`${version.id}-auto`" v-model="version.auto_sync" :aria-label="`${t(`${key}.clients.${version.id}`)} ${t(`${key}.auto`)}`" @update:model-value="saved = false" />
      </div>
      <label :for="`${version.id}-custom`" class="mb-1 mt-4 block text-sm">{{ t(`${key}.custom`) }}</label>
      <input :id="`${version.id}-custom`" v-model="version.custom_version" :disabled="version.auto_sync" type="text" maxlength="64" autocomplete="off" spellcheck="false" class="input w-full font-mono" @input="saved = false" @keydown.enter.prevent="save" />
      <p class="mt-1 text-xs text-gray-500">{{ t(`${key}.minimum`) }}: {{ version.minimum_version }}</p>
      <button type="button" class="mt-2 text-sm text-primary-600" @click="restore(version)">{{ t(`${key}.restore`) }}</button>
      <div class="mt-4 space-y-1 text-xs text-gray-500 dark:text-gray-400">
        <p>{{ t(`${key}.synced`) }}: <span class="font-mono">{{ version.synced_version || t(`${key}.never`) }}</span></p>
        <p>{{ t(`${key}.checked`) }}: {{ version.checked_at ? new Date(version.checked_at).toLocaleString() : t(`${key}.never`) }}</p>
        <p v-if="version.error" class="text-amber-600">{{ t(`${key}.syncError`) }}</p>
        <a :href="version.official_source" target="_blank" rel="noopener noreferrer" class="break-all text-primary-600">{{ version.official_source }}</a>
      </div>
    </fieldset>
    <div class="flex flex-wrap justify-end gap-3">
      <button type="button" class="btn btn-secondary" :disabled="loading || saving" @click="load">{{ t(`${key}.reload`) }}</button>
      <button type="button" class="btn btn-primary" :disabled="loading || saving || versions.length !== 3" @click="save">{{ saving ? t('admin.settings.saving') : t(`${key}.save`) }}</button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { getClientVersions, updateClientVersions, type ClientVersionView } from '@/api/admin/clientVersions'

const { t } = useI18n()
const key = 'admin.settings.clientVersions'
const versions = ref<ClientVersionView[]>([])
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const saved = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  saved.value = false
  try { versions.value = await getClientVersions() }
  catch { error.value = t(`${key}.loadError`) }
  finally { loading.value = false }
}
function restore(version: ClientVersionView) {
  version.custom_version = version.builtin_version
  version.auto_sync = false
  saved.value = false
}
async function save() {
  if (saving.value || loading.value || versions.value.length !== 3) return
  saving.value = true
  error.value = ''
  saved.value = false
  try {
    versions.value = await updateClientVersions(versions.value.map(({ id, custom_version, auto_sync }) => ({ id, custom_version: custom_version.trim(), auto_sync })))
    saved.value = true
  } catch { error.value = t(`${key}.saveError`) }
  finally { saving.value = false }
}
onMounted(load)
</script>
