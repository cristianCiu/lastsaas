package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lastsaas/internal/apierror"
	"lastsaas/internal/db"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrLocationBrandingNotFound        = errors.New("location branding not found")
	ErrLocationBrandingVersionConflict = errors.New("location branding version conflict")
)

type LocationBrandingRepository struct {
	db         *db.MongoDB
	tenantID   primitive.ObjectID
	locationID primitive.ObjectID
}

func NewLocationBrandingRepository(database *db.MongoDB, tenantID, locationID primitive.ObjectID) *LocationBrandingRepository {
	return &LocationBrandingRepository{db: database, tenantID: tenantID, locationID: locationID}
}

func (r *LocationBrandingRepository) Get(ctx context.Context) (*models.LocationBranding, error) {
	var branding models.LocationBranding
	err := r.db.LocationBranding().FindOne(ctx, bson.M{"tenantId": r.tenantID, "locationId": r.locationID}).Decode(&branding)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &models.LocationBranding{TenantID: r.tenantID, LocationID: r.locationID, Version: 0}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get location branding: %w", err)
	}
	return &branding, nil
}

func (r *LocationBrandingRepository) Put(ctx context.Context, displayName, primaryColor, accentColor string, font models.BrandingFont, expectedVersion int64) (*models.LocationBranding, error) {
	displayName = strings.TrimSpace(displayName)
	primaryColor = strings.ToLower(strings.TrimSpace(primaryColor))
	accentColor = strings.ToLower(strings.TrimSpace(accentColor))
	font = models.BrandingFont(strings.ToLower(strings.TrimSpace(string(font))))
	now := time.Now().UTC()

	if expectedVersion == 0 {
		id := primitive.NewObjectID()
		branding := models.LocationBranding{
			ID: &id, TenantID: r.tenantID, LocationID: r.locationID, DisplayName: displayName,
			PrimaryColor: primaryColor, AccentColor: accentColor, Font: font,
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := validation.Validate(&branding); err != nil {
			return nil, err
		}
		if _, err := r.db.LocationBranding().InsertOne(ctx, branding); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return nil, ErrLocationBrandingVersionConflict
			}
			return nil, fmt.Errorf("create location branding: %w", err)
		}
		return &branding, nil
	}

	candidate := models.LocationBranding{
		TenantID: r.tenantID, LocationID: r.locationID, DisplayName: displayName,
		PrimaryColor: primaryColor, AccentColor: accentColor, Font: font,
		Version: expectedVersion, CreatedAt: now, UpdatedAt: now,
	}
	if err := validation.Validate(&candidate); err != nil {
		return nil, err
	}
	var branding models.LocationBranding
	err := r.db.LocationBranding().FindOneAndUpdate(ctx,
		bson.M{"tenantId": r.tenantID, "locationId": r.locationID, "version": expectedVersion},
		bson.M{"$set": bson.M{"displayName": displayName, "primaryColor": primaryColor, "accentColor": accentColor, "font": font, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&branding)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrLocationBrandingVersionConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update location branding: %w", err)
	}
	return &branding, nil
}

func (r *LocationBrandingRepository) Delete(ctx context.Context, expectedVersion int64) error {
	result, err := r.db.LocationBranding().DeleteOne(ctx, bson.M{"tenantId": r.tenantID, "locationId": r.locationID, "version": expectedVersion})
	if err != nil {
		return fmt.Errorf("delete location branding: %w", err)
	}
	if result.DeletedCount == 1 {
		return nil
	}
	count, err := r.db.LocationBranding().CountDocuments(ctx, bson.M{"tenantId": r.tenantID, "locationId": r.locationID})
	if err != nil {
		return fmt.Errorf("check location branding: %w", err)
	}
	if count == 0 {
		return ErrLocationBrandingNotFound
	}
	return ErrLocationBrandingVersionConflict
}

func resolveLocationBranding(location models.Location, tenant *models.TenantBranding, override *models.LocationBranding) models.ResolvedLocationBranding {
	resolved := models.ResolvedLocationBranding{
		LocationID: location.ID, DisplayName: location.Name,
		LocationBrandingVersion: override.Version, TenantBrandingVersion: tenant.Version,
		Sources: map[string]string{"displayName": "location", "primaryColor": "platform", "accentColor": "platform", "font": "platform"},
	}
	if tenant.PrimaryColor != "" {
		resolved.PrimaryColor = tenant.PrimaryColor
		resolved.Sources["primaryColor"] = "tenant"
	}
	if tenant.AccentColor != "" {
		resolved.AccentColor = tenant.AccentColor
		resolved.Sources["accentColor"] = "tenant"
	}
	if tenant.Font != "" {
		resolved.Font = tenant.Font
		resolved.Sources["font"] = "tenant"
	}
	if override.DisplayName != "" {
		resolved.DisplayName = override.DisplayName
		resolved.Sources["displayName"] = "location_branding"
	}
	if override.PrimaryColor != "" {
		resolved.PrimaryColor = override.PrimaryColor
		resolved.Sources["primaryColor"] = "location_branding"
	}
	if override.AccentColor != "" {
		resolved.AccentColor = override.AccentColor
		resolved.Sources["accentColor"] = "location_branding"
	}
	if override.Font != "" {
		resolved.Font = override.Font
		resolved.Sources["font"] = "location_branding"
	}
	return resolved
}

func locationBrandingScope(w http.ResponseWriter, r *http.Request) (*models.Tenant, primitive.ObjectID, bool) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return nil, primitive.NilObjectID, false
	}
	if tenant.IsRoot {
		apierror.Forbidden(w, r, "Location branding is not available for the platform tenant")
		return nil, primitive.NilObjectID, false
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return nil, primitive.NilObjectID, false
	}
	return tenant, locationID, true
}

func (h *productHandler) getLocationBranding(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, ok := locationBrandingScope(w, r)
	if !ok {
		return
	}
	var location models.Location
	if err := h.db.Locations().FindOne(r.Context(), bson.M{"_id": locationID, "tenantId": tenant.ID}).Decode(&location); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Location not found")
		} else {
			apierror.Internal(w, r, "Failed to load location")
		}
		return
	}
	override, err := NewLocationBrandingRepository(h.db, tenant.ID, locationID).Get(r.Context())
	if err != nil {
		apierror.Internal(w, r, "Failed to get location branding")
		return
	}
	tenantBranding, err := NewTenantBrandingRepository(h.db, tenant.ID).Get(r.Context())
	if err != nil {
		apierror.Internal(w, r, "Failed to resolve location branding")
		return
	}
	entitled, err := middleware.HasBooleanEntitlement(r.Context(), h.db, tenant, "location_branding")
	if err != nil {
		apierror.Internal(w, r, "Failed to resolve location branding entitlement")
		return
	}
	overrideForResolution := override
	if !entitled {
		overrideForResolution = &models.LocationBranding{}
	}
	resolved := resolveLocationBranding(location, tenantBranding, overrideForResolution)
	w.Header().Set("ETag", fmt.Sprintf(`"location-%d-location-branding-%d-tenant-branding-%d-entitled-%t"`, location.Version, override.Version, tenantBranding.Version, entitled))
	w.Header().Set("Cache-Control", "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"branding": override, "resolved": resolved, "entitled": entitled})
}

func (h *productHandler) putLocationBranding(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, ok := locationBrandingScope(w, r)
	if !ok {
		return
	}
	var request struct {
		DisplayName  string              `json:"displayName" validate:"omitempty,max=200"`
		PrimaryColor string              `json:"primaryColor" validate:"omitempty,hex_color"`
		AccentColor  string              `json:"accentColor" validate:"omitempty,hex_color"`
		Font         models.BrandingFont `json:"font" validate:"branding_font"`
		Version      *int64              `json:"version" validate:"required,gte=0"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		apierror.BadRequest(w, r, "Invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		apierror.BadRequest(w, r, "Invalid request body")
		return
	}
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.PrimaryColor = strings.ToLower(strings.TrimSpace(request.PrimaryColor))
	request.AccentColor = strings.ToLower(strings.TrimSpace(request.AccentColor))
	request.Font = models.BrandingFont(strings.ToLower(strings.TrimSpace(string(request.Font))))
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	branding, err := NewLocationBrandingRepository(h.db, tenant.ID, locationID).Put(r.Context(), request.DisplayName, request.PrimaryColor, request.AccentColor, request.Font, *request.Version)
	switch {
	case err == nil:
		if user, found := middleware.GetUserFromContext(r.Context()); found && h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, "Location branding updated", user.ID, tenant.ID, "location_branding.updated", map[string]interface{}{"locationId": locationID.Hex(), "version": branding.Version})
		}
		writeJSON(w, http.StatusOK, map[string]any{"branding": branding})
	case errors.Is(err, ErrLocationBrandingVersionConflict):
		apierror.VersionConflict(w, r, "Location branding was modified by another request")
	case strings.HasPrefix(err.Error(), "validation failed:"):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Failed to update location branding")
	}
}

func (h *productHandler) deleteLocationBranding(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, ok := locationBrandingScope(w, r)
	if !ok {
		return
	}
	version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || version < 1 {
		apierror.Validation(w, r, "version must be at least 1")
		return
	}
	err = NewLocationBrandingRepository(h.db, tenant.ID, locationID).Delete(r.Context(), version)
	switch {
	case err == nil:
		if user, found := middleware.GetUserFromContext(r.Context()); found && h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, "Location branding reset", user.ID, tenant.ID, "location_branding.reset", map[string]interface{}{"locationId": locationID.Hex()})
		}
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrLocationBrandingNotFound):
		apierror.NotFound(w, r, "Location branding not found")
	case errors.Is(err, ErrLocationBrandingVersionConflict):
		apierror.VersionConflict(w, r, "Location branding was modified by another request")
	default:
		apierror.Internal(w, r, "Failed to reset location branding")
	}
}
