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
	ErrRecipeNotFound              = errors.New("recipe not found")
	ErrRecipeVersionNotFound       = errors.New("recipe version not found")
	ErrMappingNotFound             = errors.New("external product mapping not found")
	ErrDuplicateRecipeCode         = errors.New("recipe code already exists")
	ErrRecipeVersionConflict       = errors.New("recipe version conflict")
	ErrRecipeConflict              = errors.New("recipe version effective range overlaps")
	ErrMappingOverlap              = errors.New("external product mapping effective range overlaps")
	ErrReleasedVersionImmutable    = errors.New("released recipe versions are immutable")
	ErrRecipeVersionMustBeReleased = errors.New("mapping requires a released recipe version")
)

type RecipeRepository struct {
	db       *db.MongoDB
	tenantID primitive.ObjectID
}

func NewRecipeRepository(database *db.MongoDB, tenantID primitive.ObjectID) *RecipeRepository {
	return &RecipeRepository{db: database, tenantID: tenantID}
}

func (r *RecipeRepository) List(ctx context.Context, includeInactive bool) ([]models.Recipe, error) {
	filter := bson.M{"tenantId": r.tenantID}
	if !includeInactive {
		filter["isActive"] = true
	}
	cur, err := r.db.Recipes().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "code", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := []models.Recipe{}
	return result, cur.All(ctx, &result)
}

func (r *RecipeRepository) Create(ctx context.Context, recipe models.Recipe) (*models.Recipe, error) {
	now := time.Now().UTC()
	recipe.ID, recipe.TenantID, recipe.Code = primitive.NewObjectID(), r.tenantID, strings.ToLower(strings.TrimSpace(recipe.Code))
	recipe.Name, recipe.Description = strings.TrimSpace(recipe.Name), strings.TrimSpace(recipe.Description)
	recipe.IsActive, recipe.Version, recipe.CreatedAt, recipe.UpdatedAt = true, 1, now, now
	if err := validation.Validate(&recipe); err != nil {
		return nil, err
	}
	if _, err := r.db.Recipes().InsertOne(ctx, recipe); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDuplicateRecipeCode
		}
		return nil, err
	}
	return &recipe, nil
}

func (r *RecipeRepository) Update(ctx context.Context, id primitive.ObjectID, expectedVersion int64, update RecipeUpdate) (*models.Recipe, error) {
	var current models.Recipe
	if err := r.db.Recipes().FindOne(ctx, bson.M{"_id": id, "tenantId": r.tenantID}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrRecipeNotFound
		}
		return nil, err
	}
	candidate := current
	set := bson.M{"updatedAt": time.Now().UTC()}
	if update.Name != nil {
		candidate.Name = strings.TrimSpace(*update.Name)
		set["name"] = candidate.Name
	}
	if update.Description != nil {
		candidate.Description = strings.TrimSpace(*update.Description)
		set["description"] = candidate.Description
	}
	if update.IsActive != nil {
		candidate.IsActive = *update.IsActive
		set["isActive"] = *update.IsActive
	}
	if err := validation.Validate(&candidate); err != nil {
		return nil, err
	}
	var result models.Recipe
	err := r.db.Recipes().FindOneAndUpdate(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "version": expectedVersion}, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&result)
	if err == nil {
		return &result, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	count, countErr := r.db.Recipes().CountDocuments(ctx, bson.M{"_id": id, "tenantId": r.tenantID})
	if countErr != nil {
		return nil, countErr
	}
	if count == 0 {
		return nil, ErrRecipeNotFound
	}
	return nil, ErrRecipeVersionConflict
}

type RecipeUpdate struct {
	Name        *string
	Description *string
	IsActive    *bool
}

func (r *RecipeRepository) verifyVersionRefs(ctx context.Context, version models.RecipeVersion, components []models.RecipeComponent) error {
	if err := r.db.Recipes().FindOne(ctx, bson.M{"_id": version.RecipeID, "tenantId": r.tenantID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrRecipeNotFound
		}
		return err
	}
	if err := r.db.Units().FindOne(ctx, bson.M{"_id": version.OutputUnitID, "tenantId": r.tenantID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		return fmt.Errorf("output unit must belong to tenant and be active")
	}
	for _, component := range components {
		if component.ComponentType == models.RecipeComponentItem {
			if component.ItemID == nil {
				return fmt.Errorf("item component reference is required")
			}
			if err := r.db.Items().FindOne(ctx, bson.M{"_id": *component.ItemID, "tenantId": r.tenantID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
				return fmt.Errorf("item component must belong to tenant and be active")
			}
		} else {
			if component.SubrecipeID == nil || *component.SubrecipeID == version.RecipeID {
				return fmt.Errorf("subrecipe component must reference another tenant recipe")
			}
			if err := r.db.Recipes().FindOne(ctx, bson.M{"_id": *component.SubrecipeID, "tenantId": r.tenantID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
				return fmt.Errorf("subrecipe component must belong to tenant and be active")
			}
		}
		if err := r.db.Units().FindOne(ctx, bson.M{"_id": component.UnitID, "tenantId": r.tenantID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
			return fmt.Errorf("component unit must belong to tenant and be active")
		}
	}
	return nil
}

func (r *RecipeRepository) CreateVersion(ctx context.Context, version models.RecipeVersion, components []models.RecipeComponent) (*models.RecipeVersion, []models.RecipeComponent, error) {
	version.ID, version.TenantID, version.Status = primitive.NewObjectID(), r.tenantID, models.RecipeVersionDraft
	version.Version = 1
	if version.Number < 1 {
		var latest models.RecipeVersion
		err := r.db.RecipeVersions().FindOne(ctx, bson.M{"tenantId": r.tenantID, "recipeId": version.RecipeID}, options.FindOne().SetSort(bson.D{{Key: "number", Value: -1}})).Decode(&latest)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil, err
		}
		version.Number = latest.Number + 1
	}
	now := time.Now().UTC()
	version.CreatedAt, version.UpdatedAt = now, now
	if err := validation.Validate(&version); err != nil {
		return nil, nil, err
	}
	if err := r.verifyVersionRefs(ctx, version, components); err != nil {
		return nil, nil, err
	}
	if _, err := r.db.RecipeVersions().InsertOne(ctx, version); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, nil, ErrRecipeVersionConflict
		}
		return nil, nil, err
	}
	stored := make([]models.RecipeComponent, 0, len(components))
	for i, component := range components {
		component.ID, component.TenantID, component.RecipeVersionID = primitive.NewObjectID(), r.tenantID, version.ID
		if component.SortOrder == 0 && i > 0 {
			component.SortOrder = int32(i)
		}
		component.CreatedAt, component.UpdatedAt = now, now
		if err := validation.Validate(&component); err != nil {
			_ = r.deleteVersion(ctx, version.ID)
			return nil, nil, err
		}
		if _, err := r.db.RecipeComponents().InsertOne(ctx, component); err != nil {
			_ = r.deleteVersion(ctx, version.ID)
			return nil, nil, err
		}
		stored = append(stored, component)
	}
	return &version, stored, nil
}

func (r *RecipeRepository) deleteVersion(ctx context.Context, id primitive.ObjectID) error {
	_, _ = r.db.RecipeComponents().DeleteMany(ctx, bson.M{"tenantId": r.tenantID, "recipeVersionId": id})
	_, err := r.db.RecipeVersions().DeleteOne(ctx, bson.M{"tenantId": r.tenantID, "_id": id})
	return err
}

func (r *RecipeRepository) ListVersions(ctx context.Context, recipeID primitive.ObjectID) ([]models.RecipeVersion, error) {
	cur, err := r.db.RecipeVersions().Find(ctx, bson.M{"tenantId": r.tenantID, "recipeId": recipeID}, options.Find().SetSort(bson.D{{Key: "number", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := []models.RecipeVersion{}
	return result, cur.All(ctx, &result)
}

func (r *RecipeRepository) ListComponents(ctx context.Context, versionID primitive.ObjectID) ([]models.RecipeComponent, error) {
	cur, err := r.db.RecipeComponents().Find(ctx, bson.M{"tenantId": r.tenantID, "recipeVersionId": versionID}, options.Find().SetSort(bson.D{{Key: "sortOrder", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := []models.RecipeComponent{}
	return result, cur.All(ctx, &result)
}

func (r *RecipeRepository) ReleaseVersion(ctx context.Context, id primitive.ObjectID, expectedVersion int64) (*models.RecipeVersion, error) {
	var current models.RecipeVersion
	if err := r.db.RecipeVersions().FindOne(ctx, bson.M{"_id": id, "tenantId": r.tenantID}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrRecipeVersionNotFound
		}
		return nil, err
	}
	if current.Status != models.RecipeVersionDraft {
		return nil, ErrReleasedVersionImmutable
	}
	count, err := r.db.RecipeComponents().CountDocuments(ctx, bson.M{"tenantId": r.tenantID, "recipeVersionId": id})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("a recipe version must contain at least one component")
	}
	if overlap, err := r.versionRangeOverlaps(ctx, current); err != nil {
		return nil, err
	} else if overlap {
		return nil, ErrRecipeConflict
	}
	now := time.Now().UTC()
	var result models.RecipeVersion
	err = r.db.RecipeVersions().FindOneAndUpdate(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "status": models.RecipeVersionDraft, "version": expectedVersion}, bson.M{"$set": bson.M{"status": models.RecipeVersionReleased, "releasedAt": now, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&result)
	if err == nil {
		return &result, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	count, err = r.db.RecipeVersions().CountDocuments(ctx, bson.M{"_id": id, "tenantId": r.tenantID})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrRecipeVersionNotFound
	}
	return nil, ErrRecipeVersionConflict
}

func (r *RecipeRepository) versionRangeOverlaps(ctx context.Context, candidate models.RecipeVersion) (bool, error) {
	filter := bson.M{"tenantId": r.tenantID, "recipeId": candidate.RecipeID, "status": models.RecipeVersionReleased, "_id": bson.M{"$ne": candidate.ID}}
	if candidate.EffectiveTo != nil {
		filter["effectiveFrom"] = bson.M{"$lt": candidate.EffectiveTo}
	}
	filter["$or"] = bson.A{bson.M{"effectiveTo": bson.M{"$exists": false}}, bson.M{"effectiveTo": nil}, bson.M{"effectiveTo": bson.M{"$gt": candidate.EffectiveFrom}}}
	count, err := r.db.RecipeVersions().CountDocuments(ctx, filter)
	return count > 0, err
}

type MappingRepository struct {
	db       *db.MongoDB
	tenantID primitive.ObjectID
}

func NewMappingRepository(database *db.MongoDB, tenantID primitive.ObjectID) *MappingRepository {
	return &MappingRepository{db: database, tenantID: tenantID}
}

func (r *MappingRepository) List(ctx context.Context, adapter string, at *time.Time) ([]models.ExternalProductMapping, error) {
	filter := bson.M{"tenantId": r.tenantID, "isActive": true}
	if adapter != "" {
		filter["adapter"] = adapter
	}
	if at != nil {
		filter["effectiveFrom"] = bson.M{"$lte": *at}
		filter["$or"] = bson.A{
			bson.M{"effectiveTo": bson.M{"$exists": false}},
			bson.M{"effectiveTo": nil},
			bson.M{"effectiveTo": bson.M{"$gt": *at}},
		}
	}
	cur, err := r.db.ExternalProductMappings().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "adapter", Value: 1}, {Key: "externalProductId", Value: 1}, {Key: "effectiveFrom", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := []models.ExternalProductMapping{}
	return result, cur.All(ctx, &result)
}

func (r *MappingRepository) validateMappingRefs(ctx context.Context, mapping models.ExternalProductMapping) error {
	var version models.RecipeVersion
	if err := r.db.RecipeVersions().FindOne(ctx, bson.M{"_id": mapping.RecipeVersionID, "tenantId": r.tenantID}).Decode(&version); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrRecipeVersionNotFound
		}
		return err
	}
	if version.Status != models.RecipeVersionReleased {
		return ErrRecipeVersionMustBeReleased
	}
	return nil
}

func (r *MappingRepository) mappingOverlaps(ctx context.Context, mapping models.ExternalProductMapping, exclude primitive.ObjectID) (bool, error) {
	filter := bson.M{"tenantId": r.tenantID, "adapter": mapping.Adapter, "externalProductId": mapping.ExternalProductID, "_id": bson.M{"$ne": exclude}}
	if mapping.EffectiveTo != nil {
		filter["effectiveFrom"] = bson.M{"$lt": *mapping.EffectiveTo}
	}
	filter["$or"] = bson.A{bson.M{"effectiveTo": bson.M{"$exists": false}}, bson.M{"effectiveTo": nil}, bson.M{"effectiveTo": bson.M{"$gt": mapping.EffectiveFrom}}}
	count, err := r.db.ExternalProductMappings().CountDocuments(ctx, filter)
	return count > 0, err
}

func (r *MappingRepository) Create(ctx context.Context, mapping models.ExternalProductMapping) (*models.ExternalProductMapping, error) {
	mapping.ID, mapping.TenantID = primitive.NewObjectID(), r.tenantID
	mapping.Adapter, mapping.ExternalProductID = strings.ToLower(strings.TrimSpace(mapping.Adapter)), strings.TrimSpace(mapping.ExternalProductID)
	mapping.IsActive, mapping.Version = true, 1
	now := time.Now().UTC()
	mapping.CreatedAt, mapping.UpdatedAt = now, now
	if err := validation.Validate(&mapping); err != nil {
		return nil, err
	}
	if err := r.validateMappingRefs(ctx, mapping); err != nil {
		return nil, err
	}
	if overlap, err := r.mappingOverlaps(ctx, mapping, primitive.NilObjectID); err != nil {
		return nil, err
	} else if overlap {
		return nil, ErrMappingOverlap
	}
	if _, err := r.db.ExternalProductMappings().InsertOne(ctx, mapping); err != nil {
		return nil, err
	}
	return &mapping, nil
}

type MappingUpdate struct {
	RecipeVersionID *primitive.ObjectID
	EffectiveFrom   *time.Time
	EffectiveTo     **time.Time
	IsActive        *bool
}

func (r *MappingRepository) Update(ctx context.Context, id primitive.ObjectID, expectedVersion int64, update MappingUpdate) (*models.ExternalProductMapping, error) {
	var current models.ExternalProductMapping
	if err := r.db.ExternalProductMappings().FindOne(ctx, bson.M{"_id": id, "tenantId": r.tenantID}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrMappingNotFound
		}
		return nil, err
	}
	candidate := current
	set := bson.M{"updatedAt": time.Now().UTC()}
	if update.RecipeVersionID != nil {
		candidate.RecipeVersionID = *update.RecipeVersionID
		set["recipeVersionId"] = *update.RecipeVersionID
	}
	if update.EffectiveFrom != nil {
		candidate.EffectiveFrom = *update.EffectiveFrom
		set["effectiveFrom"] = *update.EffectiveFrom
	}
	if update.EffectiveTo != nil {
		candidate.EffectiveTo = *update.EffectiveTo
		if *update.EffectiveTo == nil {
			set["effectiveTo"] = nil
		} else {
			set["effectiveTo"] = **update.EffectiveTo
		}
	}
	if update.IsActive != nil {
		candidate.IsActive = *update.IsActive
		set["isActive"] = *update.IsActive
	}
	if err := validation.Validate(&candidate); err != nil {
		return nil, err
	}
	if update.RecipeVersionID != nil {
		if err := r.validateMappingRefs(ctx, candidate); err != nil {
			return nil, err
		}
	}
	if overlap, err := r.mappingOverlaps(ctx, candidate, id); err != nil {
		return nil, err
	} else if overlap {
		return nil, ErrMappingOverlap
	}
	var result models.ExternalProductMapping
	err := r.db.ExternalProductMappings().FindOneAndUpdate(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "version": expectedVersion}, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&result)
	if err == nil {
		return &result, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	count, err := r.db.ExternalProductMappings().CountDocuments(ctx, bson.M{"_id": id, "tenantId": r.tenantID})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrMappingNotFound
	}
	return nil, ErrRecipeVersionConflict
}

func (r *MappingRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	result, err := r.db.ExternalProductMappings().DeleteOne(ctx, bson.M{"_id": id, "tenantId": r.tenantID})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrMappingNotFound
	}
	return nil
}

type recipeCreateRequest struct {
	Code        string `json:"code" validate:"required,max=64,location_code"`
	Name        string `json:"name" validate:"required,not_blank,max=160"`
	Description string `json:"description" validate:"omitempty,max=2000"`
}
type recipeVersionCreateRequest struct {
	Number            int64                    `json:"number" validate:"omitempty,gte=1"`
	OutputUnitID      primitive.ObjectID       `json:"outputUnitId" validate:"required"`
	PortionCount      int64                    `json:"portionCount" validate:"gt=0"`
	YieldFactorMicros int64                    `json:"yieldFactorMicros,string" validate:"gt=0,lte=1000000"`
	LossFactorMicros  int64                    `json:"lossFactorMicros,string" validate:"gte=0,lte=1000000"`
	EffectiveFrom     time.Time                `json:"effectiveFrom" validate:"required"`
	EffectiveTo       *time.Time               `json:"effectiveTo"`
	Components        []recipeComponentRequest `json:"components"`
}
type recipeComponentRequest struct {
	ComponentType  models.RecipeComponentType `json:"componentType" validate:"required,recipe_component_type"`
	ItemID         *primitive.ObjectID        `json:"itemId"`
	SubrecipeID    *primitive.ObjectID        `json:"subrecipeId"`
	QuantityMicros int64                      `json:"quantityMicros,string" validate:"gt=0"`
	UnitID         primitive.ObjectID         `json:"unitId" validate:"required"`
	SortOrder      int32                      `json:"sortOrder" validate:"gte=0,lte=1000000"`
}

func (h *productHandler) listRecipes(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	result, err := NewRecipeRepository(h.db, tenant.ID).List(r.Context(), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		apierror.Internal(w, r, "Failed to list recipes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recipes": result})
}
func (h *productHandler) createRecipe(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	var req recipeCreateRequest
	if !decodeStrict(w, r, &req) {
		return
	}
	req.Code, req.Name, req.Description = strings.ToLower(strings.TrimSpace(req.Code)), strings.TrimSpace(req.Name), strings.TrimSpace(req.Description)
	if err := validation.Validate(&req); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	result, err := NewRecipeRepository(h.db, tenant.ID).Create(r.Context(), models.Recipe{Code: req.Code, Name: req.Name, Description: req.Description})
	if errors.Is(err, ErrDuplicateRecipeCode) {
		apierror.Conflict(w, r, "Recipe code already exists")
		return
	}
	if err != nil {
		apierror.ValidationOrInternal(w, r, err, "Failed to create recipe")
		return
	}
	h.auditRecipe(r, result, "recipe.created", "Recipe created")
	writeJSON(w, http.StatusCreated, map[string]any{"recipe": result})
}

func (h *productHandler) updateRecipe(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["recipeId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid recipe ID")
		return
	}
	var req struct {
		Version     int64   `json:"version" validate:"gte=1"`
		Name        *string `json:"name" validate:"omitempty,not_blank,max=160"`
		Description *string `json:"description" validate:"omitempty,max=2000"`
		IsActive    *bool   `json:"isActive"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	if req.Name == nil && req.Description == nil && req.IsActive == nil {
		apierror.BadRequest(w, r, "At least one mutable field is required")
		return
	}
	if err := validation.Validate(&req); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	result, err := NewRecipeRepository(h.db, tenant.ID).Update(r.Context(), id, req.Version, RecipeUpdate{Name: req.Name, Description: req.Description, IsActive: req.IsActive})
	switch {
	case err == nil:
		h.auditRecipe(r, result, "recipe.updated", "Recipe updated")
		writeJSON(w, http.StatusOK, map[string]any{"recipe": result})
	case errors.Is(err, ErrRecipeNotFound):
		apierror.NotFound(w, r, "Recipe not found")
	case errors.Is(err, ErrRecipeVersionConflict):
		apierror.VersionConflict(w, r, "Recipe was modified by another request")
	default:
		apierror.ValidationOrInternal(w, r, err, "Failed to update recipe")
	}
}

func (h *productHandler) createRecipeVersion(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	recipeID, err := primitive.ObjectIDFromHex(mux.Vars(r)["recipeId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid recipe ID")
		return
	}
	var req recipeVersionCreateRequest
	if !decodeStrict(w, r, &req) {
		return
	}
	if err := validation.Validate(&req); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	components := make([]models.RecipeComponent, len(req.Components))
	for i, c := range req.Components {
		components[i] = models.RecipeComponent{ComponentType: c.ComponentType, ItemID: c.ItemID, SubrecipeID: c.SubrecipeID, QuantityMicros: c.QuantityMicros, UnitID: c.UnitID, SortOrder: c.SortOrder}
	}
	result, stored, err := NewRecipeRepository(h.db, tenant.ID).CreateVersion(r.Context(), models.RecipeVersion{RecipeID: recipeID, Number: req.Number, OutputUnitID: req.OutputUnitID, PortionCount: req.PortionCount, YieldFactorMicros: req.YieldFactorMicros, LossFactorMicros: req.LossFactorMicros, EffectiveFrom: req.EffectiveFrom, EffectiveTo: req.EffectiveTo}, components)
	if err != nil {
		apierror.ValidationOrInternal(w, r, err, "Failed to create recipe version")
		return
	}
	h.auditRecipeVersion(r, result, "recipe_version.created", "Recipe version created")
	writeJSON(w, http.StatusCreated, map[string]any{"recipeVersion": result, "components": stored})
}
func (h *productHandler) listRecipeVersions(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["recipeId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid recipe ID")
		return
	}
	result, err := NewRecipeRepository(h.db, tenant.ID).ListVersions(r.Context(), id)
	if err != nil {
		apierror.Internal(w, r, "Failed to list recipe versions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recipeVersions": result})
}
func (h *productHandler) listRecipeComponents(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["recipeVersionId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid recipe version ID")
		return
	}
	result, err := NewRecipeRepository(h.db, tenant.ID).ListComponents(r.Context(), id)
	if err != nil {
		apierror.Internal(w, r, "Failed to list recipe components")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"components": result})
}
func (h *productHandler) releaseRecipeVersion(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["recipeVersionId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid recipe version ID")
		return
	}
	var req struct {
		Version int64 `json:"version" validate:"gte=1"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	if err := validation.Validate(&req); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	result, err := NewRecipeRepository(h.db, tenant.ID).ReleaseVersion(r.Context(), id, req.Version)
	switch {
	case err == nil:
		h.auditRecipeVersion(r, result, "recipe_version.released", "Recipe version released")
		writeJSON(w, http.StatusOK, map[string]any{"recipeVersion": result})
	case errors.Is(err, ErrRecipeVersionNotFound):
		apierror.NotFound(w, r, "Recipe version not found")
	case errors.Is(err, ErrRecipeConflict):
		apierror.Conflict(w, r, "Recipe version effective range overlaps")
	case errors.Is(err, ErrReleasedVersionImmutable):
		apierror.Conflict(w, r, "Recipe version is already released and immutable")
	case errors.Is(err, ErrRecipeVersionConflict):
		apierror.VersionConflict(w, r, "Recipe version was modified by another request")
	default:
		apierror.ValidationOrInternal(w, r, err, "Failed to release recipe version")
	}
}

type mappingCreateRequest struct {
	Adapter           string             `json:"adapter" validate:"required,max=64,location_code"`
	ExternalProductID string             `json:"externalProductId" validate:"required,not_blank,max=256"`
	RecipeVersionID   primitive.ObjectID `json:"recipeVersionId" validate:"required"`
	EffectiveFrom     time.Time          `json:"effectiveFrom" validate:"required"`
	EffectiveTo       *time.Time         `json:"effectiveTo"`
}

func (h *productHandler) listMappings(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	var at *time.Time
	if raw := r.URL.Query().Get("at"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			apierror.BadRequest(w, r, "Invalid mapping time")
			return
		}
		at = &parsed
	}
	result, err := NewMappingRepository(h.db, tenant.ID).List(r.Context(), strings.ToLower(strings.TrimSpace(r.URL.Query().Get("adapter"))), at)
	if err != nil {
		apierror.Internal(w, r, "Failed to list mappings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mappings": result})
}
func (h *productHandler) createMapping(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	var req mappingCreateRequest
	if !decodeStrict(w, r, &req) {
		return
	}
	req.Adapter, req.ExternalProductID = strings.ToLower(strings.TrimSpace(req.Adapter)), strings.TrimSpace(req.ExternalProductID)
	if err := validation.Validate(&req); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	result, err := NewMappingRepository(h.db, tenant.ID).Create(r.Context(), models.ExternalProductMapping{Adapter: req.Adapter, ExternalProductID: req.ExternalProductID, RecipeVersionID: req.RecipeVersionID, EffectiveFrom: req.EffectiveFrom, EffectiveTo: req.EffectiveTo})
	switch {
	case err == nil:
		h.auditMapping(r, result, "external_product_mapping.created", "External product mapping created")
		writeJSON(w, http.StatusCreated, map[string]any{"mapping": result})
	case errors.Is(err, ErrMappingOverlap):
		apierror.Conflict(w, r, "External product mapping effective range overlaps")
	case errors.Is(err, ErrRecipeVersionMustBeReleased):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.ValidationOrInternal(w, r, err, "Failed to create mapping")
	}
}
func (h *productHandler) updateMapping(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["mappingId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid mapping ID")
		return
	}
	var req struct {
		Version         int64               `json:"version" validate:"gte=1"`
		RecipeVersionID *primitive.ObjectID `json:"recipeVersionId"`
		EffectiveFrom   *time.Time          `json:"effectiveFrom"`
		EffectiveTo     json.RawMessage     `json:"effectiveTo"`
		IsActive        *bool               `json:"isActive"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	if err := validation.Validate(&req); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	var end **time.Time
	if req.EffectiveTo != nil {
		if string(req.EffectiveTo) == "null" {
			var nilEnd *time.Time
			end = &nilEnd
		} else {
			var parsed time.Time
			if err := json.Unmarshal(req.EffectiveTo, &parsed); err != nil {
				apierror.Validation(w, r, "effectiveTo must be an RFC3339 timestamp or null")
				return
			}
			parsedEnd := &parsed
			end = &parsedEnd
		}
	}
	result, err := NewMappingRepository(h.db, tenant.ID).Update(r.Context(), id, req.Version, MappingUpdate{RecipeVersionID: req.RecipeVersionID, EffectiveFrom: req.EffectiveFrom, EffectiveTo: end, IsActive: req.IsActive})
	switch {
	case err == nil:
		h.auditMapping(r, result, "external_product_mapping.updated", "External product mapping updated")
		writeJSON(w, http.StatusOK, map[string]any{"mapping": result})
	case errors.Is(err, ErrMappingNotFound):
		apierror.NotFound(w, r, "Mapping not found")
	case errors.Is(err, ErrMappingOverlap):
		apierror.Conflict(w, r, "External product mapping effective range overlaps")
	case errors.Is(err, ErrRecipeVersionConflict):
		apierror.VersionConflict(w, r, "Mapping was modified by another request")
	default:
		apierror.ValidationOrInternal(w, r, err, "Failed to update mapping")
	}
}
func (h *productHandler) deleteMapping(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["mappingId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid mapping ID")
		return
	}
	err = NewMappingRepository(h.db, tenant.ID).Delete(r.Context(), id)
	if errors.Is(err, ErrMappingNotFound) {
		apierror.NotFound(w, r, "Mapping not found")
		return
	}
	if err != nil {
		apierror.Internal(w, r, "Failed to delete mapping")
		return
	}
	if user, userOK := middleware.GetUserFromContext(r.Context()); userOK && h.logger != nil {
		h.logger.LogTenantActivity(r.Context(), models.LogLow, "External product mapping deleted", user.ID, tenant.ID, "external_product_mapping.deleted", map[string]interface{}{"mappingId": id.Hex()})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *productHandler) auditRecipe(r *http.Request, recipe *models.Recipe, action, message string) {
	user, userOK := middleware.GetUserFromContext(r.Context())
	tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
	if !userOK || !tenantOK || h.logger == nil || recipe == nil {
		return
	}
	h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{"recipeId": recipe.ID.Hex(), "version": recipe.Version})
}

func (h *productHandler) auditRecipeVersion(r *http.Request, version *models.RecipeVersion, action, message string) {
	user, userOK := middleware.GetUserFromContext(r.Context())
	tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
	if !userOK || !tenantOK || h.logger == nil || version == nil {
		return
	}
	h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{"recipeVersionId": version.ID.Hex(), "recipeId": version.RecipeID.Hex(), "version": version.Version})
}

func (h *productHandler) auditMapping(r *http.Request, mapping *models.ExternalProductMapping, action, message string) {
	user, userOK := middleware.GetUserFromContext(r.Context())
	tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
	if !userOK || !tenantOK || h.logger == nil || mapping == nil {
		return
	}
	h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{"mappingId": mapping.ID.Hex(), "adapter": mapping.Adapter, "externalProductId": mapping.ExternalProductID, "version": mapping.Version})
}
