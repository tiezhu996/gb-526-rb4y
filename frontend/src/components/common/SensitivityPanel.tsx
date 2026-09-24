import { useEffect } from 'react'
import { Alert, Button } from '@mui/material'
import { FlaskConical, History, Play } from 'lucide-react'
import { useSensitivityStore } from '@/stores/sensitivity'
import type { SensitivityCheck, SensitivityVariant } from '@/types/assessment'

const dimensionLabel: Record<SensitivityVariant['dimension'], string> = {
  depth: '深度',
  duration: '时长',
}

const directionLabel: Record<SensitivityVariant['direction'], string> = {
  minus_10_percent: '−10%',
  plus_10_percent: '+10%',
}

const bandRank: Record<string, number> = { informational: 0, caution: 1, elevated: 2, invalid: 3 }

function VariantRow({ variant, highlighted }: { variant: SensitivityVariant; highlighted: boolean }) {
  const changed = variant.dimension === 'depth' ? `${variant.original_depth_m.toFixed(1)} → ${variant.perturbed_depth_m?.toFixed(1)} m` : `${variant.original_duration_min.toFixed(1)} → ${variant.perturbed_duration_min?.toFixed(1)} min`
  return (
    <div className={`sensitivity-row ${variant.status === 'out_of_model_range' ? 'sensitivity-out' : ''} ${highlighted ? 'sensitivity-hot' : ''}`}>
      <span>#{variant.sequence_no} {variant.segment_type}</span>
      <span>{dimensionLabel[variant.dimension]}</span>
      <span>{directionLabel[variant.direction]}</span>
      <span>{changed}</span>
      {variant.status === 'out_of_model_range'
        ? <span className="sensitivity-status-out" title={variant.out_of_range_reason}>无法计算 · 越出模型范围</span>
        : <>
          <strong className={variant.score_delta > 0 ? 'delta-up' : variant.score_delta < 0 ? 'delta-down' : ''}>{variant.score_delta > 0 ? '+' : ''}{variant.score_delta.toFixed(1)}</strong>
          <span className={variant.risk_band_changed ? 'band-changed' : ''}>{variant.baseline_risk_band} → {variant.highest_risk_band}{variant.risk_band_changed ? ' ⚠' : ''}</span>
        </>}
    </div>
  )
}

export function SensitivityPanel({ assessmentId, canRun }: { assessmentId: number; canRun: boolean }) {
  const sensitivity = useSensitivityStore()
  useEffect(() => {
    void sensitivity.load(assessmentId)
    return sensitivity.reset
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [assessmentId])
  const check: SensitivityCheck | null = sensitivity.selected
  return (
    <section className="sensitivity-panel">
      <div className="section-title">
        <FlaskConical size={18} />
        <div>
          <strong>敏感性检查（±10% 单段扰动）</strong>
          <span>固定其余输入重算；每次检查独立留档，原评估快照不被修改</span>
        </div>
      </div>
      {canRun && (
        <Button variant="contained" startIcon={<Play size={16} />} disabled={sensitivity.running} onClick={() => void sensitivity.run(assessmentId)}>
          {sensitivity.running ? '重算中…' : '发起一次敏感性检查'}
        </Button>
      )}
      {sensitivity.error && <Alert severity="error">{sensitivity.error}</Alert>}
      <div className="sensitivity-archive">
        <History size={15} />
        {sensitivity.items.length === 0 && <span className="muted">尚无独立留档的检查。</span>}
        {sensitivity.items.map((item) => (
          <button key={item.id} type="button" className={check?.id === item.id ? 'selected' : ''} onClick={() => void sensitivity.select(item.id)}>
            第 {item.run_serial} 次 · {new Date(item.created_at).toLocaleString()}
          </button>
        ))}
      </div>
      {check && (
        <div className="sensitivity-result">
          <div className="sensitivity-summary">
            <div><span>基准比较指数</span><strong>{check.baseline_score.toFixed(1)}</strong></div>
            <div><span>基准风险带</span><strong>{check.baseline_risk_band}</strong></div>
            <div><span>风险带变化</span><strong>{check.band_change_count}</strong></div>
            <div><span>无法计算组合</span><strong>{check.out_range_count}</strong></div>
            <div className="sensitivity-hotbox"><span>影响最大的段</span><strong>第 {check.most_influential_sequence} 段</strong><small>{check.most_influential_reason}</small></div>
          </div>
          <div className="sensitivity-grid sensitivity-grid-head">
            <span>段</span><span>扰动维度</span><span>幅度</span><span>输入变化</span><span>比较指数 Δ</span><span>风险带变化</span>
          </div>
          <div className="sensitivity-grid">
            {check.variants.map((variant, index) => (
              <VariantRow key={`${variant.sequence_no}-${variant.dimension}-${variant.direction}-${index}`} variant={variant} highlighted={variant.sequence_no === check.most_influential_sequence} />
            ))}
          </div>
          <p className="sensitivity-note">
            共 {check.variants.length} 个组合（{new Set(check.variants.map((v) => v.sequence_no)).size} 段 × 深度/时长 × 增减一成）。“无法计算”表示该组合越出训练模型输入边界，不代表潜水安全结论；风险带排序 {check.baseline_risk_band}（基准 rank {bandRank[check.baseline_risk_band] ?? 0}）仅供对照。
          </p>
          <p className="disclaimer-line">{check.safety_disclaimer}</p>
        </div>
      )}
      {sensitivity.loading && <p className="muted">读取留档检查…</p>}
    </section>
  )
}
