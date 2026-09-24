package decompression

import (
	"reflect"
	"testing"

	"commercial-diving-decompression-control/backend/internal/constants"
	"commercial-diving-decompression-control/backend/internal/model"
)

func sensitivitySnapshot(t *testing.T, plan model.DivePlan, diver model.DiverProfile, segments []model.ExposureSegment, version string) InputSnapshot {
	t.Helper()
	ordered, err := ValidateInput(plan, segments, 24)
	if err != nil {
		t.Fatal(err)
	}
	return BuildSnapshot(plan, diver, ordered, DefaultCompartments(), version)
}

func TestRunSensitivityCheckShape(t *testing.T) {
	plan, diver, segments := testPlanAndSegments(t)
	outcome, err := RunSensitivityCheck(sensitivitySnapshot(t, plan, diver, segments, "training-v1"), 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Variants) != len(segments)*4 {
		t.Fatalf("variants=%d want=%d", len(outcome.Variants), len(segments)*4)
	}
	if outcome.PerturbationPct != 10 {
		t.Fatalf("perturbation percent=%.1f want=10", outcome.PerturbationPct)
	}
	if outcome.BaselineScore <= 0 || outcome.BaselineRiskBand == "" {
		t.Fatalf("baseline not replayed: score=%.2f band=%s", outcome.BaselineScore, outcome.BaselineRiskBand)
	}
	if outcome.MostInfluentialSequence < 1 || outcome.MostInfluentialSequence > len(segments) {
		t.Fatalf("most influential sequence %d outside 1..%d", outcome.MostInfluentialSequence, len(segments))
	}
	directions := map[string]bool{}
	dimensions := map[SensitivityDimension]bool{}
	for _, variant := range outcome.Variants {
		directions[variant.Direction] = true
		dimensions[variant.Dimension] = true
		if variant.Status != SensitivityStatusComputed && variant.Status != SensitivityStatusOutOfRange {
			t.Fatalf("unexpected variant status %q", variant.Status)
		}
		if variant.Status == SensitivityStatusComputed && variant.OutOfRangeReason != "" {
			t.Fatalf("computed variant carries an out-of-range reason: %+v", variant)
		}
		if variant.Status == SensitivityStatusOutOfRange && (variant.HighestRiskBand != "" || variant.ComparativeScore != 0) {
			t.Fatalf("out-of-range variant must not carry a computed result: %+v", variant)
		}
	}
	if !directions[SensitivityDirectionUp] || !directions[SensitivityDirectionDown] {
		t.Fatalf("missing perturbation directions: %+v", directions)
	}
	if !dimensions[SensitivityDepth] || !dimensions[SensitivityDuration] {
		t.Fatalf("missing perturbation dimensions: %+v", dimensions)
	}
}

func TestRunSensitivityCheckVariants(t *testing.T) {
	mix, err := EncodeGasMix(GasMix{O2: 0.21, N2: 0.79})
	if err != nil {
		t.Fatal(err)
	}
	plan := model.DivePlan{ID: 2, PlanCode: "TEST-EDGE", WorksitePressureBar: 1, BreathingMixJSON: mix, PlanStatus: constants.PlanDraft, Version: 1}
	diver := model.DiverProfile{ID: 2, ProfileCode: "D-2", DefaultO2Fraction: 0.21, DefaultHeFraction: 0}
	segments := []model.ExposureSegment{
		{ID: 1, PlanID: 2, SequenceNo: 1, DepthM: 115, DurationMin: 7, GasMixJSON: mix, SegmentType: "descent"},
		{ID: 2, PlanID: 2, SequenceNo: 2, DepthM: 115, DurationMin: 10, GasMixJSON: mix, SegmentType: "bottom"},
		{ID: 3, PlanID: 2, SequenceNo: 3, DepthM: 0, DurationMin: 10, AscentRateMMin: 18, GasMixJSON: mix, SegmentType: "ascent"},
	}
	outcome, err := RunSensitivityCheck(sensitivitySnapshot(t, plan, diver, segments, "training-v1"), 24)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		predicate func(SensitivityVariant) bool
		wantFound bool
	}{
		{
			"depth plus ten at 115m leaves model boundary",
			func(v SensitivityVariant) bool {
				return v.SequenceNo == 2 && v.Dimension == SensitivityDepth && v.Direction == SensitivityDirectionUp && v.Status == SensitivityStatusOutOfRange && v.PerturbedDepthM == 126.5
			},
			true,
		},
		{
			"depth minus ten keeps surface segment at zero",
			func(v SensitivityVariant) bool {
				return v.SequenceNo == 3 && v.Dimension == SensitivityDepth && v.Status == SensitivityStatusComputed && v.PerturbedDepthM == 0
			},
			true,
		},
		{
			"duration minus ten on bottom segment stays positive and computes",
			func(v SensitivityVariant) bool {
				return v.SequenceNo == 2 && v.Dimension == SensitivityDuration && v.Direction == SensitivityDirectionDown && v.Status == SensitivityStatusComputed && v.PerturbedDurationMin == 9
			},
			true,
		},
		{
			"shortening a descent below the rate boundary is archived as out of range",
			func(v SensitivityVariant) bool {
				return v.SequenceNo == 1 && v.Dimension == SensitivityDuration && v.Direction == SensitivityDirectionDown && v.Status == SensitivityStatusOutOfRange
			},
			true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			found := false
			for _, variant := range outcome.Variants {
				if test.predicate(variant) {
					found = true
				}
			}
			if found != test.wantFound {
				t.Fatalf("predicate match=%v want=%v; out-of-range count=%d", found, test.wantFound, outcome.OutRangeCount)
			}
		})
	}
	if outcome.OutRangeCount == 0 {
		t.Fatal("expected at least one out-of-model-range perturbation for a 115m profile")
	}
	if outcome.MostInfluentialSequence == 0 {
		t.Fatal("expected a most influential segment to be marked")
	}
}

func TestRunSensitivityCheckIsDeterministic(t *testing.T) {
	plan, diver, segments := testPlanAndSegments(t)
	snapshot := sensitivitySnapshot(t, plan, diver, segments, "training-v1")
	first, err := RunSensitivityCheck(snapshot, 24)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunSensitivityCheck(snapshot, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Variants) != len(second.Variants) {
		t.Fatal("fixed inputs produced a different number of variants")
	}
	for index := range first.Variants {
		if !reflect.DeepEqual(first.Variants[index], second.Variants[index]) {
			t.Fatalf("variant %d differs across runs: %+v vs %+v", index, first.Variants[index], second.Variants[index])
		}
	}
	if first.MostInfluentialSequence != second.MostInfluentialSequence || first.OutRangeCount != second.OutRangeCount {
		t.Fatal("fixed inputs produced different sensitivity conclusions")
	}
}
