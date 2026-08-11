package product

import (
	"context"
	"errors"
	"net/http"
	"time"

	"lastsaas/internal/apierror"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type onboardingStatus struct {
	Completed                  bool       `json:"completed"`
	RestaurantSettingsComplete bool       `json:"restaurantSettingsComplete"`
	FirstLocationComplete      bool       `json:"firstLocationComplete"`
	CompletedAt                *time.Time `json:"completedAt,omitempty"`
}

func loadOnboardingStatus(ctx context.Context, tenant *models.Tenant, database interface {
	RestaurantSettings() *mongo.Collection
	Locations() *mongo.Collection
}) (onboardingStatus, error) {
	settingsCount, err := database.RestaurantSettings().CountDocuments(ctx, bson.M{"tenantId": tenant.ID})
	if err != nil {
		return onboardingStatus{}, err
	}
	locationCount, err := database.Locations().CountDocuments(ctx, bson.M{"tenantId": tenant.ID, "isActive": true})
	if err != nil {
		return onboardingStatus{}, err
	}
	return onboardingStatus{
		Completed: tenant.OnboardingCompletedAt != nil, RestaurantSettingsComplete: settingsCount > 0,
		FirstLocationComplete: locationCount > 0, CompletedAt: tenant.OnboardingCompletedAt,
	}, nil
}

func (h *productHandler) getOnboarding(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	if tenant.IsRoot {
		apierror.Forbidden(w, r, "Restaurant onboarding is not available for the platform tenant")
		return
	}
	status, err := loadOnboardingStatus(r.Context(), tenant, h.db)
	if err != nil {
		apierror.Internal(w, r, "Failed to load onboarding status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"onboarding": status})
}

func (h *productHandler) completeOnboarding(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	if tenant.IsRoot {
		apierror.Forbidden(w, r, "Restaurant onboarding is not available for the platform tenant")
		return
	}
	status, err := loadOnboardingStatus(r.Context(), tenant, h.db)
	if err != nil {
		apierror.Internal(w, r, "Failed to validate onboarding")
		return
	}
	if !status.RestaurantSettingsComplete || !status.FirstLocationComplete {
		apierror.Validation(w, r, "Restaurant settings and one active location are required")
		return
	}
	if status.Completed {
		writeJSON(w, http.StatusOK, map[string]any{"onboarding": status})
		return
	}
	now := time.Now().UTC()
	completedNow := false
	result, err := h.db.Tenants().UpdateOne(r.Context(), bson.M{"_id": tenant.ID, "onboardingCompletedAt": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"onboardingCompletedAt": now, "updatedAt": now}})
	if err != nil {
		apierror.Internal(w, r, "Failed to complete onboarding")
		return
	}
	if result.ModifiedCount == 0 {
		var refreshed models.Tenant
		if err := h.db.Tenants().FindOne(r.Context(), bson.M{"_id": tenant.ID}).Decode(&refreshed); err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			apierror.Internal(w, r, "Failed to complete onboarding")
			return
		}
		status.Completed = refreshed.OnboardingCompletedAt != nil
		status.CompletedAt = refreshed.OnboardingCompletedAt
	} else {
		completedNow = true
		status.Completed = true
		status.CompletedAt = &now
	}
	if user, ok := middleware.GetUserFromContext(r.Context()); completedNow && ok && h.logger != nil {
		h.logger.LogTenantActivity(r.Context(), models.LogLow, "Restaurant onboarding completed", user.ID, tenant.ID, "restaurant.onboarding.completed", nil)
	}
	writeJSON(w, http.StatusOK, map[string]any{"onboarding": status})
}
