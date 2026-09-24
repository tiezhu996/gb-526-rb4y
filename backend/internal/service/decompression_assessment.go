package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"commercial-diving-decompression-control/backend/internal/audit"
	"commercial-diving-decompression-control/backend/internal/constants"
	"commercial-diving-decompression-control/backend/internal/decompression"
	"commercial-diving-decompression-control/backend/internal/dto"
	"commercial-diving-decompression-control/backend/internal/model"
	"commercial-diving-decompression-control/backend/internal/repository"
	"commercial-diving-decompression-control/backend/internal/util"
)

type DecompressionAssessmentService struct {
	assessments  *repository.DecompressionAssessmentRepository
	plans        *repository.DivePlanRepository
	profiles     *repository.DiverProfileRepository
	segments     *repository.ExposureSegmentRepository
	modelVersion string
	maxSegments  int
}

func NewDecompressionAssessmentService(assessments *repository.DecompressionAssessmentRepository, plans *repository.DivePlanRepository, profiles *repository.DiverProfileRepository, segments *repository.ExposureSegmentRepository, modelVersion string, maxSegments int) *DecompressionAssessmentService {
	return &DecompressionAssessmentService{assessments: assessments, plans: plans, profiles: profiles, segments: segments, modelVersion: modelVersion, maxSegments: maxSegments}
}

func (s *DecompressionAssessmentService) List(ctx context.Context, planID uint, status string, page, size int) ([]dto.AssessmentResponse, int64, error) {
	items, total, err := s.assessments.List(ctx, planID, status, page, size)
	if err != nil {
		return nil, 0, err
	}
	responses := make([]dto.AssessmentResponse, 0, len(items))
	for _, item := range items {
		response, decodeErr := dto.DecodeAssessment(item)
		if decodeErr != nil {
			return nil, 0, decodeErr
		}
		responses = append(responses, response)
	}
	return responses, total, nil
}

func (s *DecompressionAssessmentService) Get(ctx context.Context, id uint) (dto.AssessmentResponse, error) {
	item, err := s.assessments.Get(ctx, id)
	if err != nil {
		return dto.AssessmentResponse{}, err
	}
	return dto.DecodeAssessment(item)
}

func (s *DecompressionAssessmentService) Run(ctx context.Context, planID uint, req dto.RunAssessmentRequest, actor audit.Entry) (dto.AssessmentResponse, error) {
	plan, err := s.plans.Get(ctx, planID)
	if err != nil {
		return dto.AssessmentResponse{}, err
	}
	if plan.Version != req.PlanVersion {
		return dto.AssessmentResponse{}, util.Conflict("PLAN_VERSION_CONFLICT", "dive plan was changed by another user", nil)
	}
	if plan.PlanStatus != constants.PlanDraft {
		return dto.AssessmentResponse{}, util.Conflict("PLAN_NOT_DRAFT", "only a draft plan can run a new immutable assessment", nil)
	}
	profile, err := s.profiles.Get(ctx, plan.DiverProfileID)
	if err != nil {
		return dto.AssessmentResponse{}, err
	}
	segments, err := s.segments.ListByPlan(ctx, planID)
	if err != nil {
		return dto.AssessmentResponse{}, err
	}
	result, modelErr := decompression.Run(plan, profile, segments, s.modelVersion, s.maxSegments)
	if modelErr != nil {
		actor.EntityType = "dive_plan"
		_ = s.plans.ResetDraftAfterFailure(ctx, plan, modelErr.Error(), actor)
		return dto.AssessmentResponse{}, util.Unprocessable("MODEL_INPUT_INVALID", modelErr.Error(), modelErr)
	}
	snapshotJSON, curvesJSON, flagsJSON, assumptionsJSON, err := decompression.MarshalResult(result)
	if err != nil {
		return dto.AssessmentResponse{}, util.Internal(err)
	}
	item := model.DecompressionAssessment{PlanID: planID, AssessmentStatus: string(constants.PlanModeled), AlgorithmVersion: s.modelVersion, InputSnapshotJSON: snapshotJSON, CompartmentLoadsJSON: curvesJSON, RiskFlagsJSON: flagsJSON, HighestRiskBand: decompression.HighestRiskBand(result.RiskFlags), ComparativeScore: result.ComparativeScore, AssumptionsJSON: assumptionsJSON}
	actor.Action = "decompression_assessment.run"
	actor.EntityType = "decompression_assessment"
	actor.BeforeSummary = fmt.Sprintf("plan=%d version=%d algorithm=%s segments=%d", planID, plan.Version, s.modelVersion, len(segments))
	actor.AfterSummary = fmt.Sprintf("score=%.2f compartments=%d flags=%d immutable=true", result.ComparativeScore, len(result.Curves), len(result.RiskFlags))
	if err := s.assessments.CreateModeled(ctx, plan, &item, actor); err != nil {
		return dto.AssessmentResponse{}, err
	}
	return dto.DecodeAssessment(item)
}

func (s *DecompressionAssessmentService) Submit(ctx context.Context, id uint, req dto.TransitionPlanRequest, actor audit.Entry) (dto.AssessmentResponse, error) {
	return s.transition(ctx, id, req, constants.PlanPendingReview, actor)
}

func (s *DecompressionAssessmentService) Approve(ctx context.Context, id uint, req dto.TransitionPlanRequest, actor audit.Entry) (dto.AssessmentResponse, error) {
	return s.transition(ctx, id, req, constants.PlanApprovedTraining, actor)
}

func (s *DecompressionAssessmentService) transition(ctx context.Context, id uint, req dto.TransitionPlanRequest, target constants.PlanStatus, actor audit.Entry) (dto.AssessmentResponse, error) {
	if req.TargetStatus != target {
		return dto.AssessmentResponse{}, util.Unprocessable("INVALID_PLAN_TRANSITION", fmt.Sprintf("endpoint requires target_status %s", target), nil)
	}
	assessment, err := s.assessments.Get(ctx, id)
	if err != nil {
		return dto.AssessmentResponse{}, err
	}
	plan, err := s.plans.Get(ctx, assessment.PlanID)
	if err != nil {
		return dto.AssessmentResponse{}, err
	}
	if plan.Version != req.Version {
		return dto.AssessmentResponse{}, util.Conflict("PLAN_VERSION_CONFLICT", "dive plan was changed by another user", nil)
	}
	if !constants.CanTransitionPlan(plan.PlanStatus, target) {
		return dto.AssessmentResponse{}, util.Unprocessable("INVALID_PLAN_TRANSITION", fmt.Sprintf("cannot transition from %s to %s", plan.PlanStatus, target), nil)
	}
	if assessment.AssessmentStatus != string(plan.PlanStatus) {
		return dto.AssessmentResponse{}, util.Conflict("ASSESSMENT_STATE_CONFLICT", "assessment and plan review states do not match", nil)
	}
	actor.Action = "decompression_assessment.submit_review"
	if target == constants.PlanApprovedTraining {
		actor.Action = "decompression_assessment.approve_training"
	}
	actor.EntityType = "decompression_assessment"
	actor.BeforeSummary = string(plan.PlanStatus)
	actor.AfterSummary = fmt.Sprintf("%s reason=%s human_review=true", target, strings.TrimSpace(req.Reason))
	if err := s.assessments.Transition(ctx, plan, assessment, target, actor.ActorID, actor); err != nil {
		return dto.AssessmentResponse{}, err
	}
	return s.Get(ctx, id)
}

func (s *DecompressionAssessmentService) Compare(ctx context.Context, leftID, rightID uint) (dto.AssessmentComparison, error) {
	left, err := s.Get(ctx, leftID)
	if err != nil {
		return dto.AssessmentComparison{}, err
	}
	right, err := s.Get(ctx, rightID)
	if err != nil {
		return dto.AssessmentComparison{}, err
	}
	return dto.AssessmentComparison{
		Left: left, Right: right, ScoreDelta: right.ComparativeScore - left.ComparativeScore,
		FlagDelta: len(right.RiskFlags) - len(left.RiskFlags),
		Summary: []string{
			fmt.Sprintf("Comparative index changed by %.2f points", right.ComparativeScore-left.ComparativeScore),
			fmt.Sprintf("Risk flag count changed from %d to %d", len(left.RiskFlags), len(right.RiskFlags)),
			"Differences describe deterministic training assumptions, not relative dive safety.",
		}, Disclaimer: dto.SafetyDisclaimer,
	}, nil
}

// CreateSensitivityCheck replays the immutable snapshot of one assessment with
// per-segment +/-10% depth/duration nudges and archives the result as an
// independent, append-only sensitivity check. Neither the assessment nor the
// plan is modified; out-of-range variants are recorded as non-computable.
func (s *DecompressionAssessmentService) CreateSensitivityCheck(ctx context.Context, assessmentID uint, actor audit.Entry) (dto.SensitivityCheckResponse, error) {
	assessment, err := s.assessments.Get(ctx, assessmentID)
	if err != nil {
		return dto.SensitivityCheckResponse{}, err
	}
	snapshot, err := decodeInputSnapshot(assessment.InputSnapshotJSON)
	if err != nil {
		return dto.SensitivityCheckResponse{}, util.Internal(err)
	}
	report, err := decompression.RunSensitivity(snapshot, s.maxSegments)
	if err != nil {
		return dto.SensitivityCheckResponse{}, util.Unprocessable("MODEL_INPUT_INVALID", err.Error(), err)
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return dto.SensitivityCheckResponse{}, util.Internal(fmt.Errorf("marshal sensitivity report: %w", err))
	}
	item := model.SensitivityCheck{
		AssessmentID: assessment.ID, PlanID: assessment.PlanID,
		AlgorithmVersion: assessment.AlgorithmVersion, AdjustmentRatio: report.AdjustmentRatio,
		BaselineScore: report.BaselineScore, BaselineRiskBand: report.BaselineRiskBand,
		TotalVariants: report.TotalVariants, ComputableVariants: report.ComputableVariants,
		OutOfRangeVariants:     report.OutOfRangeVariants,
		MostAffectedSequenceNo: report.MostAffectedSequenceNo,
		MostAffectedAxis:       string(report.MostAffectedAxis),
		MostAffectedDirection:  string(report.MostAffectedDirection),
		ReportJSON:             string(reportJSON),
		CreatedBy:              actor.ActorID, CreatedByUsername: actor.ActorUsername,
		CreatedAt: time.Now().UTC(),
	}
	actor.Action = "sensitivity_check.run"
	actor.EntityType = "sensitivity_check"
	actor.BeforeSummary = fmt.Sprintf("assessment=%d immutable_snapshot=true baseline_score=%.2f baseline_band=%s", assessment.ID, report.BaselineScore, report.BaselineRiskBand)
	mostAffected := "none_index_stable"
	if report.MostAffectedSequenceNo > 0 {
		mostAffected = fmt.Sprintf("segment_%d_%s_%s", report.MostAffectedSequenceNo, report.MostAffectedAxis, report.MostAffectedDirection)
	}
	actor.AfterSummary = fmt.Sprintf("variants=%d computable=%d out_of_range=%d most_affected=%s assessment_unchanged=true snapshot_preserved=true", report.TotalVariants, report.ComputableVariants, report.OutOfRangeVariants, mostAffected)
	if err := s.assessments.CreateSensitivityCheck(ctx, &item, actor); err != nil {
		return dto.SensitivityCheckResponse{}, err
	}
	return dto.DecodeSensitivityCheck(item)
}

func (s *DecompressionAssessmentService) ListSensitivityChecks(ctx context.Context, assessmentID uint, page, size int) ([]dto.SensitivityCheckResponse, int64, error) {
	if _, err := s.assessments.Get(ctx, assessmentID); err != nil {
		return nil, 0, err
	}
	items, total, err := s.assessments.ListSensitivityChecks(ctx, assessmentID, page, size)
	if err != nil {
		return nil, 0, err
	}
	responses := make([]dto.SensitivityCheckResponse, 0, len(items))
	for _, item := range items {
		response, decodeErr := dto.DecodeSensitivityCheck(item)
		if decodeErr != nil {
			return nil, 0, decodeErr
		}
		responses = append(responses, response)
	}
	return responses, total, nil
}

func (s *DecompressionAssessmentService) GetSensitivityCheck(ctx context.Context, id uint) (dto.SensitivityCheckResponse, error) {
	item, err := s.assessments.GetSensitivityCheck(ctx, id)
	if err != nil {
		return dto.SensitivityCheckResponse{}, err
	}
	return dto.DecodeSensitivityCheck(item)
}

func decodeInputSnapshot(raw string) (decompression.InputSnapshot, error) {
	var snapshot decompression.InputSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return decompression.InputSnapshot{}, fmt.Errorf("decode assessment input snapshot for sensitivity replay: %w", err)
	}
	return snapshot, nil
}
