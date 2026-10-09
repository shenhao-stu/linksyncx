import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ClientVersionsSettings from '../ClientVersionsSettings.vue'
import en from '@/i18n/locales/en/admin/settings'
import { getClientVersions, updateClientVersions, type ClientVersionView } from '@/api/admin/clientVersions'
vi.mock('@/api/admin/clientVersions', () => ({ getClientVersions: vi.fn(), updateClientVersions: vi.fn() }))
const fixtures = (): ClientVersionView[] => [
  ['grok_cli', '1.0.46'], ['claude_cli', '2.1.287'], ['claude_sdk', '0.127.0']
].map(([id, version]) => ({ id: id as ClientVersionView['id'], custom_version: version!, auto_sync: false, builtin_version: version!, minimum_version: version!, effective_version: version!, effective_source: 'builtin', synced_version: '', official_source: 'https://example.test/release' }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => {
  let value: unknown = { admin: en }
  for (const part of key.split('.')) value = (value as Record<string, unknown>)?.[part]
  return typeof value === 'string' ? value.replace("{'@'}", '@') : key
} }) }))
const render = () => mount(ClientVersionsSettings)
beforeEach(() => { vi.clearAllMocks(); vi.mocked(getClientVersions).mockResolvedValue(fixtures()); vi.mocked(updateClientVersions).mockResolvedValue(fixtures()) })
describe('client version settings', () => {
  it('prefills built-ins with all opt-ins off and does not save on load', async () => {
    const wrapper = render(); await flushPromises()
    expect(wrapper.findAll('input').map(x => x.element.value)).toEqual(['1.0.46', '2.1.287', '0.127.0'])
    expect(wrapper.findAll('[role="switch"]').every(x => x.attributes('aria-checked') === 'false')).toBe(true)
    expect(updateClientVersions).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('@anthropic-ai/sdk')
  })
  it('retains the custom pin when toggling sync and submits only choices', async () => {
    const wrapper = render(); await flushPromises()
    await wrapper.get('#grok_cli-custom').setValue('1.0.47')
    await wrapper.get('#grok_cli-auto').trigger('click')
    expect(wrapper.get('#grok_cli-custom').attributes('disabled')).toBeDefined()
    await wrapper.findAll('button').at(-1)!.trigger('click'); await flushPromises()
    expect(updateClientVersions).toHaveBeenCalledWith([
      { id: 'grok_cli', custom_version: '1.0.47', auto_sync: true },
      { id: 'claude_cli', custom_version: '2.1.287', auto_sync: false },
      { id: 'claude_sdk', custom_version: '0.127.0', auto_sync: false }
    ])
  })
  it('restores the built-in and turns off sync without saving implicitly', async () => {
    const wrapper = render(); await flushPromises()
    await wrapper.get('#claude_sdk-custom').setValue('0.131.0')
    await wrapper.get('#claude_sdk-auto').trigger('click')
    await wrapper.findAll('fieldset')[2]!.findAll('button')[1]!.trigger('click')
    expect((wrapper.get('#claude_sdk-custom').element as HTMLInputElement).value).toBe('0.127.0')
    expect(wrapper.get('#claude_sdk-auto').attributes('aria-checked')).toBe('false')
    expect(updateClientVersions).not.toHaveBeenCalled()
  })
  it('blocks saving after a failed initial read, but supports retry', async () => {
    vi.mocked(getClientVersions).mockRejectedValueOnce(new Error('offline'))
    const wrapper = render(); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Could not load')
    expect(wrapper.findAll('button').at(-1)!.attributes('disabled')).toBeDefined()
    await wrapper.findAll('button').at(-2)!.trigger('click'); await flushPromises()
    expect(wrapper.findAll('input')).toHaveLength(3)
  })
  it('keeps unsaved edits when saving fails', async () => {
    vi.mocked(updateClientVersions).mockRejectedValueOnce(new Error('invalid'))
    const wrapper = render(); await flushPromises()
    await wrapper.get('#grok_cli-custom').setValue('invalid')
    await wrapper.findAll('button').at(-1)!.trigger('click'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Could not save')
    expect((wrapper.get('#grok_cli-custom').element as HTMLInputElement).value).toBe('invalid')
  })
})
