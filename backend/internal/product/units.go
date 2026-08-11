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
	ErrUnitNotFound        = errors.New("unit not found")
	ErrDuplicateUnitCode   = errors.New("unit code already exists")
	ErrUnitVersionConflict = errors.New("unit version conflict")
)

type UnitRepository struct {
	db       *db.MongoDB
	tenantID primitive.ObjectID
}

type UnitUpdate struct {
	Name      *string
	Symbol    *string
	Precision *int32
	IsActive  *bool
}

func NewUnitRepository(database *db.MongoDB, tenantID primitive.ObjectID) *UnitRepository {
	return &UnitRepository{db: database, tenantID: tenantID}
}

func (r *UnitRepository) List(ctx context.Context, includeInactive bool) ([]models.Unit, error) {
	filter := bson.M{"tenantId": r.tenantID}
	if !includeInactive {
		filter["isActive"] = true
	}
	cursor, err := r.db.Units().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "dimension", Value: 1}, {Key: "code", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	units := []models.Unit{}
	if err := cursor.All(ctx, &units); err != nil {
		return nil, err
	}
	return units, nil
}

func (r *UnitRepository) Create(ctx context.Context, code, name, symbol string, dimension models.UnitDimension, precision int32) (*models.Unit, error) {
	now := time.Now().UTC()
	unit := models.Unit{
		ID: primitive.NewObjectID(), TenantID: r.tenantID, Code: strings.ToLower(strings.TrimSpace(code)),
		Name: strings.TrimSpace(name), Symbol: strings.TrimSpace(symbol), Dimension: dimension,
		Precision: precision, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := validation.Validate(&unit); err != nil {
		return nil, err
	}
	if _, err := r.db.Units().InsertOne(ctx, unit); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDuplicateUnitCode
		}
		return nil, err
	}
	return &unit, nil
}

func (r *UnitRepository) Update(ctx context.Context, id primitive.ObjectID, expectedVersion int64, update UnitUpdate) (*models.Unit, error) {
	set := bson.M{"updatedAt": time.Now().UTC()}
	if update.Name != nil {
		set["name"] = strings.TrimSpace(*update.Name)
	}
	if update.Symbol != nil {
		set["symbol"] = strings.TrimSpace(*update.Symbol)
	}
	if update.Precision != nil {
		set["precision"] = *update.Precision
	}
	if update.IsActive != nil {
		set["isActive"] = *update.IsActive
	}
	var unit models.Unit
	err := r.db.Units().FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenantId": r.tenantID, "version": expectedVersion},
		bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&unit)
	if err == nil {
		if err := validation.Validate(&unit); err != nil {
			return nil, err
		}
		return &unit, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	count, countErr := r.db.Units().CountDocuments(ctx, bson.M{"_id": id, "tenantId": r.tenantID})
	if countErr != nil {
		return nil, countErr
	}
	if count == 0 {
		return nil, ErrUnitNotFound
	}
	return nil, ErrUnitVersionConflict
}

func (h *productHandler) listUnits(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	units, err := NewUnitRepository(h.db, tenant.ID).List(r.Context(), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		apierror.Internal(w, r, "Failed to list units")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"units": units})
}

func (h *productHandler) createUnit(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	var request struct {
		Code      string               `json:"code" validate:"required,max=32,location_code"`
		Name      string               `json:"name" validate:"required,min=1,max=100"`
		Symbol    string               `json:"symbol" validate:"required,min=1,max=16"`
		Dimension models.UnitDimension `json:"dimension" validate:"required,unit_dimension"`
		Precision int32                `json:"precision" validate:"gte=0,lte=6"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	request.Code = strings.ToLower(strings.TrimSpace(request.Code))
	request.Name = strings.TrimSpace(request.Name)
	request.Symbol = strings.TrimSpace(request.Symbol)
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	unit, err := NewUnitRepository(h.db, tenant.ID).Create(r.Context(), request.Code, request.Name, request.Symbol, request.Dimension, request.Precision)
	if err != nil {
		if errors.Is(err, ErrDuplicateUnitCode) {
			apierror.Conflict(w, r, "Unit code already exists")
		} else if strings.HasPrefix(err.Error(), "validation failed:") {
			apierror.Validation(w, r, err.Error())
		} else {
			apierror.Internal(w, r, "Failed to create unit")
		}
		return
	}
	h.auditUnit(r, unit, "unit.created", "Unit created")
	writeJSON(w, http.StatusCreated, map[string]any{"unit": unit})
}

func (h *productHandler) updateUnit(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["unitId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid unit ID")
		return
	}
	var request struct {
		Version   int64   `json:"version" validate:"gte=1"`
		Name      *string `json:"name" validate:"omitempty,min=1,max=100"`
		Symbol    *string `json:"symbol" validate:"omitempty,min=1,max=16"`
		Precision *int32  `json:"precision" validate:"omitempty,gte=0,lte=6"`
		IsActive  *bool   `json:"isActive"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.Name == nil && request.Symbol == nil && request.Precision == nil && request.IsActive == nil {
		apierror.BadRequest(w, r, "At least one mutable field is required")
		return
	}
	if request.Name != nil {
		trimmed := strings.TrimSpace(*request.Name)
		request.Name = &trimmed
	}
	if request.Symbol != nil {
		trimmed := strings.TrimSpace(*request.Symbol)
		request.Symbol = &trimmed
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	unit, err := NewUnitRepository(h.db, tenant.ID).Update(r.Context(), id, request.Version, UnitUpdate{Name: request.Name, Symbol: request.Symbol, Precision: request.Precision, IsActive: request.IsActive})
	switch {
	case err == nil:
		action, message := "unit.updated", "Unit updated"
		if request.IsActive != nil {
			if *request.IsActive {
				action, message = "unit.reactivated", "Unit reactivated"
			} else {
				action, message = "unit.deactivated", "Unit deactivated"
			}
		}
		h.auditUnit(r, unit, action, message)
		writeJSON(w, http.StatusOK, map[string]any{"unit": unit})
	case errors.Is(err, ErrUnitNotFound):
		apierror.NotFound(w, r, "Unit not found")
	case errors.Is(err, ErrUnitVersionConflict):
		apierror.VersionConflict(w, r, "Unit was modified by another request")
	case strings.HasPrefix(err.Error(), "validation failed:"):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Failed to update unit")
	}
}

func (h *productHandler) auditUnit(r *http.Request, unit *models.Unit, action, message string) {
	user, userOK := middleware.GetUserFromContext(r.Context())
	tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
	if !userOK || !tenantOK || h.logger == nil {
		return
	}
	h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{"unitId": unit.ID.Hex(), "code": unit.Code, "version": unit.Version})
}
