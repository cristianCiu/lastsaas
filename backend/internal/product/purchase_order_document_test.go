package product

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestRenderPurchaseOrderPDFHasPDFHeaderAndContent(t *testing.T) {
	snapshot := PurchaseOrderDocumentSnapshot{
		TenantID: primitive.NewObjectID(), LocationID: primitive.NewObjectID(), OrderID: primitive.NewObjectID(),
		TenantName: "Tenant", OrderNumber: "PO-100", Status: models.PurchaseOrderApproved,
		CreatedAt: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC), DeliveryDate: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC), Currency: "USD",
		Supplier:      PurchaseOrderSupplierSnapshot{Code: "SUP-1", Name: "Supplier One"},
		Location:      PurchaseOrderLocationSnapshot{Code: "MAIN", Name: "Main Kitchen"},
		Branding:      PurchaseOrderBrandingSnapshot{DisplayName: "Tenant", PrimaryColor: "#123456"},
		Lines:         []PurchaseOrderLineSnapshot{{LineNumber: 1, ItemSKU: "MILK", ItemName: "Milk", Description: "Whole milk", OrderedQuantityMicros: 2000000, OrderedPacks: 2, MOQ: 1, OrderedUnitCode: "L", UnitPriceMinor: 325, LineTotalMinor: 650, Currency: "USD"}},
		SubtotalMinor: 650, TotalMinor: 650,
	}
	pdf, err := RenderPurchaseOrderPDF(snapshot)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(pdf) < 100 || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("expected non-empty PDF output, got %d bytes with header %q", len(pdf), pdf[:minDocumentTestInt(len(pdf), 5)])
	}
}

func TestPurchaseOrderDocumentBrandingUsesLocationThenTenantFallback(t *testing.T) {
	location := models.Location{ID: primitive.NewObjectID(), Name: "Kitchen"}
	tenant := &models.TenantBranding{PrimaryColor: "#111111", AccentColor: "#222222", Font: models.BrandingFontSerif, Version: 2}
	override := &models.LocationBranding{DisplayName: "Flagship", PrimaryColor: "#333333", Version: 1}
	resolved := resolveLocationBranding(location, tenant, override)
	if resolved.DisplayName != "Flagship" || resolved.PrimaryColor != "#333333" || resolved.AccentColor != "#222222" || resolved.Font != models.BrandingFontSerif {
		t.Fatalf("unexpected resolved branding: %#v", resolved)
	}
	resolved = resolveLocationBranding(location, &models.TenantBranding{}, &models.LocationBranding{})
	if resolved.DisplayName != "Kitchen" || resolved.Sources["primaryColor"] != "platform" {
		t.Fatalf("platform fallback was not applied: %#v", resolved)
	}
}

func TestValidatedStoredPurchaseOrderLogoRequiresMatchingMetadata(t *testing.T) {
	data := testPurchaseOrderPNG(t)
	asset := &models.TenantBrandingAsset{Kind: models.TenantBrandingAssetPrimary, StorageKey: "0123456789abcdef0123456789abcdef", ContentType: "image/png", Data: data, Size: int64(len(data)), Width: 32, Height: 16, Version: 1}
	logo, valid := validatedStoredPurchaseOrderLogo(asset)
	if !valid || logo == nil || len(logo.Data) == 0 {
		t.Fatal("valid stored logo was rejected")
	}
	asset.Width = 31
	if _, valid := validatedStoredPurchaseOrderLogo(asset); valid {
		t.Fatal("logo with mismatched dimensions was accepted")
	}
	asset.Width = 32
	asset.Data = []byte("https://example.invalid/logo.png")
	asset.Size = int64(len(asset.Data))
	if _, valid := validatedStoredPurchaseOrderLogo(asset); valid {
		t.Fatal("external URL bytes were accepted as a logo")
	}
}

func TestPurchaseOrderDocumentMaterializationRejectsCrossTenantOrder(t *testing.T) {
	tenantID := primitive.NewObjectID()
	h := &productHandler{}
	_, err := h.materializePurchaseOrderDocument(nil, &models.Tenant{ID: tenantID}, models.PurchaseOrder{TenantID: primitive.NewObjectID()})
	if err != ErrPurchaseOrderNotFound {
		t.Fatalf("expected cross-tenant order rejection, got %v", err)
	}
}

func TestPurchaseOrderDocumentLocationScopeRejectsUnassignedLocation(t *testing.T) {
	assigned := primitive.NewObjectID()
	other := primitive.NewObjectID()
	profile := &models.StaffProfile{AllLocations: false, LocationIDs: []primitive.ObjectID{assigned}}
	if !profileHasLocation(profile, assigned) {
		t.Fatal("assigned location was rejected")
	}
	if profileHasLocation(profile, other) {
		t.Fatal("unassigned location was authorized")
	}
}

func testPurchaseOrderPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: 20, G: 40, B: 60, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buffer.Bytes()
}

func minDocumentTestInt(value, max int) int {
	if value < max {
		return value
	}
	return max
}
