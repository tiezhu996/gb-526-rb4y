package decompression

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"commercial-diving-decompression-control/backend/internal/model"
)

func sensitivityProfile(t *testing.T, segments []model.ExposureSegment) (model.DivePlan, model.DiverProfile) {
	t.Helper()
	mix, err := EncodeGasMix(GasMix{O2: 0.21, N2: 0.79})
	if err != nil {
		t.Fatal(err)
	}
	for index := range segments {
		segments[index].PlanID = 1
		if segments[index].GasMixJSON == "" {
			segments[index].GasMixJSON = mix
		}
	}
	plan := model.DivePlan{ID: 1, PlanCode: "SENS-1", WorksitePressureBar: 1, BreathingMixJSON: mix, Version: 1}
	diver := model.DiverProfile{ID: 1, ProfileCode: "D-1", DefaultO2Fraction: 0.21, DefaultHeFraction: 0}
	return plan, diver
}

func findVariant(impact SegmentSensitivityImpact, axis SensitivityAxis, direction SensitivityDirection) SensitivityVariant {
	for _, variant := range impact.Variants {
		if variant.Axis == axis && variant.Direction == direction {
			return variant
		}
	}
	return SensitivityVariant{}
}

func TestRunSensitivityIsDeterministic(t *testing.T) {
	plan, diver := sensitivityProfile(t, []model.ExposureSegment{
		{SequenceNo: 1, DepthM: 20, DurationMin: 2, SegmentType: "descent"},
		{SequenceNo: 2, DepthM: 20, DurationMin: 18, SegmentType: "bottom"},
		{SequenceNo: 3, DepthM: 0, DurationMin: 2, AscentRateMMin: 18, SegmentType: "ascent"},
	})
	segments := []model.ExposureSegment{
		{PlanID: 1, SequenceNo: 1, DepthM: 20, DurationMin: 2, GasMixJSON: plan.BreathingMixJSON, SegmentType: "descent"},
		{PlanID: 1, SequenceNo: 2, DepthM: 20, DurationMin: 18, GasMixJSON: plan.BreathingMixJSON, SegmentType: "bottom"},
		{PlanID: 1, SequenceNo: 3, DepthM: 0, DurationMin: 2, AscentRateMMin: 18, GasMixJSON: plan.BreathingMixJSON, SegmentType: "ascent"},
	}
	first, err := RunSensitivity(BuildSnapshot(plan, diver, segments, DefaultCompartments(), "training-v1"), 24)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunSensitivity(BuildSnapshot(plan, diver, segments, DefaultCompartments(), "training-v1"), 24)
	if err != nil {
		t.Fatal(err)
	}
	if first.BaselineScore != second.BaselineScore || first.TotalVariants != second.TotalVariants {
		t.Fatal("fixed snapshot produced different sensitivity reports")
	}
	for index := range first.SegmentImpacts {
		a, b := first.SegmentImpacts[index], second.SegmentImpacts[index]
		if a.MaxAbsoluteScoreDelta != b.MaxAbsoluteScoreDelta || a.OutOfRangeCount != b.OutOfRangeCount {
			t.Fatalf("segment %d sensitivity output was not deterministic", a.SequenceNo)
		}
	}
}

func TestRunSensitivityVariantShape(t *testing.T) {
	plan, diver := sensitivityProfile(t, []model.ExposureSegment{
		{SequenceNo: 1, DepthM: 30, DurationMin: 3, SegmentType: "descent"},
		{SequenceNo: 2, DepthM: 30, DurationMin: 24, SegmentType: "bottom"},
		{SequenceNo: 3, DepthM: 15, DurationMin: 2, AscentRateMMin: 18, SegmentType: "ascent"},
		{SequenceNo: 4, DepthM: 0, DurationMin: 2, AscentRateMMin: 18, SegmentType: "ascent"},
	})
	segments := []model.ExposureSegment{
		{PlanID: 1, SequenceNo: 1, DepthM: 30, DurationMin: 3, GasMixJSON: plan.BreathingMixJSON, SegmentType: "descent"},
		{PlanID: 1, SequenceNo: 2, DepthM: 30, DurationMin: 24, GasMixJSON: plan.BreathingMixJSON, SegmentType: "bottom"},
		{PlanID: 1, SequenceNo: 3, DepthM: 15, DurationMin: 2, AscentRateMMin: 18, GasMixJSON: plan.BreathingMixJSON, SegmentType: "ascent"},
		{PlanID: 1, SequenceNo: 4, DepthM: 0, DurationMin: 2, AscentRateMMin: 18, GasMixJSON: plan.BreathingMixJSON, SegmentType: "ascent"},
	}
	snapshot := BuildSnapshot(plan, diver, segments, DefaultCompartments(), "training-v1")
	report, err := RunSensitivity(snapshot, 24)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalVariants != 4*len(segments) || report.OutOfRangeVariants != 0 || report.ComputableVariants != report.TotalVariants {
		t.Fatalf("unexpected variant counts: total=%d computable=%d oor=%d", report.TotalVariants, report.ComputableVariants, report.OutOfRangeVariants)
	}
	if math.Abs(report.AdjustmentRatio-0.1) > 0.0001 {
		t.Fatalf("adjustment ratio = %.2f", report.AdjustmentRatio)
	}
	for _, impact := range report.SegmentImpacts {
		if len(impact.Variants) != 4 || impact.ComputableCount != 4 {
			t.Fatalf("segment %d must expose four computable variants", impact.SequenceNo)
		}
		depthDown := findVariant(impact, SensitivityAxisDepth, SensitivityDirectionDecrease)
		if impact.DepthM > 0 {
			if depthDown.AdjustedDepthM >= depthDown.OriginalDepthM {
				t.Fatalf("segment %d depth decrease nudged the wrong way: %.2f >= %.2f", impact.SequenceNo, depthDown.AdjustedDepthM, depthDown.OriginalDepthM)
			}
			if math.Abs(depthDown.AdjustedDepthM-depthDown.OriginalDepthM*0.9) > 0.011 {
				t.Fatalf("segment %d depth decrease is not -10%%: %.3f", impact.SequenceNo, depthDown.AdjustedDepthM)
			}
		} else if depthDown.AdjustedDepthM != 0 {
			t.Fatalf("surface segment %d depth decrease must stay at 0 m, got %.2f", impact.SequenceNo, depthDown.AdjustedDepthM)
		}
		durationUp := findVariant(impact, SensitivityAxisDuration, SensitivityDirectionIncrease)
		if durationUp.AdjustedDuration <= durationUp.OriginalDuration {
			t.Fatalf("segment %d duration increase nudged the wrong way", impact.SequenceNo)
		}
		if depthDown.OutOfRangeReason != "" {
			t.Fatalf("computable variant carries an out-of-range reason: %s", depthDown.OutOfRangeReason)
		}
	}
	if report.MostAffectedSequenceNo < 1 || report.MostAffectedSequenceNo > len(segments) {
		t.Fatalf("most affected segment %d is outside the snapshot", report.MostAffectedSequenceNo)
	}
	if snapshot.Segments[0].DepthM != 30 || snapshot.Segments[1].DurationMin != 24 {
		t.Fatal("sensitivity replay mutated the archived snapshot inputs")
	}
}

func TestRunSensitivityOutOfRangeVariants(t *testing.T) {
	tests := []struct {
		name              string
		segments          []model.ExposureSegment
		wantOutOfRange    int
		wantNonComputable map[int]map[SensitivityAxis]SensitivityDirection
		wantReasonAny     []string
	}{
		{
			name: "depth increase crosses 120m boundary and shortened transition crosses 18m/min",
			segments: []model.ExposureSegment{
				{SequenceNo: 1, DepthM: 115, DurationMin: 7, SegmentType: "descent"},
				{SequenceNo: 2, DepthM: 115, DurationMin: 10, SegmentType: "bottom"},
				{SequenceNo: 3, DepthM: 0, DurationMin: 7, AscentRateMMin: 18, SegmentType: "ascent"},
			},
			wantOutOfRange: 4,
			wantNonComputable: map[int]map[SensitivityAxis]SensitivityDirection{
				1: {SensitivityAxisDepth: SensitivityDirectionIncrease, SensitivityAxisDuration: SensitivityDirectionDecrease},
				2: {SensitivityAxisDepth: SensitivityDirectionIncrease},
				3: {SensitivityAxisDuration: SensitivityDirectionDecrease},
			},
			wantReasonAny: []string{"between 0 and 120", "boundary"},
		},
		{
			name: "duration increase crosses 240min boundary",
			segments: []model.ExposureSegment{
				{SequenceNo: 1, DepthM: 20, DurationMin: 2, SegmentType: "descent"},
				{SequenceNo: 2, DepthM: 20, DurationMin: 220, SegmentType: "bottom"},
				{SequenceNo: 3, DepthM: 0, DurationMin: 2, AscentRateMMin: 18, SegmentType: "ascent"},
			},
			wantOutOfRange: 1,
			wantNonComputable: map[int]map[SensitivityAxis]SensitivityDirection{
				2: {SensitivityAxisDuration: SensitivityDirectionIncrease},
			},
			wantReasonAny: []string{"240"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, diver := sensitivityProfile(t, test.segments)
			snapshot := BuildSnapshot(plan, diver, test.segments, DefaultCompartments(), "training-v1")
			report, err := RunSensitivity(snapshot, 24)
			if err != nil {
				t.Fatal(err)
			}
			if report.OutOfRangeVariants != test.wantOutOfRange {
				t.Fatalf("out-of-range variants = %d want %d", report.OutOfRangeVariants, test.wantOutOfRange)
			}
			if report.ComputableVariants+report.OutOfRangeVariants != report.TotalVariants {
				t.Fatal("computable and out-of-range variants must cover every perturbation")
			}
			actual := map[int]map[SensitivityAxis]SensitivityDirection{}
			for _, impact := range report.SegmentImpacts {
				for _, variant := range impact.Variants {
					if variant.Computable {
						continue
					}
					if variant.OutOfRangeReason == "" {
						t.Fatalf("segment %d %s/%s is non-computable without a reason", variant.SequenceNo, variant.Axis, variant.Direction)
					}
					if variant.ScoreDelta != 0 || variant.BandRankDelta != 0 {
						t.Fatal("non-computable variant must not carry comparative index or risk band deltas")
					}
					if _, ok := actual[variant.SequenceNo]; !ok {
						actual[variant.SequenceNo] = map[SensitivityAxis]SensitivityDirection{}
					}
					actual[variant.SequenceNo][variant.Axis] = variant.Direction
					matched := false
					for _, fragment := range test.wantReasonAny {
						if strings.Contains(variant.OutOfRangeReason, fragment) {
							matched = true
						}
					}
					if !matched {
						t.Fatalf("reason %q does not mention any of %v", variant.OutOfRangeReason, test.wantReasonAny)
					}
				}
			}
			if len(flattenVariantKeys(actual)) != test.wantOutOfRange {
				t.Fatalf("non-computable set %v does not match expected count %d", flattenVariantKeys(actual), test.wantOutOfRange)
			}
			for sequenceNo, axes := range test.wantNonComputable {
				for axis, direction := range axes {
					if actual[sequenceNo][axis] != direction {
						t.Fatalf("expected segment %d %s/%s to be non-computable", sequenceNo, axis, direction)
					}
				}
			}
		})
	}
}

func flattenVariantKeys(grouped map[int]map[SensitivityAxis]SensitivityDirection) []string {
	keys := []string{}
	for sequenceNo, axes := range grouped {
		for axis, direction := range axes {
			keys = append(keys, fmt.Sprintf("%d/%s/%s", sequenceNo, axis, direction))
		}
	}
	return keys
}

func TestRunSensitivityMarksMostAffectedSegment(t *testing.T) {
	// Bottom at 20 m for 85 min keeps the aggregate duration at 89 min; only
	// the bottom segment's duration +10% variant crosses the 90-minute
	// aggregate caution threshold, so it alone moves the comparative index.
	plan, diver := sensitivityProfile(t, nil)
	segments := []model.ExposureSegment{
		{PlanID: 1, SequenceNo: 1, DepthM: 20, DurationMin: 2, GasMixJSON: plan.BreathingMixJSON, SegmentType: "descent"},
		{PlanID: 1, SequenceNo: 2, DepthM: 20, DurationMin: 85, GasMixJSON: plan.BreathingMixJSON, SegmentType: "bottom"},
		{PlanID: 1, SequenceNo: 3, DepthM: 0, DurationMin: 2, AscentRateMMin: 18, GasMixJSON: plan.BreathingMixJSON, SegmentType: "ascent"},
	}
	report, err := RunSensitivity(BuildSnapshot(plan, diver, segments, DefaultCompartments(), "training-v1"), 24)
	if err != nil {
		t.Fatal(err)
	}
	if report.MostAffectedSequenceNo != 2 || report.MostAffectedAxis != SensitivityAxisDuration || report.MostAffectedDirection != SensitivityDirectionIncrease {
		t.Fatalf("most affected = S%d %s/%s, want S2 duration/increase", report.MostAffectedSequenceNo, report.MostAffectedAxis, report.MostAffectedDirection)
	}
	var marked SegmentSensitivityImpact
	for _, impact := range report.SegmentImpacts {
		if impact.SequenceNo == report.MostAffectedSequenceNo {
			marked = impact
		}
		for _, variant := range impact.Variants {
			if variant.Computable && math.Abs(variant.ScoreDelta) > impact.MaxAbsoluteScoreDelta {
				t.Fatalf("segment %d max index delta %.2f is smaller than variant delta %.2f", impact.SequenceNo, impact.MaxAbsoluteScoreDelta, variant.ScoreDelta)
			}
		}
	}
	if marked.MaxAbsoluteScoreDelta != 7 {
		t.Fatalf("crossing the aggregate duration threshold must move the index by 7, got %.2f", marked.MaxAbsoluteScoreDelta)
	}
	for _, impact := range report.SegmentImpacts {
		if impact.MaxAbsoluteScoreDelta > marked.MaxAbsoluteScoreDelta {
			t.Fatalf("segment %d moves the index more than the marked segment %d", impact.SequenceNo, marked.SequenceNo)
		}
	}
}

func TestRunSensitivityStableIndexNamesNoSegment(t *testing.T) {
	// A shallow, short profile stays inside every model threshold even after
	// +/-10% nudges, so no segment may be named as most affected.
	plan, diver := sensitivityProfile(t, nil)
	segments := []model.ExposureSegment{
		{PlanID: 1, SequenceNo: 1, DepthM: 10, DurationMin: 2, GasMixJSON: plan.BreathingMixJSON, SegmentType: "descent"},
		{PlanID: 1, SequenceNo: 2, DepthM: 10, DurationMin: 10, GasMixJSON: plan.BreathingMixJSON, SegmentType: "bottom"},
		{PlanID: 1, SequenceNo: 3, DepthM: 0, DurationMin: 2, AscentRateMMin: 18, GasMixJSON: plan.BreathingMixJSON, SegmentType: "ascent"},
	}
	report, err := RunSensitivity(BuildSnapshot(plan, diver, segments, DefaultCompartments(), "training-v1"), 24)
	if err != nil {
		t.Fatal(err)
	}
	if report.MostAffectedSequenceNo != 0 {
		t.Fatalf("stable index must not name a most affected segment, got S%d", report.MostAffectedSequenceNo)
	}
	for _, impact := range report.SegmentImpacts {
		if impact.MaxAbsoluteScoreDelta != 0 {
			t.Fatalf("segment %d unexpectedly moved the index by %.2f", impact.SequenceNo, impact.MaxAbsoluteScoreDelta)
		}
	}
}
