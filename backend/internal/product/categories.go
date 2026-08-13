package product

import (
	"context"
	"errors"
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
	ErrCategoryNotFound        = errors.New("category not found")
	ErrDuplicateCategoryCode   = errors.New("category code already exists")
	ErrCategoryVersionConflict = errors.New("category version conflict")
)

type CategoryRepository struct {
	db       *db.MongoDB
	tenantID primitive.ObjectID
}

type CategoryUpdate struct {
	Name     *string
	IsActive *bool
}

func NewCategoryRepository(database *db.MongoDB, tenantID primitive.ObjectID) *CategoryRepository {
	return &CategoryRepository{db: database, tenantID: tenantID}
}

func (r *CategoryRepository) List(ctx context.Context, includeInactive bool) ([]models.Category, error) {
	filter := bson.M{"tenantId": r.tenantID}
	if !includeInactive {
		filter["isActive"] = true
	}
	cursor, err := r.db.Categories().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "code", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	categories := []models.Category{}
	if err := cursor.All(ctx, &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *CategoryRepository) Create(ctx context.Context, code, name string) (*models.Category, error) {
	now := time.Now().UTC()
	category := models.Category{
		ID: primitive.NewObjectID(), TenantID: r.tenantID,
		Code: strings.ToLower(strings.TrimSpace(code)), Name: strings.TrimSpace(name),
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := validation.Validate(&category); err != nil {
		return nil, err
	}
	if _, err := r.db.Categories().InsertOne(ctx, category); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDuplicateCategoryCode
		}
		return nil, err
	}
	return &category, nil
}

func (r *CategoryRepository) Update(ctx context.Context, id primitive.ObjectID, expectedVersion int64, update CategoryUpdate) (*models.Category, error) {
	set := bson.M{"updatedAt": time.Now().UTC()}
	if update.Name != nil {
		set["name"] = strings.TrimSpace(*update.Name)
	}
	if update.IsActive != nil {
		set["isActive"] = *update.IsActive
	}
	var category models.Category
	err := r.db.Categories().FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenantId": r.tenantID, "version": expectedVersion},
		bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&category)
	if err == nil {
		if err := validation.Validate(&category); err != nil {
			return nil, err
		}
		return &category, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	count, countErr := r.db.Categories().CountDocuments(ctx, bson.M{"_id": id, "tenantId": r.tenantID})
	if countErr != nil {
		return nil, countErr
	}
	if count == 0 {
		return nil, ErrCategoryNotFound
	}
	return nil, ErrCategoryVersionConflict
}

func (h *productHandler) listCategories(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	categories, err := NewCategoryRepository(h.db, tenant.ID).List(r.Context(), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		apierror.Internal(w, r, "Failed to list categories")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": categories})
}

func (h *productHandler) createCategory(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	var request struct {
		Code string `json:"code" validate:"required,max=32,location_code"`
		Name string `json:"name" validate:"required,min=1,max=100"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	request.Code = strings.ToLower(strings.TrimSpace(request.Code))
	request.Name = strings.TrimSpace(request.Name)
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	category, err := NewCategoryRepository(h.db, tenant.ID).Create(r.Context(), request.Code, request.Name)
	if err != nil {
		if errors.Is(err, ErrDuplicateCategoryCode) {
			apierror.Conflict(w, r, "Category code already exists")
		} else if strings.HasPrefix(err.Error(), "validation failed:") {
			apierror.Validation(w, r, err.Error())
		} else {
			apierror.Internal(w, r, "Failed to create category")
		}
		return
	}
	h.auditCategory(r, category, "category.created", "Category created")
	writeJSON(w, http.StatusCreated, map[string]any{"category": category})
}

func (h *productHandler) updateCategory(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["categoryId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid category ID")
		return
	}
	var request struct {
		Version  int64   `json:"version" validate:"gte=1"`
		Name     *string `json:"name" validate:"omitempty,min=1,max=100"`
		IsActive *bool   `json:"isActive"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.Name == nil && request.IsActive == nil {
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
	category, err := NewCategoryRepository(h.db, tenant.ID).Update(r.Context(), id, request.Version, CategoryUpdate{Name: request.Name, IsActive: request.IsActive})
	switch {
	case err == nil:
		action, message := "category.updated", "Category updated"
		if request.IsActive != nil {
			if *request.IsActive {
				action, message = "category.reactivated", "Category reactivated"
			} else {
				action, message = "category.deactivated", "Category deactivated"
			}
		}
		h.auditCategory(r, category, action, message)
		writeJSON(w, http.StatusOK, map[string]any{"category": category})
	case errors.Is(err, ErrCategoryNotFound):
		apierror.NotFound(w, r, "Category not found")
	case errors.Is(err, ErrCategoryVersionConflict):
		apierror.VersionConflict(w, r, "Category was modified by another request")
	case strings.HasPrefix(err.Error(), "validation failed:"):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Failed to update category")
	}
}

func (h *productHandler) auditCategory(r *http.Request, category *models.Category, action, message string) {
	user, userOK := middleware.GetUserFromContext(r.Context())
	tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
	if !userOK || !tenantOK || h.logger == nil {
		return
	}
	h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{
		"categoryId": category.ID.Hex(), "code": category.Code, "version": category.Version,
	})
}
