import { Alert, Button, Chip } from '@mui/material'
import { Ban, Crosshair, History, Play, TrendingDown, TrendingUp } from 'lucide-react'
import { useSensitivityStore } from '@/stores/sensitivity'
import type { SensitivityAxis, SensitivityCheck, SensitivityDirection, SensitivityVariant } from '@/types/sensitivity'

const axisLabel = (axis: SensitivityAxis) => axis === 'depth' ? 'Depth' : 'Duration'
const directionIcon = (direction: SensitivityDirection) => direction === 'increase' ? <TrendingUp size={13} /> : <TrendingDown size={13} />

function VariantCell({ variant }: { variant: SensitivityVariant }) {
  const magnitude = variant.axis === 'depth'
    ? `${variant.original_depth_m.toFixed(1)} → ${variant.adjusted_depth_m.toFixed(1)} m`
    : `${variant.original_duration_min.toFixed(1)} → ${variant.adjusted_duration_min.toFixed(1)} min`
  return (
    <article className={`sensitivity-variant ${variant.computable ? '' : 'cannot-compute'}`}>
      <header><span>{directionIcon(variant.direction)}{axisLabel(variant.axis)} {variant.direction === 'increase' ? '+10%' : '−10%'}</span></header>
      <strong>{magnitude}</strong>
      {variant.computable ? <>
        <div className="variant-delta">
          <span className={variant.score_delta > 0 ? 'delta-up' : variant.score_delta < 0 ? 'delta-down' : ''}>
            {variant.score_delta > 0 ? '+' : ''}{variant.score_delta.toFixed(1)} index
          </span>
          <span className={variant.band_rank_delta > 0 ? 'delta-up' : variant.band_rank_delta < 0 ? 'delta-down' : ''}>
            band {variant.band_rank_delta > 0 ? '+' : ''}{variant.band_rank_delta}
          </span>
        </div>
        <small>{variant.risk_band}{(variant.added_risk_flags.length + variant.removed_risk_flags.length) > 0 ? ` · flags +${variant.added_risk_flags.length}/−${variant.removed_risk_flags.length}` : ' · flags unchanged'}</small>
      </> : <div className="variant-blocked"><Ban size={14} /><span>Cannot compute</span><small>{variant.out_of_range_reason}</small></div>}
    </article>
  )
}

export function SensitivityPanel({ assessmentId, canRun }: { assessmentId: number; canRun: boolean }) {
  const sensitivity = useSensitivityStore()
  const load = () => { void sensitivity.load(assessmentId) }
  const run = async () => {
    try { await sensitivity.run(assessmentId) } catch { /* error is surfaced by the store */ }
  }
  const selected: SensitivityCheck | null = sensitivity.selected?.assessment_id === assessmentId ? sensitivity.selected : null
  const report = selected?.report ?? null
  return (
    <section className="sensitivity-panel">
      <div className="section-title">
        <Crosshair size={18} />
        <div>
          <strong>Sensitivity check</strong>
          <span>Replays the immutable snapshot with ±10% per-segment depth or duration nudges; the assessment itself is never changed.</span>
        </div>
        {canRun && <Button variant="outlined" size="small" startIcon={<Play size={15} />} disabled={sensitivity.running} onClick={() => void run()}>{sensitivity.running ? 'Recomputing…' : 'Run ±10% check'}</Button>}
        <Button size="small" startIcon={<History size={15} />} onClick={load}>Refresh archive</Button>
      </div>
      {sensitivity.error && <Alert severity="error">{sensitivity.error}</Alert>}
      <div className="sensitivity-archive">
        {sensitivity.items.filter((item) => item.assessment_id === assessmentId).map((item) => (
          <button key={item.id} type="button" className={`sensitivity-archive-row ${selected?.id === item.id ? 'selected' : ''}`} onClick={() => void sensitivity.select(item.id)}>
            <strong>#{item.id}</strong>
            <span>{new Date(item.created_at).toLocaleString()}</span>
            <span>{item.created_by_username}</span>
            <span>{item.computable_variants}/{item.total_variants} computable</span>
            <span>{item.out_of_range_variants > 0 ? `${item.out_of_range_variants} out of range` : 'all in range'}</span>
            <Chip size="small" color={item.most_affected_sequence_no > 0 ? 'primary' : 'default'} label={item.most_affected_sequence_no > 0 ? `Most affected S${item.most_affected_sequence_no}` : 'No index change'} />
          </button>
        ))}
        {!sensitivity.items.some((item) => item.assessment_id === assessmentId) && <div className="empty-inline">No archived sensitivity check for this assessment. Each run is stored independently.</div>}
      </div>
      {selected && report && <>
        <div className="sensitivity-summary">
          <div><span>Baseline index</span><strong>{report.baseline_comparative_score.toFixed(1)}</strong></div>
          <div><span>Baseline risk band</span><strong>{report.baseline_risk_band}</strong></div>
          <div><span>Variants</span><strong>{report.computable_variants} of {report.total_variants}</strong></div>
          <div><span>Out of model range</span><strong className={report.out_of_range_variants > 0 ? 'delta-up' : ''}>{report.out_of_range_variants}</strong></div>
          <div><span>Most affected segment</span><strong>{report.most_affected_sequence_no > 0 ? `S${report.most_affected_sequence_no} · ${report.most_affected_axis} ${report.most_affected_direction}` : 'None — index stable'}</strong></div>
        </div>
        <div className="sensitivity-segments">
          {report.segment_impacts.map((impact) => (
            <details key={impact.sequence_no} open={impact.sequence_no === report.most_affected_sequence_no} className={`sensitivity-segment ${impact.sequence_no === report.most_affected_sequence_no ? 'most-affected' : ''}`}>
              <summary>
                <span className="sequence-token">{impact.sequence_no}</span>
                <strong>S{impact.sequence_no} · {impact.segment_type}</strong>
                <small>{impact.depth_m.toFixed(1)} m · {impact.duration_min.toFixed(1)} min</small>
                <span className="segment-impact">max |Δindex| <b>{impact.max_absolute_score_delta.toFixed(1)}</b></span>
                {impact.out_of_range_count > 0 && <Chip size="small" color="warning" label={`${impact.out_of_range_count} out of range`} />}
                {impact.sequence_no === report.most_affected_sequence_no && <Chip size="small" color="secondary" label="MOST AFFECTED" />}
              </summary>
              <div className="sensitivity-variant-grid">
                {impact.variants.map((variant) => <VariantCell key={`${variant.axis}-${variant.direction}`} variant={variant} />)}
              </div>
            </details>
          ))}
        </div>
        <p className="disclaimer-line">{report.disclaimer}</p>
      </>}
    </section>
  )
}
