import { apiClient } from '../client'

export type ClientVersionID = 'grok_cli' | 'claude_cli' | 'claude_sdk'
export interface ClientVersionChoice {
  id: ClientVersionID
  custom_version: string
  auto_sync: boolean
}
export interface ClientVersionView extends ClientVersionChoice {
  builtin_version: string
  minimum_version: string
  effective_version: string
  effective_source: 'builtin' | 'environment' | 'custom' | 'official'
  synced_version: string
  official_source: string
  checked_at?: string
  error?: string
}
export async function getClientVersions(): Promise<ClientVersionView[]> {
  const { data } = await apiClient.get<ClientVersionView[]>('/admin/settings/client-versions')
  return data
}
export async function updateClientVersions(choices: ClientVersionChoice[]): Promise<ClientVersionView[]> {
  const { data } = await apiClient.put<ClientVersionView[]>('/admin/settings/client-versions', choices)
  return data
}
