package product

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"lastsaas/internal/email"
	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type purchaseOrderEmailFakeSender struct {
	to         string
	subject    string
	html       string
	attachment email.Attachment
	calls      int
}

func (s *purchaseOrderEmailFakeSender) SendEmailWithAttachment(to, subject, html string, attachment email.Attachment) error {
	s.to, s.subject, s.html, s.attachment = to, subject, html, attachment
	s.calls++
	return nil
}

func TestPurchaseOrderDocumentEmailRequiresPurchasingManager(t *testing.T) {
	for _, test := range []struct {
		name    string
		profile *models.StaffProfile
		status  int
	}{
		{name: "viewer", profile: &models.StaffProfile{BusinessRole: models.BusinessRoleViewer}, status: http.StatusForbidden},
		{name: "operations manager", profile: &models.StaffProfile{BusinessRole: models.BusinessRoleOperationsManager}, status: http.StatusOK},
		{name: "company owner", profile: &models.StaffProfile{BusinessRole: models.BusinessRoleCompanyOwner}, status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			reached := false
			handler := requirePurchasingManager()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
			r := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(context.WithValue(context.Background(), staffProfileContextKey{}, test.profile))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			wantReached := test.status != http.StatusForbidden
			if w.Code != test.status || reached != wantReached {
				t.Fatalf("status=%d reached=%t, want status=%d reached=%t", w.Code, reached, test.status, wantReached)
			}
		})
	}
}

func TestPurchaseOrderDocumentEmailValidatesRecipientAndIdempotencyKey(t *testing.T) {
	valid := purchaseOrderDocumentSendRequest{RecipientEmail: "recipient@example.com", IdempotencyKey: "document-send-1"}
	if err := validation.Validate(&valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	for _, invalid := range []purchaseOrderDocumentSendRequest{
		{RecipientEmail: "not-an-email", IdempotencyKey: "document-send-1"},
		{RecipientEmail: "recipient@example.com", IdempotencyKey: "short"},
	} {
		if err := validation.Validate(&invalid); err == nil {
			t.Fatalf("invalid request was accepted: %#v", invalid)
		}
	}
}

func TestPurchaseOrderDocumentEmailReplayAndConflict(t *testing.T) {
	recipient, key := "recipient@example.com", "document-send-1"
	hash := purchaseOrderDocumentRequestHash(recipient, key)
	delivery := models.PurchaseOrderEmailDelivery{
		ID: primitive.NewObjectID(), RecipientEmail: recipient, IdempotencyKey: key, RequestHash: hash, Status: "sent",
	}
	if !purchaseOrderEmailDeliveryMatches(delivery, recipient, key, hash) {
		t.Fatal("same request did not match delivery record")
	}
	if purchaseOrderEmailDeliveryMatches(delivery, "other@example.com", key, hash) || purchaseOrderEmailDeliveryMatches(delivery, recipient, "other-key", hash) {
		t.Fatal("conflicting retry matched delivery record")
	}
}

func TestPurchaseOrderDocumentEmailUsesFakeAttachmentSender(t *testing.T) {
	sender := &purchaseOrderEmailFakeSender{}
	attachment := email.Attachment{Filename: "po-100.pdf", ContentType: "application/pdf", Data: []byte("%PDF-test")}
	if err := sender.SendEmailWithAttachment("recipient@example.com", "Purchase order PO-100", "<p>attached</p>", attachment); err != nil {
		t.Fatalf("fake sender failed: %v", err)
	}
	if sender.calls != 1 || sender.to != "recipient@example.com" || sender.attachment.ContentType != "application/pdf" || string(sender.attachment.Data) != "%PDF-test" {
		t.Fatalf("fake sender did not receive expected attachment: %#v", sender)
	}
}

func TestPurchaseOrderDocumentEmailIsTenantAndLocationScoped(t *testing.T) {
	tenantA, tenantB := primitive.NewObjectID(), primitive.NewObjectID()
	locationA, locationB := primitive.NewObjectID(), primitive.NewObjectID()
	order := models.PurchaseOrder{TenantID: tenantA, LocationID: locationA, ID: primitive.NewObjectID(), Version: 1}
	delivery := models.PurchaseOrderEmailDelivery{TenantID: tenantA, LocationID: locationA, PurchaseOrderID: order.ID, OrderVersion: order.Version}
	if delivery.TenantID != order.TenantID || delivery.LocationID != order.LocationID {
		t.Fatal("delivery scope was not copied from order scope")
	}
	if delivery.TenantID == tenantB || delivery.LocationID == locationB {
		t.Fatal("delivery scope unexpectedly crossed tenant/location boundary")
	}
}
