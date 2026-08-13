package product

import (
	"context"
	"encoding/json"
	"errors"
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
	ErrSupplierNotFound        = errors.New("supplier not found")
	ErrDuplicateSupplierCode   = errors.New("supplier code already exists")
	ErrSupplierVersionConflict = errors.New("supplier version conflict")
)

type SupplierRepository struct {
	db       *db.MongoDB
	tenantID primitive.ObjectID
}
type SupplierItemRepository struct {
	db                   *db.MongoDB
	tenantID, supplierID primitive.ObjectID
}

var (
	ErrSupplierItemNotFound        = errors.New("supplier item not found")
	ErrDuplicateSupplierItem       = errors.New("supplier item already exists")
	ErrSupplierItemVersionConflict = errors.New("supplier item version conflict")
	ErrSupplierItemSupplierInvalid = errors.New("supplier must be active and belong to the tenant")
	ErrSupplierItemItemInvalid     = errors.New("item must be active and belong to the tenant")
)

func NewSupplierRepository(database *db.MongoDB, tenantID primitive.ObjectID) *SupplierRepository {
	return &SupplierRepository{db: database, tenantID: tenantID}
}

func normalizeOrderingDays(days []int32) []int32 {
	seen := make(map[int32]struct{}, len(days))
	result := make([]int32, 0, len(days))
	for _, day := range days {
		if _, ok := seen[day]; !ok {
			seen[day] = struct{}{}
			result = append(result, day)
		}
	}
	return result
}

func (r *SupplierRepository) List(ctx context.Context, includeInactive bool) ([]models.Supplier, error) {
	filter := bson.M{"tenantId": r.tenantID}
	if !includeInactive {
		filter["isActive"] = true
	}
	cursor, err := r.db.Suppliers().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "code", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	result := []models.Supplier{}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *SupplierRepository) Create(ctx context.Context, supplier models.Supplier) (*models.Supplier, error) {
	supplier.ID = primitive.NewObjectID()
	supplier.TenantID = r.tenantID
	supplier.Code = strings.ToLower(strings.TrimSpace(supplier.Code))
	supplier.Name = strings.TrimSpace(supplier.Name)
	supplier.ContactName = strings.TrimSpace(supplier.ContactName)
	supplier.Email = strings.TrimSpace(supplier.Email)
	supplier.Phone = strings.TrimSpace(supplier.Phone)
	supplier.OrderingDays = normalizeOrderingDays(supplier.OrderingDays)
	supplier.IsActive = true
	supplier.Version = 1
	supplier.CreatedAt = time.Now().UTC()
	supplier.UpdatedAt = supplier.CreatedAt
	if err := validation.Validate(&supplier); err != nil {
		return nil, err
	}
	if _, err := r.db.Suppliers().InsertOne(ctx, supplier); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDuplicateSupplierCode
		}
		return nil, err
	}
	return &supplier, nil
}

func (r *SupplierRepository) Update(ctx context.Context, id primitive.ObjectID, version int64, update models.Supplier) (*models.Supplier, error) {
	set := bson.M{"updatedAt": time.Now().UTC()}
	set["name"] = strings.TrimSpace(update.Name)
	set["contactName"] = strings.TrimSpace(update.ContactName)
	set["email"] = strings.TrimSpace(update.Email)
	set["phone"] = strings.TrimSpace(update.Phone)
	set["orderingDays"] = normalizeOrderingDays(update.OrderingDays)
	set["defaultLeadTimeDays"] = update.DefaultLeadTimeDays
	set["isActive"] = update.IsActive
	var result models.Supplier
	err := r.db.Suppliers().FindOneAndUpdate(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "version": version}, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&result)
	if err == nil {
		if err := validation.Validate(&result); err != nil {
			return nil, err
		}
		return &result, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	count, err := r.db.Suppliers().CountDocuments(ctx, bson.M{"_id": id, "tenantId": r.tenantID})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrSupplierNotFound
	}
	return nil, ErrSupplierVersionConflict
}

func supplierIDFromRequest(w http.ResponseWriter, r *http.Request) (*models.Tenant, primitive.ObjectID, bool) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return nil, primitive.NilObjectID, false
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["supplierId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid supplier ID")
		return nil, primitive.NilObjectID, false
	}
	return tenant, id, true
}

func (h *productHandler) listSuppliers(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	suppliers, err := NewSupplierRepository(h.db, tenant.ID).List(r.Context(), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		apierror.Internal(w, r, "Failed to list suppliers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suppliers": suppliers})
}

func (h *productHandler) createSupplier(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	var request models.Supplier
	if !decodeStrict(w, r, &request) {
		return
	}
	request.Code = strings.ToLower(strings.TrimSpace(request.Code))
	request.Name = strings.TrimSpace(request.Name)
	request.ContactName = strings.TrimSpace(request.ContactName)
	request.Email = strings.TrimSpace(request.Email)
	request.Phone = strings.TrimSpace(request.Phone)
	request.OrderingDays = normalizeOrderingDays(request.OrderingDays)
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	supplier, err := NewSupplierRepository(h.db, tenant.ID).Create(r.Context(), request)
	switch {
	case err == nil:
		h.auditSupplier(r, supplier, "supplier.created", "Supplier created")
		writeJSON(w, http.StatusCreated, map[string]any{"supplier": supplier})
	case errors.Is(err, ErrDuplicateSupplierCode):
		apierror.Conflict(w, r, "Supplier code already exists")
	default:
		apierror.Validation(w, r, err.Error())
	}
}

func (h *productHandler) updateSupplier(w http.ResponseWriter, r *http.Request) {
	tenant, id, ok := supplierIDFromRequest(w, r)
	if !ok {
		return
	}
	var request struct {
		Version             int64    `json:"version" validate:"gte=1"`
		Name                *string  `json:"name" validate:"omitempty,not_blank,max=160"`
		ContactName         *string  `json:"contactName" validate:"omitempty,max=100"`
		Email               *string  `json:"email" validate:"omitempty,email,max=254"`
		Phone               *string  `json:"phone" validate:"omitempty,max=32"`
		OrderingDays        *[]int32 `json:"orderingDays" validate:"omitempty,dive,gte=1,lte=7"`
		DefaultLeadTimeDays *int32   `json:"defaultLeadTimeDays" validate:"omitempty,gte=0,lte=3650"`
		IsActive            *bool    `json:"isActive"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.Name == nil && request.ContactName == nil && request.Email == nil && request.Phone == nil && request.OrderingDays == nil && request.DefaultLeadTimeDays == nil && request.IsActive == nil {
		apierror.BadRequest(w, r, "At least one mutable field is required")
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	var current models.Supplier
	if err := h.db.Suppliers().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Supplier not found")
		} else {
			apierror.Internal(w, r, "Failed to update supplier")
		}
		return
	}
	if request.Name != nil {
		current.Name = *request.Name
	}
	if request.ContactName != nil {
		current.ContactName = *request.ContactName
	}
	if request.Email != nil {
		current.Email = *request.Email
	}
	if request.Phone != nil {
		current.Phone = *request.Phone
	}
	if request.OrderingDays != nil {
		current.OrderingDays = *request.OrderingDays
	}
	if request.DefaultLeadTimeDays != nil {
		current.DefaultLeadTimeDays = *request.DefaultLeadTimeDays
	}
	if request.IsActive != nil {
		current.IsActive = *request.IsActive
	}
	supplier, err := NewSupplierRepository(h.db, tenant.ID).Update(r.Context(), id, request.Version, current)
	switch {
	case err == nil:
		h.auditSupplier(r, supplier, "supplier.updated", "Supplier updated")
		writeJSON(w, http.StatusOK, map[string]any{"supplier": supplier})
	case errors.Is(err, ErrSupplierNotFound):
		apierror.NotFound(w, r, "Supplier not found")
	case errors.Is(err, ErrSupplierVersionConflict):
		apierror.VersionConflict(w, r, "Supplier was modified by another request")
	default:
		apierror.Validation(w, r, err.Error())
	}
}

func (h *productHandler) auditSupplier(r *http.Request, supplier *models.Supplier, action, message string) {
	if user, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
		if tenant, ok := middleware.GetTenantFromContext(r.Context()); ok {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{"supplierId": supplier.ID.Hex(), "code": supplier.Code, "version": supplier.Version})
		}
	}
}

func parseInt64String(raw json.RawMessage, field string, allowEmpty bool) (int64, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || (!allowEmpty && value == "") {
		return 0, errors.New(field + " must be a decimal string")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, errors.New(field + " must be a valid signed int64 decimal string")
	}
	return parsed, nil
}

func (r *SupplierItemRepository) verifyReferences(ctx context.Context, itemID primitive.ObjectID, supplierActive, itemActive bool) error {
	supplierFilter := bson.M{"_id": r.supplierID, "tenantId": r.tenantID}
	if supplierActive {
		supplierFilter["isActive"] = true
	}
	if err := r.db.Suppliers().FindOne(ctx, supplierFilter, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrSupplierItemSupplierInvalid
		}
		return err
	}
	itemFilter := bson.M{"_id": itemID, "tenantId": r.tenantID}
	if itemActive {
		itemFilter["isActive"] = true
	}
	if err := r.db.Items().FindOne(ctx, itemFilter, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrSupplierItemItemInvalid
		}
		return err
	}
	return nil
}

func (r *SupplierItemRepository) List(ctx context.Context) ([]models.SupplierItem, error) {
	if err := r.db.Suppliers().FindOne(ctx, bson.M{"_id": r.supplierID, "tenantId": r.tenantID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrSupplierItemSupplierInvalid
		}
		return nil, err
	}
	cursor, err := r.db.SupplierItems().Find(ctx, bson.M{"tenantId": r.tenantID, "supplierId": r.supplierID}, options.Find().SetSort(bson.D{{Key: "itemId", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	result := []models.SupplierItem{}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *SupplierItemRepository) Create(ctx context.Context, item models.SupplierItem) (*models.SupplierItem, error) {
	if err := r.verifyReferences(ctx, item.ItemID, true, true); err != nil {
		return nil, err
	}
	item.ID = primitive.NewObjectID()
	item.TenantID = r.tenantID
	item.SupplierID = r.supplierID
	item.SupplierSKU = strings.TrimSpace(item.SupplierSKU)
	item.IsActive = true
	item.Version = 1
	item.CreatedAt = time.Now().UTC()
	item.UpdatedAt = item.CreatedAt
	if err := validation.Validate(&item); err != nil {
		return nil, err
	}
	if _, err := r.db.SupplierItems().InsertOne(ctx, item); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDuplicateSupplierItem
		}
		return nil, err
	}
	return &item, nil
}

func (r *SupplierItemRepository) Update(ctx context.Context, id primitive.ObjectID, version int64, item models.SupplierItem, itemChanged bool, leadTimeChanged bool) (*models.SupplierItem, error) {
	var current models.SupplierItem
	if err := r.db.SupplierItems().FindOne(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "supplierId": r.supplierID}, options.FindOne()).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrSupplierItemNotFound
		}
		return nil, err
	}
	if err := r.verifyReferences(ctx, item.ItemID, false, itemChanged); err != nil {
		return nil, err
	}
	set := bson.M{"updatedAt": time.Now().UTC(), "itemId": item.ItemID, "supplierSKU": strings.TrimSpace(item.SupplierSKU), "packSizeMicros": item.PackSizeMicros, "moq": item.MOQ, "unitPriceMinor": item.UnitPriceMinor, "currency": item.Currency, "isActive": item.IsActive}
	if leadTimeChanged {
		set["leadTimeDays"] = item.LeadTimeDays
		if item.LeadTimeDays == nil {
			delete(set, "leadTimeDays")
		}
	}
	var result models.SupplierItem
	err := r.db.SupplierItems().FindOneAndUpdate(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "supplierId": r.supplierID, "version": version}, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&result)
	if err == nil {
		if err := validation.Validate(&result); err != nil {
			return nil, err
		}
		return &result, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	count, err := r.db.SupplierItems().CountDocuments(ctx, bson.M{"_id": id, "tenantId": r.tenantID, "supplierId": r.supplierID})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrSupplierItemNotFound
	}
	return nil, ErrSupplierItemVersionConflict
}

func supplierItemScope(w http.ResponseWriter, r *http.Request) (*models.Tenant, primitive.ObjectID, bool) {
	tenant, supplierID, ok := supplierIDFromRequest(w, r)
	return tenant, supplierID, ok
}

func (h *productHandler) listSupplierItems(w http.ResponseWriter, r *http.Request) {
	tenant, supplierID, ok := supplierItemScope(w, r)
	if !ok {
		return
	}
	result, err := (&SupplierItemRepository{db: h.db, tenantID: tenant.ID, supplierID: supplierID}).List(r.Context())
	if err != nil {
		if errors.Is(err, ErrSupplierItemSupplierInvalid) {
			apierror.NotFound(w, r, "Supplier not found")
		} else {
			apierror.Internal(w, r, "Failed to list supplier items")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"supplierItems": result})
}

func (h *productHandler) createSupplierItem(w http.ResponseWriter, r *http.Request) {
	tenant, supplierID, ok := supplierItemScope(w, r)
	if !ok {
		return
	}
	var request struct {
		ItemID         primitive.ObjectID `json:"itemId" validate:"required"`
		SupplierSKU    string             `json:"supplierSKU" validate:"omitempty,max=100"`
		PackSizeMicros json.RawMessage    `json:"packSizeMicros"`
		MOQ            int32              `json:"moq" validate:"gte=1,lte=1000000"`
		UnitPriceMinor json.RawMessage    `json:"unitPriceMinor"`
		Currency       string             `json:"currency" validate:"required,currency_code"`
		LeadTimeDays   *int32             `json:"leadTimeDays" validate:"omitempty,gte=0,lte=3650"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	pack, err := parseInt64String(request.PackSizeMicros, "packSizeMicros", false)
	if err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	price, err := parseInt64String(request.UnitPriceMinor, "unitPriceMinor", false)
	if err != nil || price < 0 {
		apierror.Validation(w, r, "unitPriceMinor must be a non-negative decimal string")
		return
	}
	request.SupplierSKU = strings.TrimSpace(request.SupplierSKU)
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	item, err := (&SupplierItemRepository{db: h.db, tenantID: tenant.ID, supplierID: supplierID}).Create(r.Context(), models.SupplierItem{ItemID: request.ItemID, SupplierSKU: request.SupplierSKU, PackSizeMicros: pack, MOQ: request.MOQ, UnitPriceMinor: price, Currency: request.Currency, LeadTimeDays: request.LeadTimeDays})
	switch {
	case err == nil:
		h.auditSupplierItem(r, item, "supplier_item.created", "Supplier item created")
		writeJSON(w, http.StatusCreated, map[string]any{"supplierItem": item})
	case errors.Is(err, ErrDuplicateSupplierItem):
		apierror.Conflict(w, r, "Supplier item already exists")
	case errors.Is(err, ErrSupplierItemSupplierInvalid):
		apierror.NotFound(w, r, "Supplier not found")
	case errors.Is(err, ErrSupplierItemItemInvalid):
		apierror.Validation(w, r, "Item must be active and belong to the tenant")
	default:
		apierror.Validation(w, r, err.Error())
	}
}

func (h *productHandler) updateSupplierItem(w http.ResponseWriter, r *http.Request) {
	tenant, supplierID, ok := supplierItemScope(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["supplierItemId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid supplier item ID")
		return
	}
	var request struct {
		Version        int64               `json:"version" validate:"gte=1"`
		ItemID         *primitive.ObjectID `json:"itemId"`
		SupplierSKU    *string             `json:"supplierSKU" validate:"omitempty,max=100"`
		PackSizeMicros *json.RawMessage    `json:"packSizeMicros"`
		MOQ            *int32              `json:"moq" validate:"omitempty,gte=1,lte=1000000"`
		UnitPriceMinor *json.RawMessage    `json:"unitPriceMinor"`
		Currency       *string             `json:"currency" validate:"omitempty,currency_code"`
		LeadTimeDays   json.RawMessage     `json:"leadTimeDays"`
		IsActive       *bool               `json:"isActive"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.ItemID == nil && request.SupplierSKU == nil && request.PackSizeMicros == nil && request.MOQ == nil && request.UnitPriceMinor == nil && request.Currency == nil && request.LeadTimeDays == nil && request.IsActive == nil {
		apierror.BadRequest(w, r, "At least one mutable field is required")
		return
	}
	var current models.SupplierItem
	if err := h.db.SupplierItems().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID, "supplierId": supplierID}).Decode(&current); err != nil {
		apierror.NotFound(w, r, "Supplier item not found")
		return
	}
	originalItemID := current.ItemID
	if request.ItemID != nil {
		current.ItemID = *request.ItemID
	}
	if request.SupplierSKU != nil {
		current.SupplierSKU = *request.SupplierSKU
	}
	if request.MOQ != nil {
		current.MOQ = *request.MOQ
	}
	if request.Currency != nil {
		current.Currency = *request.Currency
	}
	itemChanged := request.ItemID != nil && *request.ItemID != originalItemID
	if request.PackSizeMicros != nil {
		value, parseErr := parseInt64String(*request.PackSizeMicros, "packSizeMicros", false)
		if parseErr != nil {
			apierror.Validation(w, r, parseErr.Error())
			return
		}
		current.PackSizeMicros = value
	}
	if request.UnitPriceMinor != nil {
		value, parseErr := parseInt64String(*request.UnitPriceMinor, "unitPriceMinor", false)
		if parseErr != nil || value < 0 {
			apierror.Validation(w, r, "unitPriceMinor must be a non-negative decimal string")
			return
		}
		current.UnitPriceMinor = value
	}
	leadChanged := request.LeadTimeDays != nil
	if leadChanged {
		if string(request.LeadTimeDays) == "null" {
			current.LeadTimeDays = nil
		} else {
			var value int32
			if json.Unmarshal(request.LeadTimeDays, &value) != nil {
				apierror.Validation(w, r, "leadTimeDays must be a whole number or null")
				return
			}
			current.LeadTimeDays = &value
		}
	}
	if request.IsActive != nil {
		current.IsActive = *request.IsActive
	}
	if err := validation.Validate(&current); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	updated, err := (&SupplierItemRepository{db: h.db, tenantID: tenant.ID, supplierID: supplierID}).Update(r.Context(), id, request.Version, current, itemChanged, leadChanged)
	switch {
	case err == nil:
		h.auditSupplierItem(r, updated, "supplier_item.updated", "Supplier item updated")
		writeJSON(w, http.StatusOK, map[string]any{"supplierItem": updated})
	case errors.Is(err, ErrSupplierItemNotFound):
		apierror.NotFound(w, r, "Supplier item not found")
	case errors.Is(err, ErrSupplierItemVersionConflict):
		apierror.VersionConflict(w, r, "Supplier item was modified by another request")
	case errors.Is(err, ErrSupplierItemItemInvalid):
		apierror.Validation(w, r, "Item must be active and belong to the tenant")
	default:
		apierror.Validation(w, r, err.Error())
	}
}

func (h *productHandler) auditSupplierItem(r *http.Request, item *models.SupplierItem, action, message string) {
	if user, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
		if tenant, ok := middleware.GetTenantFromContext(r.Context()); ok {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{"supplierItemId": item.ID.Hex(), "supplierId": item.SupplierID.Hex(), "itemId": item.ItemID.Hex(), "version": item.Version})
		}
	}
}
