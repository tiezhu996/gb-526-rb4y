import { request } from './client'
import type { Page } from '@/types/common'
import type { SensitivityCheck } from '@/types/assessment'

export const listSensitivityChecks = (assessmentId: number) => request<Page<SensitivityCheck>>(`/assessments/${assessmentId}/sensitivity-checks?size=100`)
export const getSensitivityCheck = (id: number) => request<SensitivityCheck>(`/assessments/sensitivity-checks/${id}`)
export const runSensitivityCheck = (assessmentId: number) => request<SensitivityCheck>(`/assessments/${assessmentId}/sensitivity-checks`, { method: 'POST', body: JSON.stringify({}) })
