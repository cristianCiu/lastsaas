package product

import (
	"context"
	"errors"
	"net/http"
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
	ErrItemConversionNotFound        = errors.New("item conversion not found")
	ErrDuplicateItemConversion       = errors.New("item conversion already exists")
	ErrItemConversionVersionConflict = errors.New("item conversion version conflict")
	ErrItemConversionItemInvalid     = errors.New("item must be active and belong to the tenant")
	ErrItemConversionSourceInvalid   = errors.New("source unit must be active, same-tenant, and match item dimension")
	ErrItemConversionSelfSource      = errors.New("source unit cannot equal item base unit")
	ErrItemConversionDimension       = errors.New("source unit dimension must match item base unit dimension")
)

type ItemConversionRepository struct {
	db       *db.MongoDB
	tenantID primitive.ObjectID
	itemID   primitive.ObjectID
}

type ItemConversionUpdate struct {
	Numerator   *int64
	Denominator *int64
	IsActive    *bool
}

func NewItemConversionRepository(database *db.MongoDB, tenantID, itemID primitive.ObjectID) *ItemConversionRepository {
	return &ItemConversionRepository{db: database, tenantID: tenantID, itemID: itemID}
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func normalizeConversion(numerator, denominator int64) (int64, int64, error) {
	if numerator <= 0 || denominator <= 0 || numerator > 1_000_000_000 || denominator > 1_000_000_000 {
		return 0, 0, errors.New("numerator and denominator must be positive and at most 1000000000")
	}
	divisor := gcd(numerator, denominator)
	return numerator / divisor, denominator / divisor, nil
}

func (r *ItemConversionRepository) verifyReferences(ctx context.Context, fromUnitID primitive.ObjectID, requireActive bool) (*models.Item, error) {
	var item models.Item
	itemFilter := bson.M{"_id": r.itemID, "tenantId": r.tenantID}
	if requireActive {
		itemFilter["isActive"] = true
	}
	if err := r.db.Items().FindOne(ctx, itemFilter).Decode(&item); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrItemConversionItemInvalid
		}
		return nil, err
	}
	var baseUnit models.Unit
	baseFilter := bson.M{"_id": item.BaseUnitID, "tenantId": r.tenantID}
	if requireActive {
		baseFilter["isActive"] = true
	}
	if err := r.db.Units().FindOne(ctx, baseFilter, options.FindOne().SetProjection(bson.M{"dimension": 1})).Decode(&baseUnit); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrItemConversionItemInvalid
		}
		return nil, err
	}
	if fromUnitID == item.BaseUnitID {
		return nil, ErrItemConversionSelfSource
	}
	unitFilter := bson.M{"_id": fromUnitID, "tenantId": r.tenantID}
	if requireActive {
		unitFilter["isActive"] = true
	}
	var sourceUnit models.Unit
	if err := r.db.Units().FindOne(ctx, unitFilter, options.FindOne().SetProjection(bson.M{"dimension": 1})).Decode(&sourceUnit); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrItemConversionSourceInvalid
		}
		return nil, err
	}
	if sourceUnit.Dimension != baseUnit.Dimension {
		return nil, ErrItemConversionDimension
	}
	return &item, nil
}

func (r *ItemConversionRepository) List(ctx context.Context) ([]models.ItemConversion, error) {
	if err := r.db.Items().FindOne(ctx, bson.M{"_id": r.itemID, "tenantId": r.tenantID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrItemConversionItemInvalid
		}
		return nil, err
	}
	cursor, err := r.db.ItemConversions().Find(ctx, bson.M{"tenantId": r.tenantID, "itemId": r.itemID}, options.Find().SetSort(bson.D{{Key: "fromUnitId", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	conversions := []models.ItemConversion{}
	if err := cursor.All(ctx, &conversions); err != nil {
		return nil, err
	}
	return conversions, nil
}

func (r *ItemConversionRepository) Create(ctx context.Context, fromUnitID primitive.ObjectID, numerator, denominator int64) (*models.ItemConversion, error) {
	item, err := r.verifyReferences(ctx, fromUnitID, true)
	if err != nil {
		return nil, err
	}
	numerator, denominator, err = normalizeConversion(numerator, denominator)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	conversion := models.ItemConversion{ID: primitive.NewObjectID(), TenantID: r.tenantID, ItemID: item.ID, FromUnitID: fromUnitID, Numerator: numerator, Denominator: denominator, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&conversion); err != nil {
		return nil, err
	}
	if _, err := r.db.ItemConversions().InsertOne(ctx, conversion); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDuplicateItemConversion
		}
		return nil, err
	}
	return &conversion, nil
}

func (r *ItemConversionRepository) Update(ctx context.Context, id primitive.ObjectID, version int64, update ItemConversionUpdate) (*models.ItemConversion, error) {
	var current models.ItemConversion
	if err := r.db.ItemConversions().FindOne(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "itemId": r.itemID}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrItemConversionNotFound
		}
		return nil, err
	}
	if update.Numerator != nil || update.Denominator != nil {
		numerator, denominator := current.Numerator, current.Denominator
		if update.Numerator != nil {
			numerator = *update.Numerator
		}
		if update.Denominator != nil {
			denominator = *update.Denominator
		}
		numerator, denominator, err := normalizeConversion(numerator, denominator)
		if err != nil {
			return nil, err
		}
		update.Numerator, update.Denominator = &numerator, &denominator
	}
	set := bson.M{"updatedAt": time.Now().UTC()}
	if update.Numerator != nil {
		set["numerator"] = *update.Numerator
	}
	if update.Denominator != nil {
		set["denominator"] = *update.Denominator
	}
	if update.IsActive != nil {
		set["isActive"] = *update.IsActive
	}
	var conversion models.ItemConversion
	err := r.db.ItemConversions().FindOneAndUpdate(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "itemId": r.itemID, "version": version}, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&conversion)
	if err == nil {
		return &conversion, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	count, countErr := r.db.ItemConversions().CountDocuments(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "itemId": r.itemID})
	if countErr != nil {
		return nil, countErr
	}
	if count == 0 {
		return nil, ErrItemConversionNotFound
	}
	return nil, ErrItemConversionVersionConflict
}

func conversionItemScope(w http.ResponseWriter, r *http.Request) (*models.Tenant, primitive.ObjectID, bool) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return nil, primitive.NilObjectID, false
	}
	itemID, err := primitive.ObjectIDFromHex(mux.Vars(r)["itemId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid item ID")
		return nil, primitive.NilObjectID, false
	}
	return tenant, itemID, true
}

func (h *productHandler) listItemConversions(w http.ResponseWriter, r *http.Request) {
	tenant, itemID, ok := conversionItemScope(w, r)
	if !ok {
		return
	}
	conversions, err := NewItemConversionRepository(h.db, tenant.ID, itemID).List(r.Context())
	if err != nil {
		if errors.Is(err, ErrItemConversionItemInvalid) {
			apierror.NotFound(w, r, "Item not found")
		} else {
			apierror.Internal(w, r, "Failed to list item conversions")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversions": conversions})
}

func (h *productHandler) createItemConversion(w http.ResponseWriter, r *http.Request) {
	tenant, itemID, ok := conversionItemScope(w, r)
	if !ok {
		return
	}
	var request struct {
		FromUnitID  primitive.ObjectID `json:"fromUnitId" validate:"required"`
		Numerator   int64              `json:"numerator" validate:"gt=0,lte=1000000000"`
		Denominator int64              `json:"denominator" validate:"gt=0,lte=1000000000"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	conversion, err := NewItemConversionRepository(h.db, tenant.ID, itemID).Create(r.Context(), request.FromUnitID, request.Numerator, request.Denominator)
	switch {
	case err == nil:
		h.auditItemConversion(r, conversion, "item_conversion.created", "Item conversion created")
		writeJSON(w, http.StatusCreated, map[string]any{"conversion": conversion})
	case errors.Is(err, ErrDuplicateItemConversion):
		apierror.Conflict(w, r, "Item conversion already exists")
	case errors.Is(err, ErrItemConversionItemInvalid):
		apierror.NotFound(w, r, "Item not found")
	case errors.Is(err, ErrItemConversionSelfSource), errors.Is(err, ErrItemConversionSourceInvalid), errors.Is(err, ErrItemConversionDimension):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Validation(w, r, err.Error())
	}
}

func (h *productHandler) updateItemConversion(w http.ResponseWriter, r *http.Request) {
	tenant, itemID, ok := conversionItemScope(w, r)
	if !ok {
		return
	}
	conversionID, err := primitive.ObjectIDFromHex(mux.Vars(r)["conversionId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid conversion ID")
		return
	}
	var request struct {
		Version     int64  `json:"version" validate:"gte=1"`
		Numerator   *int64 `json:"numerator" validate:"omitempty,gt=0,lte=1000000000"`
		Denominator *int64 `json:"denominator" validate:"omitempty,gt=0,lte=1000000000"`
		IsActive    *bool  `json:"isActive"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.Numerator == nil && request.Denominator == nil && request.IsActive == nil {
		apierror.BadRequest(w, r, "At least one mutable field is required")
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	conversion, err := NewItemConversionRepository(h.db, tenant.ID, itemID).Update(r.Context(), conversionID, request.Version, ItemConversionUpdate{Numerator: request.Numerator, Denominator: request.Denominator, IsActive: request.IsActive})
	switch {
	case err == nil:
		action, message := "item_conversion.updated", "Item conversion updated"
		if request.IsActive != nil {
			if *request.IsActive {
				action, message = "item_conversion.reactivated", "Item conversion reactivated"
			} else {
				action, message = "item_conversion.deactivated", "Item conversion deactivated"
			}
		}
		h.auditItemConversion(r, conversion, action, message)
		writeJSON(w, http.StatusOK, map[string]any{"conversion": conversion})
	case errors.Is(err, ErrItemConversionNotFound):
		apierror.NotFound(w, r, "Item conversion not found")
	case errors.Is(err, ErrItemConversionVersionConflict):
		apierror.VersionConflict(w, r, "Item conversion was modified by another request")
	default:
		apierror.Validation(w, r, err.Error())
	}
}

func (h *productHandler) auditItemConversion(r *http.Request, conversion *models.ItemConversion, action, message string) {
	user, userOK := middleware.GetUserFromContext(r.Context())
	tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
	if !userOK || !tenantOK || h.logger == nil {
		return
	}
	h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{"conversionId": conversion.ID.Hex(), "itemId": conversion.ItemID.Hex(), "fromUnitId": conversion.FromUnitID.Hex(), "version": conversion.Version})
}
