package product

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"lastsaas/internal/apierror"
	"lastsaas/internal/email"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	ErrPurchaseOrderEmailConflict = errors.New("purchase order document email idempotency conflict")
	ErrPurchaseOrderEmailPending  = errors.New("purchase order document email is already being sent")
)

type purchaseOrderDocumentSendRequest struct {
	RecipientEmail string `json:"recipientEmail" validate:"required,email,max=254"`
	IdempotencyKey string `json:"idempotencyKey" validate:"required,min=8,max=128"`
}

func purchaseOrderDocumentRequestHash(recipientEmail, idempotencyKey string) string {
	sum := sha256.Sum256([]byte(recipientEmail + "\x00" + idempotencyKey))
	return hex.EncodeToString(sum[:])
}

func purchaseOrderEmailDeliveryMatches(delivery models.PurchaseOrderEmailDelivery, recipientEmail, idempotencyKey, requestHash string) bool {
	return delivery.RecipientEmail == recipientEmail && delivery.IdempotencyKey == idempotencyKey && delivery.RequestHash == requestHash
}

func purchaseOrderDocumentSendable(order models.PurchaseOrder) bool {
	if order.ApprovedBy == nil || order.ApprovedAt == nil {
		return false
	}
	switch order.Status {
	case models.PurchaseOrderApproved, models.PurchaseOrderOrdered, models.PurchaseOrderPartiallyReceived, models.PurchaseOrderReceived:
		return true
	default:
		return false
	}
}

func (h *productHandler) sendPurchaseOrderDocument(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}

	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["orderId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid purchase order ID")
		return
	}
	var request purchaseOrderDocumentSendRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	request.RecipientEmail = strings.ToLower(strings.TrimSpace(request.RecipientEmail))
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if headerKey := strings.TrimSpace(r.Header.Get("Idempotency-Key")); headerKey != "" {
		if request.IdempotencyKey != "" && request.IdempotencyKey != headerKey {
			apierror.BadRequest(w, r, "Idempotency key does not match the request header")
			return
		}
		request.IdempotencyKey = headerKey
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}

	var order models.PurchaseOrder
	if err := h.db.PurchaseOrders().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&order); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Purchase order not found")
		} else {
			apierror.Internal(w, r, "Failed to load purchase order")
		}
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, order.LocationID); err != nil {
		apierror.NotFound(w, r, "Purchase order not found")
		return
	}
	if !purchaseOrderDocumentSendable(order) {
		apierror.Conflict(w, r, "Purchase order must be approved before its document can be emailed")
		return
	}

	hash := purchaseOrderDocumentRequestHash(request.RecipientEmail, request.IdempotencyKey)
	filter := bson.M{"tenantId": tenant.ID, "purchaseOrderId": order.ID, "orderVersion": order.Version}
	var existing models.PurchaseOrderEmailDelivery
	lookupErr := h.db.PurchaseOrderEmailDeliveries().FindOne(r.Context(), filter).Decode(&existing)
	if lookupErr == nil {
		if !purchaseOrderEmailDeliveryMatches(existing, request.RecipientEmail, request.IdempotencyKey, hash) {
			apierror.Conflict(w, r, "This purchase-order version has already been assigned a different email delivery request")
			return
		}
		if existing.Status == "sent" {
			writePurchaseOrderEmailResponse(w, existing)
			return
		}
		apierror.Conflict(w, r, ErrPurchaseOrderEmailPending.Error())
		return
	}
	if !errors.Is(lookupErr, mongo.ErrNoDocuments) {
		apierror.Internal(w, r, "Failed to load purchase order email delivery")
		return
	}
	if h.emailSender == nil {
		apierror.Write(w, http.StatusServiceUnavailable, apierror.CodeServiceUnavail, "Email service is not configured", r)
		return
	}

	// Materialization and rendering happen before the delivery claim. A failed
	// render therefore cannot reserve a send or cause an email side effect.
	snapshot, err := h.materializePurchaseOrderDocument(r.Context(), tenant, order)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) || errors.Is(err, ErrPurchaseOrderNotFound) {
			apierror.NotFound(w, r, "Purchase order not found")
		} else {
			apierror.Internal(w, r, "Failed to materialize purchase order document")
		}
		return
	}
	pdf, err := RenderPurchaseOrderPDF(snapshot)
	if err != nil {
		apierror.Internal(w, r, "Failed to render purchase order document")
		return
	}

	now := time.Now().UTC()
	delivery := models.PurchaseOrderEmailDelivery{
		ID:              primitive.NewObjectID(),
		TenantID:        tenant.ID,
		LocationID:      order.LocationID,
		PurchaseOrderID: order.ID,
		OrderVersion:    order.Version,
		RecipientEmail:  request.RecipientEmail,
		IdempotencyKey:  request.IdempotencyKey,
		RequestHash:     hash,
		Status:          "pending",
		ClaimedAt:       now,
	}
	if err := validation.Validate(&delivery); err != nil {
		apierror.Internal(w, r, "Failed to prepare purchase order email delivery")
		return
	}
	if _, err := h.db.PurchaseOrderEmailDeliveries().InsertOne(r.Context(), delivery); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			if lookupErr := h.db.PurchaseOrderEmailDeliveries().FindOne(r.Context(), filter).Decode(&existing); lookupErr == nil {
				if !purchaseOrderEmailDeliveryMatches(existing, request.RecipientEmail, request.IdempotencyKey, hash) {
					apierror.Conflict(w, r, "This purchase-order version has already been assigned a different email delivery request")
				} else if existing.Status == "sent" {
					writePurchaseOrderEmailResponse(w, existing)
				} else {
					apierror.Conflict(w, r, ErrPurchaseOrderEmailPending.Error())
				}
				return
			}
		}
		apierror.Internal(w, r, "Failed to claim purchase order email delivery")
		return
	}

	attachment := email.Attachment{
		Filename:    safeDocumentFilename(snapshot.OrderNumber) + ".pdf",
		ContentType: "application/pdf",
		Data:        pdf,
	}
	if err := h.emailSender.SendEmailWithAttachment(request.RecipientEmail, fmt.Sprintf("Purchase order %s", snapshot.OrderNumber), purchaseOrderEmailHTML(snapshot), attachment); err != nil {
		// Keep the pending claim. The provider may have accepted a message before
		// returning an error, so retrying here would risk a duplicate delivery.
		if h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogMedium, "Purchase order document email failed", tenantUserID(r), tenant.ID, "purchase_order.document_email.failed", map[string]interface{}{"purchaseOrderId": order.ID.Hex(), "locationId": order.LocationID.Hex(), "version": order.Version})
		}
		apierror.Write(w, http.StatusServiceUnavailable, apierror.CodeServiceUnavail, "Failed to send purchase order document email", r)
		return
	}

	sentAt := time.Now().UTC()
	if _, err := h.db.PurchaseOrderEmailDeliveries().UpdateOne(r.Context(), bson.M{"_id": delivery.ID, "status": "pending"}, bson.M{"$set": bson.M{"status": "sent", "sentAt": sentAt}}); err != nil {
		apierror.Internal(w, r, "Purchase order document email was sent but delivery status could not be recorded")
		return
	}
	delivery.Status = "sent"
	delivery.SentAt = &sentAt
	if h.logger != nil {
		h.logger.LogTenantActivity(r.Context(), models.LogMedium, "Purchase order document emailed", tenantUserID(r), tenant.ID, "purchase_order.document_email.sent", map[string]interface{}{"purchaseOrderId": order.ID.Hex(), "locationId": order.LocationID.Hex(), "version": order.Version})
	}
	writePurchaseOrderEmailResponse(w, delivery)
}

func tenantUserID(r *http.Request) primitive.ObjectID {
	if user, ok := middleware.GetUserFromContext(r.Context()); ok {
		return user.ID
	}
	return primitive.NilObjectID
}

func writePurchaseOrderEmailResponse(w http.ResponseWriter, delivery models.PurchaseOrderEmailDelivery) {
	writeJSON(w, http.StatusOK, map[string]any{"delivery": map[string]any{
		"id": delivery.ID, "purchaseOrderId": delivery.PurchaseOrderID, "orderVersion": delivery.OrderVersion, "status": delivery.Status,
	}})
}

func purchaseOrderEmailHTML(snapshot PurchaseOrderDocumentSnapshot) string {
	return fmt.Sprintf("<!doctype html><html><body><p>Your purchase order document is attached.</p><p>Order <strong>%s</strong> for <strong>%s</strong>.</p></body></html>", html.EscapeString(snapshot.OrderNumber), html.EscapeString(snapshot.Location.Name))
}
