import { request } from './client'
import type { Page } from '@/types/common'
import type { SensitivityCheck } from '@/types/sensitivity'

export const listSensitivityChecks = (assessmentId: number) => request<Page<SensitivityCheck>>(`/assessments/${assessmentId}/sensitivity-checks?size=100`)
export const runSensitivityCheck = (assessmentId: number) => request<SensitivityCheck>(`/assessments/${assessmentId}/sensitivity-checks`, { method: 'POST' })
export const getSensitivityCheck = (id: number) => request<SensitivityCheck>(`/sensitivity-checks/${id}`)
