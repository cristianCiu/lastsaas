package product

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"lastsaas/internal/apierror"
	"lastsaas/internal/db"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	"lastsaas/internal/syslog"
	"lastsaas/internal/validation"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type productHandler struct {
	db     *db.MongoDB
	logger *syslog.Logger
}

// RegisterRoutes installs all product routes beneath an already guarded API
// router. Production and integration tests call this same registration path.
func RegisterRoutes(guarded *mux.Router, database *db.MongoDB, auth *middleware.AuthMiddleware, tenant *middleware.TenantMiddleware, logger *syslog.Logger) {
	handler := &productHandler{db: database, logger: logger}
	requireProfile := RequireStaffProfile(database)
	onboardingAPI := guarded.PathPrefix("/product/onboarding").Subrouter()
	onboardingAPI.Use(auth.RequireAuth)
	onboardingAPI.Use(tenant.RequireTenant)
	onboardingAPI.Handle("", withProductMiddleware(http.HandlerFunc(handler.getOnboarding), requireProfile)).Methods(http.MethodGet)
	onboardingAPI.Handle("/complete", withProductMiddleware(http.HandlerFunc(handler.completeOnboarding), requireProfile, requireCoreRole(models.RoleOwner))).Methods(http.MethodPost)

	productAPI := guarded.PathPrefix("/product").Subrouter()
	productAPI.Use(auth.RequireAuth)
	productAPI.Use(tenant.RequireTenant)
	productAPI.Use(middleware.RequireActiveBilling())

	productAPI.Handle("/restaurant-settings", withProductMiddleware(http.HandlerFunc(handler.getRestaurantSettings), requireProfile)).Methods(http.MethodGet)
	productAPI.Handle("/branding", withProductMiddleware(http.HandlerFunc(handler.getTenantBranding), requireProfile)).Methods(http.MethodGet)
	productAPI.Handle("/branding/assets", withProductMiddleware(http.HandlerFunc(handler.listTenantBrandingAssets), requireProfile)).Methods(http.MethodGet)
	productAPI.Handle("/branding/assets/{kind}", withProductMiddleware(http.HandlerFunc(handler.serveTenantBrandingAsset), requireProfile)).Methods(http.MethodGet)
	productAPI.Handle("/locations", withProductMiddleware(http.HandlerFunc(handler.create), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodPost)
	productAPI.Handle("/restaurant-settings", withProductMiddleware(http.HandlerFunc(handler.putRestaurantSettings), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodPut)
	productAPI.Handle("/branding", withProductMiddleware(http.HandlerFunc(handler.putTenantBranding), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodPut)
	productAPI.Handle("/branding/assets/{kind}", withProductMiddleware(http.HandlerFunc(handler.putTenantBrandingAsset), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodPut)
	productAPI.Handle("/branding/assets/{kind}", withProductMiddleware(http.HandlerFunc(handler.deleteTenantBrandingAsset), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodDelete)
	productAPI.HandleFunc("/staff-profile", handler.getStaffProfile).Methods(http.MethodGet)
	productAPI.Handle("/locations", withProductMiddleware(http.HandlerFunc(handler.list), requireProfile)).Methods(http.MethodGet)
	productAPI.Handle("/staff-profiles", withProductMiddleware(http.HandlerFunc(handler.listStaffProfiles), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodGet)
	productAPI.Handle("/staff-profiles/{userId}", withProductMiddleware(http.HandlerFunc(handler.replaceStaffProfile), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodPut)

	requireLocation := RequireAuthorizedLocation(database, "locationId")
	productAPI.Handle("/locations/{locationId}", withProductMiddleware(http.HandlerFunc(handler.update), requireProfile, requireCoreRole(models.RoleAdmin), requireLocation)).Methods(http.MethodPatch)
	productAPI.Handle("/locations/{locationId}/branding", withProductMiddleware(http.HandlerFunc(handler.getLocationBranding), requireProfile, requireLocation)).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/branding", withProductMiddleware(http.HandlerFunc(handler.putLocationBranding), requireProfile, requireLocation, requireCoreRole(models.RoleAdmin), middleware.RequireEntitlement(database, "location_branding"))).Methods(http.MethodPut)
	productAPI.Handle("/locations/{locationId}/branding", withProductMiddleware(http.HandlerFunc(handler.deleteLocationBranding), requireProfile, requireLocation, requireCoreRole(models.RoleAdmin), middleware.RequireEntitlement(database, "location_branding"))).Methods(http.MethodDelete)
	productAPI.Handle("/locations/{locationId}/storage-areas", withProductMiddleware(http.HandlerFunc(handler.listStorageAreas), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionStorageAreasRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/storage-areas", withProductMiddleware(http.HandlerFunc(handler.createStorageArea), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionStorageAreasManage))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/storage-areas/{storageAreaId}", withProductMiddleware(http.HandlerFunc(handler.updateStorageArea), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionStorageAreasManage))).Methods(http.MethodPatch)
}

func withProductMiddleware(handler http.Handler, chain ...func(http.Handler) http.Handler) http.Handler {
	for i := len(chain) - 1; i >= 0; i-- {
		handler = chain[i](handler)
	}
	return handler
}

func (h *productHandler) update(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}

	var request struct {
		Version  int64   `json:"version" validate:"gte=1"`
		Name     *string `json:"name" validate:"omitempty,min=1,max=200"`
		Timezone *string `json:"timezone" validate:"omitempty,max=100,iana_timezone"`
		IsActive *bool   `json:"isActive"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		apierror.BadRequest(w, r, "Invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		apierror.BadRequest(w, r, "Invalid request body")
		return
	}
	if request.Name == nil && request.Timezone == nil && request.IsActive == nil {
		apierror.BadRequest(w, r, "At least one mutable field is required")
		return
	}
	if request.Name != nil {
		trimmed := strings.TrimSpace(*request.Name)
		request.Name = &trimmed
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}

	location, err := NewLocationRepository(h.db, tenant).Update(r.Context(), locationID, request.Version, LocationUpdate{
		Name: request.Name, Timezone: request.Timezone, IsActive: request.IsActive,
	})
	switch {
	case err == nil:
		action := "location.updated"
		message := "Location updated"
		if request.IsActive != nil {
			if *request.IsActive {
				action = "location.reactivated"
				message = "Location reactivated"
			} else {
				action = "location.deactivated"
				message = "Location deactivated"
			}
		}
		if user, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{
				"locationId": location.ID.Hex(),
				"code":       location.Code,
				"version":    location.Version,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"location": location})
	case errors.Is(err, ErrLocationNotFound):
		apierror.NotFound(w, r, "Location not found")
	case errors.Is(err, ErrLocationVersionConflict):
		apierror.VersionConflict(w, r, "Location was modified by another request")
	default:
		if strings.HasPrefix(err.Error(), "validation failed:") {
			apierror.Validation(w, r, err.Error())
			return
		}
		apierror.Internal(w, r, "Failed to update location")
	}
}

func (h *productHandler) list(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	profile, ok := GetStaffProfileFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Active staff profile required")
		return
	}
	var assigned []primitive.ObjectID
	if !profile.AllLocations {
		assigned = make([]primitive.ObjectID, len(profile.LocationIDs))
		copy(assigned, profile.LocationIDs)
	}
	locations, err := NewLocationRepository(h.db, tenant).ListAssigned(r.Context(), assigned)
	if err != nil {
		apierror.Internal(w, r, "Failed to list locations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"locations": locations})
}

func (h *productHandler) create(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	var request struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Timezone string `json:"timezone"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		apierror.BadRequest(w, r, "Invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		apierror.BadRequest(w, r, "Invalid request body")
		return
	}

	location, err := NewLocationRepository(h.db, tenant).Create(
		r.Context(), request.Code, strings.TrimSpace(request.Name), request.Timezone,
	)
	switch {
	case err == nil:
		if user, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, "Location created", user.ID, tenant.ID, "location.created", map[string]interface{}{
				"locationId": location.ID.Hex(),
				"code":       location.Code,
			})
		}
		writeJSON(w, http.StatusCreated, map[string]any{"location": location})
	case errors.Is(err, ErrDuplicateLocationCode):
		apierror.Conflict(w, r, "Location code already exists")
	case errors.Is(err, ErrLocationLimitReached):
		apierror.PlanLimit(w, r, "Maximum number of locations reached")
	default:
		if strings.HasPrefix(err.Error(), "validation failed:") {
			apierror.Validation(w, r, err.Error())
			return
		}
		apierror.Internal(w, r, "Failed to create location")
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
