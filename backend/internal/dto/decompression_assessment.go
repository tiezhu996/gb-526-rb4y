package dto

import (
	"encoding/json"
	"fmt"
	"time"

	"commercial-diving-decompression-control/backend/internal/constants"
	"commercial-diving-decompression-control/backend/internal/decompression"
	"commercial-diving-decompression-control/backend/internal/model"
)

type RunAssessmentRequest struct {
	PlanVersion uint `json:"plan_version" binding:"required,min=1"`
}

type AssessmentResponse struct {
	ID               uint                             `json:"id"`
	PlanID           uint                             `json:"plan_id"`
	AssessmentStatus string                           `json:"assessment_status"`
	AlgorithmVersion string                           `json:"algorithm_version"`
	InputSnapshot    decompression.InputSnapshot      `json:"input_snapshot"`
	CompartmentLoads []decompression.CompartmentCurve `json:"compartment_loads"`
	RiskFlags        []decompression.RiskFlag         `json:"risk_flags"`
	HighestRiskBand  constants.RiskBand               `json:"highest_risk_band"`
	ComparativeScore float64                          `json:"comparative_score"`
	Assumptions      decompression.ModelAssumptions   `json:"assumptions"`
	CreatedAt        time.Time                        `json:"created_at"`
	ReviewedAt       *time.Time                       `json:"reviewed_at"`
	SafetyDisclaimer string                           `json:"safety_disclaimer"`
}

type AssessmentComparison struct {
	Left       AssessmentResponse `json:"left"`
	Right      AssessmentResponse `json:"right"`
	ScoreDelta float64            `json:"score_delta"`
	FlagDelta  int                `json:"flag_delta"`
	Summary    []string           `json:"summary"`
	Disclaimer string             `json:"disclaimer"`
}

type SensitivityVariantResponse = decompression.SensitivityVariant

type SensitivityCheckResponse struct {
	ID                      uint                         `json:"id"`
	AssessmentID            uint                         `json:"assessment_id"`
	RunSerial               int                          `json:"run_serial"`
	AlgorithmVersion        string                       `json:"algorithm_version"`
	PerturbationPercent     float64                      `json:"perturbation_percent"`
	BaselineScore           float64                      `json:"baseline_score"`
	BaselineRiskBand        constants.RiskBand           `json:"baseline_risk_band"`
	MostInfluentialSequence int                          `json:"most_influential_sequence"`
	MostInfluentialReason   string                       `json:"most_influential_reason"`
	OutRangeCount           int                          `json:"out_range_count"`
	BandChangeCount         int                          `json:"band_change_count"`
	Variants                []SensitivityVariantResponse `json:"variants"`
	CreatedBy               uint                         `json:"created_by"`
	CreatedAt               time.Time                    `json:"created_at"`
	SafetyDisclaimer        string                       `json:"safety_disclaimer"`
}

func DecodeSensitivityCheck(item model.SensitivityCheck) (SensitivityCheckResponse, error) {
	response := SensitivityCheckResponse{
		ID: item.ID, AssessmentID: item.AssessmentID, RunSerial: item.RunSerial,
		AlgorithmVersion: item.AlgorithmVersion, PerturbationPercent: item.PerturbationPercent,
		BaselineScore: item.BaselineScore, BaselineRiskBand: item.BaselineRiskBand,
		MostInfluentialSequence: item.MostInfluentialSequence, MostInfluentialReason: item.MostInfluentialReason,
		OutRangeCount: item.OutRangeCount, BandChangeCount: item.BandChangeCount,
		CreatedBy: item.CreatedBy, CreatedAt: item.CreatedAt, SafetyDisclaimer: SafetyDisclaimer,
	}
	if err := json.Unmarshal([]byte(item.VariantsJSON), &response.Variants); err != nil {
		return SensitivityCheckResponse{}, fmt.Errorf("decode sensitivity check %d variants: %w", item.ID, err)
	}
	if response.Variants == nil {
		response.Variants = []SensitivityVariantResponse{}
	}
	return response, nil
}

const SafetyDisclaimer = "Training and decision support only. This result is not medical advice, a certified dive table, a safety clearance, or an executable decompression instruction. Human supervisor review is required."

func DecodeAssessment(item model.DecompressionAssessment) (AssessmentResponse, error) {
	response := AssessmentResponse{ID: item.ID, PlanID: item.PlanID, AssessmentStatus: item.AssessmentStatus, AlgorithmVersion: item.AlgorithmVersion, HighestRiskBand: item.HighestRiskBand, ComparativeScore: item.ComparativeScore, CreatedAt: item.CreatedAt, ReviewedAt: item.ReviewedAt, SafetyDisclaimer: SafetyDisclaimer}
	parts := []struct {
		name string
		raw  string
		to   any
	}{
		{"input snapshot", item.InputSnapshotJSON, &response.InputSnapshot},
		{"compartment loads", item.CompartmentLoadsJSON, &response.CompartmentLoads},
		{"risk flags", item.RiskFlagsJSON, &response.RiskFlags},
		{"assumptions", item.AssumptionsJSON, &response.Assumptions},
	}
	for _, part := range parts {
		if err := json.Unmarshal([]byte(part.raw), part.to); err != nil {
			return AssessmentResponse{}, fmt.Errorf("decode assessment %d %s: %w", item.ID, part.name, err)
		}
	}
	return response, nil
}
