import { create } from 'zustand'
import { getSensitivityCheck, listSensitivityChecks, runSensitivityCheck } from '@/api/sensitivity'
import { errorMessage } from '@/api/client'
import type { SensitivityCheck } from '@/types/assessment'

interface SensitivityStore {
  items: SensitivityCheck[]
  selected: SensitivityCheck | null
  loading: boolean
  running: boolean
  error: string | null
  load: (assessmentId: number) => Promise<void>
  select: (id: number) => Promise<void>
  reset: () => void
  run: (assessmentId: number) => Promise<SensitivityCheck | null>
}

export const useSensitivityStore = create<SensitivityStore>((set) => ({
  items: [], selected: null, loading: false, running: false, error: null,
  load: async (assessmentId) => {
    set({ loading: true, error: null })
    try {
      const page = await listSensitivityChecks(assessmentId)
      set({ items: page.items, selected: page.items[0] ?? null, loading: false })
    } catch (error) {
      set({ error: errorMessage(error), loading: false })
    }
  },
  select: async (id) => {
    try {
      set({ selected: await getSensitivityCheck(id), error: null })
    } catch (error) {
      set({ error: errorMessage(error) })
    }
  },
  reset: () => set({ items: [], selected: null, loading: false, running: false, error: null }),
  run: async (assessmentId) => {
    set({ running: true, error: null })
    try {
      const check = await runSensitivityCheck(assessmentId)
      set((state) => ({ items: [check, ...state.items], selected: check, running: false }))
      return check
    } catch (error) {
      set({ error: errorMessage(error), running: false })
      return null
    }
  },
}))
