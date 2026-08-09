// Package product implements tenant-scoped product features.
package product

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	locationCodeIndex = "locations_tenant_code_unique"
	locationSlotIndex = "locations_tenant_limit_slot_unique"
)

var (
	ErrDuplicateLocationCode   = errors.New("location code already exists")
	ErrLocationLimitReached    = errors.New("maximum number of locations reached")
	ErrLocationNotFound        = errors.New("location not found")
	ErrLocationVersionConflict = errors.New("location version conflict")
)

type LocationUpdate struct {
	Name     *string `validate:"omitempty,min=1,max=200"`
	Timezone *string `validate:"omitempty,max=100,iana_timezone"`
	IsActive *bool
}

// LocationRepository binds a tenant once so callers cannot issue unscoped
// location queries by accident.
type LocationRepository struct {
	db     *db.MongoDB
	tenant models.Tenant
}

func NewLocationRepository(database *db.MongoDB, tenant *models.Tenant) *LocationRepository {
	return &LocationRepository{db: database, tenant: *tenant}
}

func (r *LocationRepository) List(ctx context.Context) ([]models.Location, error) {
	return r.ListAssigned(ctx, nil)
}

func (r *LocationRepository) ListAssigned(ctx context.Context, locationIDs []primitive.ObjectID) ([]models.Location, error) {
	filter := bson.M{"tenantId": r.tenant.ID}
	if locationIDs != nil {
		filter["_id"] = bson.M{"$in": locationIDs}
	}
	cursor, err := r.db.Locations().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	defer cursor.Close(ctx)

	locations := make([]models.Location, 0)
	if err := cursor.All(ctx, &locations); err != nil {
		return nil, fmt.Errorf("decode locations: %w", err)
	}
	return locations, nil
}

func (r *LocationRepository) Create(ctx context.Context, code, name, timezone string) (*models.Location, error) {
	now := time.Now().UTC()
	location := models.Location{
		TenantID: r.tenant.ID,
		Code:     code, Name: name, Timezone: timezone,
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now, LimitSlot: 1,
	}
	if err := validation.Validate(&location); err != nil {
		return nil, err
	}

	limit, unlimited, err := r.locationLimit(ctx)
	if err != nil {
		return nil, err
	}

	if unlimited {
		return r.createUnlimited(ctx, &location)
	}
	for slot := int64(1); slot <= limit; slot++ {
		location.LimitSlot = slot
		location.ID = primitive.NewObjectID()
		_, err := r.db.Locations().InsertOne(ctx, location)
		if err == nil {
			return &location, nil
		}
		if !mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("create location: %w", err)
		}
		if duplicateIndex(err, locationCodeIndex) {
			return nil, ErrDuplicateLocationCode
		}
		if !duplicateIndex(err, locationSlotIndex) && r.codeExists(ctx, code) {
			return nil, ErrDuplicateLocationCode
		}
	}
	if r.codeExists(ctx, code) {
		return nil, ErrDuplicateLocationCode
	}
	return nil, ErrLocationLimitReached
}

func (r *LocationRepository) Update(ctx context.Context, id primitive.ObjectID, expectedVersion int64, changes LocationUpdate) (*models.Location, error) {
	if changes.Name != nil {
		trimmed := strings.TrimSpace(*changes.Name)
		changes.Name = &trimmed
	}
	if err := validation.Validate(&changes); err != nil {
		return nil, err
	}

	set := bson.M{"updatedAt": time.Now().UTC()}
	if changes.Name != nil {
		set["name"] = *changes.Name
	}
	if changes.Timezone != nil {
		set["timezone"] = *changes.Timezone
	}
	if changes.IsActive != nil {
		set["isActive"] = *changes.IsActive
	}

	var location models.Location
	err := r.db.Locations().FindOneAndUpdate(
		ctx,
		bson.M{"tenantId": r.tenant.ID, "_id": id, "version": expectedVersion},
		bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&location)
	if err == nil {
		return &location, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, fmt.Errorf("update location: %w", err)
	}

	err = r.db.Locations().FindOne(ctx, bson.M{"tenantId": r.tenant.ID, "_id": id}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrLocationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("check location version: %w", err)
	}
	return nil, ErrLocationVersionConflict
}

func (r *LocationRepository) createUnlimited(ctx context.Context, location *models.Location) (*models.Location, error) {
	for {
		var latest models.Location
		err := r.db.Locations().FindOne(ctx, bson.M{"tenantId": r.tenant.ID}, options.FindOne().SetSort(bson.D{{Key: "limitSlot", Value: -1}})).Decode(&latest)
		switch {
		case errors.Is(err, mongo.ErrNoDocuments):
			location.LimitSlot = 1
		case err != nil:
			return nil, fmt.Errorf("allocate location slot: %w", err)
		default:
			location.LimitSlot = latest.LimitSlot + 1
		}
		location.ID = primitive.NewObjectID()
		_, err = r.db.Locations().InsertOne(ctx, location)
		if err == nil {
			return location, nil
		}
		if !mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("create location: %w", err)
		}
		if duplicateIndex(err, locationCodeIndex) || r.codeExists(ctx, location.Code) {
			return nil, ErrDuplicateLocationCode
		}
		// Another request won the next slot. Re-read the maximum and retry.
	}
}

func (r *LocationRepository) locationLimit(ctx context.Context) (limit int64, unlimited bool, err error) {
	if r.tenant.IsRoot || r.tenant.BillingWaived {
		return 0, true, nil
	}
	if r.tenant.PlanID == nil {
		return 1, false, nil
	}

	var plan models.Plan
	err = r.db.Plans().FindOne(ctx, bson.M{"_id": *r.tenant.PlanID}).Decode(&plan)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 1, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("load location entitlement: %w", err)
	}
	entitlement, ok := plan.Entitlements["max_locations"]
	if !ok || entitlement.Type != models.EntitlementTypeNumeric {
		return 1, false, nil
	}
	return entitlement.NumericValue, false, nil
}

func (r *LocationRepository) codeExists(ctx context.Context, code string) bool {
	err := r.db.Locations().FindOne(ctx, bson.M{"tenantId": r.tenant.ID, "code": code}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	return err == nil
}

func duplicateIndex(err error, name string) bool {
	return strings.Contains(err.Error(), name)
}
