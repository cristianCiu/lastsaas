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

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrDuplicateStorageArea       = errors.New("storage area name already exists")
	ErrStorageAreaNotFound        = errors.New("storage area not found")
	ErrStorageAreaVersionConflict = errors.New("storage area version conflict")
)

type StorageAreaUpdate struct {
	Name     *string
	Type     *models.StorageAreaType
	IsActive *bool
}

type StorageAreaRepository struct {
	db         *db.MongoDB
	tenantID   primitive.ObjectID
	locationID primitive.ObjectID
}

func NewStorageAreaRepository(database *db.MongoDB, tenantID, locationID primitive.ObjectID) *StorageAreaRepository {
	return &StorageAreaRepository{db: database, tenantID: tenantID, locationID: locationID}
}

func (r *StorageAreaRepository) verifyLocation(ctx context.Context) error {
	err := r.db.Locations().FindOne(ctx, bson.M{"_id": r.locationID, "tenantId": r.tenantID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrStorageAreaNotFound
	}
	if err != nil {
		return fmt.Errorf("verify storage area location: %w", err)
	}
	return nil
}

func (r *StorageAreaRepository) List(ctx context.Context) ([]models.StorageArea, error) {
	if err := r.verifyLocation(ctx); err != nil {
		return nil, err
	}
	cursor, err := r.db.StorageAreas().Find(ctx, bson.M{"tenantId": r.tenantID, "locationId": r.locationID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list storage areas: %w", err)
	}
	defer cursor.Close(ctx)
	areas := make([]models.StorageArea, 0)
	if err := cursor.All(ctx, &areas); err != nil {
		return nil, fmt.Errorf("decode storage areas: %w", err)
	}
	return areas, nil
}

func (r *StorageAreaRepository) Create(ctx context.Context, name string, areaType models.StorageAreaType) (*models.StorageArea, error) {
	if err := r.verifyLocation(ctx); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	area := models.StorageArea{ID: primitive.NewObjectID(), TenantID: r.tenantID, LocationID: r.locationID, Name: strings.TrimSpace(name), Type: areaType, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&area); err != nil {
		return nil, err
	}
	if _, err := r.db.StorageAreas().InsertOne(ctx, area); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDuplicateStorageArea
		}
		return nil, fmt.Errorf("create storage area: %w", err)
	}
	return &area, nil
}

func (r *StorageAreaRepository) Update(ctx context.Context, id primitive.ObjectID, version int64, changes StorageAreaUpdate) (*models.StorageArea, bool, error) {
	if err := r.verifyLocation(ctx); err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	set := bson.M{"updatedAt": now}
	var name *string
	if changes.Name != nil {
		trimmed := strings.TrimSpace(*changes.Name)
		if err := validation.Validate(&struct {
			Name string `validate:"required,min=1,max=200"`
		}{Name: trimmed}); err != nil {
			return nil, false, err
		}
		name = &trimmed
		set["name"] = trimmed
	}
	if changes.Type != nil {
		if !models.ValidStorageAreaType(*changes.Type) {
			return nil, false, fmt.Errorf("validation failed: Type failed storage_area_type validation")
		}
		set["type"] = *changes.Type
	}
	if changes.IsActive != nil {
		set["isActive"] = *changes.IsActive
	}
	var previous models.StorageArea
	err := r.db.StorageAreas().FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenantId": r.tenantID, "locationId": r.locationID, "version": version},
		bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.Before),
	).Decode(&previous)
	if err == nil {
		area := previous
		area.Version++
		area.UpdatedAt = now
		if name != nil {
			area.Name = *name
		}
		if changes.Type != nil {
			area.Type = *changes.Type
		}
		transitioned := changes.IsActive != nil && previous.IsActive != *changes.IsActive
		if changes.IsActive != nil {
			area.IsActive = *changes.IsActive
		}
		return &area, transitioned, nil
	}
	if mongo.IsDuplicateKeyError(err) {
		return nil, false, ErrDuplicateStorageArea
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, fmt.Errorf("update storage area: %w", err)
	}
	err = r.db.StorageAreas().FindOne(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "locationId": r.locationID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, ErrStorageAreaNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("check storage area version: %w", err)
	}
	return nil, false, ErrStorageAreaVersionConflict
}

func storagePathScope(w http.ResponseWriter, r *http.Request) (*models.Tenant, primitive.ObjectID, bool) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return nil, primitive.NilObjectID, false
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return nil, primitive.NilObjectID, false
	}
	return tenant, locationID, true
}

func (h *productHandler) listStorageAreas(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, ok := storagePathScope(w, r)
	if !ok {
		return
	}
	areas, err := NewStorageAreaRepository(h.db, tenant.ID, locationID).List(r.Context())
	if errors.Is(err, ErrStorageAreaNotFound) {
		apierror.NotFound(w, r, "Location not found")
		return
	}
	if err != nil {
		apierror.Internal(w, r, "Failed to list storage areas")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"storageAreas": areas})
}

func (h *productHandler) createStorageArea(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, ok := storagePathScope(w, r)
	if !ok {
		return
	}
	var request struct {
		Name string                 `json:"name" validate:"required"`
		Type models.StorageAreaType `json:"type" validate:"required,storage_area_type"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	area, err := NewStorageAreaRepository(h.db, tenant.ID, locationID).Create(r.Context(), request.Name, request.Type)
	switch {
	case err == nil:
		h.auditStorageArea(r, tenant, area, "storage_area.created", "Storage area created")
		writeJSON(w, http.StatusCreated, map[string]any{"storageArea": area})
	case errors.Is(err, ErrStorageAreaNotFound):
		apierror.NotFound(w, r, "Location not found")
	case errors.Is(err, ErrDuplicateStorageArea):
		apierror.Conflict(w, r, "Storage area name already exists")
	case strings.HasPrefix(err.Error(), "validation failed:"):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Failed to create storage area")
	}
}

func (h *productHandler) updateStorageArea(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, ok := storagePathScope(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["storageAreaId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid storage area ID")
		return
	}
	var request struct {
		Version  int64                   `json:"version" validate:"gte=1"`
		Name     *string                 `json:"name"`
		Type     *models.StorageAreaType `json:"type"`
		IsActive *bool                   `json:"isActive"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.Name == nil && request.Type == nil && request.IsActive == nil {
		apierror.BadRequest(w, r, "At least one mutable field is required")
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	area, activeTransitioned, err := NewStorageAreaRepository(h.db, tenant.ID, locationID).Update(r.Context(), id, request.Version, StorageAreaUpdate{Name: request.Name, Type: request.Type, IsActive: request.IsActive})
	switch {
	case err == nil:
		action, message := "storage_area.updated", "Storage area updated"
		if activeTransitioned && *request.IsActive {
			action, message = "storage_area.reactivated", "Storage area reactivated"
		}
		if activeTransitioned && !*request.IsActive {
			action, message = "storage_area.deactivated", "Storage area deactivated"
		}
		h.auditStorageArea(r, tenant, area, action, message)
		writeJSON(w, http.StatusOK, map[string]any{"storageArea": area})
	case errors.Is(err, ErrStorageAreaNotFound):
		apierror.NotFound(w, r, "Storage area not found")
	case errors.Is(err, ErrStorageAreaVersionConflict):
		apierror.VersionConflict(w, r, "Storage area was modified by another request")
	case errors.Is(err, ErrDuplicateStorageArea):
		apierror.Conflict(w, r, "Storage area name already exists")
	case strings.HasPrefix(err.Error(), "validation failed:"):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Failed to update storage area")
	}
}

func decodeStrict(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		apierror.BadRequest(w, r, "Invalid request body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		apierror.BadRequest(w, r, "Invalid request body")
		return false
	}
	return true
}

func (h *productHandler) auditStorageArea(r *http.Request, tenant *models.Tenant, area *models.StorageArea, action, message string) {
	if user, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
		h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{"storageAreaId": area.ID.Hex(), "locationId": area.LocationID.Hex(), "version": area.Version})
	}
}
