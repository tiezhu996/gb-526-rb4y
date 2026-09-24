package model

import (
	"time"

	"commercial-diving-decompression-control/backend/internal/constants"
)

type DecompressionAssessment struct {
	ID                   uint               `gorm:"primaryKey" json:"id"`
	PlanID               uint               `gorm:"not null;index" json:"plan_id"`
	AssessmentStatus     string             `gorm:"size:40;not null;index;check:assessment_status IN ('modeled','pending_supervisor_review','approved_for_training','archived')" json:"assessment_status"`
	AlgorithmVersion     string             `gorm:"size:48;not null;index" json:"algorithm_version"`
	InputSnapshotJSON    string             `gorm:"type:text;not null" json:"input_snapshot_json"`
	CompartmentLoadsJSON string             `gorm:"type:text;not null" json:"compartment_loads_json"`
	RiskFlagsJSON        string             `gorm:"type:text;not null" json:"risk_flags_json"`
	HighestRiskBand      constants.RiskBand `gorm:"size:20;not null;default:'informational';index;check:highest_risk_band IN ('informational','caution','elevated','invalid')" json:"highest_risk_band"`
	ComparativeScore     float64            `gorm:"not null" json:"comparative_score"`
	AssumptionsJSON      string             `gorm:"type:text;not null" json:"assumptions_json"`
	CreatedAt            time.Time          `gorm:"not null;index" json:"created_at"`
	ReviewedAt           *time.Time         `json:"reviewed_at"`
}

func (DecompressionAssessment) TableName() string { return "decompression_assessments" }

type SensitivityCheck struct {
	ID                      uint               `gorm:"primaryKey" json:"id"`
	AssessmentID            uint               `gorm:"not null;uniqueIndex:idx_sensitivity_run" json:"assessment_id"`
	RunSerial               int                `gorm:"not null;uniqueIndex:idx_sensitivity_run" json:"run_serial"`
	AlgorithmVersion        string             `gorm:"size:48;not null;index" json:"algorithm_version"`
	PerturbationPercent     float64            `gorm:"not null" json:"perturbation_percent"`
	BaselineScore           float64            `gorm:"not null" json:"baseline_score"`
	BaselineRiskBand        constants.RiskBand `gorm:"size:20;not null;check:baseline_risk_band IN ('informational','caution','elevated','invalid')" json:"baseline_risk_band"`
	MostInfluentialSequence int                `gorm:"not null;default:0" json:"most_influential_sequence"`
	MostInfluentialReason   string             `gorm:"size:240;not null;default:''" json:"most_influential_reason"`
	OutRangeCount           int                `gorm:"not null;default:0" json:"out_range_count"`
	BandChangeCount         int                `gorm:"not null;default:0" json:"band_change_count"`
	VariantsJSON            string             `gorm:"type:text;not null" json:"variants_json"`
	CreatedBy               uint               `gorm:"not null;index" json:"created_by"`
	CreatedAt               time.Time          `gorm:"not null;index" json:"created_at"`
}

func (SensitivityCheck) TableName() string { return "sensitivity_checks" }
