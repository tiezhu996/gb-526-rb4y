import type { RiskBand } from './risk'

export type SensitivityAxis = 'depth' | 'duration'
export type SensitivityDirection = 'decrease' | 'increase'

export interface SensitivityVariant {
  sequence_no: number
  axis: SensitivityAxis
  direction: SensitivityDirection
  adjustment_ratio: number
  original_depth_m: number
  adjusted_depth_m: number
  original_duration_min: number
  adjusted_duration_min: number
  computable: boolean
  out_of_range_reason: string
  comparative_score: number
  score_delta: number
  risk_band: RiskBand
  band_rank_delta: number
  added_risk_flags: string[]
  removed_risk_flags: string[]
}

export interface SegmentSensitivityImpact {
  sequence_no: number
  segment_type: string
  depth_m: number
  duration_min: number
  computable_count: number
  out_of_range_count: number
  max_absolute_score_delta: number
  max_band_rank_delta: number
  driver_axis: SensitivityAxis | ''
  driver_direction: SensitivityDirection | ''
  variants: SensitivityVariant[]
}

export interface SensitivityReport {
  algorithm_version: string
  adjustment_ratio: number
  baseline_comparative_score: number
  baseline_risk_band: RiskBand
  total_variants: number
  computable_variants: number
  out_of_range_variants: number
  most_affected_sequence_no: number
  most_affected_axis: SensitivityAxis | ''
  most_affected_direction: SensitivityDirection | ''
  segment_impacts: SegmentSensitivityImpact[]
  disclaimer: string
}

export interface SensitivityCheck {
  id: number
  assessment_id: number
  plan_id: number
  algorithm_version: string
  baseline_comparative_score: number
  baseline_risk_band: RiskBand
  adjustment_ratio: number
  total_variants: number
  computable_variants: number
  out_of_range_variants: number
  most_affected_sequence_no: number
  most_affected_axis: string
  most_affected_direction: string
  report: SensitivityReport
  created_by: number
  created_by_username: string
  created_at: string
  safety_disclaimer: string
}
