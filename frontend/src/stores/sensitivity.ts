import { create } from 'zustand'
import { getSensitivityCheck, listSensitivityChecks, runSensitivityCheck } from '@/api/sensitivity'
import { errorMessage } from '@/api/client'
import type { SensitivityCheck } from '@/types/sensitivity'

interface SensitivityStore {
  items: SensitivityCheck[]
  selected: SensitivityCheck | null
  assessmentId: number | null
  loading: boolean
  running: boolean
  error: string | null
  load: (assessmentId: number) => Promise<void>
  select: (id: number) => Promise<void>
  run: (assessmentId: number) => Promise<void>
  reset: () => void
}

export const useSensitivityStore = create<SensitivityStore>((set, get) => ({
  items: [], selected: null, assessmentId: null, loading: false, running: false, error: null,
  load: async (assessmentId) => {
    set({ loading: true, error: null })
    try {
      const page = await listSensitivityChecks(assessmentId)
      const preferred = get().selected?.assessment_id === assessmentId ? get().selected?.id : undefined
      const next = page.items.find((item) => item.id === preferred) ?? page.items[0] ?? null
      set({ items: page.items, selected: next, assessmentId, loading: false })
    } catch (error) { set({ error: errorMessage(error), loading: false }) }
  },
  select: async (id) => {
    try { set({ selected: await getSensitivityCheck(id), error: null }) }
    catch (error) { set({ error: errorMessage(error) }) }
  },
  run: async (assessmentId) => {
    set({ running: true, error: null })
    try {
      const item = await runSensitivityCheck(assessmentId)
      set((state) => ({ items: [item, ...state.items], selected: item, assessmentId, running: false }))
    } catch (error) {
      set({ error: errorMessage(error), running: false })
      throw error
    }
  },
  reset: () => set({ items: [], selected: null, assessmentId: null, loading: false, running: false, error: null }),
}))
