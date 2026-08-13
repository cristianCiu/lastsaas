package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	ErrItemNotFound               = errors.New("item not found")
	ErrDuplicateItemSKU           = errors.New("item sku already exists")
	ErrItemVersionConflict        = errors.New("item version conflict")
	ErrItemCategoryInvalid        = errors.New("item category must be active and belong to the tenant")
	ErrItemBaseUnitInvalid        = errors.New("item base unit must be active and belong to the tenant")
	ErrItemBaseUnitHasConversions = errors.New("item base unit cannot change while conversions exist")
)

type ItemRepository struct {
	db       *db.MongoDB
	tenantID primitive.ObjectID
}

type ItemUpdate struct {
	Name          *string
	Description   *string
	Brand         *string
	CategoryID    *primitive.ObjectID
	BaseUnitID    *primitive.ObjectID
	Allergens     *[]string
	ShelfLifeDays **int32
	Stockable     *bool
	IsActive      *bool
}

func NewItemRepository(database *db.MongoDB, tenantID primitive.ObjectID) *ItemRepository {
	return &ItemRepository{db: database, tenantID: tenantID}
}

func normalizeAllergens(allergens []string) []string {
	if allergens == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(allergens))
	result := make([]string, 0, len(allergens))
	for _, allergen := range allergens {
		if _, exists := seen[allergen]; exists {
			continue
		}
		seen[allergen] = struct{}{}
		result = append(result, allergen)
	}
	return result
}

func (r *ItemRepository) verifyReferences(ctx context.Context, categoryID, baseUnitID primitive.ObjectID, categoryMustBeActive, baseUnitMustBeActive bool) error {
	categoryFilter := bson.M{"_id": categoryID, "tenantId": r.tenantID}
	if categoryMustBeActive {
		categoryFilter["isActive"] = true
	}
	err := r.db.Categories().FindOne(ctx, categoryFilter, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrItemCategoryInvalid
	}
	if err != nil {
		return fmt.Errorf("verify item category: %w", err)
	}
	unitFilter := bson.M{"_id": baseUnitID, "tenantId": r.tenantID}
	if baseUnitMustBeActive {
		unitFilter["isActive"] = true
	}
	err = r.db.Units().FindOne(ctx, unitFilter, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrItemBaseUnitInvalid
	}
	if err != nil {
		return fmt.Errorf("verify item base unit: %w", err)
	}
	return nil
}

func (r *ItemRepository) List(ctx context.Context, includeInactive bool) ([]models.Item, error) {
	filter := bson.M{"tenantId": r.tenantID}
	if !includeInactive {
		filter["isActive"] = true
	}
	cursor, err := r.db.Items().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "sku", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	items := []models.Item{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *ItemRepository) Create(ctx context.Context, item models.Item) (*models.Item, error) {
	item.ID = primitive.NewObjectID()
	item.TenantID = r.tenantID
	item.SKU = strings.ToLower(strings.TrimSpace(item.SKU))
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	item.Brand = strings.TrimSpace(item.Brand)
	item.Allergens = normalizeAllergens(item.Allergens)
	item.IsActive = true
	item.Version = 1
	item.CreatedAt = time.Now().UTC()
	item.UpdatedAt = item.CreatedAt
	if err := validation.Validate(&item); err != nil {
		return nil, err
	}
	if err := r.verifyReferences(ctx, item.CategoryID, item.BaseUnitID, true, true); err != nil {
		return nil, err
	}
	if _, err := r.db.Items().InsertOne(ctx, item); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDuplicateItemSKU
		}
		return nil, err
	}
	return &item, nil
}

func (r *ItemRepository) Update(ctx context.Context, id primitive.ObjectID, expectedVersion int64, update ItemUpdate) (*models.Item, error) {
	var current models.Item
	if err := r.db.Items().FindOne(ctx, bson.M{"_id": id, "tenantId": r.tenantID}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrItemNotFound
		}
		return nil, err
	}

	// Validate the complete post-update document before issuing the write. This
	// keeps invalid updates from being persisted when schema validation is not
	// enabled (and makes the Go tags and MongoDB schema enforce the same rules).
	candidate := current
	categoryChanged := update.CategoryID != nil && *update.CategoryID != current.CategoryID
	baseUnitChanged := update.BaseUnitID != nil && *update.BaseUnitID != current.BaseUnitID
	set := bson.M{"updatedAt": time.Now().UTC()}
	if update.Name != nil {
		candidate.Name = strings.TrimSpace(*update.Name)
		set["name"] = candidate.Name
	}
	if update.Description != nil {
		candidate.Description = strings.TrimSpace(*update.Description)
		set["description"] = candidate.Description
	}
	if update.Brand != nil {
		candidate.Brand = strings.TrimSpace(*update.Brand)
		set["brand"] = candidate.Brand
	}
	if update.CategoryID != nil {
		candidate.CategoryID = *update.CategoryID
		set["categoryId"] = candidate.CategoryID
	}
	if update.BaseUnitID != nil {
		candidate.BaseUnitID = *update.BaseUnitID
		set["baseUnitId"] = candidate.BaseUnitID
	}
	if update.Allergens != nil {
		candidate.Allergens = normalizeAllergens(*update.Allergens)
		set["allergens"] = candidate.Allergens
	}
	if update.ShelfLifeDays != nil {
		if *update.ShelfLifeDays == nil {
			candidate.ShelfLifeDays = nil
		} else {
			value := **update.ShelfLifeDays
			candidate.ShelfLifeDays = &value
			set["shelfLifeDays"] = value
		}
	}
	if update.Stockable != nil {
		candidate.Stockable = *update.Stockable
		set["stockable"] = candidate.Stockable
	}
	if update.IsActive != nil {
		candidate.IsActive = *update.IsActive
		set["isActive"] = candidate.IsActive
	}

	if err := validation.Validate(&candidate); err != nil {
		return nil, err
	}
	if baseUnitChanged {
		count, err := r.db.ItemConversions().CountDocuments(ctx, bson.M{"tenantId": r.tenantID, "itemId": id})
		if err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, ErrItemBaseUnitHasConversions
		}
	}
	if err := r.verifyReferences(ctx, candidate.CategoryID, candidate.BaseUnitID, categoryChanged, baseUnitChanged); err != nil {
		return nil, err
	}

	var item models.Item
	updateDoc := bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}}
	if update.ShelfLifeDays != nil && *update.ShelfLifeDays == nil {
		updateDoc["$unset"] = bson.M{"shelfLifeDays": ""}
	}
	err := r.db.Items().FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenantId": r.tenantID, "version": expectedVersion},
		updateDoc,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&item)
	if err == nil {
		return &item, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	count, countErr := r.db.Items().CountDocuments(ctx, bson.M{"_id": id, "tenantId": r.tenantID})
	if countErr != nil {
		return nil, countErr
	}
	if count == 0 {
		return nil, ErrItemNotFound
	}
	return nil, ErrItemVersionConflict
}

func (h *productHandler) listItems(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	items, err := NewItemRepository(h.db, tenant.ID).List(r.Context(), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		apierror.Internal(w, r, "Failed to list items")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *productHandler) createItem(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	var request struct {
		SKU           string             `json:"sku" validate:"required,max=64,location_code"`
		Name          string             `json:"name" validate:"required,min=1,max=160"`
		Description   string             `json:"description" validate:"omitempty,max=2000"`
		Brand         string             `json:"brand" validate:"omitempty,max=100"`
		CategoryID    primitive.ObjectID `json:"categoryId" validate:"required"`
		BaseUnitID    primitive.ObjectID `json:"baseUnitId" validate:"required"`
		Allergens     []string           `json:"allergens" validate:"omitempty,dive,eu_allergen"`
		ShelfLifeDays *int32             `json:"shelfLifeDays" validate:"omitempty,gte=0,lte=36500"`
		Stockable     *bool              `json:"stockable" validate:"required"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	request.SKU = strings.ToLower(strings.TrimSpace(request.SKU))
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	request.Brand = strings.TrimSpace(request.Brand)
	request.Allergens = normalizeAllergens(request.Allergens)
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	item, err := NewItemRepository(h.db, tenant.ID).Create(r.Context(), models.Item{
		SKU: request.SKU, Name: request.Name, Description: request.Description, Brand: request.Brand,
		CategoryID: request.CategoryID, BaseUnitID: request.BaseUnitID, Allergens: request.Allergens,
		ShelfLifeDays: request.ShelfLifeDays, Stockable: *request.Stockable,
	})
	switch {
	case err == nil:
		h.auditItem(r, item, "item.created", "Item created")
		writeJSON(w, http.StatusCreated, map[string]any{"item": item})
	case errors.Is(err, ErrDuplicateItemSKU):
		apierror.Conflict(w, r, "Item SKU already exists")
	case errors.Is(err, ErrItemCategoryInvalid), errors.Is(err, ErrItemBaseUnitInvalid):
		apierror.Validation(w, r, err.Error())
	case strings.HasPrefix(err.Error(), "validation failed:"):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Failed to create item")
	}
}

func (h *productHandler) updateItem(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["itemId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid item ID")
		return
	}
	var request struct {
		Version       int64               `json:"version" validate:"gte=1"`
		Name          *string             `json:"name" validate:"omitempty,min=1,max=160"`
		Description   *string             `json:"description" validate:"omitempty,max=2000"`
		Brand         *string             `json:"brand" validate:"omitempty,max=100"`
		CategoryID    *primitive.ObjectID `json:"categoryId"`
		BaseUnitID    *primitive.ObjectID `json:"baseUnitId"`
		Allergens     *[]string           `json:"allergens" validate:"omitempty,dive,eu_allergen"`
		ShelfLifeDays json.RawMessage     `json:"shelfLifeDays"`
		Stockable     *bool               `json:"stockable"`
		IsActive      *bool               `json:"isActive"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.Name == nil && request.Description == nil && request.Brand == nil && request.CategoryID == nil && request.BaseUnitID == nil && request.Allergens == nil && request.ShelfLifeDays == nil && request.Stockable == nil && request.IsActive == nil {
		apierror.BadRequest(w, r, "At least one mutable field is required")
		return
	}
	if request.Name != nil {
		trimmed := strings.TrimSpace(*request.Name)
		request.Name = &trimmed
	}
	if request.Description != nil {
		trimmed := strings.TrimSpace(*request.Description)
		request.Description = &trimmed
	}
	if request.Brand != nil {
		trimmed := strings.TrimSpace(*request.Brand)
		request.Brand = &trimmed
	}
	if request.Allergens != nil {
		allergens := normalizeAllergens(*request.Allergens)
		request.Allergens = &allergens
	}
	var shelfLifeDays **int32
	if request.ShelfLifeDays != nil {
		if string(request.ShelfLifeDays) == "null" {
			nilDays := (*int32)(nil)
			shelfLifeDays = &nilDays
		} else {
			var days int32
			if err := json.Unmarshal(request.ShelfLifeDays, &days); err != nil {
				apierror.Validation(w, r, "shelfLifeDays must be a whole number or null")
				return
			}
			if err := validation.Validate(&struct {
				ShelfLifeDays *int32 `validate:"gte=0,lte=36500"`
			}{ShelfLifeDays: &days}); err != nil {
				apierror.Validation(w, r, err.Error())
				return
			}
			daysPtr := &days
			shelfLifeDays = &daysPtr
		}
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	item, err := NewItemRepository(h.db, tenant.ID).Update(r.Context(), id, request.Version, ItemUpdate{
		Name: request.Name, Description: request.Description, Brand: request.Brand, CategoryID: request.CategoryID,
		BaseUnitID: request.BaseUnitID, Allergens: request.Allergens, ShelfLifeDays: shelfLifeDays,
		Stockable: request.Stockable, IsActive: request.IsActive,
	})
	switch {
	case err == nil:
		action, message := "item.updated", "Item updated"
		if request.IsActive != nil {
			if *request.IsActive {
				action, message = "item.reactivated", "Item reactivated"
			} else {
				action, message = "item.deactivated", "Item deactivated"
			}
		}
		h.auditItem(r, item, action, message)
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
	case errors.Is(err, ErrItemNotFound):
		apierror.NotFound(w, r, "Item not found")
	case errors.Is(err, ErrItemVersionConflict):
		apierror.VersionConflict(w, r, "Item was modified by another request")
	case errors.Is(err, ErrItemBaseUnitHasConversions):
		apierror.Conflict(w, r, "Item base unit cannot change while conversions exist")
	case errors.Is(err, ErrItemCategoryInvalid), errors.Is(err, ErrItemBaseUnitInvalid):
		apierror.Validation(w, r, err.Error())
	case strings.HasPrefix(err.Error(), "validation failed:"):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Failed to update item")
	}
}

func (h *productHandler) auditItem(r *http.Request, item *models.Item, action, message string) {
	user, userOK := middleware.GetUserFromContext(r.Context())
	tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
	if !userOK || !tenantOK || h.logger == nil {
		return
	}
	h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{
		"itemId": item.ID.Hex(), "sku": item.SKU, "version": item.Version,
	})
}
