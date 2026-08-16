package product

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"lastsaas/internal/apierror"
	forecastcore "lastsaas/internal/forecast"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type guestPlanRequest struct {
	PlanDate      time.Time `json:"planDate" validate:"required"`
	ServicePeriod string    `json:"servicePeriod" validate:"required,location_code,max=64"`
	GuestCount    int64     `json:"guestCount" validate:"gte=0,lte=1000000000"`
	Notes         string    `json:"notes,omitempty" validate:"omitempty,max=1000"`
}

type forecastPolicyRequest struct {
	Name            string `json:"name" validate:"required,not_blank,max=160"`
	HorizonDays     int32  `json:"horizonDays" validate:"gte=1,lte=365"`
	LookbackDays    int32  `json:"lookbackDays" validate:"gte=1,lte=3650"`
	SafetyStockDays int32  `json:"safetyStockDays" validate:"gte=0,lte=365"`
	IsActive        bool   `json:"isActive"`
}

type forecastPolicyUpdateRequest struct {
	Version         int64   `json:"version" validate:"gte=1"`
	Name            *string `json:"name,omitempty" validate:"omitempty,not_blank,max=160"`
	HorizonDays     *int32  `json:"horizonDays,omitempty" validate:"omitempty,gte=1,lte=365"`
	LookbackDays    *int32  `json:"lookbackDays,omitempty" validate:"omitempty,gte=1,lte=3650"`
	SafetyStockDays *int32  `json:"safetyStockDays,omitempty" validate:"omitempty,gte=0,lte=365"`
	IsActive        *bool   `json:"isActive,omitempty"`
}

type forecastRunRequest struct {
	PolicyID       primitive.ObjectID `json:"policyId" validate:"required"`
	IdempotencyKey string             `json:"idempotencyKey" validate:"required,min=8,max=128"`
	CutoffAt       *time.Time         `json:"cutoffAt,omitempty"`
}

func requireForecastManager() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile, ok := GetStaffProfileFromContext(r.Context())
			if !ok || (profile.BusinessRole != models.BusinessRoleCompanyOwner && profile.BusinessRole != models.BusinessRoleOperationsManager) {
				apierror.Forbidden(w, r, "Company owner or operations manager management required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *productHandler) listGuestPlans(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Tenant context required")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	cur, err := h.db.GuestPlans().Find(r.Context(), bson.M{"tenantId": tenant.ID, "locationId": locationID}, options.Find().SetSort(bson.D{{Key: "planDate", Value: 1}, {Key: "servicePeriod", Value: 1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list guest plans")
		return
	}
	defer cur.Close(r.Context())
	plans := []models.GuestPlan{}
	if err := cur.All(r.Context(), &plans); err != nil {
		apierror.Internal(w, r, "Failed to list guest plans")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"guestPlans": plans})
}

func (h *productHandler) createGuestPlan(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	var request guestPlanRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	now := time.Now().UTC()
	plan := models.GuestPlan{ID: primitive.NewObjectID(), TenantID: tenant.ID, LocationID: locationID, PlanDate: request.PlanDate.UTC(), ServicePeriod: strings.TrimSpace(request.ServicePeriod), GuestCount: request.GuestCount, Source: "manual", IsActual: false, Notes: strings.TrimSpace(request.Notes), CreatedBy: user.ID, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&plan); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	if _, err := h.db.GuestPlans().InsertOne(r.Context(), plan); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			apierror.Conflict(w, r, "Guest plan already exists for this date and service period")
		} else {
			apierror.Internal(w, r, "Failed to create guest plan")
		}
		return
	}
	h.auditForecast(r, "forecast.guest_plan.created", plan.LocationID, plan.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"guestPlan": plan})
}

func (h *productHandler) listForecastPolicies(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Tenant context required")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	cur, err := h.db.ForecastPolicies().Find(r.Context(), bson.M{"tenantId": tenant.ID, "locationId": locationID}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list forecast policies")
		return
	}
	defer cur.Close(r.Context())
	policies := []models.ForecastPolicy{}
	if err := cur.All(r.Context(), &policies); err != nil {
		apierror.Internal(w, r, "Failed to list forecast policies")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"forecastPolicies": policies})
}

func (h *productHandler) createForecastPolicy(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	var request forecastPolicyRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	now := time.Now().UTC()
	policy := models.ForecastPolicy{ID: primitive.NewObjectID(), TenantID: tenant.ID, LocationID: locationID, Name: strings.TrimSpace(request.Name), HorizonDays: request.HorizonDays, LookbackDays: request.LookbackDays, SafetyStockDays: request.SafetyStockDays, IsActive: request.IsActive, Version: 1, CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&policy); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	if _, err := h.db.ForecastPolicies().InsertOne(r.Context(), policy); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			apierror.Conflict(w, r, "Forecast policy name already exists for this location")
		} else {
			apierror.Internal(w, r, "Failed to create forecast policy")
		}
		return
	}
	h.auditForecast(r, "forecast.policy.created", policy.LocationID, policy.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"forecastPolicy": policy})
}

func (h *productHandler) updateForecastPolicy(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	policyID, err := primitive.ObjectIDFromHex(mux.Vars(r)["policyId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid forecast policy ID")
		return
	}
	var request forecastPolicyUpdateRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	var current models.ForecastPolicy
	if err := h.db.ForecastPolicies().FindOne(r.Context(), bson.M{"_id": policyID, "tenantId": tenant.ID, "locationId": locationID}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Forecast policy not found")
		} else {
			apierror.Internal(w, r, "Failed to load forecast policy")
		}
		return
	}
	set := bson.M{"updatedAt": time.Now().UTC()}
	if request.Name != nil {
		set["name"] = strings.TrimSpace(*request.Name)
	}
	if request.HorizonDays != nil {
		set["horizonDays"] = *request.HorizonDays
	}
	if request.LookbackDays != nil {
		set["lookbackDays"] = *request.LookbackDays
	}
	if request.SafetyStockDays != nil {
		set["safetyStockDays"] = *request.SafetyStockDays
	}
	if request.IsActive != nil {
		set["isActive"] = *request.IsActive
	}
	candidate := current
	candidate.Version++
	candidate.UpdatedAt = set["updatedAt"].(time.Time)
	if request.Name != nil {
		candidate.Name = strings.TrimSpace(*request.Name)
	}
	if request.HorizonDays != nil {
		candidate.HorizonDays = *request.HorizonDays
	}
	if request.LookbackDays != nil {
		candidate.LookbackDays = *request.LookbackDays
	}
	if request.SafetyStockDays != nil {
		candidate.SafetyStockDays = *request.SafetyStockDays
	}
	if request.IsActive != nil {
		candidate.IsActive = *request.IsActive
	}
	if err := validation.Validate(&candidate); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	var updated models.ForecastPolicy
	if err := h.db.ForecastPolicies().FindOneAndUpdate(r.Context(), bson.M{"_id": policyID, "tenantId": tenant.ID, "locationId": locationID, "version": request.Version}, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updated); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.VersionConflict(w, r, "Forecast policy was modified by another request")
		} else if mongo.IsDuplicateKeyError(err) {
			apierror.Conflict(w, r, "Forecast policy name already exists for this location")
		} else {
			apierror.Internal(w, r, "Failed to update forecast policy")
		}
		return
	}
	h.auditForecast(r, "forecast.policy.updated", updated.LocationID, updated.ID)
	_ = user
	writeJSON(w, http.StatusOK, map[string]any{"forecastPolicy": updated})
}

func (h *productHandler) listForecastRuns(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Tenant context required")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	cur, err := h.db.ForecastRuns().Find(r.Context(), bson.M{"tenantId": tenant.ID, "locationId": locationID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list forecast runs")
		return
	}
	defer cur.Close(r.Context())
	runs := []models.ForecastRun{}
	if err := cur.All(r.Context(), &runs); err != nil {
		apierror.Internal(w, r, "Failed to list forecast runs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"forecastRuns": runs})
}

func (h *productHandler) listForecastDatasets(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Tenant context required")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	cur, err := h.db.ForecastDatasets().Find(r.Context(), bson.M{"tenantId": tenant.ID, "locationId": locationID}, options.Find().SetSort(bson.D{{Key: "effectiveFrom", Value: -1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list forecast datasets")
		return
	}
	defer cur.Close(r.Context())
	datasets := []models.ForecastDataset{}
	if err := cur.All(r.Context(), &datasets); err != nil {
		apierror.Internal(w, r, "Failed to list forecast datasets")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"forecastDatasets": datasets})
}

func (h *productHandler) listForecastPoints(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Tenant context required")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	runID, err := primitive.ObjectIDFromHex(mux.Vars(r)["runId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid forecast run ID")
		return
	}
	cur, err := h.db.ForecastPoints().Find(r.Context(), bson.M{"tenantId": tenant.ID, "locationId": locationID, "runId": runID}, options.Find().SetSort(bson.D{{Key: "targetDate", Value: 1}, {Key: "itemId", Value: 1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list forecast points")
		return
	}
	defer cur.Close(r.Context())
	points := []models.ForecastPoint{}
	if err := cur.All(r.Context(), &points); err != nil {
		apierror.Internal(w, r, "Failed to list forecast points")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"forecastPoints": points})
}

func (h *productHandler) createForecastRun(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	var request forecastRunRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	var policy models.ForecastPolicy
	if err := h.db.ForecastPolicies().FindOne(r.Context(), bson.M{"_id": request.PolicyID, "tenantId": tenant.ID, "locationId": locationID, "isActive": true}).Decode(&policy); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Forecast policy not found")
		} else {
			apierror.Internal(w, r, "Failed to load forecast policy")
		}
		return
	}
	cutoff := time.Now().UTC()
	if request.CutoffAt != nil {
		cutoff = request.CutoffAt.UTC()
	}
	if cutoff.After(time.Now().UTC()) {
		apierror.BadRequest(w, r, "Cutoff cannot be in the future")
		return
	}
	store := forecastcore.NewMongoStore(h.db)
	snapshot, err := store.MaterializeAndSeal(r.Context(), forecastcore.Scope{TenantID: tenant.ID, LocationID: locationID, ActorID: user.ID}, forecastcore.MaterializeRequest{CutoffAt: cutoff, EffectiveFrom: cutoff.AddDate(0, 0, -int(policy.LookbackDays)), EffectiveTo: cutoff.AddDate(0, 0, int(policy.HorizonDays)+1)}, "journal-"+request.IdempotencyKey, "journal")
	if err != nil {
		apierror.Internal(w, r, "Failed to seal forecast inputs")
		return
	}
	job, err := forecastcore.NewJobStore(h.db, forecastcore.DefaultLeaseConfig()).Enqueue(r.Context(), forecastcore.Scope{TenantID: tenant.ID, LocationID: locationID, ActorID: user.ID}, snapshot.Dataset.ID, policy.ID, request.IdempotencyKey, time.Now().UTC())
	if err != nil {
		if mongo.IsDuplicateKeyError(err) || errors.Is(err, forecastcore.ErrEnqueueConflict) {
			apierror.Conflict(w, r, "Forecast run already queued")
		} else {
			apierror.Internal(w, r, "Failed to enqueue forecast run")
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"forecastJob": job, "forecastDataset": snapshot.Dataset})
}

func (h *productHandler) listForecastJobs(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Tenant context required")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	cur, err := h.db.ForecastJobs().Find(r.Context(), bson.M{"tenantId": tenant.ID, "locationId": locationID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(100))
	if err != nil {
		apierror.Internal(w, r, "Failed to list forecast jobs")
		return
	}
	defer cur.Close(r.Context())
	var jobs []models.ForecastJob
	if err := cur.All(r.Context(), &jobs); err != nil {
		apierror.Internal(w, r, "Failed to list forecast jobs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"forecastJobs": jobs})
}

func (h *productHandler) listForecastInputs(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Tenant context required")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	datasetID, err := primitive.ObjectIDFromHex(mux.Vars(r)["datasetId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid forecast dataset ID")
		return
	}
	cur, err := h.db.ForecastInputRows().Find(r.Context(), bson.M{"tenantId": tenant.ID, "locationId": locationID, "datasetId": datasetID}, options.Find().SetSort(bson.D{{Key: "rowNumber", Value: 1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list forecast inputs")
		return
	}
	defer cur.Close(r.Context())
	var rows []models.ForecastInputRow
	if err := cur.All(r.Context(), &rows); err != nil {
		apierror.Internal(w, r, "Failed to list forecast inputs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"forecastInputs": rows})
}

func (h *productHandler) listForecastMaturity(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Tenant context required")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}
	runID, err := primitive.ObjectIDFromHex(mux.Vars(r)["runId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid forecast run ID")
		return
	}
	var run models.ForecastRun
	if err := h.db.ForecastRuns().FindOne(r.Context(), bson.M{"_id": runID, "tenantId": tenant.ID, "locationId": locationID}).Decode(&run); err != nil {
		apierror.NotFound(w, r, "Forecast run not found")
		return
	}
	cur, err := h.db.ForecastInputRows().Find(r.Context(), bson.M{"tenantId": tenant.ID, "locationId": locationID, "datasetId": run.DatasetID, "kind": models.ForecastInputDemand})
	if err != nil {
		apierror.Internal(w, r, "Failed to load forecast coverage")
		return
	}
	defer cur.Close(r.Context())
	var rows []models.ForecastInputRow
	if err := cur.All(r.Context(), &rows); err != nil {
		apierror.Internal(w, r, "Failed to load forecast coverage")
		return
	}
	series := map[primitive.ObjectID][]forecastcore.Day{}
	for _, row := range rows {
		if row.ItemID != nil {
			series[*row.ItemID] = append(series[*row.ItemID], forecastcore.Day{Date: row.EffectiveAt, DemandMicros: row.QuantityMicros})
		}
	}
	items := make([]map[string]any, 0, len(series))
	itemIDs := make([]primitive.ObjectID, 0, len(series))
	for itemID := range series {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Slice(itemIDs, func(i, j int) bool { return itemIDs[i].Hex() < itemIDs[j].Hex() })
	for _, itemID := range itemIDs {
		values := series[itemID]
		q, e := forecastcore.AssessQuality(values, forecastcore.DefaultConfig())
		if e != nil {
			apierror.Internal(w, r, "Failed to assess data maturity")
			return
		}
		flags := make([]string, len(q.Flags))
		for i, f := range q.Flags {
			flags[i] = string(f)
		}
		items = append(items, map[string]any{"itemId": itemID, "quality": q, "flags": flags})
	}
	writeJSON(w, http.StatusOK, map[string]any{"dataMaturity": items})
}

func forecastReadScope(r *http.Request) (primitive.ObjectID, primitive.ObjectID, bool) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		return primitive.NilObjectID, primitive.NilObjectID, false
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		return tenant.ID, primitive.NilObjectID, false
	}
	return tenant.ID, locationID, true
}

func forecastTenantLocationFilter(tenantID, locationID primitive.ObjectID) bson.M {
	return bson.M{"tenantId": tenantID, "locationId": locationID}
}

func (h *productHandler) listReorderRecommendations(w http.ResponseWriter, r *http.Request) {
	tenantID, locationID, ok := forecastReadScope(r)
	if !ok {
		apierror.BadRequest(w, r, "Invalid forecast location or tenant context")
		return
	}
	filter := forecastTenantLocationFilter(tenantID, locationID)
	rawRunID := r.URL.Query().Get("runId")
	if rawRunID == "" {
		rawRunID = mux.Vars(r)["runId"]
	}
	if raw := rawRunID; raw != "" {
		runID, err := primitive.ObjectIDFromHex(raw)
		if err != nil {
			apierror.BadRequest(w, r, "Invalid forecast run ID")
			return
		}
		filter["runId"] = runID
	}
	cur, err := h.db.ReorderRecommendations().Find(r.Context(), filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "itemId", Value: 1}}).SetLimit(500))
	if err != nil {
		apierror.Internal(w, r, "Failed to list reorder recommendations")
		return
	}
	defer cur.Close(r.Context())
	var recommendations []models.ReorderRecommendation
	if err := cur.All(r.Context(), &recommendations); err != nil {
		apierror.Internal(w, r, "Failed to list reorder recommendations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recommendations": recommendations})
}

func (h *productHandler) getReorderRecommendation(w http.ResponseWriter, r *http.Request) {
	tenantID, locationID, ok := forecastReadScope(r)
	if !ok {
		apierror.BadRequest(w, r, "Invalid forecast location or tenant context")
		return
	}
	recommendationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["recommendationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid reorder recommendation ID")
		return
	}
	var recommendation models.ReorderRecommendation
	filter := forecastTenantLocationFilter(tenantID, locationID)
	filter["_id"] = recommendationID
	if rawRunID := mux.Vars(r)["runId"]; rawRunID != "" {
		runID, parseErr := primitive.ObjectIDFromHex(rawRunID)
		if parseErr != nil {
			apierror.BadRequest(w, r, "Invalid forecast run ID")
			return
		}
		filter["runId"] = runID
	}
	err = h.db.ReorderRecommendations().FindOne(r.Context(), filter).Decode(&recommendation)
	if errors.Is(err, mongo.ErrNoDocuments) {
		apierror.NotFound(w, r, "Reorder recommendation not found")
		return
	}
	if err != nil {
		apierror.Internal(w, r, "Failed to load reorder recommendation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recommendation": recommendation})
}

func (h *productHandler) listForecastCoverage(w http.ResponseWriter, r *http.Request) {
	tenantID, locationID, ok := forecastReadScope(r)
	if !ok {
		apierror.BadRequest(w, r, "Invalid forecast location or tenant context")
		return
	}
	filter := forecastTenantLocationFilter(tenantID, locationID)
	rawRunID := r.URL.Query().Get("runId")
	if rawRunID == "" {
		rawRunID = mux.Vars(r)["runId"]
	}
	if raw := rawRunID; raw != "" {
		runID, err := primitive.ObjectIDFromHex(raw)
		if err != nil {
			apierror.BadRequest(w, r, "Invalid forecast run ID")
			return
		}
		filter["runId"] = runID
	}
	cur, err := h.db.ForecastCoverages().Find(r.Context(), filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "itemId", Value: 1}}).SetLimit(500))
	if err != nil {
		apierror.Internal(w, r, "Failed to list forecast coverage")
		return
	}
	defer cur.Close(r.Context())
	var coverage []models.ReorderCoverage
	if err := cur.All(r.Context(), &coverage); err != nil {
		apierror.Internal(w, r, "Failed to list forecast coverage")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"coverage": coverage})
}

func (h *productHandler) getForecastCoverage(w http.ResponseWriter, r *http.Request) {
	tenantID, locationID, ok := forecastReadScope(r)
	if !ok {
		apierror.BadRequest(w, r, "Invalid forecast location or tenant context")
		return
	}
	coverageID, err := primitive.ObjectIDFromHex(mux.Vars(r)["coverageId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid forecast coverage ID")
		return
	}
	var coverage models.ReorderCoverage
	filter := forecastTenantLocationFilter(tenantID, locationID)
	filter["_id"] = coverageID
	if rawRunID := mux.Vars(r)["runId"]; rawRunID != "" {
		runID, parseErr := primitive.ObjectIDFromHex(rawRunID)
		if parseErr != nil {
			apierror.BadRequest(w, r, "Invalid forecast run ID")
			return
		}
		filter["runId"] = runID
	}
	err = h.db.ForecastCoverages().FindOne(r.Context(), filter).Decode(&coverage)
	if errors.Is(err, mongo.ErrNoDocuments) {
		apierror.NotFound(w, r, "Forecast coverage not found")
		return
	}
	if err != nil {
		apierror.Internal(w, r, "Failed to load forecast coverage")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"coverage": coverage})
}

func (h *productHandler) auditForecast(r *http.Request, action string, locationID, id primitive.ObjectID) {
	if h.logger == nil {
		return
	}
	user, userOK := middleware.GetUserFromContext(r.Context())
	tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
	if userOK && tenantOK {
		h.logger.LogTenantActivity(r.Context(), models.LogMedium, "Forecast data changed", user.ID, tenant.ID, action, map[string]interface{}{"locationId": locationID.Hex(), "id": id.Hex()})
	}
}
