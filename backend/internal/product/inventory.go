package product

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lastsaas/internal/apierror"
	"lastsaas/internal/db"
	"lastsaas/internal/inventory"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type inventoryWriteRequest struct {
	StorageAreaID  primitive.ObjectID  `json:"storageAreaId"`
	ItemID         primitive.ObjectID  `json:"itemId"`
	Quantity       string              `json:"quantity"`
	IdempotencyKey string              `json:"idempotencyKey"`
	EffectiveAt    *time.Time          `json:"effectiveAt"`
	LotID          *primitive.ObjectID `json:"lotId"`
	Reason         string              `json:"reason"`
}

type transferRequest struct {
	DestinationLocationID    primitive.ObjectID  `json:"destinationLocationId"`
	DestinationStorageAreaID primitive.ObjectID  `json:"destinationStorageAreaId"`
	SourceStorageAreaID      primitive.ObjectID  `json:"sourceStorageAreaId"`
	ItemID                   primitive.ObjectID  `json:"itemId"`
	Quantity                 string              `json:"quantity"`
	LotID                    *primitive.ObjectID `json:"lotId"`
	Reason                   string              `json:"reason"`
	IdempotencyKey           string              `json:"idempotencyKey"`
	EffectiveAt              *time.Time          `json:"effectiveAt"`
}

type lotRequest struct {
	Code       string           `json:"code"`
	ExpiresAt  *time.Time       `json:"expiresAt"`
	ReceivedAt *time.Time       `json:"receivedAt"`
	Status     models.LotStatus `json:"status"`
}

type inventoryReverseRequest struct {
	IdempotencyKey string `json:"idempotencyKey"`
}

type stockCountCreateRequest struct {
	StorageAreaID  primitive.ObjectID `json:"storageAreaId"`
	IdempotencyKey string             `json:"idempotencyKey"`
}

type stockCountVersionRequest struct {
	Version int64 `json:"version"`
}

type stockCountLineRequest struct {
	CountVersion    int64  `json:"countVersion"`
	LineVersion     int64  `json:"lineVersion"`
	CountedQuantity string `json:"countedQuantity"`
	Note            string `json:"note"`
}

type stockCountPostRequest struct {
	IdempotencyKey string `json:"idempotencyKey"`
}

type stockCountDiscoverLineRequest struct {
	CountVersion int64               `json:"countVersion"`
	ItemID       primitive.ObjectID  `json:"itemId"`
	LotID        *primitive.ObjectID `json:"lotId"`
}

type reconciliationRequest struct {
	StorageAreaID  *primitive.ObjectID `json:"storageAreaId"`
	IdempotencyKey string              `json:"idempotencyKey"`
	Repair         bool                `json:"repair"`
}

func (h *productHandler) postOpeningBalance(w http.ResponseWriter, r *http.Request) {
	h.postInventory(w, r, true)
}

func (h *productHandler) postAdjustment(w http.ResponseWriter, r *http.Request) {
	h.postInventory(w, r, false)
}

func (h *productHandler) postInventory(w http.ResponseWriter, r *http.Request, opening bool) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	var request inventoryWriteRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	quantity, err := inventory.ParseQuantity(request.Quantity)
	if err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	profile, _ := GetStaffProfileFromContext(r.Context())
	command := inventory.Command{TenantID: tenant.ID, UserID: userID, LocationID: locationID, StorageAreaID: request.StorageAreaID, ItemID: request.ItemID, QuantityMicros: quantity, IdempotencyKey: strings.TrimSpace(request.IdempotencyKey), LotID: request.LotID, Reason: request.Reason, LotOverride: profile != nil && HasBusinessPermission(profile, models.PermissionInventoryLotOverride)}
	if request.EffectiveAt != nil {
		command.EffectiveAt = request.EffectiveAt.UTC()
	}
	service := inventory.NewService(h.db)
	var result *inventory.Result
	if opening {
		result, err = service.OpeningBalance(r.Context(), command)
	} else {
		result, err = service.ManualAdjustment(r.Context(), command)
	}
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventory(r, tenant, result.Posting, opening)
	writeJSON(w, http.StatusCreated, result)
}

func (h *productHandler) postWaste(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	var request inventoryWriteRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	quantity, err := inventory.ParseQuantity(request.Quantity)
	if err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	profile, _ := GetStaffProfileFromContext(r.Context())
	command := inventory.WasteCommand{Command: inventory.Command{TenantID: tenant.ID, UserID: userID, LocationID: locationID, StorageAreaID: request.StorageAreaID, ItemID: request.ItemID, QuantityMicros: quantity, IdempotencyKey: strings.TrimSpace(request.IdempotencyKey), LotID: request.LotID, Reason: request.Reason, LotOverride: profile != nil && HasBusinessPermission(profile, models.PermissionInventoryLotOverride)}}
	if request.EffectiveAt != nil {
		command.EffectiveAt = request.EffectiveAt.UTC()
	}
	result, err := inventory.NewService(h.db).Waste(r.Context(), command)
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventory(r, tenant, result.Posting, false)
	writeJSON(w, http.StatusCreated, result)
}

func (h *productHandler) postTransfer(w http.ResponseWriter, r *http.Request) {
	tenant, sourceLocationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	var request transferRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	quantity, err := inventory.ParseQuantity(request.Quantity)
	if err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	profile, _ := GetStaffProfileFromContext(r.Context())
	command := inventory.TransferCommand{TenantID: tenant.ID, UserID: userID, SourceLocationID: sourceLocationID, SourceStorageAreaID: request.SourceStorageAreaID, DestinationLocationID: request.DestinationLocationID, DestinationStorageAreaID: request.DestinationStorageAreaID, ItemID: request.ItemID, QuantityMicros: quantity, LotID: request.LotID, Reason: request.Reason, LotOverride: profile != nil && HasBusinessPermission(profile, models.PermissionInventoryLotOverride), IdempotencyKey: strings.TrimSpace(request.IdempotencyKey)}
	if request.EffectiveAt != nil {
		command.EffectiveAt = request.EffectiveAt.UTC()
	}
	result, err := inventory.NewService(h.db).Transfer(r.Context(), command)
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventory(r, tenant, result.Posting, false)
	writeJSON(w, http.StatusCreated, result)
}

func (h *productHandler) createInventoryLot(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	itemID, err := primitive.ObjectIDFromHex(mux.Vars(r)["itemId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid item ID")
		return
	}
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Authenticated user required")
		return
	}
	var request lotRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	command := inventory.LotCommand{TenantID: tenant.ID, UserID: user.ID, ItemID: itemID, Code: request.Code, ExpiresAt: request.ExpiresAt, Status: request.Status}
	if request.ReceivedAt != nil {
		command.ReceivedAt = request.ReceivedAt.UTC()
	}
	lot, err := inventory.NewService(h.db).CreateLot(r.Context(), command)
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	if actor, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
		h.logger.LogTenantActivity(r.Context(), models.LogMedium, "Inventory lot created", actor.ID, tenant.ID, "inventory.lot.created", map[string]interface{}{"lotId": lot.ID.Hex(), "itemId": lot.ItemID.Hex()})
	}
	writeJSON(w, http.StatusCreated, map[string]any{"lot": lot})
}

func (h *productHandler) listInventoryLots(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	itemID, err := primitive.ObjectIDFromHex(mux.Vars(r)["itemId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid item ID")
		return
	}
	lots, err := inventory.NewService(h.db).ListLots(r.Context(), tenant.ID, itemID)
	if err != nil {
		apierror.Internal(w, r, "Failed to list inventory lots")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lots": lots})
}

func (h *productHandler) updateInventoryLot(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	itemID, err := primitive.ObjectIDFromHex(mux.Vars(r)["itemId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid item ID")
		return
	}
	lotID, err := primitive.ObjectIDFromHex(mux.Vars(r)["lotId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid lot ID")
		return
	}
	var request struct {
		Status models.LotStatus `json:"status"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	lot, err := inventory.NewService(h.db).UpdateLotStatus(r.Context(), tenant.ID, itemID, lotID, request.Status)
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	if actor, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
		h.logger.LogTenantActivity(r.Context(), models.LogMedium, "Inventory lot status updated", actor.ID, tenant.ID, "inventory.lot.updated", map[string]interface{}{"lotId": lot.ID.Hex(), "itemId": lot.ItemID.Hex(), "status": lot.Status})
	}
	writeJSON(w, http.StatusOK, map[string]any{"lot": lot})
}

func (h *productHandler) updateLotTrackingMode(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	itemID, err := primitive.ObjectIDFromHex(mux.Vars(r)["itemId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid item ID")
		return
	}
	var request struct {
		Version int64                  `json:"version"`
		Mode    models.LotTrackingMode `json:"mode"`
	}
	if !decodeStrict(w, r, &request) {
		return
	}
	item, err := inventory.NewService(h.db).SetLotTrackingMode(r.Context(), tenant.ID, itemID, request.Mode, request.Version)
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	if actor, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
		h.logger.LogTenantActivity(r.Context(), models.LogMedium, "Inventory lot tracking mode updated", actor.ID, tenant.ID, "inventory.item_lot_tracking.updated", map[string]interface{}{"itemId": item.ID.Hex(), "lotTracking": item.LotTracking, "version": item.Version})
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (h *productHandler) createStockCount(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	var request stockCountCreateRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	result, err := inventory.NewService(h.db).CreateCount(r.Context(), inventory.CountCreateCommand{TenantID: tenant.ID, UserID: userID, LocationID: locationID, StorageAreaID: request.StorageAreaID, IdempotencyKey: strings.TrimSpace(request.IdempotencyKey)})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventoryCount(r, tenant, "inventory.count.created", "Inventory count created", result.Count.ID)
	writeJSON(w, http.StatusCreated, result)
}

func (h *productHandler) listStockCounts(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, _, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	if state := query.Get("state"); state != "" && state != "active" {
		apierror.Validation(w, r, "state must be active")
		return
	}
	var limit int64
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			apierror.Validation(w, r, "limit must be an integer")
			return
		}
		if parsed < 1 {
			apierror.Validation(w, r, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	result, err := inventory.NewService(h.db).ListActiveCounts(r.Context(), inventory.ActiveCountListCommand{TenantID: tenant.ID, LocationID: locationID, Limit: limit, Cursor: query.Get("cursor")})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *productHandler) getStockCount(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, _, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	countID, err := primitive.ObjectIDFromHex(mux.Vars(r)["countId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid count ID")
		return
	}
	result, err := inventory.NewService(h.db).GetCount(r.Context(), tenant.ID, countID, locationID)
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *productHandler) listStockCountLotOptions(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, _, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	countID, err := primitive.ObjectIDFromHex(mux.Vars(r)["countId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid count ID")
		return
	}
	itemID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("itemId"))
	if err != nil {
		apierror.BadRequest(w, r, "Invalid item ID")
		return
	}
	result, err := inventory.NewService(h.db).ListCountLotOptions(r.Context(), inventory.CountLotOptionsCommand{TenantID: tenant.ID, LocationID: locationID, CountID: countID, ItemID: itemID})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *productHandler) freezeStockCount(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	countID, err := primitive.ObjectIDFromHex(mux.Vars(r)["countId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid count ID")
		return
	}
	var request stockCountVersionRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	result, err := inventory.NewService(h.db).FreezeCount(r.Context(), inventory.CountVersionCommand{TenantID: tenant.ID, UserID: userID, LocationID: locationID, CountID: countID, Version: request.Version})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventoryCount(r, tenant, "inventory.count.frozen", "Inventory count frozen", result.Count.ID)
	writeJSON(w, http.StatusOK, result)
}

func (h *productHandler) recordStockCountLine(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	countID, err := primitive.ObjectIDFromHex(mux.Vars(r)["countId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid count ID")
		return
	}
	lineID, err := primitive.ObjectIDFromHex(mux.Vars(r)["lineId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid count line ID")
		return
	}
	var request stockCountLineRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	quantity, err := inventory.ParseQuantity(request.CountedQuantity)
	if err != nil || quantity < 0 {
		apierror.Validation(w, r, "counted quantity must be a non-negative decimal")
		return
	}
	line, err := inventory.NewService(h.db).RecordCountLine(r.Context(), inventory.CountLineCommand{CountVersionCommand: inventory.CountVersionCommand{TenantID: tenant.ID, UserID: userID, LocationID: locationID, CountID: countID, Version: request.CountVersion}, LineID: lineID, CountedQuantityMicros: quantity, Note: request.Note, LineVersion: request.LineVersion})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventoryCount(r, tenant, "inventory.count.line_recorded", "Inventory count line recorded", line.ID)
	writeJSON(w, http.StatusOK, map[string]any{"line": line})
}

func (h *productHandler) discoverStockCountLine(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	countID, err := primitive.ObjectIDFromHex(mux.Vars(r)["countId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid count ID")
		return
	}
	var request stockCountDiscoverLineRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	line, err := inventory.NewService(h.db).DiscoverCountLine(r.Context(), inventory.CountDiscoverLineCommand{CountVersionCommand: inventory.CountVersionCommand{TenantID: tenant.ID, UserID: userID, LocationID: locationID, CountID: countID, Version: request.CountVersion}, ItemID: request.ItemID, LotID: request.LotID})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventoryCount(r, tenant, "inventory.count.line_discovered", "Inventory count line discovered", line.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"line": line})
}

func (h *productHandler) reviewStockCount(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	countID, err := primitive.ObjectIDFromHex(mux.Vars(r)["countId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid count ID")
		return
	}
	var request stockCountVersionRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	result, err := inventory.NewService(h.db).ReviewCount(r.Context(), inventory.CountVersionCommand{TenantID: tenant.ID, UserID: userID, LocationID: locationID, CountID: countID, Version: request.Version})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventoryCount(r, tenant, "inventory.count.reviewed", "Inventory count reviewed", result.Count.ID)
	writeJSON(w, http.StatusOK, result)
}

func (h *productHandler) cancelStockCount(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	countID, err := primitive.ObjectIDFromHex(mux.Vars(r)["countId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid count ID")
		return
	}
	var request stockCountVersionRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	result, err := inventory.NewService(h.db).CancelCount(r.Context(), inventory.CountVersionCommand{TenantID: tenant.ID, UserID: userID, LocationID: locationID, CountID: countID, Version: request.Version})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventoryCount(r, tenant, "inventory.count.cancelled", "Inventory count cancelled", result.Count.ID)
	writeJSON(w, http.StatusOK, result)
}

func (h *productHandler) postStockCount(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	countID, err := primitive.ObjectIDFromHex(mux.Vars(r)["countId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid count ID")
		return
	}
	var request stockCountPostRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	result, err := inventory.NewService(h.db).PostCount(r.Context(), inventory.CountPostCommand{TenantID: tenant.ID, UserID: userID, LocationID: locationID, CountID: countID, IdempotencyKey: strings.TrimSpace(request.IdempotencyKey)})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventoryCount(r, tenant, "inventory.count.posted", "Inventory count posted", result.Count.ID)
	writeJSON(w, http.StatusCreated, result)
}

func (h *productHandler) runInventoryReconciliation(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	var request reconciliationRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	service := inventory.NewService(h.db).SetLogger(h.logger)
	result, err := service.RunReconciliation(r.Context(), inventory.ReconciliationCommand{TenantID: tenant.ID, UserID: userID, LocationID: locationID, StorageAreaID: request.StorageAreaID, IdempotencyKey: strings.TrimSpace(request.IdempotencyKey), Repair: request.Repair})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventoryCount(r, tenant, "inventory.reconciliation.completed", "Inventory reconciliation completed", result.Run.ID)
	writeJSON(w, http.StatusCreated, result)
}

func (h *productHandler) getInventoryReconciliation(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, _, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	runID, err := primitive.ObjectIDFromHex(mux.Vars(r)["runId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid reconciliation ID")
		return
	}
	run, err := inventory.NewService(h.db).GetReconciliation(r.Context(), tenant.ID, runID, locationID)
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}

func (h *productHandler) reverseInventoryPosting(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, userID, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	postingID, err := primitive.ObjectIDFromHex(mux.Vars(r)["postingId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid posting ID")
		return
	}
	var request inventoryReverseRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	result, err := inventory.NewService(h.db).Reverse(r.Context(), inventory.ReverseCommand{TenantID: tenant.ID, UserID: userID, LocationID: locationID, PostingID: postingID, IdempotencyKey: strings.TrimSpace(request.IdempotencyKey)})
	if err != nil {
		writeInventoryError(w, r, err)
		return
	}
	h.auditInventory(r, tenant, result.Posting, false)
	writeJSON(w, http.StatusCreated, result)
}

func (h *productHandler) listInventoryJournal(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, _, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	var itemID *primitive.ObjectID
	if value := r.URL.Query().Get("itemId"); value != "" {
		parsed, err := primitive.ObjectIDFromHex(value)
		if err != nil {
			apierror.BadRequest(w, r, "Invalid item ID")
			return
		}
		itemID = &parsed
	}
	entries, err := inventory.NewService(h.db).ListJournal(r.Context(), tenant.ID, locationID, itemID)
	if err != nil {
		apierror.Internal(w, r, "Failed to list inventory journal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"journal": entries})
}

func (h *productHandler) listInventoryBalances(w http.ResponseWriter, r *http.Request) {
	tenant, locationID, _, ok := inventoryRequestScope(w, r)
	if !ok {
		return
	}
	balances, err := inventory.NewService(h.db).ListBalances(r.Context(), tenant.ID, locationID)
	if err != nil {
		apierror.Internal(w, r, "Failed to list inventory balances")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"balances": balances})
}

type inventoryItemReference struct {
	ID   primitive.ObjectID `json:"id" bson:"_id"`
	SKU  string             `json:"sku" bson:"sku"`
	Name string             `json:"name" bson:"name"`
}

type inventoryStorageAreaReference struct {
	ID         primitive.ObjectID     `json:"id" bson:"_id"`
	LocationID primitive.ObjectID     `json:"locationId" bson:"locationId"`
	Name       string                 `json:"name" bson:"name"`
	Type       models.StorageAreaType `json:"type" bson:"type"`
}

type inventoryReferences struct {
	Items        []inventoryItemReference        `json:"items"`
	StorageAreas []inventoryStorageAreaReference `json:"storageAreas"`
}

// listInventoryReferences is deliberately separate from catalog and storage
// area reads. Inventory principals need stable selectors for posting, but do
// not thereby gain access to the broader master-data APIs.
func (h *productHandler) listInventoryReferences(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	profile, ok := GetStaffProfileFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Active staff profile required")
		return
	}

	// The path form is used by location-scoped inventory clients. Keep the
	// query form for callers that use the references selector endpoint; in
	// either case a requested location is checked against this tenant and the
	// caller's assigned locations before any references are returned.
	requestedLocation := mux.Vars(r)["locationId"]
	if requestedLocation == "" {
		requestedLocation = r.URL.Query().Get("locationId")
	}
	var requestedLocationID primitive.ObjectID
	var err error
	if requestedLocation != "" {
		requestedLocationID, err = primitive.ObjectIDFromHex(requestedLocation)
		if err != nil {
			apierror.BadRequest(w, r, "Invalid location ID")
			return
		}
		if !profile.AllLocations && !profileHasLocation(profile, requestedLocationID) {
			apierror.NotFound(w, r, "Location not found")
			return
		}
	}

	locationFilter := bson.M{"tenantId": tenant.ID, "isActive": true}
	if !requestedLocationID.IsZero() {
		locationFilter["_id"] = requestedLocationID
	} else if !profile.AllLocations {
		locationFilter["_id"] = bson.M{"$in": profile.LocationIDs}
	}
	locationCursor, err := h.db.Locations().Find(r.Context(), locationFilter, options.Find().SetProjection(bson.M{"_id": 1}).SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list inventory locations")
		return
	}
	defer locationCursor.Close(r.Context())
	var locations []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := locationCursor.All(r.Context(), &locations); err != nil {
		apierror.Internal(w, r, "Failed to list inventory locations")
		return
	}
	locationIDs := make([]primitive.ObjectID, 0, len(locations))
	for _, location := range locations {
		locationIDs = append(locationIDs, location.ID)
	}
	if !requestedLocationID.IsZero() && len(locationIDs) == 0 {
		// Use the same non-disclosing response as the existing location
		// authorization middleware when the location is foreign or inactive.
		apierror.NotFound(w, r, "Location not found")
		return
	}

	itemsCursor, err := h.db.Items().Find(r.Context(), bson.M{"tenantId": tenant.ID, "isActive": true, "stockable": true}, options.Find().SetProjection(bson.M{"_id": 1, "sku": 1, "name": 1}).SetSort(bson.D{{Key: "sku", Value: 1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list inventory items")
		return
	}
	defer itemsCursor.Close(r.Context())
	items := make([]inventoryItemReference, 0)
	if err := itemsCursor.All(r.Context(), &items); err != nil {
		apierror.Internal(w, r, "Failed to list inventory items")
		return
	}

	areas := make([]inventoryStorageAreaReference, 0)
	if len(locationIDs) > 0 {
		areaCursor, err := h.db.StorageAreas().Find(r.Context(), bson.M{"tenantId": tenant.ID, "locationId": bson.M{"$in": locationIDs}, "isActive": true}, options.Find().SetProjection(bson.M{"_id": 1, "locationId": 1, "name": 1, "type": 1}).SetSort(bson.D{{Key: "locationId", Value: 1}, {Key: "name", Value: 1}}))
		if err != nil {
			apierror.Internal(w, r, "Failed to list inventory storage areas")
			return
		}
		defer areaCursor.Close(r.Context())
		if err := areaCursor.All(r.Context(), &areas); err != nil {
			apierror.Internal(w, r, "Failed to list inventory storage areas")
			return
		}
	}
	writeJSON(w, http.StatusOK, inventoryReferences{Items: items, StorageAreas: areas})
}

func inventoryRequestScope(w http.ResponseWriter, r *http.Request) (*models.Tenant, primitive.ObjectID, primitive.ObjectID, bool) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return nil, primitive.NilObjectID, primitive.NilObjectID, false
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return nil, primitive.NilObjectID, primitive.NilObjectID, false
	}
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		apierror.Forbidden(w, r, "Authenticated user required")
		return nil, primitive.NilObjectID, primitive.NilObjectID, false
	}
	return tenant, locationID, user.ID, true
}

// RequireInventoryDestinationLocation authorizes the second side of a
// transfer without trusting a destination supplied only to the service.
func RequireInventoryDestinationLocation(database *db.MongoDB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
			profile, profileOK := GetStaffProfileFromContext(r.Context())
			if !tenantOK || !profileOK {
				apierror.Forbidden(w, r, "Active staff profile required")
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				apierror.BadRequest(w, r, "Invalid request body")
				return
			}
			r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
			var request struct {
				DestinationLocationID primitive.ObjectID `json:"destinationLocationId"`
			}
			if err := json.Unmarshal(body, &request); err != nil {
				apierror.BadRequest(w, r, "Invalid request body")
				return
			}
			if value := mux.Vars(r)["destinationLocationId"]; value != "" {
				request.DestinationLocationID, err = primitive.ObjectIDFromHex(value)
			}
			if err != nil || request.DestinationLocationID.IsZero() {
				apierror.BadRequest(w, r, "Invalid destination location ID")
				return
			}
			if err := database.Locations().FindOne(r.Context(), bson.M{"_id": request.DestinationLocationID, "tenantId": tenant.ID}, nil).Err(); err != nil {
				apierror.NotFound(w, r, "Destination location not found")
				return
			}
			if !profile.AllLocations && !profileHasLocation(profile, request.DestinationLocationID) {
				apierror.NotFound(w, r, "Destination location not found")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireInventoryReferencePermission grants selector access to any inventory
// principal allowed to read, manage, or post. It remains scoped to the
// inventory reference endpoint and does not widen generic catalog reads.
func RequireInventoryReferencePermission() func(http.Handler) http.Handler {
	return RequireInventoryReadManagePostPermission()
}

// RequireInventoryCountReadPermission authorizes recovery list/detail reads
// for any inventory read/manage/post principal. Count mutations keep their
// existing exact manage/post middleware.
func RequireInventoryCountReadPermission() func(http.Handler) http.Handler {
	return RequireInventoryReadManagePostPermission()
}

func RequireInventoryReadManagePostPermission() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile, ok := GetStaffProfileFromContext(r.Context())
			if !ok || !hasInventoryReadManagePostPermission(profile) {
				apierror.Write(w, http.StatusForbidden, apierror.Code("BUSINESS_PERMISSION_DENIED"), "Business permission denied", r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func hasInventoryReferencePermission(profile *models.StaffProfile) bool {
	return hasInventoryReadManagePostPermission(profile)
}

func hasInventoryReadManagePostPermission(profile *models.StaffProfile) bool {
	return HasBusinessPermission(profile, models.PermissionInventoryRead) ||
		HasBusinessPermission(profile, models.PermissionInventoryManage) ||
		HasBusinessPermission(profile, models.PermissionInventoryPost)
}

func allReversalLocationsAuthorized(profile *models.StaffProfile, movements []models.StockMovement) bool {
	if profile == nil {
		return false
	}
	if profile.AllLocations {
		return true
	}
	seen := make(map[primitive.ObjectID]struct{}, len(movements))
	for _, movement := range movements {
		if _, ok := seen[movement.LocationID]; ok {
			continue
		}
		seen[movement.LocationID] = struct{}{}
		if !profileHasLocation(profile, movement.LocationID) {
			return false
		}
	}
	return true
}

// RequireAuthorizedReversalLocations covers every movement restored by a
// reversal. Transfer postings have both source and destination movements,
// so checking only the posting header location is insufficient.
func RequireAuthorizedReversalLocations(database *db.MongoDB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
			profile, profileOK := GetStaffProfileFromContext(r.Context())
			postingID, err := primitive.ObjectIDFromHex(mux.Vars(r)["postingId"])
			if err != nil {
				apierror.BadRequest(w, r, "Invalid posting ID")
				return
			}
			if !tenantOK || !profileOK {
				apierror.Forbidden(w, r, "Active staff profile required")
				return
			}
			var posting models.StockPosting
			if err := database.StockPostings().FindOne(r.Context(), bson.M{"_id": postingID, "tenantId": tenant.ID}).Decode(&posting); err != nil {
				apierror.NotFound(w, r, "Inventory posting not found")
				return
			}
			cursor, err := database.StockMovements().Find(r.Context(), bson.M{"postingId": posting.ID, "tenantId": tenant.ID})
			if err != nil {
				apierror.Internal(w, r, "Failed to authorize inventory reversal")
				return
			}
			defer cursor.Close(r.Context())
			var movements []models.StockMovement
			if err := cursor.All(r.Context(), &movements); err != nil {
				apierror.Internal(w, r, "Failed to authorize inventory reversal")
				return
			}
			if len(movements) == 0 {
				apierror.NotFound(w, r, "Inventory posting not found")
				return
			}
			if !allReversalLocationsAuthorized(profile, movements) {
				apierror.NotFound(w, r, "Inventory posting not found")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeInventoryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, inventory.ErrPostingNotFound):
		apierror.NotFound(w, r, "Inventory posting not found")
	case errors.Is(err, inventory.ErrInsufficientStock), errors.Is(err, inventory.ErrIdempotencyConflict), errors.Is(err, inventory.ErrPostingAlreadyReversed), errors.Is(err, inventory.ErrInventoryAreaLocked), errors.Is(err, inventory.ErrCountLineExists), errors.Is(err, inventory.ErrCountDiscoveryNotZero):
		apierror.Conflict(w, r, err.Error())
	case errors.Is(err, inventory.ErrLotNotFound), errors.Is(err, inventory.ErrLotUnavailable), errors.Is(err, inventory.ErrLotRequired), errors.Is(err, inventory.ErrLotOverrideRequired), errors.Is(err, inventory.ErrLotTrackingDisabled), errors.Is(err, inventory.ErrLotTrackingUnsafe), errors.Is(err, inventory.ErrTransferInvalid), errors.Is(err, inventory.ErrDuplicateLot), errors.Is(err, inventory.ErrQuantityOverflow):
		apierror.Validation(w, r, err.Error())
	case errors.Is(err, inventory.ErrLotTrackingVersionConflict):
		apierror.VersionConflict(w, r, err.Error())
	case errors.Is(err, inventory.ErrCountVersionConflict):
		apierror.VersionConflict(w, r, err.Error())
	case errors.Is(err, inventory.ErrCountNotFound), errors.Is(err, inventory.ErrCountLineNotFound), errors.Is(err, inventory.ErrReconciliationNotFound):
		apierror.NotFound(w, r, err.Error())
	case errors.Is(err, inventory.ErrCountState), errors.Is(err, inventory.ErrCountSnapshotInvalid), errors.Is(err, inventory.ErrCountOwnershipRequired), errors.Is(err, inventory.ErrReconciliationTooLarge), errors.Is(err, inventory.ErrReconciliationNegative), errors.Is(err, inventory.ErrReconciliationRepairNeeded):
		apierror.Conflict(w, r, err.Error())
	case errors.Is(err, inventory.ErrInvalidReference), strings.HasPrefix(err.Error(), "validation failed:"), strings.Contains(err.Error(), "quantity"), strings.Contains(err.Error(), "idempotency"):
		apierror.Validation(w, r, err.Error())
	case errors.Is(err, inventory.ErrActiveCountCursorInvalid), errors.Is(err, inventory.ErrActiveCountLimitInvalid):
		apierror.Validation(w, r, err.Error())
	default:
		apierror.Internal(w, r, "Failed to post inventory journal entry")
	}
}

func (h *productHandler) auditInventory(r *http.Request, tenant *models.Tenant, posting models.StockPosting, opening bool) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok || h.logger == nil {
		return
	}
	action := "inventory.adjustment.posted"
	message := "Inventory adjustment posted"
	if opening {
		action, message = "inventory.opening_balance.posted", "Inventory opening balance posted"
	} else {
		switch posting.Type {
		case models.StockPostingWaste:
			action, message = "inventory.waste.posted", "Inventory waste posted"
		case models.StockPostingTransfer:
			action, message = "inventory.transfer.posted", "Inventory transfer posted"
		case models.StockPostingReversal:
			action, message = "inventory.reversal.posted", "Inventory posting reversed"
		}
	}
	h.logger.LogTenantActivity(r.Context(), models.LogMedium, message, user.ID, tenant.ID, action, map[string]interface{}{"postingId": posting.ID.Hex(), "locationId": posting.LocationID.Hex(), "storageAreaId": posting.StorageAreaID.Hex(), "type": posting.Type})
}

func (h *productHandler) auditInventoryCount(r *http.Request, tenant *models.Tenant, action, message string, recordID primitive.ObjectID) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok || h.logger == nil {
		return
	}
	h.logger.LogTenantActivity(r.Context(), models.LogMedium, message, user.ID, tenant.ID, action, map[string]interface{}{"recordId": recordID.Hex()})
}
