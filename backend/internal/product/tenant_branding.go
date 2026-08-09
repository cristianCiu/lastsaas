package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"lastsaas/internal/apierror"
	"lastsaas/internal/db"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrTenantBrandingVersionConflict = errors.New("tenant branding version conflict")

type TenantBrandingRepository struct {
	db       *db.MongoDB
	tenantID primitive.ObjectID
}

func NewTenantBrandingRepository(database *db.MongoDB, tenantID primitive.ObjectID) *TenantBrandingRepository {
	return &TenantBrandingRepository{db: database, tenantID: tenantID}
}

func (r *TenantBrandingRepository) Get(ctx context.Context) (*models.TenantBranding, error) {
	var branding models.TenantBranding
	err := r.db.TenantBranding().FindOne(ctx, bson.M{"tenantId": r.tenantID}).Decode(&branding)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &models.TenantBranding{TenantID: r.tenantID, Version: 0}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant branding: %w", err)
	}
	return &branding, nil
}

func (r *TenantBrandingRepository) Put(ctx context.Context, primaryColor, accentColor string, font models.BrandingFont, expectedVersion int64) (*models.TenantBranding, error) {
	primaryColor = strings.ToLower(strings.TrimSpace(primaryColor))
	accentColor = strings.ToLower(strings.TrimSpace(accentColor))
	font = models.BrandingFont(strings.ToLower(strings.TrimSpace(string(font))))
	now := time.Now().UTC()

	if expectedVersion == 0 {
		id := primitive.NewObjectID()
		branding := models.TenantBranding{
			ID: &id, TenantID: r.tenantID, PrimaryColor: primaryColor, AccentColor: accentColor,
			Font: font, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := validation.Validate(&branding); err != nil {
			return nil, err
		}
		if _, err := r.db.TenantBranding().InsertOne(ctx, branding); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return nil, ErrTenantBrandingVersionConflict
			}
			return nil, fmt.Errorf("create tenant branding: %w", err)
		}
		return &branding, nil
	}

	candidate := models.TenantBranding{
		TenantID: r.tenantID, PrimaryColor: primaryColor, AccentColor: accentColor,
		Font: font, Version: expectedVersion, CreatedAt: now, UpdatedAt: now,
	}
	if err := validation.Validate(&candidate); err != nil {
		return nil, err
	}
	var branding models.TenantBranding
	err := r.db.TenantBranding().FindOneAndUpdate(ctx,
		bson.M{"tenantId": r.tenantID, "version": expectedVersion},
		bson.M{"$set": bson.M{"primaryColor": primaryColor, "accentColor": accentColor, "font": font, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&branding)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrTenantBrandingVersionConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update tenant branding: %w", err)
	}
	return &branding, nil
}

func (h *productHandler) getTenantBranding(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	if tenant.IsRoot {
		apierror.Forbidden(w, r, "Restaurant branding is not available for the platform tenant")
		return
	}
	branding, err := NewTenantBrandingRepository(h.db, tenant.ID).Get(r.Context())
	if err != nil {
		apierror.Internal(w, r, "Failed to get tenant branding")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"branding": branding})
}

func (h *productHandler) putTenantBranding(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	if tenant.IsRoot {
		apierror.Forbidden(w, r, "Restaurant branding is not available for the platform tenant")
		return
	}
	var request struct {
		PrimaryColor string              `json:"primaryColor" validate:"omitempty,hex_color"`
		AccentColor  string              `json:"accentColor" validate:"omitempty,hex_color"`
		Font         models.BrandingFont `json:"font" validate:"branding_font"`
		Version      *int64              `json:"version" validate:"required,gte=0"`
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
	request.PrimaryColor = strings.ToLower(strings.TrimSpace(request.PrimaryColor))
	request.AccentColor = strings.ToLower(strings.TrimSpace(request.AccentColor))
	request.Font = models.BrandingFont(strings.ToLower(strings.TrimSpace(string(request.Font))))
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}

	branding, err := NewTenantBrandingRepository(h.db, tenant.ID).Put(r.Context(), request.PrimaryColor, request.AccentColor, request.Font, *request.Version)
	switch {
	case err == nil:
		if user, found := middleware.GetUserFromContext(r.Context()); found && h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, "Tenant branding updated", user.ID, tenant.ID, "tenant_branding.updated", map[string]interface{}{"version": branding.Version})
		}
		writeJSON(w, http.StatusOK, map[string]any{"branding": branding})
	case errors.Is(err, ErrTenantBrandingVersionConflict):
		apierror.VersionConflict(w, r, "Tenant branding was modified by another request")
	case strings.HasPrefix(err.Error(), "validation failed:"):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Failed to update tenant branding")
	}
}
