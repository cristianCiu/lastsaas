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

const (
	defaultCurrency = "EUR"
	defaultLanguage = "de"
	defaultTimezone = "Europe/Berlin"
)

var ErrRestaurantSettingsVersionConflict = errors.New("restaurant settings version conflict")

type RestaurantSettingsRepository struct {
	db       *db.MongoDB
	tenantID primitive.ObjectID
}

func NewRestaurantSettingsRepository(database *db.MongoDB, tenantID primitive.ObjectID) *RestaurantSettingsRepository {
	return &RestaurantSettingsRepository{db: database, tenantID: tenantID}
}

func (r *RestaurantSettingsRepository) Get(ctx context.Context) (*models.RestaurantSettings, error) {
	var settings models.RestaurantSettings
	err := r.db.RestaurantSettings().FindOne(ctx, bson.M{"tenantId": r.tenantID}).Decode(&settings)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &models.RestaurantSettings{
			TenantID: r.tenantID, Currency: defaultCurrency, Language: defaultLanguage,
			DefaultTimezone: defaultTimezone, Version: 0,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get restaurant settings: %w", err)
	}
	return &settings, nil
}

func (r *RestaurantSettingsRepository) Put(ctx context.Context, currency, language, timezone string, expectedVersion int64) (*models.RestaurantSettings, error) {
	currency = strings.ToUpper(currency)
	language = normalizeLanguage(language)
	now := time.Now().UTC()

	if expectedVersion == 0 {
		id := primitive.NewObjectID()
		settings := models.RestaurantSettings{
			ID: &id, TenantID: r.tenantID, Currency: currency, Language: language,
			DefaultTimezone: timezone, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := validation.Validate(&settings); err != nil {
			return nil, err
		}
		if _, err := r.db.RestaurantSettings().InsertOne(ctx, settings); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return nil, ErrRestaurantSettingsVersionConflict
			}
			return nil, fmt.Errorf("create restaurant settings: %w", err)
		}
		return &settings, nil
	}

	candidate := models.RestaurantSettings{
		TenantID: r.tenantID, Currency: currency, Language: language,
		DefaultTimezone: timezone, Version: expectedVersion, CreatedAt: now, UpdatedAt: now,
	}
	if err := validation.Validate(&candidate); err != nil {
		return nil, err
	}
	var settings models.RestaurantSettings
	err := r.db.RestaurantSettings().FindOneAndUpdate(ctx,
		bson.M{"tenantId": r.tenantID, "version": expectedVersion},
		bson.M{"$set": bson.M{"currency": currency, "language": language, "defaultTimezone": timezone, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&settings)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrRestaurantSettingsVersionConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update restaurant settings: %w", err)
	}
	return &settings, nil
}

func normalizeLanguage(value string) string {
	parts := strings.Split(value, "-")
	parts[0] = strings.ToLower(parts[0])
	if len(parts) == 2 {
		parts[1] = strings.ToUpper(parts[1])
	}
	return strings.Join(parts, "-")
}

func (h *productHandler) getRestaurantSettings(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	settings, err := NewRestaurantSettingsRepository(h.db, tenant.ID).Get(r.Context())
	if err != nil {
		apierror.Internal(w, r, "Failed to get restaurant settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (h *productHandler) putRestaurantSettings(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	var request struct {
		Currency        string `json:"currency" validate:"required"`
		Language        string `json:"language" validate:"required"`
		DefaultTimezone string `json:"defaultTimezone" validate:"required"`
		Version         *int64 `json:"version" validate:"required,gte=0"`
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
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	settings, err := NewRestaurantSettingsRepository(h.db, tenant.ID).Put(
		r.Context(), request.Currency, request.Language, request.DefaultTimezone, *request.Version,
	)
	switch {
	case err == nil:
		if user, found := middleware.GetUserFromContext(r.Context()); found && h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, "Restaurant settings updated", user.ID, tenant.ID, "restaurant_settings.updated", map[string]interface{}{"version": settings.Version})
		}
		writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
	case errors.Is(err, ErrRestaurantSettingsVersionConflict):
		apierror.VersionConflict(w, r, "Restaurant settings were modified by another request")
	case strings.HasPrefix(err.Error(), "validation failed:"):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Failed to update restaurant settings")
	}
}
