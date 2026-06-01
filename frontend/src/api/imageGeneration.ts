import { apiClient } from './client'

export interface ImageGenerationSettings {
  enabled: boolean
  default_group_id: number
  default_model: string
  retention_days: number
}

export interface ImageGenerationResult {
  id: number
  index: number
  mime_type: string
  size_bytes: number
  url: string
}

export interface ImageGenerationTask {
  id: number
  mode: 'generation' | 'edit' | string
  status: string
  model: string
  prompt: string
  size?: string
  error_message?: string
  expires_at: string
  created_at: string
  results: ImageGenerationResult[]
}

export interface ImageGenerationListResponse {
  items: ImageGenerationTask[]
  total: number
  page: number
  page_size: number
  pages: number
}

export async function getImageGenerationBootstrap(): Promise<ImageGenerationSettings> {
  const { data } = await apiClient.get<ImageGenerationSettings>('/image-generation/bootstrap')
  return data
}

export async function listImageGenerationTasks(page = 1, pageSize = 24): Promise<ImageGenerationListResponse> {
  const { data } = await apiClient.get<ImageGenerationListResponse>('/image-generation/tasks', {
    params: { page, page_size: pageSize },
  })
  return data
}

export async function getImageGenerationTask(id: number): Promise<ImageGenerationTask> {
  const { data } = await apiClient.get<ImageGenerationTask>(`/image-generation/tasks/${id}`)
  return data
}

export async function createImageGeneration(payload: Record<string, unknown>): Promise<ImageGenerationTask> {
  const { data } = await apiClient.post<ImageGenerationTask>('/image-generation/generations', payload, {
    timeout: 180000,
  })
  return data
}

export async function createImageEdit(form: FormData): Promise<ImageGenerationTask> {
  const { data } = await apiClient.post<ImageGenerationTask>('/image-generation/edits', form, {
    timeout: 180000,
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data
}

export async function deleteImageGenerationTask(id: number): Promise<void> {
  await apiClient.delete(`/image-generation/tasks/${id}`)
}

export async function getImageGenerationFileBlob(resultId: number): Promise<Blob> {
  const { data } = await apiClient.get<Blob>(`/image-generation/files/${resultId}`, {
    responseType: 'blob',
  })
  return data
}
