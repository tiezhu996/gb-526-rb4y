package decompression

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"commercial-diving-decompression-control/backend/internal/constants"
	"commercial-diving-decompression-control/backend/internal/model"
)

type CompartmentSpec struct {
	Name          string  `json:"name"`
	N2HalfTimeMin float64 `json:"n2_half_time_min"`
	HeHalfTimeMin float64 `json:"he_half_time_min"`
}

type LoadPoint struct {
	SequenceNo     int     `json:"sequence_no"`
	DepthM         float64 `json:"depth_m"`
	ElapsedMin     float64 `json:"elapsed_min"`
	AmbientBar     float64 `json:"ambient_bar"`
	InspiredN2Bar  float64 `json:"inspired_n2_bar"`
	InspiredHeBar  float64 `json:"inspired_he_bar"`
	N2LoadBar      float64 `json:"n2_load_bar"`
	HeLoadBar      float64 `json:"he_load_bar"`
	TotalInertBar  float64 `json:"total_inert_bar"`
	RelativeChange float64 `json:"relative_change"`
}

type CompartmentCurve struct {
	Name          string      `json:"name"`
	N2HalfTimeMin float64     `json:"n2_half_time_min"`
	HeHalfTimeMin float64     `json:"he_half_time_min"`
	Points        []LoadPoint `json:"points"`
}

type ModelAssumptions struct {
	Purpose                string            `json:"purpose"`
	PressureConversion     string            `json:"pressure_conversion"`
	WaterVaporBar          float64           `json:"water_vapor_bar"`
	Kinetics               string            `json:"kinetics"`
	ConfiguredCompartments []CompartmentSpec `json:"configured_compartments"`
	ProhibitedUse          []string          `json:"prohibited_use"`
}

type InputSnapshot struct {
	Plan             model.DivePlan          `json:"plan"`
	Diver            model.DiverProfile      `json:"diver"`
	Segments         []model.ExposureSegment `json:"segments"`
	Compartments     []CompartmentSpec       `json:"compartments"`
	AlgorithmVersion string                  `json:"algorithm_version"`
	SafetyBoundary   string                  `json:"safety_boundary"`
}

type Result struct {
	Snapshot         InputSnapshot      `json:"snapshot"`
	Curves           []CompartmentCurve `json:"curves"`
	RiskFlags        []RiskFlag         `json:"risk_flags"`
	ComparativeScore float64            `json:"comparative_score"`
	Assumptions      ModelAssumptions   `json:"assumptions"`
}

func DefaultCompartments() []CompartmentSpec {
	return []CompartmentSpec{
		{Name: "C05", N2HalfTimeMin: 5, HeHalfTimeMin: 2},
		{Name: "C10", N2HalfTimeMin: 10, HeHalfTimeMin: 4},
		{Name: "C20", N2HalfTimeMin: 20, HeHalfTimeMin: 8},
		{Name: "C40", N2HalfTimeMin: 40, HeHalfTimeMin: 16},
		{Name: "C80", N2HalfTimeMin: 80, HeHalfTimeMin: 32},
		{Name: "C120", N2HalfTimeMin: 120, HeHalfTimeMin: 48},
	}
}

func BuildSnapshot(plan model.DivePlan, diver model.DiverProfile, segments []model.ExposureSegment, specs []CompartmentSpec, version string) InputSnapshot {
	ordered := append([]model.ExposureSegment(nil), segments...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].SequenceNo < ordered[j].SequenceNo })
	return InputSnapshot{
		Plan: plan, Diver: diver, Segments: ordered, Compartments: append([]CompartmentSpec(nil), specs...),
		AlgorithmVersion: version,
		SafetyBoundary:   "Training comparison and decision support only; not an executable decompression schedule or safety clearance.",
	}
}

func Run(plan model.DivePlan, diver model.DiverProfile, segments []model.ExposureSegment, modelVersion string, maxSegments int) (Result, error) {
	ordered, err := ValidateInput(plan, segments, maxSegments)
	if err != nil {
		return Result{}, err
	}
	specs := DefaultCompartments()
	curves, err := calculateCurves(plan, diver, ordered, specs)
	if err != nil {
		return Result{}, err
	}
	flags := EvaluateRisk(plan, ordered, curves)
	return Result{
		Snapshot:         BuildSnapshot(plan, diver, ordered, specs, modelVersion),
		Curves:           curves,
		RiskFlags:        flags,
		ComparativeScore: ComparativeScore(flags, curves),
		Assumptions: ModelAssumptions{
			Purpose:                "Deterministic offline training comparison",
			PressureConversion:     "ambient_bar = worksite_pressure_bar + depth_m / 10",
			WaterVaporBar:          waterVaporPressureBar,
			Kinetics:               "Constant-depth exponential approach per segment; helium half-time is independently configured",
			ConfiguredCompartments: specs,
			ProhibitedUse:          []string{"live dive control", "medical diagnosis", "executable stop depth or duration", "equipment connection", "safety certification"},
		},
	}, nil
}

func calculateCurves(plan model.DivePlan, diver model.DiverProfile, segments []model.ExposureSegment, specs []CompartmentSpec) ([]CompartmentCurve, error) {
	baselineAmbient := AmbientPressureBar(0, plan.WorksitePressureBar)
	defaultMix := GasMix{O2: diver.DefaultO2Fraction, He: diver.DefaultHeFraction, N2: diver.DefaultN2Fraction()}
	if err := defaultMix.Validate(); err != nil {
		return nil, fmt.Errorf("default profile gas assumption: %w", err)
	}
	baselineN2 := InspiredPartialPressureBar(baselineAmbient, defaultMix.N2)
	baselineHe := InspiredPartialPressureBar(baselineAmbient, defaultMix.He)
	curves := make([]CompartmentCurve, 0, len(specs))
	for _, spec := range specs {
		n2Load, heLoad, elapsed := baselineN2, baselineHe, 0.0
		points := make([]LoadPoint, 0, len(segments)+1)
		points = append(points, LoadPoint{SequenceNo: 0, DepthM: 0, ElapsedMin: 0, AmbientBar: baselineAmbient, InspiredN2Bar: baselineN2, InspiredHeBar: baselineHe, N2LoadBar: n2Load, HeLoadBar: heLoad, TotalInertBar: round4(n2Load + heLoad)})
		for _, segment := range segments {
			mix, err := DecodeGasMix(segment.GasMixJSON)
			if err != nil {
				return nil, fmt.Errorf("segment %d gas: %w", segment.SequenceNo, err)
			}
			ambient := AmbientPressureBar(segment.DepthM, plan.WorksitePressureBar)
			inspiredN2 := InspiredPartialPressureBar(ambient, mix.N2)
			inspiredHe := InspiredPartialPressureBar(ambient, mix.He)
			n2Load = exponentialLoad(n2Load, inspiredN2, segment.DurationMin, spec.N2HalfTimeMin)
			heLoad = exponentialLoad(heLoad, inspiredHe, segment.DurationMin, spec.HeHalfTimeMin)
			elapsed += segment.DurationMin
			total := n2Load + heLoad
			baseline := math.Max(0.0001, baselineN2+baselineHe)
			points = append(points, LoadPoint{
				SequenceNo: segment.SequenceNo, DepthM: round2(segment.DepthM), ElapsedMin: round2(elapsed), AmbientBar: ambient,
				InspiredN2Bar: inspiredN2, InspiredHeBar: inspiredHe, N2LoadBar: round4(n2Load), HeLoadBar: round4(heLoad),
				TotalInertBar: round4(total), RelativeChange: round4((total - baseline) / baseline),
			})
		}
		curves = append(curves, CompartmentCurve{Name: spec.Name, N2HalfTimeMin: spec.N2HalfTimeMin, HeHalfTimeMin: spec.HeHalfTimeMin, Points: points})
	}
	return curves, nil
}

func exponentialLoad(initial, inspired, duration, halfTime float64) float64 {
	if duration <= 0 || halfTime <= 0 {
		return initial
	}
	fraction := 1 - math.Exp(-math.Ln2*duration/halfTime)
	return initial + (inspired-initial)*fraction
}

func MarshalResult(result Result) (snapshot, curves, flags, assumptions string, err error) {
	values := []any{result.Snapshot, result.Curves, result.RiskFlags, result.Assumptions}
	encoded := make([]string, len(values))
	for index, value := range values {
		data, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return "", "", "", "", fmt.Errorf("marshal model result part %d: %w", index, marshalErr)
		}
		encoded[index] = string(data)
	}
	return encoded[0], encoded[1], encoded[2], encoded[3], nil
}

// Sensitivity inputs are fixed at a symmetrical ten-percent nudge; planners can
// only ask the engine to re-run the preserved snapshot, never to edit it.
const SensitivityAdjustmentRatio = 0.1

type SensitivityAxis string

const (
	SensitivityAxisDepth    SensitivityAxis = "depth"
	SensitivityAxisDuration SensitivityAxis = "duration"
)

type SensitivityDirection string

const (
	SensitivityDirectionDecrease SensitivityDirection = "decrease"
	SensitivityDirectionIncrease SensitivityDirection = "increase"
)

type SensitivityVariant struct {
	SequenceNo       int                  `json:"sequence_no"`
	Axis             SensitivityAxis      `json:"axis"`
	Direction        SensitivityDirection `json:"direction"`
	AdjustmentRatio  float64              `json:"adjustment_ratio"`
	OriginalDepthM   float64              `json:"original_depth_m"`
	AdjustedDepthM   float64              `json:"adjusted_depth_m"`
	OriginalDuration float64              `json:"original_duration_min"`
	AdjustedDuration float64              `json:"adjusted_duration_min"`
	Computable       bool                 `json:"computable"`
	OutOfRangeReason string               `json:"out_of_range_reason"`
	ComparativeScore float64              `json:"comparative_score"`
	ScoreDelta       float64              `json:"score_delta"`
	RiskBand         constants.RiskBand   `json:"risk_band"`
	BandRankDelta    int                  `json:"band_rank_delta"`
	AddedRiskFlags   []string             `json:"added_risk_flags"`
	RemovedRiskFlags []string             `json:"removed_risk_flags"`
}

type SegmentSensitivityImpact struct {
	SequenceNo            int                  `json:"sequence_no"`
	SegmentType           string               `json:"segment_type"`
	DepthM                float64              `json:"depth_m"`
	DurationMin           float64              `json:"duration_min"`
	ComputableCount       int                  `json:"computable_count"`
	OutOfRangeCount       int                  `json:"out_of_range_count"`
	MaxAbsoluteScoreDelta float64              `json:"max_absolute_score_delta"`
	MaxBandRankDelta      int                  `json:"max_band_rank_delta"`
	DriverAxis            SensitivityAxis      `json:"driver_axis"`
	DriverDirection       SensitivityDirection `json:"driver_direction"`
	Variants              []SensitivityVariant `json:"variants"`
}

type SensitivityReport struct {
	AlgorithmVersion       string                     `json:"algorithm_version"`
	AdjustmentRatio        float64                    `json:"adjustment_ratio"`
	BaselineScore          float64                    `json:"baseline_comparative_score"`
	BaselineRiskBand       constants.RiskBand         `json:"baseline_risk_band"`
	TotalVariants          int                        `json:"total_variants"`
	ComputableVariants     int                        `json:"computable_variants"`
	OutOfRangeVariants     int                        `json:"out_of_range_variants"`
	MostAffectedSequenceNo int                        `json:"most_affected_sequence_no"`
	MostAffectedAxis       SensitivityAxis            `json:"most_affected_axis"`
	MostAffectedDirection  SensitivityDirection       `json:"most_affected_direction"`
	SegmentImpacts         []SegmentSensitivityImpact `json:"segment_impacts"`
	Disclaimer             string                     `json:"disclaimer"`
}

type sensitivityPerturbation struct {
	axis      SensitivityAxis
	direction SensitivityDirection
}

var sensitivityPerturbations = []sensitivityPerturbation{
	{axis: SensitivityAxisDepth, direction: SensitivityDirectionDecrease},
	{axis: SensitivityAxisDepth, direction: SensitivityDirectionIncrease},
	{axis: SensitivityAxisDuration, direction: SensitivityDirectionDecrease},
	{axis: SensitivityAxisDuration, direction: SensitivityDirectionIncrease},
}

// RunSensitivity replays an immutable snapshot and re-runs the model once per
// segment for each +/-10% depth/duration perturbation. The original snapshot,
// plan and live segments are never modified; a perturbation that fails model
// input validation is archived as a non-computable variant, not as a run error.
func RunSensitivity(snapshot InputSnapshot, maxSegments int) (SensitivityReport, error) {
	plan := snapshot.Plan
	diver := snapshot.Diver
	segments := append([]model.ExposureSegment(nil), snapshot.Segments...)
	baseline, err := Run(plan, diver, segments, snapshot.AlgorithmVersion, maxSegments)
	if err != nil {
		return SensitivityReport{}, fmt.Errorf("replay sensitivity baseline: %w", err)
	}
	baselineFlags := flagCodeSet(baseline.RiskFlags)
	baselineBand := HighestRiskBand(baseline.RiskFlags)
	report := SensitivityReport{
		AlgorithmVersion: snapshot.AlgorithmVersion,
		AdjustmentRatio:  SensitivityAdjustmentRatio,
		BaselineScore:    baseline.ComparativeScore,
		BaselineRiskBand: baselineBand,
		Disclaimer:       "Sensitivity nudges are archived training comparisons of the immutable snapshot; they do not change the assessment and are not executable decompression guidance.",
	}
	for _, segment := range segments {
		impact := SegmentSensitivityImpact{
			SequenceNo: segment.SequenceNo, SegmentType: segment.SegmentType,
			DepthM: round2(segment.DepthM), DurationMin: round2(segment.DurationMin),
			Variants: make([]SensitivityVariant, 0, len(sensitivityPerturbations)),
		}
		for _, perturbation := range sensitivityPerturbations {
			variant := SensitivityVariant{
				SequenceNo:       segment.SequenceNo,
				Axis:             perturbation.axis,
				Direction:        perturbation.direction,
				AdjustmentRatio:  SensitivityAdjustmentRatio,
				OriginalDepthM:   round2(segment.DepthM),
				AdjustedDepthM:   round2(segment.DepthM),
				OriginalDuration: round2(segment.DurationMin),
				AdjustedDuration: round2(segment.DurationMin),
				AddedRiskFlags:   []string{},
				RemovedRiskFlags: []string{},
			}
			candidate := segment
			factor := 1 + SensitivityAdjustmentRatio
			if perturbation.direction == SensitivityDirectionDecrease {
				factor = 1 - SensitivityAdjustmentRatio
			}
			if perturbation.axis == SensitivityAxisDepth {
				candidate.DepthM = round4(segment.DepthM * factor)
				variant.AdjustedDepthM = round2(candidate.DepthM)
			} else {
				candidate.DurationMin = round4(segment.DurationMin * factor)
				variant.AdjustedDuration = round2(candidate.DurationMin)
			}
			perturbed := append(append([]model.ExposureSegment(nil), segments[:segment.SequenceNo-1]...), candidate)
			perturbed = append(perturbed, segments[segment.SequenceNo:]...)
			result, runErr := Run(plan, diver, perturbed, snapshot.AlgorithmVersion, maxSegments)
			report.TotalVariants++
			if runErr != nil {
				variant.Computable = false
				variant.OutOfRangeReason = runErr.Error()
				impact.OutOfRangeCount++
				report.OutOfRangeVariants++
				impact.Variants = append(impact.Variants, variant)
				continue
			}
			variant.Computable = true
			variant.ComparativeScore = result.ComparativeScore
			variant.ScoreDelta = round2(result.ComparativeScore - baseline.ComparativeScore)
			variant.RiskBand = HighestRiskBand(result.RiskFlags)
			variant.BandRankDelta = RiskBandRank(variant.RiskBand) - RiskBandRank(baselineBand)
			variant.AddedRiskFlags, variant.RemovedRiskFlags = diffRiskFlags(baselineFlags, flagCodeSet(result.RiskFlags))
			impact.ComputableCount++
			report.ComputableVariants++
			if absScore := math.Abs(variant.ScoreDelta); absScore > impact.MaxAbsoluteScoreDelta {
				impact.MaxAbsoluteScoreDelta = absScore
			}
			if variant.BandRankDelta > impact.MaxBandRankDelta {
				impact.MaxBandRankDelta = variant.BandRankDelta
			}
			impact.Variants = append(impact.Variants, variant)
		}
		report.SegmentImpacts = append(report.SegmentImpacts, impact)
	}
	selectMostAffectedSegment(&report)
	return report, nil
}

func diffRiskFlags(baseline, variant map[string]struct{}) (added, removed []string) {
	added, removed = []string{}, []string{}
	for code := range variant {
		if _, exists := baseline[code]; !exists {
			added = append(added, code)
		}
	}
	for code := range baseline {
		if _, exists := variant[code]; !exists {
			removed = append(removed, code)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

// selectMostAffectedSegment ranks segments by absolute comparative-index change
// and, as deterministic tie-breakers, by upward risk-band movement then the
// earliest segment and perturbation order.
func selectMostAffectedSegment(report *SensitivityReport) {
	bestIndex := -1
	for index, impact := range report.SegmentImpacts {
		if impact.ComputableCount == 0 {
			continue
		}
		if bestIndex == -1 {
			bestIndex = index
			continue
		}
		best := report.SegmentImpacts[bestIndex]
		if impact.MaxAbsoluteScoreDelta < best.MaxAbsoluteScoreDelta {
			continue
		}
		if impact.MaxAbsoluteScoreDelta == best.MaxAbsoluteScoreDelta && impact.MaxBandRankDelta <= best.MaxBandRankDelta {
			continue
		}
		bestIndex = index
	}
	if bestIndex < 0 {
		return
	}
	impact := &report.SegmentImpacts[bestIndex]
	// When no computable perturbation moves the comparative index, the honest
	// answer to a robustness question is "no segment stands out"; marking an
	// arbitrary segment would mislead the reviewing supervisor.
	if impact.MaxAbsoluteScoreDelta == 0 {
		return
	}
	for _, variant := range impact.Variants {
		if !variant.Computable {
			continue
		}
		if math.Abs(variant.ScoreDelta) == impact.MaxAbsoluteScoreDelta {
			impact.DriverAxis = variant.Axis
			impact.DriverDirection = variant.Direction
			report.MostAffectedSequenceNo = impact.SequenceNo
			report.MostAffectedAxis = variant.Axis
			report.MostAffectedDirection = variant.Direction
			return
		}
	}
}
