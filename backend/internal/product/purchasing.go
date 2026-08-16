package product

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
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
	ErrPurchaseOrderNotFound        = errors.New("purchase order not found")
	ErrPurchaseOrderVersionConflict = errors.New("purchase order version conflict")
	ErrPurchaseOrderState           = errors.New("purchase order state does not allow this operation")
	ErrDeliveryCalendarNotFound     = errors.New("delivery calendar rule not found")
	ErrDeliveryCalendarConflict     = errors.New("delivery calendar rule already exists")
	ErrDeliveryCalendarVersion      = errors.New("delivery calendar version conflict")
	ErrDeliveryDateInvalid          = errors.New("delivery date is not valid for the supplier calendar")
	ErrPurchasingLocation           = errors.New("location is not authorized for purchasing")
	ErrPurchasingReference          = errors.New("supplier, supplier item, or unit reference is invalid")
	ErrPurchaseRoundingOverflow     = errors.New("purchase quantity rounding overflow")
	ErrRecommendationNotFound       = errors.New("reorder recommendation not found")
	ErrRecommendationStale          = errors.New("reorder recommendation is stale")
	ErrRecommendationState          = errors.New("reorder recommendation is not ready")
	ErrRecommendationConflict       = errors.New("reorder recommendation has already been converted")
	ErrRecommendationIdempotency    = errors.New("idempotency key conflicts with an existing draft")
)

type purchaseQuantityRounding struct {
	OrderedPacks          int64
	OrderedQuantityMicros int64
	RoundingDeltaMicros   int64
	LineTotalMinor        int64
}

// RoundPurchaseQuantity performs pack and MOQ rounding using only int64
// arithmetic. UnitPriceMinor is the price per ordered pack.
func RoundPurchaseQuantity(requestedQuantityMicros, packSizeMicros int64, moq int32, unitPriceMinor int64) (purchaseQuantityRounding, error) {
	if requestedQuantityMicros <= 0 || packSizeMicros <= 0 || moq <= 0 || unitPriceMinor < 0 {
		return purchaseQuantityRounding{}, ErrPurchaseRoundingOverflow
	}
	packs := requestedQuantityMicros / packSizeMicros
	if requestedQuantityMicros%packSizeMicros != 0 {
		if packs == math.MaxInt64 {
			return purchaseQuantityRounding{}, ErrPurchaseRoundingOverflow
		}
		packs++
	}
	if packs < int64(moq) {
		packs = int64(moq)
	}
	if packs > math.MaxInt64/packSizeMicros || packs > math.MaxInt64/unitPriceMinor && unitPriceMinor > 0 {
		return purchaseQuantityRounding{}, ErrPurchaseRoundingOverflow
	}
	ordered := packs * packSizeMicros
	if ordered < requestedQuantityMicros {
		return purchaseQuantityRounding{}, ErrPurchaseRoundingOverflow
	}
	return purchaseQuantityRounding{OrderedPacks: packs, OrderedQuantityMicros: ordered, RoundingDeltaMicros: ordered - requestedQuantityMicros, LineTotalMinor: packs * unitPriceMinor}, nil
}

type purchaseOrderLineRequest struct {
	SupplierItemID          primitive.ObjectID  `json:"supplierItemId" validate:"required"`
	RequestedQuantityMicros int64               `json:"requestedQuantityMicros,string" validate:"gt=0"`
	OrderedUnitID           *primitive.ObjectID `json:"orderedUnitId,omitempty"`
}

type purchaseOrderCreateRequest struct {
	LocationID   primitive.ObjectID         `json:"locationId" validate:"required"`
	SupplierID   primitive.ObjectID         `json:"supplierId" validate:"required"`
	DeliveryDate time.Time                  `json:"deliveryDate" validate:"required"`
	Notes        string                     `json:"notes,omitempty" validate:"omitempty,max=2000"`
	Lines        []purchaseOrderLineRequest `json:"lines" validate:"required,min=1,max=1000,dive"`
}

type purchaseOrderDraftFromRecommendationRequest struct {
	SupplierItemID primitive.ObjectID `json:"supplierItemId" validate:"required"`
	IdempotencyKey string             `json:"idempotencyKey" validate:"required,min=8,max=128"`
}

type purchaseOrderUpdateRequest struct {
	Version      int64                       `json:"version" validate:"gte=1"`
	DeliveryDate *time.Time                  `json:"deliveryDate,omitempty"`
	Notes        *string                     `json:"notes,omitempty" validate:"omitempty,max=2000"`
	Lines        *[]purchaseOrderLineRequest `json:"lines,omitempty" validate:"omitempty,min=1,max=1000,dive"`
}

type purchaseOrderActionRequest struct {
	Version int64  `json:"version" validate:"gte=1"`
	Note    string `json:"note,omitempty" validate:"omitempty,max=500"`
}

type deliveryCalendarRequest struct {
	LocationID    primitive.ObjectID `json:"locationId" validate:"required"`
	SupplierID    primitive.ObjectID `json:"supplierId" validate:"required"`
	Weekday       int32              `json:"weekday" validate:"gte=1,lte=7"`
	CutoffMinutes int32              `json:"cutoffMinutes" validate:"gte=0,lte=1439"`
	LeadTimeDays  int32              `json:"leadTimeDays" validate:"gte=0,lte=3650"`
}

type deliveryCalendarUpdateRequest struct {
	Version       int64  `json:"version" validate:"gte=1"`
	Weekday       *int32 `json:"weekday,omitempty" validate:"omitempty,gte=1,lte=7"`
	CutoffMinutes *int32 `json:"cutoffMinutes,omitempty" validate:"omitempty,gte=0,lte=1439"`
	LeadTimeDays  *int32 `json:"leadTimeDays,omitempty" validate:"omitempty,gte=0,lte=3650"`
	IsActive      *bool  `json:"isActive,omitempty"`
}

func isoWeekday(t time.Time) int32 {
	return int32((int(t.Weekday())+6)%7 + 1)
}

func authorizePurchasingLocation(ctx context.Context, database *db.MongoDB, tenantID primitive.ObjectID, locationID primitive.ObjectID) error {
	profile, ok := GetStaffProfileFromContext(ctx)
	if !ok {
		return ErrPurchasingLocation
	}
	if err := database.Locations().FindOne(ctx, bson.M{"_id": locationID, "tenantId": tenantID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		return ErrPurchasingLocation
	}
	if profile.AllLocations {
		return nil
	}
	for _, assigned := range profile.LocationIDs {
		if assigned == locationID {
			return nil
		}
	}
	return ErrPurchasingLocation
}

func requirePurchasingManager() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile, ok := GetStaffProfileFromContext(r.Context())
			if !ok || (profile.BusinessRole != models.BusinessRoleCompanyOwner && profile.BusinessRole != models.BusinessRoleOperationsManager) {
				apierror.Forbidden(w, r, "Company owner or operations manager approval required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *productHandler) buildPurchaseLine(ctx mongo.SessionContext, tenantID, locationID, supplierID primitive.ObjectID, number int32, request purchaseOrderLineRequest) (models.PurchaseOrderLine, error) {
	var supplierItem models.SupplierItem
	if err := h.db.SupplierItems().FindOne(ctx, bson.M{"_id": request.SupplierItemID, "tenantId": tenantID, "supplierId": supplierID, "isActive": true}).Decode(&supplierItem); err != nil {
		return models.PurchaseOrderLine{}, ErrPurchasingReference
	}
	var supplier models.Supplier
	if err := h.db.Suppliers().FindOne(ctx, bson.M{"_id": supplierID, "tenantId": tenantID, "isActive": true}).Decode(&supplier); err != nil {
		return models.PurchaseOrderLine{}, ErrPurchasingReference
	}
	var item models.Item
	if err := h.db.Items().FindOne(ctx, bson.M{"_id": supplierItem.ItemID, "tenantId": tenantID, "isActive": true}).Decode(&item); err != nil {
		return models.PurchaseOrderLine{}, ErrPurchasingReference
	}
	var baseUnit models.Unit
	if err := h.db.Units().FindOne(ctx, bson.M{"_id": item.BaseUnitID, "tenantId": tenantID, "isActive": true}).Decode(&baseUnit); err != nil {
		return models.PurchaseOrderLine{}, ErrPurchasingReference
	}
	orderedUnitID := item.BaseUnitID
	orderedUnit := baseUnit
	if request.OrderedUnitID != nil {
		orderedUnitID = *request.OrderedUnitID
		if err := h.db.Units().FindOne(ctx, bson.M{"_id": orderedUnitID, "tenantId": tenantID, "isActive": true}).Decode(&orderedUnit); err != nil {
			return models.PurchaseOrderLine{}, ErrPurchasingReference
		}
	}
	rounding, err := RoundPurchaseQuantity(request.RequestedQuantityMicros, supplierItem.PackSizeMicros, supplierItem.MOQ, supplierItem.UnitPriceMinor)
	if err != nil {
		return models.PurchaseOrderLine{}, err
	}
	now := time.Now().UTC()
	code := supplierItem.SupplierSKU
	if strings.TrimSpace(code) == "" {
		code = item.SKU
	}
	description := strings.TrimSpace(item.Description)
	if description == "" {
		description = item.Name
	}
	line := models.PurchaseOrderLine{ID: primitive.NewObjectID(), TenantID: tenantID, LocationID: locationID, LineNumber: number, SupplierItemID: supplierItem.ID, SupplierItemVersion: supplierItem.Version, SupplierCode: supplier.Code, SupplierName: supplier.Name, ItemID: item.ID, ItemSKU: item.SKU, ItemName: item.Name, SupplierItemCode: code, Description: description, RequestedQuantityMicros: request.RequestedQuantityMicros, OrderedQuantityMicros: rounding.OrderedQuantityMicros, ReceivedQuantityMicros: 0, PackSizeMicros: supplierItem.PackSizeMicros, MOQ: supplierItem.MOQ, OrderedPacks: rounding.OrderedPacks, RoundingDeltaMicros: rounding.RoundingDeltaMicros, UnitPriceMinor: supplierItem.UnitPriceMinor, LineTotalMinor: rounding.LineTotalMinor, Currency: supplierItem.Currency, ItemBaseUnitID: item.BaseUnitID, ItemBaseUnitCode: baseUnit.Code, OrderedUnitID: orderedUnitID, OrderedUnitCode: orderedUnit.Code, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&line); err != nil {
		return models.PurchaseOrderLine{}, err
	}
	return line, nil
}

func (h *productHandler) validateDeliveryDate(ctx context.Context, tenantID, locationID, supplierID primitive.ObjectID, deliveryDate, now time.Time) error {
	date := deliveryDate.UTC()
	filter := bson.M{"tenantId": tenantID, "locationId": locationID, "supplierId": supplierID, "weekday": isoWeekday(date), "isActive": true}
	var rule models.DeliveryCalendarRule
	if err := h.db.DeliveryCalendars().FindOne(ctx, filter).Decode(&rule); err != nil {
		return ErrDeliveryDateInvalid
	}
	minimum := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(rule.LeadTimeDays))
	requested := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	if requested.Before(minimum) {
		return ErrDeliveryDateInvalid
	}
	if requested.Equal(time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)) && now.UTC().Hour()*60+now.UTC().Minute() >= int(rule.CutoffMinutes) {
		return ErrDeliveryDateInvalid
	}
	return nil
}

// nextDeliveryDate chooses the first date offered by an active, scoped
// supplier calendar. The conversion endpoint intentionally accepts no delivery
// date: it creates a draft for the next available calendar date and leaves all
// later purchasing decisions to the normal purchase-order workflow.
func (h *productHandler) nextDeliveryDate(ctx context.Context, tenantID, locationID, supplierID primitive.ObjectID, now time.Time) (time.Time, error) {
	cur, err := h.db.DeliveryCalendars().Find(ctx, bson.M{"tenantId": tenantID, "locationId": locationID, "supplierId": supplierID, "isActive": true})
	if err != nil {
		return time.Time{}, err
	}
	defer cur.Close(ctx)
	var rules []models.DeliveryCalendarRule
	if err := cur.All(ctx, &rules); err != nil {
		return time.Time{}, err
	}
	if len(rules) == 0 {
		return time.Time{}, ErrDeliveryDateInvalid
	}
	start := now.UTC()
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	var selected time.Time
	for _, rule := range rules {
		minimum := startDay.AddDate(0, 0, int(rule.LeadTimeDays))
		for offset := 0; offset <= 3656; offset++ {
			candidate := startDay.AddDate(0, 0, offset)
			if isoWeekday(candidate) != rule.Weekday || candidate.Before(minimum) {
				continue
			}
			if candidate.Equal(startDay) && start.Hour()*60+start.Minute() >= int(rule.CutoffMinutes) {
				continue
			}
			if selected.IsZero() || candidate.Before(selected) {
				selected = candidate
			}
			break
		}
	}
	if selected.IsZero() {
		return time.Time{}, ErrDeliveryDateInvalid
	}
	if err := h.validateDeliveryDate(ctx, tenantID, locationID, supplierID, selected, now); err != nil {
		return time.Time{}, err
	}
	return selected, nil
}

func purchaseOrderDraftRequestHash(recommendationID, supplierItemID primitive.ObjectID) string {
	sum := sha256.Sum256([]byte(recommendationID.Hex() + "\x00" + supplierItemID.Hex()))
	return hex.EncodeToString(sum[:])
}

// createPurchaseOrderInTransaction is shared by the ordinary purchase-order
// create endpoint and recommendation conversion. Keeping line construction and
// rounding here prevents the two lanes from drifting apart.
func (h *productHandler) createPurchaseOrderInTransaction(ctx mongo.SessionContext, tenantID, userID, locationID, supplierID primitive.ObjectID, deliveryDate time.Time, notes string, lineRequests []purchaseOrderLineRequest, recommendationID *primitive.ObjectID, idempotencyKey, requestHash string) (models.PurchaseOrder, []models.PurchaseOrderLine, error) {
	if err := h.validateDeliveryDate(ctx, tenantID, locationID, supplierID, deliveryDate, time.Now()); err != nil {
		return models.PurchaseOrder{}, nil, err
	}
	now := time.Now().UTC()
	order := models.PurchaseOrder{ID: primitive.NewObjectID(), TenantID: tenantID, LocationID: locationID, SupplierID: supplierID, OrderNumber: "po-" + primitive.NewObjectID().Hex(), Status: models.PurchaseOrderDraft, DeliveryDate: deliveryDate.UTC(), Notes: strings.TrimSpace(notes), CreatedBy: userID, ReorderRecommendationID: recommendationID, IdempotencyKey: idempotencyKey, RequestHash: requestHash, Audit: []models.PurchaseOrderAuditEntry{{Action: "created", UserID: userID, At: now}}, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&order); err != nil {
		return models.PurchaseOrder{}, nil, err
	}
	if _, err := h.db.PurchaseOrders().InsertOne(ctx, order); err != nil {
		return models.PurchaseOrder{}, nil, err
	}
	lines := make([]models.PurchaseOrderLine, 0, len(lineRequests))
	for number, lineRequest := range lineRequests {
		line, lineErr := h.buildPurchaseLine(ctx, tenantID, locationID, supplierID, int32(number+1), lineRequest)
		if lineErr != nil {
			return models.PurchaseOrder{}, nil, lineErr
		}
		line.PurchaseOrderID = order.ID
		if _, err := h.db.PurchaseOrderLines().InsertOne(ctx, line); err != nil {
			return models.PurchaseOrder{}, nil, err
		}
		lines = append(lines, line)
	}
	return order, lines, nil
}

func (h *productHandler) createPurchaseOrderDraftFromRecommendation(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	recommendationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid reorder recommendation ID")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid forecast location ID")
		return
	}
	var request purchaseOrderDraftFromRecommendationRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	requestHash := purchaseOrderDraftRequestHash(recommendationID, request.SupplierItemID)

	session, err := h.db.Client.StartSession()
	if err != nil {
		apierror.Internal(w, r, "Purchase order transaction unavailable")
		return
	}
	defer session.EndSession(r.Context())

	var order models.PurchaseOrder
	var lines []models.PurchaseOrderLine
	idempotent := false
	_, err = session.WithTransaction(r.Context(), func(sc mongo.SessionContext) (interface{}, error) {
		// The idempotency lookup is deliberately inside the transaction as well
		// as before the insert. It makes retries return the exact existing draft
		// and makes a reused key with a different request an explicit conflict.
		var existing models.PurchaseOrder
		lookupErr := h.db.PurchaseOrders().FindOne(sc, bson.M{"tenantId": tenant.ID, "idempotencyKey": request.IdempotencyKey}).Decode(&existing)
		if lookupErr == nil {
			if existing.RequestHash != requestHash || existing.ReorderRecommendationID == nil || *existing.ReorderRecommendationID != recommendationID {
				return nil, ErrRecommendationIdempotency
			}
			order = existing
			lines, err = h.loadPurchaseLines(sc, tenant.ID, order.ID)
			if err != nil {
				return nil, err
			}
			idempotent = true
			return nil, nil
		}
		if !errors.Is(lookupErr, mongo.ErrNoDocuments) {
			return nil, lookupErr
		}

		var linked models.PurchaseOrder
		linkErr := h.db.PurchaseOrders().FindOne(sc, bson.M{"tenantId": tenant.ID, "reorderRecommendationId": recommendationID}).Decode(&linked)
		if linkErr == nil {
			return nil, ErrRecommendationConflict
		}
		if !errors.Is(linkErr, mongo.ErrNoDocuments) {
			return nil, linkErr
		}
		if err := h.db.Locations().FindOne(sc, bson.M{"_id": locationID, "tenantId": tenant.ID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
			return nil, ErrRecommendationNotFound
		}

		var recommendation models.ReorderRecommendation
		if err := h.db.ReorderRecommendations().FindOne(sc, bson.M{"_id": recommendationID, "tenantId": tenant.ID, "locationId": locationID}).Decode(&recommendation); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrRecommendationNotFound
			}
			return nil, err
		}
		now := time.Now().UTC()
		if recommendation.Status != models.ReorderRecommendationReady {
			return nil, ErrRecommendationState
		}
		if recommendation.ExpiresAt != nil && !recommendation.ExpiresAt.After(now) {
			return nil, ErrRecommendationStale
		}
		if recommendation.RequestedQuantityMicros <= 0 {
			return nil, ErrRecommendationState
		}

		var supplierItem models.SupplierItem
		if err := h.db.SupplierItems().FindOne(sc, bson.M{"_id": request.SupplierItemID, "tenantId": tenant.ID, "itemId": recommendation.ItemID, "isActive": true}).Decode(&supplierItem); err != nil {
			return nil, ErrPurchasingReference
		}
		var supplier models.Supplier
		if err := h.db.Suppliers().FindOne(sc, bson.M{"_id": supplierItem.SupplierID, "tenantId": tenant.ID, "isActive": true}).Decode(&supplier); err != nil {
			return nil, ErrPurchasingReference
		}
		deliveryDate, dateErr := h.nextDeliveryDate(sc, tenant.ID, recommendation.LocationID, supplier.ID, now)
		if dateErr != nil {
			return nil, dateErr
		}
		order, lines, err = h.createPurchaseOrderInTransaction(sc, tenant.ID, user.ID, recommendation.LocationID, supplier.ID, deliveryDate, "", []purchaseOrderLineRequest{{SupplierItemID: request.SupplierItemID, RequestedQuantityMicros: recommendation.RequestedQuantityMicros}}, &recommendationID, request.IdempotencyKey, requestHash)
		return nil, err
	})
	if err != nil {
		// A concurrent first request can win either uniqueness constraint. A
		// retry with the same payload should still observe that winning draft.
		if mongo.IsDuplicateKeyError(err) {
			lookupErr := h.db.PurchaseOrders().FindOne(r.Context(), bson.M{"tenantId": tenant.ID, "idempotencyKey": request.IdempotencyKey}).Decode(&order)
			if lookupErr == nil {
				if order.RequestHash == requestHash && order.ReorderRecommendationID != nil && *order.ReorderRecommendationID == recommendationID {
					lines, lookupErr = h.loadPurchaseLines(r.Context(), tenant.ID, order.ID)
					if lookupErr == nil {
						idempotent = true
						err = nil
					}
				} else {
					err = ErrRecommendationIdempotency
				}
			} else if errors.Is(lookupErr, mongo.ErrNoDocuments) {
				var linked models.PurchaseOrder
				if linkErr := h.db.PurchaseOrders().FindOne(r.Context(), bson.M{"tenantId": tenant.ID, "reorderRecommendationId": recommendationID}).Decode(&linked); linkErr == nil {
					err = ErrRecommendationConflict
				}
			}
		}
		if err != nil {
			h.purchaseError(w, r, err)
			return
		}
	}
	if !idempotent {
		h.auditPurchaseOrder(r, &order, "purchase_order.draft_from_recommendation", "Purchase order draft created from reorder recommendation")
	}
	status := http.StatusCreated
	if idempotent {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"purchaseOrder": order, "lines": lines})
}

func (h *productHandler) createPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	var request purchaseOrderCreateRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, request.LocationID); err != nil {
		apierror.NotFound(w, r, "Location not found")
		return
	}
	session, err := h.db.Client.StartSession()
	if err != nil {
		apierror.Internal(w, r, "Purchase order transaction unavailable")
		return
	}
	defer session.EndSession(r.Context())
	var order models.PurchaseOrder
	var lines []models.PurchaseOrderLine
	_, err = session.WithTransaction(r.Context(), func(sc mongo.SessionContext) (interface{}, error) {
		var createErr error
		order, lines, createErr = h.createPurchaseOrderInTransaction(sc, tenant.ID, user.ID, request.LocationID, request.SupplierID, request.DeliveryDate, request.Notes, request.Lines, nil, "", "")
		return nil, createErr
	})
	if err != nil {
		h.purchaseError(w, r, err)
		return
	}
	h.auditPurchaseOrder(r, &order, "purchase_order.created", "Purchase order created")
	writeJSON(w, http.StatusCreated, map[string]any{"purchaseOrder": order, "lines": lines})
}

func (h *productHandler) updatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["orderId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid purchase order ID")
		return
	}
	var request purchaseOrderUpdateRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	session, err := h.db.Client.StartSession()
	if err != nil {
		apierror.Internal(w, r, "Purchase order transaction unavailable")
		return
	}
	defer session.EndSession(r.Context())
	var order models.PurchaseOrder
	var lines []models.PurchaseOrderLine
	_, err = session.WithTransaction(r.Context(), func(sc mongo.SessionContext) (interface{}, error) {
		if err := h.db.PurchaseOrders().FindOne(sc, bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&order); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrPurchaseOrderNotFound
			}
			return nil, err
		}
		if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, order.LocationID); err != nil {
			return nil, ErrPurchaseOrderNotFound
		}
		if order.Status != models.PurchaseOrderDraft {
			return nil, ErrPurchaseOrderState
		}
		set := bson.M{"updatedAt": time.Now().UTC()}
		if request.DeliveryDate != nil {
			if err := h.validateDeliveryDate(sc, tenant.ID, order.LocationID, order.SupplierID, request.DeliveryDate.UTC(), time.Now()); err != nil {
				return nil, err
			}
			set["deliveryDate"] = request.DeliveryDate.UTC()
		}
		if request.Notes != nil {
			set["notes"] = strings.TrimSpace(*request.Notes)
		}
		if request.Lines != nil {
			if _, err := h.db.PurchaseOrderLines().DeleteMany(sc, bson.M{"tenantId": tenant.ID, "purchaseOrderId": id}); err != nil {
				return nil, err
			}
			lines = make([]models.PurchaseOrderLine, 0, len(*request.Lines))
			for number, lineRequest := range *request.Lines {
				line, lineErr := h.buildPurchaseLine(sc, tenant.ID, order.LocationID, order.SupplierID, int32(number+1), lineRequest)
				if lineErr != nil {
					return nil, lineErr
				}
				line.PurchaseOrderID = id
				if _, err := h.db.PurchaseOrderLines().InsertOne(sc, line); err != nil {
					return nil, err
				}
				lines = append(lines, line)
			}
		}
		if request.Lines == nil {
			lines, err = h.loadPurchaseLines(sc, tenant.ID, id)
			if err != nil {
				return nil, err
			}
		}
		result := h.db.PurchaseOrders().FindOneAndUpdate(sc, bson.M{"_id": id, "tenantId": tenant.ID, "status": models.PurchaseOrderDraft, "version": request.Version}, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}, "$push": bson.M{"audit": models.PurchaseOrderAuditEntry{Action: "updated", UserID: user.ID, At: time.Now().UTC()}}}, options.FindOneAndUpdate().SetReturnDocument(options.After))
		if err := result.Decode(&order); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrPurchaseOrderVersionConflict
			}
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		h.purchaseError(w, r, err)
		return
	}
	h.auditPurchaseOrder(r, &order, "purchase_order.updated", "Purchase order updated")
	writeJSON(w, http.StatusOK, map[string]any{"purchaseOrder": order, "lines": lines})
}

func (h *productHandler) loadPurchaseLines(ctx context.Context, tenantID, orderID primitive.ObjectID) ([]models.PurchaseOrderLine, error) {
	cur, err := h.db.PurchaseOrderLines().Find(ctx, bson.M{"tenantId": tenantID, "purchaseOrderId": orderID}, options.Find().SetSort(bson.D{{Key: "lineNumber", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	lines := []models.PurchaseOrderLine{}
	return lines, cur.All(ctx, &lines)
}

func (h *productHandler) listPurchaseOrders(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	filter := bson.M{"tenantId": tenant.ID}
	if raw := r.URL.Query().Get("locationId"); raw != "" {
		id, err := primitive.ObjectIDFromHex(raw)
		if err != nil {
			apierror.BadRequest(w, r, "Invalid location ID")
			return
		}
		if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, id); err != nil {
			apierror.NotFound(w, r, "Location not found")
			return
		}
		filter["locationId"] = id
	} else if profile, profileOK := GetStaffProfileFromContext(r.Context()); profileOK && !profile.AllLocations {
		filter["locationId"] = bson.M{"$in": profile.LocationIDs}
	}
	if raw := r.URL.Query().Get("supplierId"); raw != "" {
		id, err := primitive.ObjectIDFromHex(raw)
		if err != nil {
			apierror.BadRequest(w, r, "Invalid supplier ID")
			return
		}
		filter["supplierId"] = id
	}
	if status := r.URL.Query().Get("status"); status != "" {
		if !models.ValidPurchaseOrderStatus(models.PurchaseOrderStatus(status)) {
			apierror.BadRequest(w, r, "Invalid purchase order status")
			return
		}
		filter["status"] = status
	}
	cur, err := h.db.PurchaseOrders().Find(r.Context(), filter, options.Find().SetSort(bson.D{{Key: "deliveryDate", Value: 1}, {Key: "createdAt", Value: -1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list purchase orders")
		return
	}
	defer cur.Close(r.Context())
	orders := []models.PurchaseOrder{}
	if err := cur.All(r.Context(), &orders); err != nil {
		apierror.Internal(w, r, "Failed to list purchase orders")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"purchaseOrders": orders})
}

func (h *productHandler) getPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["orderId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid purchase order ID")
		return
	}
	var order models.PurchaseOrder
	if err := h.db.PurchaseOrders().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&order); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Purchase order not found")
		} else {
			apierror.Internal(w, r, "Failed to get purchase order")
		}
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, order.LocationID); err != nil {
		apierror.NotFound(w, r, "Purchase order not found")
		return
	}
	lines, err := h.loadPurchaseLines(r.Context(), tenant.ID, id)
	if err != nil {
		apierror.Internal(w, r, "Failed to get purchase order lines")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"purchaseOrder": order, "lines": lines})
}

func (h *productHandler) transitionPurchaseOrder(w http.ResponseWriter, r *http.Request, target models.PurchaseOrderStatus) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["orderId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid purchase order ID")
		return
	}
	var request purchaseOrderActionRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	var current models.PurchaseOrder
	if err := h.db.PurchaseOrders().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Purchase order not found")
		} else {
			apierror.Internal(w, r, "Failed to load purchase order")
		}
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, current.LocationID); err != nil {
		apierror.NotFound(w, r, "Purchase order not found")
		return
	}
	if target == models.PurchaseOrderSubmitted {
		if err := h.validateDeliveryDate(r.Context(), tenant.ID, current.LocationID, current.SupplierID, current.DeliveryDate, time.Now()); err != nil {
			h.purchaseError(w, r, err)
			return
		}
	}
	var order models.PurchaseOrder
	var set bson.M
	now := time.Now().UTC()
	switch target {
	case models.PurchaseOrderSubmitted:
		set = bson.M{"status": target, "submittedBy": user.ID, "submittedAt": now}
	case models.PurchaseOrderApproved:
		set = bson.M{"status": target, "approvedBy": user.ID, "approvedAt": now, "approvalNote": strings.TrimSpace(request.Note)}
	case models.PurchaseOrderSupplierConfirmed:
		set = bson.M{"status": target, "supplierConfirmedBy": user.ID, "supplierConfirmedAt": now}
	case models.PurchaseOrderCancelled:
		set = bson.M{"status": target, "cancelledBy": user.ID, "cancelledAt": now}
	}
	set["updatedAt"] = now
	auditAction := string(target)
	if target == models.PurchaseOrderSupplierConfirmed {
		auditAction = "supplier-confirmed"
	}
	audit := models.PurchaseOrderAuditEntry{Action: auditAction, UserID: user.ID, At: now, Note: strings.TrimSpace(request.Note)}
	filter := bson.M{"_id": id, "tenantId": tenant.ID, "version": request.Version}
	var allowed []models.PurchaseOrderStatus
	switch target {
	case models.PurchaseOrderSubmitted:
		allowed = []models.PurchaseOrderStatus{models.PurchaseOrderDraft}
	case models.PurchaseOrderApproved:
		allowed = []models.PurchaseOrderStatus{models.PurchaseOrderSubmitted}
	case models.PurchaseOrderSupplierConfirmed:
		allowed = []models.PurchaseOrderStatus{models.PurchaseOrderApproved}
	case models.PurchaseOrderCancelled:
		allowed = []models.PurchaseOrderStatus{models.PurchaseOrderDraft, models.PurchaseOrderSubmitted, models.PurchaseOrderApproved, models.PurchaseOrderOrdered}
	}
	filter["status"] = bson.M{"$in": allowed}
	result := h.db.PurchaseOrders().FindOneAndUpdate(r.Context(), filter, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}, "$push": bson.M{"audit": audit}}, options.FindOneAndUpdate().SetReturnDocument(options.After))
	if err := result.Decode(&order); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			var exists models.PurchaseOrder
			lookupErr := h.db.PurchaseOrders().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&exists)
			if errors.Is(lookupErr, mongo.ErrNoDocuments) {
				apierror.NotFound(w, r, "Purchase order not found")
			} else {
				apierror.VersionConflict(w, r, "Purchase order was modified or state changed")
			}
			return
		}
		apierror.Internal(w, r, "Failed to update purchase order")
		return
	}
	h.auditPurchaseOrder(r, &order, "purchase_order."+string(target), "Purchase order "+string(target))
	writeJSON(w, http.StatusOK, map[string]any{"purchaseOrder": order})
}

func (h *productHandler) submitPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	h.transitionPurchaseOrder(w, r, models.PurchaseOrderSubmitted)
}
func (h *productHandler) approvePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	h.transitionPurchaseOrder(w, r, models.PurchaseOrderApproved)
}
func (h *productHandler) confirmSupplierPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	h.transitionPurchaseOrder(w, r, models.PurchaseOrderSupplierConfirmed)
}
func (h *productHandler) cancelPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	h.transitionPurchaseOrder(w, r, models.PurchaseOrderCancelled)
}

func (h *productHandler) listDeliveryCalendars(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	filter := bson.M{"tenantId": tenant.ID}
	if raw := r.URL.Query().Get("locationId"); raw != "" {
		id, err := primitive.ObjectIDFromHex(raw)
		if err != nil {
			apierror.BadRequest(w, r, "Invalid location ID")
			return
		}
		if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, id); err != nil {
			apierror.NotFound(w, r, "Location not found")
			return
		}
		filter["locationId"] = id
	} else if profile, profileOK := GetStaffProfileFromContext(r.Context()); profileOK && !profile.AllLocations {
		filter["locationId"] = bson.M{"$in": profile.LocationIDs}
	}
	if raw := r.URL.Query().Get("supplierId"); raw != "" {
		id, err := primitive.ObjectIDFromHex(raw)
		if err != nil {
			apierror.BadRequest(w, r, "Invalid supplier ID")
			return
		}
		filter["supplierId"] = id
	}
	cur, err := h.db.DeliveryCalendars().Find(r.Context(), filter, options.Find().SetSort(bson.D{{Key: "locationId", Value: 1}, {Key: "supplierId", Value: 1}, {Key: "weekday", Value: 1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list delivery calendars")
		return
	}
	defer cur.Close(r.Context())
	rules := []models.DeliveryCalendarRule{}
	if err := cur.All(r.Context(), &rules); err != nil {
		apierror.Internal(w, r, "Failed to list delivery calendars")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveryCalendars": rules})
}

func (h *productHandler) createDeliveryCalendar(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	var request deliveryCalendarRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, request.LocationID); err != nil {
		apierror.NotFound(w, r, "Location not found")
		return
	}
	now := time.Now().UTC()
	rule := models.DeliveryCalendarRule{ID: primitive.NewObjectID(), TenantID: tenant.ID, LocationID: request.LocationID, SupplierID: request.SupplierID, Weekday: request.Weekday, CutoffMinutes: request.CutoffMinutes, LeadTimeDays: request.LeadTimeDays, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&rule); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	if err := h.validateCalendarReferences(r.Context(), tenant.ID, rule.LocationID, rule.SupplierID); err != nil {
		h.purchaseError(w, r, err)
		return
	}
	if _, err := h.db.DeliveryCalendars().InsertOne(r.Context(), rule); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			apierror.Conflict(w, r, "Delivery calendar rule already exists")
		} else {
			apierror.Internal(w, r, "Failed to create delivery calendar")
		}
		return
	}
	h.auditDeliveryCalendar(r, &rule, user.ID, "delivery_calendar.created")
	writeJSON(w, http.StatusCreated, map[string]any{"deliveryCalendar": rule})
}

func (h *productHandler) updateDeliveryCalendar(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["calendarId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid delivery calendar ID")
		return
	}
	var request deliveryCalendarUpdateRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	var current models.DeliveryCalendarRule
	if err := h.db.DeliveryCalendars().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Delivery calendar rule not found")
		} else {
			apierror.Internal(w, r, "Failed to load delivery calendar")
		}
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, current.LocationID); err != nil {
		apierror.NotFound(w, r, "Delivery calendar rule not found")
		return
	}
	set := bson.M{"updatedAt": time.Now().UTC()}
	if request.Weekday != nil {
		set["weekday"] = *request.Weekday
	}
	if request.CutoffMinutes != nil {
		set["cutoffMinutes"] = *request.CutoffMinutes
	}
	if request.LeadTimeDays != nil {
		set["leadTimeDays"] = *request.LeadTimeDays
	}
	if request.IsActive != nil {
		set["isActive"] = *request.IsActive
	}
	candidate := current
	if request.Weekday != nil {
		candidate.Weekday = *request.Weekday
	}
	if request.CutoffMinutes != nil {
		candidate.CutoffMinutes = *request.CutoffMinutes
	}
	if request.LeadTimeDays != nil {
		candidate.LeadTimeDays = *request.LeadTimeDays
	}
	if request.IsActive != nil {
		candidate.IsActive = *request.IsActive
	}
	candidate.Version++
	candidate.UpdatedAt = set["updatedAt"].(time.Time)
	if err := validation.Validate(&candidate); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	var result models.DeliveryCalendarRule
	err = h.db.DeliveryCalendars().FindOneAndUpdate(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID, "version": request.Version}, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.VersionConflict(w, r, "Delivery calendar was modified")
		} else if mongo.IsDuplicateKeyError(err) {
			apierror.Conflict(w, r, "Delivery calendar rule already exists")
		} else {
			apierror.Internal(w, r, "Failed to update delivery calendar")
		}
		return
	}
	h.auditDeliveryCalendar(r, &result, user.ID, "delivery_calendar.updated")
	writeJSON(w, http.StatusOK, map[string]any{"deliveryCalendar": result})
}

func (h *productHandler) deleteDeliveryCalendar(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["calendarId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid delivery calendar ID")
		return
	}
	var rule models.DeliveryCalendarRule
	if err := h.db.DeliveryCalendars().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&rule); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Delivery calendar rule not found")
		} else {
			apierror.Internal(w, r, "Failed to load delivery calendar")
		}
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, rule.LocationID); err != nil {
		apierror.NotFound(w, r, "Delivery calendar rule not found")
		return
	}
	if _, err := h.db.DeliveryCalendars().DeleteOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}); err != nil {
		apierror.Internal(w, r, "Failed to delete delivery calendar")
		return
	}
	h.auditDeliveryCalendar(r, &rule, user.ID, "delivery_calendar.deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (h *productHandler) validateCalendarReferences(ctx context.Context, tenantID, locationID, supplierID primitive.ObjectID) error {
	if err := h.db.Locations().FindOne(ctx, bson.M{"_id": locationID, "tenantId": tenantID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		return ErrPurchasingReference
	}
	if err := h.db.Suppliers().FindOne(ctx, bson.M{"_id": supplierID, "tenantId": tenantID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		return ErrPurchasingReference
	}
	return nil
}

func (h *productHandler) purchaseError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrRecommendationNotFound):
		apierror.NotFound(w, r, "Reorder recommendation not found")
	case errors.Is(err, ErrRecommendationStale), errors.Is(err, ErrRecommendationState):
		apierror.Conflict(w, r, err.Error())
	case errors.Is(err, ErrRecommendationConflict), errors.Is(err, ErrRecommendationIdempotency):
		apierror.Conflict(w, r, err.Error())
	case errors.Is(err, ErrPurchaseOrderNotFound):
		apierror.NotFound(w, r, "Purchase order not found")
	case errors.Is(err, ErrPurchaseOrderVersionConflict):
		apierror.VersionConflict(w, r, "Purchase order was modified")
	case errors.Is(err, ErrPurchaseOrderState):
		apierror.Conflict(w, r, "Purchase order state does not allow this operation")
	case errors.Is(err, ErrDeliveryDateInvalid):
		apierror.Validation(w, r, "Delivery date is not valid for the supplier calendar")
	case errors.Is(err, ErrPurchasingReference), errors.Is(err, ErrPurchaseRoundingOverflow):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Purchase order operation failed")
	}
}

func (h *productHandler) auditPurchaseOrder(r *http.Request, order *models.PurchaseOrder, action, message string) {
	if h.logger == nil || order == nil {
		return
	}
	user, userOK := middleware.GetUserFromContext(r.Context())
	tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
	if userOK && tenantOK {
		h.logger.LogTenantActivity(r.Context(), models.LogMedium, message, user.ID, tenant.ID, action, map[string]interface{}{"purchaseOrderId": order.ID.Hex(), "status": order.Status, "version": order.Version})
	}
}
func (h *productHandler) auditDeliveryCalendar(r *http.Request, rule *models.DeliveryCalendarRule, userID primitive.ObjectID, action string) {
	if h.logger == nil || rule == nil {
		return
	}
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if ok {
		h.logger.LogTenantActivity(r.Context(), models.LogLow, "Delivery calendar updated", userID, tenant.ID, action, map[string]interface{}{"deliveryCalendarId": rule.ID.Hex(), "locationId": rule.LocationID.Hex(), "supplierId": rule.SupplierID.Hex()})
	}
}
