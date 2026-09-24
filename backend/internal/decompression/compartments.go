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

// SensitivityDimension identifies which segment input is perturbed.
type SensitivityDimension string

const (
	SensitivityDepth    SensitivityDimension = "depth"
	SensitivityDuration SensitivityDimension = "duration"

	SensitivityDirectionDown = "minus_10_percent"
	SensitivityDirectionUp   = "plus_10_percent"

	SensitivityStatusComputed   = "computed"
	SensitivityStatusOutOfRange = "out_of_model_range"
	SensitivityPerturbationPct  = 10.0
)

// SensitivityVariant is one single-segment perturbation outcome.
type SensitivityVariant struct {
	SequenceNo           int                  `json:"sequence_no"`
	SegmentType          string               `json:"segment_type"`
	Dimension            SensitivityDimension `json:"dimension"`
	Direction            string               `json:"direction"`
	PerturbationPct      float64              `json:"perturbation_percent"`
	OriginalDepthM       float64              `json:"original_depth_m"`
	PerturbedDepthM      float64              `json:"perturbed_depth_m,omitempty"`
	OriginalDurationMin  float64              `json:"original_duration_min"`
	PerturbedDurationMin float64              `json:"perturbed_duration_min,omitempty"`
	Status               string               `json:"status"`
	ComparativeScore     float64              `json:"comparative_score,omitempty"`
	ScoreDelta           float64              `json:"score_delta"`
	HighestRiskBand      constants.RiskBand   `json:"highest_risk_band,omitempty"`
	BaselineRiskBand     constants.RiskBand   `json:"baseline_risk_band"`
	RiskBandChanged      bool                 `json:"risk_band_changed"`
	RiskFlagCodes        []string             `json:"risk_flag_codes,omitempty"`
	OutOfRangeReason     string               `json:"out_of_range_reason,omitempty"`
}

// SensitivityOutcome is the full independent sensitivity archive content.
type SensitivityOutcome struct {
	AlgorithmVersion        string               `json:"algorithm_version"`
	PerturbationPct         float64              `json:"perturbation_percent"`
	BaselineScore           float64              `json:"baseline_score"`
	BaselineRiskBand        constants.RiskBand   `json:"baseline_risk_band"`
	BaselineFlagCodes       []string             `json:"baseline_flag_codes"`
	Variants                []SensitivityVariant `json:"variants"`
	MostInfluentialSequence int                  `json:"most_influential_sequence"`
	MostInfluentialReason   string               `json:"most_influential_reason"`
	OutRangeCount           int                  `json:"out_range_count"`
	BandChangeCount         int                  `json:"band_change_count"`
	SafetyBoundary          string               `json:"safety_boundary"`
}

type segmentImpact struct {
	sequence   int
	outRange   int
	bandChange int
	maxAbs     float64
}

// RunSensitivityCheck replays the immutable assessment snapshot, perturbs one
// segment depth or duration at a time by perturbPercent, and recomputes with
// all other inputs fixed. Combinations rejected by the model boundary are
// archived as out-of-model-range instead of failing the whole check.
func RunSensitivityCheck(snapshot InputSnapshot, maxSegments int) (SensitivityOutcome, error) {
	plan := snapshot.Plan
	diver := snapshot.Diver
	segments := append([]model.ExposureSegment(nil), snapshot.Segments...)
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].SequenceNo < segments[j].SequenceNo })
	version := snapshot.AlgorithmVersion
	baseline, err := Run(plan, diver, segments, version, maxSegments)
	if err != nil {
		return SensitivityOutcome{}, fmt.Errorf("replay assessment baseline: %w", err)
	}
	baselineBand := HighestRiskBand(baseline.RiskFlags)
	outcome := SensitivityOutcome{
		AlgorithmVersion: version, PerturbationPct: SensitivityPerturbationPct,
		BaselineScore: baseline.ComparativeScore, BaselineRiskBand: baselineBand,
		BaselineFlagCodes: riskFlagCodes(baseline.RiskFlags),
		SafetyBoundary:    "Sensitivity evidence for training comparison only; the original immutable assessment snapshot is never modified.",
	}
	dimensions := []struct {
		dimension SensitivityDimension
		direction string
		factor    float64
	}{
		{SensitivityDepth, SensitivityDirectionDown, 1 - SensitivityPerturbationPct/100},
		{SensitivityDepth, SensitivityDirectionUp, 1 + SensitivityPerturbationPct/100},
		{SensitivityDuration, SensitivityDirectionDown, 1 - SensitivityPerturbationPct/100},
		{SensitivityDuration, SensitivityDirectionUp, 1 + SensitivityPerturbationPct/100},
	}
	maxAbsDelta := 0.0
	impacts := make(map[int]*segmentImpact)
	for _, segment := range segments {
		impacts[segment.SequenceNo] = &segmentImpact{sequence: segment.SequenceNo}
		for _, setup := range dimensions {
			variant := SensitivityVariant{
				SequenceNo: segment.SequenceNo, SegmentType: segment.SegmentType,
				Dimension: setup.dimension, Direction: setup.direction, PerturbationPct: SensitivityPerturbationPct,
				OriginalDepthM: round2(segment.DepthM), OriginalDurationMin: round2(segment.DurationMin),
				BaselineRiskBand: baselineBand,
			}
			perturbed := append([]model.ExposureSegment(nil), segments...)
			target := &perturbed[segment.SequenceNo-1]
			switch setup.dimension {
			case SensitivityDepth:
				value := round2(target.DepthM * setup.factor)
				variant.PerturbedDepthM = value
				target.DepthM = value
			case SensitivityDuration:
				value := round2(target.DurationMin * setup.factor)
				variant.PerturbedDurationMin = value
				target.DurationMin = value
			}
			result, runErr := Run(plan, diver, perturbed, version, maxSegments)
			impact := impacts[segment.SequenceNo]
			if runErr != nil {
				variant.Status = SensitivityStatusOutOfRange
				variant.OutOfRangeReason = runErr.Error()
				impact.outRange++
				outcome.OutRangeCount++
			} else {
				band := HighestRiskBand(result.RiskFlags)
				delta := round2(result.ComparativeScore - baseline.ComparativeScore)
				variant.Status = SensitivityStatusComputed
				variant.ComparativeScore = result.ComparativeScore
				variant.ScoreDelta = delta
				variant.HighestRiskBand = band
				variant.RiskBandChanged = riskBandRank(band) != riskBandRank(baselineBand)
				variant.RiskFlagCodes = riskFlagCodes(result.RiskFlags)
				if variant.RiskBandChanged {
					impact.bandChange++
					outcome.BandChangeCount++
				}
				impact.maxAbs = math.Max(impact.maxAbs, math.Abs(delta))
				maxAbsDelta = math.Max(maxAbsDelta, math.Abs(delta))
			}
			outcome.Variants = append(outcome.Variants, variant)
		}
	}
	outcome.MostInfluentialSequence, outcome.MostInfluentialReason = selectMostInfluential(segments, impacts, maxAbsDelta)
	return outcome, nil
}

func selectMostInfluential(segments []model.ExposureSegment, impacts map[int]*segmentImpact, maxAbsDelta float64) (int, string) {
	winner := 0
	var best *segmentImpact
	for _, segment := range segments {
		candidate := impacts[segment.SequenceNo]
		if best == nil || moreInfluential(candidate, best) {
			best = candidate
			winner = segment.SequenceNo
		}
	}
	if best == nil {
		return 0, "No exposure segments were available for perturbation."
	}
	switch {
	case best.bandChange > 0:
		return winner, fmt.Sprintf("Segment %d changes the highest risk band in %d of 4 perturbations; largest index movement %.2f points.", winner, best.bandChange, best.maxAbs)
	case best.outRange > 0:
		return winner, fmt.Sprintf("Segment %d pushes %d of 4 perturbations outside the model boundary; largest in-range index movement %.2f points.", winner, best.outRange, best.maxAbs)
	case best.maxAbs > 0:
		return winner, fmt.Sprintf("Segment %d shows the largest comparative-index movement, up to %.2f points, without crossing a risk band.", winner, best.maxAbs)
	default:
		return winner, fmt.Sprintf("No perturbation moves the comparative index more than %.2f points or changes a risk band; segment %d is listed first.", maxAbsDelta, winner)
	}
}

// moreInfluential ranks risk-band changes above boundary exits above score movement.
func moreInfluential(candidate, best *segmentImpact) bool {
	if candidate.bandChange != best.bandChange {
		return candidate.bandChange > best.bandChange
	}
	if candidate.outRange != best.outRange {
		return candidate.outRange > best.outRange
	}
	if candidate.maxAbs != best.maxAbs {
		return candidate.maxAbs > best.maxAbs
	}
	return candidate.sequence < best.sequence
}

func riskBandRank(band constants.RiskBand) int {
	switch band {
	case constants.RiskInvalid:
		return 3
	case constants.RiskElevated:
		return 2
	case constants.RiskCaution:
		return 1
	default:
		return 0
	}
}

func riskFlagCodes(flags []RiskFlag) []string {
	codes := make([]string, 0, len(flags))
	for _, flag := range flags {
		codes = append(codes, flag.Code)
	}
	return codes
}
