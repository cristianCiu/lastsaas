package product

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"lastsaas/internal/apierror"
	"lastsaas/internal/inventory"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type goodsReceiptLineRequest struct {
	PurchaseOrderLineID  primitive.ObjectID `json:"purchaseOrderLineId" validate:"required"`
	ReceivedQuantity     string             `json:"receivedQuantity" validate:"required"`
	ActualUnitPriceMinor int64              `json:"actualUnitPriceMinor" validate:"gte=0"`
	VarianceReason       string             `json:"varianceReason,omitempty" validate:"omitempty,max=500,not_blank"`
	LotCode              string             `json:"lotCode,omitempty" validate:"omitempty,max=128,not_blank"`
	ExpiresAt            *time.Time         `json:"expiresAt,omitempty"`
}

type goodsReceiptCreateRequest struct {
	StorageAreaID  primitive.ObjectID        `json:"storageAreaId" validate:"required"`
	ReceivedAt     *time.Time                `json:"receivedAt,omitempty"`
	IdempotencyKey string                    `json:"idempotencyKey,omitempty"`
	Lines          []goodsReceiptLineRequest `json:"lines" validate:"required,min=1,max=1000,dive"`
}

type goodsReceiptReverseRequest struct {
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

func RequirePurchasingReceiveReadPermission() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile, ok := GetStaffProfileFromContext(r.Context())
			if !ok || (!HasBusinessPermission(profile, models.PermissionPurchasingRead) && !HasBusinessPermission(profile, models.PermissionPurchasingReceive)) {
				apierror.Write(w, http.StatusForbidden, apierror.Code("BUSINESS_PERMISSION_DENIED"), "Business permission denied", r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *productHandler) createGoodsReceipt(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	orderID, err := primitive.ObjectIDFromHex(mux.Vars(r)["orderId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid purchase order ID")
		return
	}
	var request goodsReceiptCreateRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	var order models.PurchaseOrder
	if err := h.db.PurchaseOrders().FindOne(r.Context(), bson.M{"_id": orderID, "tenantId": tenant.ID}).Decode(&order); err != nil {
		apierror.NotFound(w, r, "Purchase order not found")
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, order.LocationID); err != nil {
		apierror.NotFound(w, r, "Purchase order not found")
		return
	}
	profile, _ := GetStaffProfileFromContext(r.Context())
	command := inventory.GoodsReceiptCommand{TenantID: tenant.ID, UserID: user.ID, LocationID: order.LocationID, StorageAreaID: request.StorageAreaID, PurchaseOrderID: orderID, IdempotencyKey: request.IdempotencyKey, VarianceApproved: profile != nil && (profile.BusinessRole == models.BusinessRoleCompanyOwner || profile.BusinessRole == models.BusinessRoleOperationsManager)}
	if request.ReceivedAt != nil {
		command.ReceivedAt = request.ReceivedAt.UTC()
	}
	for _, line := range request.Lines {
		quantity, parseErr := inventory.ParseQuantity(line.ReceivedQuantity)
		if parseErr != nil {
			apierror.Validation(w, r, parseErr.Error())
			return
		}
		command.Lines = append(command.Lines, inventory.GoodsReceiptLineCommand{PurchaseOrderLineID: line.PurchaseOrderLineID, ReceivedQuantityMicros: quantity, ActualUnitPriceMinor: line.ActualUnitPriceMinor, VarianceReason: line.VarianceReason, LotCode: line.LotCode, ExpiresAt: line.ExpiresAt})
	}
	result, err := inventory.NewService(h.db).ReceiveGoodsReceipt(r.Context(), command)
	if err != nil {
		writeGoodsReceiptError(w, r, err)
		return
	}
	h.auditGoodsReceipt(r, tenant, result.Receipt, "goods_receipt.posted", "Goods receipt posted")
	writeJSON(w, http.StatusCreated, result)
}

func (h *productHandler) listGoodsReceipts(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	var locationID primitive.ObjectID
	if raw := r.URL.Query().Get("locationId"); raw != "" {
		var err error
		locationID, err = primitive.ObjectIDFromHex(raw)
		if err != nil {
			apierror.BadRequest(w, r, "Invalid location ID")
			return
		}
		if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, locationID); err != nil {
			apierror.NotFound(w, r, "Location not found")
			return
		}
	} else if profile, profileOK := GetStaffProfileFromContext(r.Context()); profileOK && !profile.AllLocations { // service query is narrowed to assigned locations below
		// The list endpoint is intentionally location-scoped; callers without a
		// single location use the first-class assigned-location filter.
		receipts, err := h.db.GoodsReceipts().Find(r.Context(), bson.M{"tenantId": tenant.ID, "locationId": bson.M{"$in": profile.LocationIDs}}, options.Find().SetSort(bson.D{{Key: "receivedAt", Value: -1}}))
		if err != nil {
			apierror.Internal(w, r, "Failed to list goods receipts")
			return
		}
		defer receipts.Close(r.Context())
		var rows []models.GoodsReceipt
		if err := receipts.All(r.Context(), &rows); err != nil {
			apierror.Internal(w, r, "Failed to list goods receipts")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"goodsReceipts": rows})
		return
	}
	rows, err := inventory.NewService(h.db).ListGoodsReceipts(r.Context(), tenant.ID, locationID, primitive.NilObjectID)
	if err != nil {
		apierror.Internal(w, r, "Failed to list goods receipts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"goodsReceipts": rows})
}

func (h *productHandler) getGoodsReceipt(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["receiptId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid goods receipt ID")
		return
	}
	var receipt models.GoodsReceipt
	if err := h.db.GoodsReceipts().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&receipt); err != nil {
		apierror.NotFound(w, r, "Goods receipt not found")
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, receipt.LocationID); err != nil {
		apierror.NotFound(w, r, "Goods receipt not found")
		return
	}
	result, err := inventory.NewService(h.db).GetGoodsReceipt(r.Context(), tenant.ID, receipt.LocationID, id)
	if err != nil {
		writeGoodsReceiptError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *productHandler) listOrderGoodsReceipts(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	orderID, err := primitive.ObjectIDFromHex(mux.Vars(r)["orderId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid purchase order ID")
		return
	}
	var order models.PurchaseOrder
	if err := h.db.PurchaseOrders().FindOne(r.Context(), bson.M{"_id": orderID, "tenantId": tenant.ID}).Decode(&order); err != nil {
		apierror.NotFound(w, r, "Purchase order not found")
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, order.LocationID); err != nil {
		apierror.NotFound(w, r, "Purchase order not found")
		return
	}
	rows, err := inventory.NewService(h.db).ListGoodsReceipts(r.Context(), tenant.ID, order.LocationID, orderID)
	if err != nil {
		apierror.Internal(w, r, "Failed to list goods receipts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"goodsReceipts": rows})
}

func (h *productHandler) reverseGoodsReceipt(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["receiptId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid goods receipt ID")
		return
	}
	var request goodsReceiptReverseRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	var receipt models.GoodsReceipt
	if err := h.db.GoodsReceipts().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&receipt); err != nil {
		apierror.NotFound(w, r, "Goods receipt not found")
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, receipt.LocationID); err != nil {
		apierror.NotFound(w, r, "Goods receipt not found")
		return
	}
	result, err := inventory.NewService(h.db).ReverseGoodsReceipt(r.Context(), tenant.ID, user.ID, receipt.LocationID, id, request.IdempotencyKey)
	if err != nil {
		writeGoodsReceiptError(w, r, err)
		return
	}
	h.auditGoodsReceipt(r, tenant, result.Receipt, "goods_receipt.cancelled", "Goods receipt reversed")
	writeJSON(w, http.StatusCreated, result)
}

func (h *productHandler) auditGoodsReceipt(r *http.Request, tenant *models.Tenant, receipt models.GoodsReceipt, action, message string) {
	if h.logger == nil {
		return
	}
	if user, ok := middleware.GetUserFromContext(r.Context()); ok {
		h.logger.LogTenantActivity(r.Context(), models.LogMedium, message, user.ID, tenant.ID, action, map[string]interface{}{"goodsReceiptId": receipt.ID.Hex(), "purchaseOrderId": receipt.PurchaseOrderID.Hex(), "locationId": receipt.LocationID.Hex()})
	}
}

func writeGoodsReceiptError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, inventory.ErrGoodsReceiptNotFound), errors.Is(err, inventory.ErrReceiptLineNotFound):
		apierror.NotFound(w, r, err.Error())
	case errors.Is(err, inventory.ErrPurchaseOrderNotReady), errors.Is(err, inventory.ErrReceiptOverage), errors.Is(err, inventory.ErrReceiptVarianceReason), errors.Is(err, inventory.ErrGoodsReceiptState), errors.Is(err, inventory.ErrPostingAlreadyReversed), errors.Is(err, inventory.ErrIdempotencyConflict):
		apierror.Conflict(w, r, err.Error())
	case errors.Is(err, inventory.ErrInvalidReference), errors.Is(err, inventory.ErrLotRequired), errors.Is(err, inventory.ErrDuplicateLot), errors.Is(err, inventory.ErrQuantityOverflow), strings.HasPrefix(err.Error(), "validation failed:"):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Goods receipt operation failed")
	}
}
