package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Forecast datasets and rows are snapshots. A sealed dataset is the only
// dataset a run may use; rows belonging to a sealed dataset are immutable.
type ForecastDatasetStatus string

const (
	ForecastDatasetDraft  ForecastDatasetStatus = "draft"
	ForecastDatasetSealed ForecastDatasetStatus = "sealed"
)

func ValidForecastDatasetStatus(v ForecastDatasetStatus) bool {
	return v == ForecastDatasetDraft || v == ForecastDatasetSealed
}

type ForecastInputKind string

const (
	ForecastInputDemand           ForecastInputKind = "demand"
	ForecastInputGuestPlan        ForecastInputKind = "guest_plan"
	ForecastInputConfirmedInbound ForecastInputKind = "confirmed_inbound"
)

func ValidForecastInputKind(v ForecastInputKind) bool {
	return v == ForecastInputDemand || v == ForecastInputGuestPlan || v == ForecastInputConfirmedInbound
}

type ForecastDataset struct {
	ID              primitive.ObjectID    `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID        primitive.ObjectID    `json:"-" bson:"tenantId" validate:"required"`
	LocationID      primitive.ObjectID    `json:"locationId" bson:"locationId" validate:"required"`
	Name            string                `json:"name" bson:"name" validate:"required,not_blank,max=160"`
	Source          string                `json:"source" bson:"source" validate:"required,location_code,max=64"`
	Status          ForecastDatasetStatus `json:"status" bson:"status" validate:"required,forecast_dataset_status"`
	EffectiveFrom   time.Time             `json:"effectiveFrom" bson:"effectiveFrom" validate:"required"`
	EffectiveTo     time.Time             `json:"effectiveTo" bson:"effectiveTo" validate:"required"`
	CutoffAt        *time.Time            `json:"cutoffAt,omitempty" bson:"cutoffAt,omitempty"`
	ManifestVersion string                `json:"manifestVersion,omitempty" bson:"manifestVersion,omitempty" validate:"omitempty,location_code,max=64"`
	RowCount        int64                 `json:"rowCount" bson:"rowCount" validate:"gte=0,lte=10000000"`
	ContentHash     string                `json:"contentHash,omitempty" bson:"contentHash,omitempty" validate:"omitempty,len=64,sha256_hex"`
	SealedBy        *primitive.ObjectID   `json:"sealedBy,omitempty" bson:"sealedBy,omitempty"`
	SealedAt        *time.Time            `json:"sealedAt,omitempty" bson:"sealedAt,omitempty"`
	CreatedBy       primitive.ObjectID    `json:"createdBy" bson:"createdBy" validate:"required"`
	Version         int64                 `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt       time.Time             `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt       time.Time             `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

// ForecastInputRow is normalized without losing both effective business time
// and the time at which the fact became known to the system.
type ForecastInputRow struct {
	ID             primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID       primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	LocationID     primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	DatasetID      primitive.ObjectID  `json:"datasetId" bson:"datasetId" validate:"required"`
	RowNumber      int64               `json:"rowNumber" bson:"rowNumber" validate:"gte=1,lte=10000000"`
	Kind           ForecastInputKind   `json:"kind" bson:"kind" validate:"required,forecast_input_kind"`
	ItemID         *primitive.ObjectID `json:"itemId,omitempty" bson:"itemId,omitempty"`
	ServicePeriod  string              `json:"servicePeriod,omitempty" bson:"servicePeriod,omitempty" validate:"omitempty,location_code,max=64"`
	SourceIDs      []string            `json:"sourceIds,omitempty" bson:"sourceIds,omitempty" validate:"omitempty,max=1000,dive,not_blank,max=128"`
	QuantityMicros int64               `json:"quantityMicros,string" bson:"quantityMicros" validate:"gte=0,quantity_micros"`
	GuestCount     int64               `json:"guestCount,omitempty" bson:"guestCount,omitempty" validate:"omitempty,gte=0,lte=1000000000"`
	EffectiveAt    time.Time           `json:"effectiveAt" bson:"effectiveAt" validate:"required"`
	RecordedAt     time.Time           `json:"recordedAt" bson:"recordedAt" validate:"required"`
	CreatedAt      time.Time           `json:"createdAt" bson:"createdAt" validate:"required"`
}

type ForecastJobStatus string

const (
	ForecastJobQueued     ForecastJobStatus = "queued"
	ForecastJobRunning    ForecastJobStatus = "running"
	ForecastJobLeased     ForecastJobStatus = ForecastJobRunning
	ForecastJobRetryWait  ForecastJobStatus = "retry_wait"
	ForecastJobSucceeded  ForecastJobStatus = "succeeded"
	ForecastJobFailed     ForecastJobStatus = "failed"
	ForecastJobDeadLetter ForecastJobStatus = "dead_letter"
	ForecastJobCancelled  ForecastJobStatus = "cancelled"
)

func ValidForecastJobStatus(v ForecastJobStatus) bool {
	switch v {
	case ForecastJobQueued, ForecastJobRunning, ForecastJobRetryWait, ForecastJobSucceeded, ForecastJobFailed, ForecastJobDeadLetter, ForecastJobCancelled:
		return true
	default:
		return false
	}
}

type ForecastJob struct {
	ID             primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID       primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	LocationID     primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	DatasetID      primitive.ObjectID  `json:"datasetId" bson:"datasetId" validate:"required"`
	PolicyID       primitive.ObjectID  `json:"policyId,omitempty" bson:"policyId,omitempty" validate:"omitempty"`
	RunID          *primitive.ObjectID `json:"runId,omitempty" bson:"runId,omitempty"`
	Status         ForecastJobStatus   `json:"status" bson:"status" validate:"required,forecast_job_status"`
	IdempotencyKey string              `json:"idempotencyKey" bson:"idempotencyKey" validate:"required,min=8,max=128"`
	Owner          string              `json:"owner,omitempty" bson:"owner,omitempty" validate:"omitempty,min=1,max=128"`
	LeaseToken     string              `json:"-" bson:"leaseToken,omitempty" validate:"omitempty,min=16,max=256"`
	LeaseExpiresAt *time.Time          `json:"leaseExpiresAt,omitempty" bson:"leaseExpiresAt,omitempty"`
	HeartbeatAt    *time.Time          `json:"heartbeatAt,omitempty" bson:"heartbeatAt,omitempty"`
	NextAttemptAt  *time.Time          `json:"nextAttemptAt,omitempty" bson:"nextAttemptAt,omitempty"`
	Attempt        int32               `json:"attempt" bson:"attempt" validate:"gte=0,lte=100"`
	MaxAttempts    int32               `json:"maxAttempts" bson:"maxAttempts" validate:"gte=1,lte=100"`
	LastError      string              `json:"lastError,omitempty" bson:"lastError,omitempty" validate:"omitempty,max=2000"`
	CreatedAt      time.Time           `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt      time.Time           `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type ForecastMetric struct {
	ID         primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID   primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	LocationID primitive.ObjectID `json:"locationId" bson:"locationId" validate:"required"`
	RunID      primitive.ObjectID `json:"runId" bson:"runId" validate:"required"`
	Name       string             `json:"name" bson:"name" validate:"required,location_code,max=96"`
	Value      int64              `json:"value,string" bson:"value"`
	CreatedAt  time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
}

type ForecastRunStatus string

const (
	ForecastRunPending   ForecastRunStatus = "pending"
	ForecastRunRunning   ForecastRunStatus = "running"
	ForecastRunSucceeded ForecastRunStatus = "succeeded"
	ForecastRunFailed    ForecastRunStatus = "failed"
)

func ValidForecastRunStatus(v ForecastRunStatus) bool {
	return v == ForecastRunPending || v == ForecastRunRunning || v == ForecastRunSucceeded || v == ForecastRunFailed
}

type ForecastRun struct {
	ID               primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID         primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	LocationID       primitive.ObjectID `json:"locationId" bson:"locationId" validate:"required"`
	DatasetID        primitive.ObjectID `json:"datasetId" bson:"datasetId" validate:"required"`
	PolicyID         primitive.ObjectID `json:"policyId" bson:"policyId" validate:"required"`
	Status           ForecastRunStatus  `json:"status" bson:"status" validate:"required,forecast_run_status"`
	Algorithm        string             `json:"algorithm" bson:"algorithm" validate:"required,location_code,max=64"`
	AlgorithmVersion string             `json:"algorithmVersion" bson:"algorithmVersion" validate:"required,location_code,max=64"`
	// Parameters and Metrics are immutable run metadata copied from the sealed
	// v6 core result; they are not live configuration or mutable sale state.
	Parameters  map[string]string `json:"parameters,omitempty" bson:"parameters,omitempty"`
	Metrics     map[string]int64  `json:"metrics,omitempty" bson:"metrics,omitempty"`
	StartedAt   *time.Time        `json:"startedAt,omitempty" bson:"startedAt,omitempty"`
	CompletedAt *time.Time        `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
	Error       string            `json:"error,omitempty" bson:"error,omitempty" validate:"omitempty,max=2000"`
	CreatedAt   time.Time         `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt   time.Time         `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type ForecastPoint struct {
	ID             primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID       primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	LocationID     primitive.ObjectID `json:"locationId" bson:"locationId" validate:"required"`
	RunID          primitive.ObjectID `json:"runId" bson:"runId" validate:"required"`
	ItemID         primitive.ObjectID `json:"itemId" bson:"itemId" validate:"required"`
	ModelID        string             `json:"modelId,omitempty" bson:"modelId,omitempty" validate:"omitempty,location_code,max=64"`
	TargetDate     time.Time          `json:"targetDate" bson:"targetDate" validate:"required"`
	ForecastMicros int64              `json:"forecastMicros,string" bson:"forecastMicros" validate:"gte=0,quantity_micros"`
	LowerMicros    int64              `json:"lowerMicros,string" bson:"lowerMicros" validate:"gte=0,quantity_micros"`
	UpperMicros    int64              `json:"upperMicros,string" bson:"upperMicros" validate:"gte=0,quantity_micros"`
	CreatedAt      time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
}

type ForecastPolicy struct {
	ID              primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID        primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	LocationID      primitive.ObjectID `json:"locationId" bson:"locationId" validate:"required"`
	Name            string             `json:"name" bson:"name" validate:"required,not_blank,max=160"`
	HorizonDays     int32              `json:"horizonDays" bson:"horizonDays" validate:"gte=1,lte=365"`
	LookbackDays    int32              `json:"lookbackDays" bson:"lookbackDays" validate:"gte=1,lte=3650"`
	SafetyStockDays int32              `json:"safetyStockDays" bson:"safetyStockDays" validate:"gte=0,lte=365"`
	IsActive        bool               `json:"isActive" bson:"isActive"`
	Version         int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedBy       primitive.ObjectID `json:"createdBy" bson:"createdBy" validate:"required"`
	CreatedAt       time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt       time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

// GuestPlan is an explicit manual planning assumption, never a sale actual.
type GuestPlan struct {
	ID            primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID      primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	LocationID    primitive.ObjectID `json:"locationId" bson:"locationId" validate:"required"`
	PlanDate      time.Time          `json:"planDate" bson:"planDate" validate:"required"`
	ServicePeriod string             `json:"servicePeriod" bson:"servicePeriod" validate:"required,location_code,max=64"`
	GuestCount    int64              `json:"guestCount" bson:"guestCount" validate:"gte=0,lte=1000000000"`
	Source        string             `json:"source" bson:"source" validate:"required,location_code,max=32"`
	IsActual      bool               `json:"isActual" bson:"isActual"`
	Notes         string             `json:"notes,omitempty" bson:"notes,omitempty" validate:"omitempty,max=1000"`
	CreatedBy     primitive.ObjectID `json:"createdBy" bson:"createdBy" validate:"required"`
	Version       int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt     time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt     time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type ForecastOverride struct {
	ID             primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID       primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	LocationID     primitive.ObjectID `json:"locationId" bson:"locationId" validate:"required"`
	ItemID         primitive.ObjectID `json:"itemId" bson:"itemId" validate:"required"`
	TargetDate     time.Time          `json:"targetDate" bson:"targetDate" validate:"required"`
	QuantityMicros int64              `json:"quantityMicros,string" bson:"quantityMicros" validate:"gte=0,quantity_micros"`
	Reason         string             `json:"reason" bson:"reason" validate:"required,not_blank,max=500"`
	CreatedBy      primitive.ObjectID `json:"createdBy" bson:"createdBy" validate:"required"`
	CreatedAt      time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
}

type ReorderRecommendationStatus string

const (
	ReorderRecommendationReady  ReorderRecommendationStatus = "ready"
	ReorderRecommendationNoNeed ReorderRecommendationStatus = "no-reorder-needed"
)

func ValidReorderRecommendationStatus(v ReorderRecommendationStatus) bool {
	return v == ReorderRecommendationReady || v == ReorderRecommendationNoNeed
}

// ReorderRecommendation is an immutable calculation snapshot. It contains no
// supplier or purchase-order reference: a later purchasing workflow may use
// this record as advice, but this lane never creates or changes an order.
type ReorderRecommendation struct {
	ID                      primitive.ObjectID          `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID                primitive.ObjectID          `json:"-" bson:"tenantId" validate:"required"`
	LocationID              primitive.ObjectID          `json:"locationId" bson:"locationId" validate:"required"`
	RunID                   primitive.ObjectID          `json:"runId" bson:"runId" validate:"required"`
	DatasetID               primitive.ObjectID          `json:"datasetId" bson:"datasetId" validate:"required"`
	PolicyID                primitive.ObjectID          `json:"policyId" bson:"policyId" validate:"required"`
	ItemID                  primitive.ObjectID          `json:"itemId" bson:"itemId" validate:"required"`
	ModelID                 string                      `json:"modelId" bson:"modelId" validate:"required,location_code,max=64"`
	AlgorithmVersion        string                      `json:"algorithmVersion" bson:"algorithmVersion" validate:"required,location_code,max=64"`
	HorizonDays             int32                       `json:"horizonDays" bson:"horizonDays" validate:"gte=1,lte=365"`
	P90DemandMicros         int64                       `json:"p90DemandMicros,string" bson:"p90DemandMicros" validate:"gte=0,quantity_micros"`
	SafetyDemandMicros      int64                       `json:"safetyDemandMicros,string" bson:"safetyDemandMicros" validate:"gte=0,quantity_micros"`
	UsableStockMicros       int64                       `json:"usableStockMicros,string" bson:"usableStockMicros" validate:"gte=0,quantity_micros"`
	ConfirmedInboundMicros  int64                       `json:"confirmedInboundMicros,string" bson:"confirmedInboundMicros" validate:"gte=0,quantity_micros"`
	RequestedQuantityMicros int64                       `json:"requestedQuantityMicros,string" bson:"requestedQuantityMicros" validate:"gte=0,quantity_micros"`
	RoundedQuantityMicros   int64                       `json:"roundedQuantityMicros,string" bson:"roundedQuantityMicros" validate:"gte=0,quantity_micros"`
	QuantityMicros          int64                       `json:"quantityMicros,string" bson:"quantityMicros" validate:"gte=0,quantity_micros"`
	PackSizeMicros          int64                       `json:"packSizeMicros,string" bson:"packSizeMicros" validate:"gte=0,quantity_micros"`
	MOQ                     int64                       `json:"moq" bson:"moq" validate:"gte=0,lte=1000000"`
	PackMOQDeltaMicros      int64                       `json:"packMoqDeltaMicros,string" bson:"packMoqDeltaMicros" validate:"gte=0,quantity_micros"`
	Status                  ReorderRecommendationStatus `json:"status" bson:"status" validate:"required,reorder_recommendation_status"`
	QualityFlags            []string                    `json:"qualityFlags,omitempty" bson:"qualityFlags,omitempty" validate:"omitempty,max=32,dive,not_blank,max=64"`
	ObservedDays            int32                       `json:"observedDays" bson:"observedDays" validate:"gte=0,lte=100000"`
	ExpectedDays            int32                       `json:"expectedDays" bson:"expectedDays" validate:"gte=0,lte=100000"`
	CoveragePermille        int64                       `json:"coveragePermille" bson:"coveragePermille" validate:"gte=0,lte=1000"`
	Maturity                string                      `json:"maturity" bson:"maturity" validate:"required,not_blank,max=32"`
	Formula                 string                      `json:"formula" bson:"formula" validate:"required,not_blank,max=1000"`
	ReasonCodes             []string                    `json:"reasonCodes" bson:"reasonCodes" validate:"required,min=1,max=32,dive,not_blank,max=64"`
	InboundSourceIDs        []string                    `json:"inboundSourceIds,omitempty" bson:"inboundSourceIds,omitempty" validate:"omitempty,max=1000,dive,not_blank,max=128"`
	ExpiresAt               *time.Time                  `json:"expiresAt,omitempty" bson:"expiresAt,omitempty"`
	CreatedAt               time.Time                   `json:"createdAt" bson:"createdAt" validate:"required"`
}

// ReorderCoverage is the immutable coverage ledger for one run/item. Keeping
// it separate from the recommendation makes zero-quantity and immature items
// auditable without inventing a purchase action.
type ReorderCoverage struct {
	ID                       primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID                 primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	LocationID               primitive.ObjectID `json:"locationId" bson:"locationId" validate:"required"`
	RunID                    primitive.ObjectID `json:"runId" bson:"runId" validate:"required"`
	DatasetID                primitive.ObjectID `json:"datasetId" bson:"datasetId" validate:"required"`
	PolicyID                 primitive.ObjectID `json:"policyId" bson:"policyId" validate:"required"`
	ItemID                   primitive.ObjectID `json:"itemId" bson:"itemId" validate:"required"`
	ModelID                  string             `json:"modelId" bson:"modelId" validate:"required,location_code,max=64"`
	AlgorithmVersion         string             `json:"algorithmVersion" bson:"algorithmVersion" validate:"required,location_code,max=64"`
	HorizonDays              int32              `json:"horizonDays" bson:"horizonDays" validate:"gte=1,lte=365"`
	P90DemandMicros          int64              `json:"p90DemandMicros,string" bson:"p90DemandMicros" validate:"gte=0,quantity_micros"`
	SafetyDemandMicros       int64              `json:"safetyDemandMicros,string" bson:"safetyDemandMicros" validate:"gte=0,quantity_micros"`
	UsableStockMicros        int64              `json:"usableStockMicros,string" bson:"usableStockMicros" validate:"gte=0,quantity_micros"`
	ConfirmedInboundMicros   int64              `json:"confirmedInboundMicros,string" bson:"confirmedInboundMicros" validate:"gte=0,quantity_micros"`
	ProjectedAvailableMicros int64              `json:"projectedAvailableMicros,string" bson:"projectedAvailableMicros" validate:"gte=0,quantity_micros"`
	RequestedQuantityMicros  int64              `json:"requestedQuantityMicros,string" bson:"requestedQuantityMicros" validate:"gte=0,quantity_micros"`
	RoundedQuantityMicros    int64              `json:"roundedQuantityMicros,string" bson:"roundedQuantityMicros" validate:"gte=0,quantity_micros"`
	CoverageDays             int64              `json:"coverageDays" bson:"coverageDays" validate:"gte=0,lte=100000"`
	QualityFlags             []string           `json:"qualityFlags,omitempty" bson:"qualityFlags,omitempty" validate:"omitempty,max=32,dive,not_blank,max=64"`
	ObservedDays             int32              `json:"observedDays" bson:"observedDays" validate:"gte=0,lte=100000"`
	ExpectedDays             int32              `json:"expectedDays" bson:"expectedDays" validate:"gte=0,lte=100000"`
	CoveragePermille         int64              `json:"coveragePermille" bson:"coveragePermille" validate:"gte=0,lte=1000"`
	Maturity                 string             `json:"maturity" bson:"maturity" validate:"required,not_blank,max=32"`
	Formula                  string             `json:"formula" bson:"formula" validate:"required,not_blank,max=1000"`
	ReasonCodes              []string           `json:"reasonCodes" bson:"reasonCodes" validate:"required,min=1,max=32,dive,not_blank,max=64"`
	InboundSourceIDs         []string           `json:"inboundSourceIds,omitempty" bson:"inboundSourceIds,omitempty" validate:"omitempty,max=1000,dive,not_blank,max=128"`
	CreatedAt                time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
}
