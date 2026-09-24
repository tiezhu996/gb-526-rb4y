import type { DivePlan } from './plan'
import type { ExposureSegment } from './segment'
import type { DiverProfile } from './diver'
import type { RiskBand, RiskFlag } from './risk'

export interface LoadPoint {
  sequence_no: number
  depth_m: number
  elapsed_min: number
  ambient_bar: number
  inspired_n2_bar: number
  inspired_he_bar: number
  n2_load_bar: number
  he_load_bar: number
  total_inert_bar: number
  relative_change: number
}

export interface CompartmentCurve {
  name: string
  n2_half_time_min: number
  he_half_time_min: number
  points: LoadPoint[]
}

export interface ModelAssumptions {
  purpose: string
  pressure_conversion: string
  water_vapor_bar: number
  kinetics: string
  configured_compartments: Array<{ name: string; n2_half_time_min: number; he_half_time_min: number }>
  prohibited_use: string[]
}

export interface InputSnapshot {
  plan: DivePlan & { breathing_mix_json?: string }
  diver: DiverProfile
  segments: ExposureSegment[]
  algorithm_version: string
  safety_boundary: string
}

export interface DecompressionAssessment {
  id: number
  plan_id: number
  assessment_status: string
  algorithm_version: string
  input_snapshot: InputSnapshot
  compartment_loads: CompartmentCurve[]
  risk_flags: RiskFlag[]
  highest_risk_band: RiskBand
  comparative_score: number
  assumptions: ModelAssumptions
  created_at: string
  reviewed_at?: string
  safety_disclaimer: string
}

export interface AssessmentComparison {
  left: DecompressionAssessment
  right: DecompressionAssessment
  score_delta: number
  flag_delta: number
  summary: string[]
  disclaimer: string
}

export type SensitivityDimension = 'depth' | 'duration'
export type SensitivityDirection = 'minus_10_percent' | 'plus_10_percent'
export type SensitivityVariantStatus = 'computed' | 'out_of_model_range'

export interface SensitivityVariant {
  sequence_no: number
  segment_type: string
  dimension: SensitivityDimension
  direction: SensitivityDirection
  perturbation_percent: number
  original_depth_m: number
  perturbed_depth_m?: number
  original_duration_min: number
  perturbed_duration_min?: number
  status: SensitivityVariantStatus
  comparative_score?: number
  score_delta: number
  highest_risk_band?: RiskBand
  baseline_risk_band: RiskBand
  risk_band_changed: boolean
  risk_flag_codes?: string[]
  out_of_range_reason?: string
}

export interface SensitivityCheck {
  id: number
  assessment_id: number
  run_serial: number
  algorithm_version: string
  perturbation_percent: number
  baseline_score: number
  baseline_risk_band: RiskBand
  most_influential_sequence: number
  most_influential_reason: string
  out_range_count: number
  band_change_count: number
  variants: SensitivityVariant[]
  created_by: number
  created_at: string
  safety_disclaimer: string
}
