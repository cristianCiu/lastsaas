package product

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lastsaas/internal/apierror"
	"lastsaas/internal/models"

	"github.com/gorilla/mux"
	"github.com/jung-kurt/gofpdf"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PurchaseOrderDocumentSnapshot is the complete, immutable input to the PDF
// renderer. In particular, its lines are purchase-order line snapshots and
// must not be rebuilt from catalog or supplier-item records.
type PurchaseOrderDocumentSnapshot struct {
	TenantID      primitive.ObjectID
	LocationID    primitive.ObjectID
	OrderID       primitive.ObjectID
	TenantName    string
	OrderNumber   string
	Status        models.PurchaseOrderStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeliveryDate  time.Time
	Notes         string
	Currency      string
	Supplier      PurchaseOrderSupplierSnapshot
	Location      PurchaseOrderLocationSnapshot
	Branding      PurchaseOrderBrandingSnapshot
	Lines         []PurchaseOrderLineSnapshot
	SubtotalMinor int64
	TotalMinor    int64
}

type PurchaseOrderSupplierSnapshot struct {
	Code        string
	Name        string
	ContactName string
	Email       string
	Phone       string
}

type PurchaseOrderLocationSnapshot struct {
	Code     string
	Name     string
	Timezone string
}

type PurchaseOrderLogoSnapshot struct {
	ContentType string
	Data        []byte
	Width       int
	Height      int
}

type PurchaseOrderBrandingSnapshot struct {
	DisplayName  string
	PrimaryColor string
	AccentColor  string
	Font         models.BrandingFont
	Logo         *PurchaseOrderLogoSnapshot
}

type PurchaseOrderLineSnapshot struct {
	LineNumber              int32
	ItemSKU                 string
	ItemName                string
	Description             string
	SupplierItemCode        string
	RequestedQuantityMicros int64
	OrderedQuantityMicros   int64
	PackSizeMicros          int64
	MOQ                     int32
	OrderedPacks            int64
	UnitPriceMinor          int64
	LineTotalMinor          int64
	Currency                string
	OrderedUnitCode         string
	ItemBaseUnitCode        string
}

// RenderPurchaseOrderPDF renders only the supplied snapshot. It performs no
// database, filesystem, or network access, making it safe to use after all
// mutable records have been materialized by the caller.
func RenderPurchaseOrderPDF(snapshot PurchaseOrderDocumentSnapshot) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(14, 14, 14)
	pdf.SetAutoPageBreak(true, 14)
	pdf.AddPage()

	if snapshot.Branding.Logo != nil {
		logo := snapshot.Branding.Logo
		imageType := "PNG"
		if logo.ContentType == "image/jpeg" {
			imageType = "JPG"
		}
		pdf.RegisterImageOptionsReader("purchase-order-logo", gofpdf.ImageOptions{ImageType: imageType}, bytes.NewReader(logo.Data))
		if pdf.Err() == false {
			width, height := fitLogo(logo.Width, logo.Height, 34, 18)
			pdf.ImageOptions("purchase-order-logo", 166, 14, width, height, false, gofpdf.ImageOptions{ImageType: imageType}, 0, "")
		}
	}

	setPDFColor(pdf, snapshot.Branding.PrimaryColor, 35, 66, 99)
	pdf.SetFont("Helvetica", "B", 20)
	pdf.CellFormat(0, 10, "PURCHASE ORDER", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 11)
	brandName := safePDFText(snapshot.Branding.DisplayName)
	if snapshot.TenantName != "" && snapshot.TenantName != snapshot.Branding.DisplayName {
		brandName = safePDFText(snapshot.TenantName) + " — " + brandName
	}
	pdf.CellFormat(0, 6, brandName, "", 1, "L", false, 0, "")
	pdf.SetTextColor(70, 70, 70)
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(0, 5, fmt.Sprintf("Order %s  |  Status: %s", safePDFText(snapshot.OrderNumber), safePDFText(string(snapshot.Status))), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, fmt.Sprintf("Created: %s  |  Delivery: %s  |  Updated: %s", formatDocumentDate(snapshot.CreatedAt), formatDocumentDate(snapshot.DeliveryDate), formatDocumentDate(snapshot.UpdatedAt)), "", 1, "L", false, 0, "")
	pdf.Ln(6)

	pdf.SetTextColor(35, 35, 35)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(88, 6, "SUPPLIER", "", 0, "L", false, 0, "")
	pdf.CellFormat(88, 6, "DELIVER TO", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(88, 5, safePDFText(snapshot.Supplier.Name), "", 0, "L", false, 0, "")
	pdf.CellFormat(88, 5, safePDFText(snapshot.Location.Name), "", 1, "L", false, 0, "")
	pdf.CellFormat(88, 5, safePDFText(snapshot.Supplier.Code), "", 0, "L", false, 0, "")
	pdf.CellFormat(88, 5, safePDFText(snapshot.Location.Code), "", 1, "L", false, 0, "")
	if snapshot.Supplier.ContactName != "" || snapshot.Supplier.Email != "" || snapshot.Supplier.Phone != "" {
		contact := strings.TrimSpace(strings.Join([]string{snapshot.Supplier.ContactName, snapshot.Supplier.Email, snapshot.Supplier.Phone}, " | "))
		pdf.CellFormat(88, 5, safePDFText(contact), "", 0, "L", false, 0, "")
		pdf.CellFormat(88, 5, safePDFText(snapshot.Location.Timezone), "", 1, "L", false, 0, "")
	}
	pdf.Ln(7)

	setPDFColor(pdf, snapshot.Branding.AccentColor, 235, 240, 245)
	pdf.SetTextColor(35, 35, 35)
	pdf.SetFont("Helvetica", "B", 8)
	widths := []float64{10, 57, 28, 25, 25, 31}
	headers := []string{"#", "ITEM", "QTY / UNIT", "PACKS / MOQ", "UNIT PRICE", "TOTAL"}
	for i, header := range headers {
		pdf.CellFormat(widths[i], 7, header, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", 8)
	for _, line := range snapshot.Lines {
		qty := fmt.Sprintf("%s %s / req %s", formatMicros(line.OrderedQuantityMicros), safePDFText(line.OrderedUnitCode), formatMicros(line.RequestedQuantityMicros))
		packs := fmt.Sprintf("%d / %d (pack %s)", line.OrderedPacks, line.MOQ, formatMicros(line.PackSizeMicros))
		item := safePDFText(line.ItemName)
		if line.ItemSKU != "" {
			item += " (" + safePDFText(line.ItemSKU) + ")"
		}
		if line.SupplierItemCode != "" {
			item += " [supplier " + safePDFText(line.SupplierItemCode) + "]"
		}
		values := []string{strconv.FormatInt(int64(line.LineNumber), 10), item, qty, packs, formatMinor(line.Currency, line.UnitPriceMinor), formatMinor(line.Currency, line.LineTotalMinor)}
		for i, value := range values {
			align := "L"
			if i == 0 || i >= 2 {
				align = "R"
			}
			pdf.CellFormat(widths[i], 7, value, "1", 0, align, false, 0, "")
		}
		pdf.Ln(-1)
		if line.Description != "" {
			pdf.SetTextColor(95, 95, 95)
			pdf.CellFormat(67, 5, "  "+safePDFText(line.Description), "LRB", 0, "L", false, 0, "")
			pdf.CellFormat(109, 5, "", "RB", 1, "L", false, 0, "")
			pdf.SetTextColor(35, 35, 35)
		}
	}
	pdf.Ln(5)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(120, 7, "Subtotal", "", 0, "R", false, 0, "")
	pdf.CellFormat(56, 7, formatMinor(snapshot.Currency, snapshot.SubtotalMinor), "", 1, "R", false, 0, "")
	pdf.CellFormat(120, 7, "Total", "", 0, "R", false, 0, "")
	pdf.CellFormat(56, 7, formatMinor(snapshot.Currency, snapshot.TotalMinor), "T", 1, "R", false, 0, "")
	if snapshot.Notes != "" {
		pdf.Ln(5)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(0, 5, "Notes", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 9)
		pdf.MultiCell(0, 5, safePDFText(snapshot.Notes), "", "L", false)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func setPDFColor(pdf *gofpdf.Fpdf, color string, fallbackR, fallbackG, fallbackB int) {
	r, g, b := fallbackR, fallbackG, fallbackB
	if len(color) == 7 && color[0] == '#' {
		if value, err := strconv.ParseUint(color[1:], 16, 32); err == nil {
			r, g, b = int(value>>16)&255, int(value>>8)&255, int(value)&255
		}
	}
	pdf.SetFillColor(r, g, b)
	pdf.SetTextColor(r, g, b)
}

func fitLogo(width, height, maxWidth, maxHeight int) (float64, float64) {
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	ratio := float64(width) / float64(height)
	w, h := float64(maxWidth), float64(maxWidth)/ratio
	if h > float64(maxHeight) {
		h = float64(maxHeight)
		w = h * ratio
	}
	return w, h
}

func formatDocumentDate(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	return value.UTC().Format("2006-01-02")
}

func formatMicros(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	result := fmt.Sprintf("%d.%06d", value/1000000, value%1000000)
	if negative {
		return "-" + result
	}
	return result
}

func formatMinor(currency string, value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	result := fmt.Sprintf("%s %d.%02d", safePDFText(currency), value/100, value%100)
	if negative {
		return "-" + result
	}
	return result
}

func safePDFText(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return ' '
		}
		return r
	}, value)
}

// validatedStoredPurchaseOrderLogo rejects a stored asset unless both its
// metadata and its actual image bytes agree. Invalid legacy data is ignored
// rather than embedded in a document.
func validatedStoredPurchaseOrderLogo(asset *models.TenantBrandingAsset) (*PurchaseOrderLogoSnapshot, bool) {
	if asset == nil || !models.ValidTenantBrandingAssetKind(asset.Kind) || asset.Version < 1 || asset.Size != int64(len(asset.Data)) || len(asset.StorageKey) != 32 {
		return nil, false
	}
	if _, err := hex.DecodeString(asset.StorageKey); err != nil || (asset.ContentType != "image/png" && asset.ContentType != "image/jpeg") {
		return nil, false
	}
	detected, width, height, err := validateTenantLogo(asset.Data, asset.ContentType)
	if err != nil || detected != asset.ContentType || width != asset.Width || height != asset.Height {
		return nil, false
	}
	return &PurchaseOrderLogoSnapshot{ContentType: detected, Data: append([]byte(nil), asset.Data...), Width: width, Height: height}, true
}

func (h *productHandler) materializePurchaseOrderDocument(ctx context.Context, tenant *models.Tenant, order models.PurchaseOrder) (PurchaseOrderDocumentSnapshot, error) {
	if tenant == nil || tenant.ID != order.TenantID {
		return PurchaseOrderDocumentSnapshot{}, ErrPurchaseOrderNotFound
	}
	var location models.Location
	if err := h.db.Locations().FindOne(ctx, bson.M{"_id": order.LocationID, "tenantId": order.TenantID}).Decode(&location); err != nil {
		return PurchaseOrderDocumentSnapshot{}, err
	}
	var supplier models.Supplier
	if err := h.db.Suppliers().FindOne(ctx, bson.M{"_id": order.SupplierID, "tenantId": order.TenantID}).Decode(&supplier); err != nil {
		return PurchaseOrderDocumentSnapshot{}, err
	}
	lines, err := h.loadPurchaseDocumentLines(ctx, order)
	if err != nil {
		return PurchaseOrderDocumentSnapshot{}, err
	}
	tenantBranding, err := NewTenantBrandingRepository(h.db, order.TenantID).Get(ctx)
	if err != nil {
		return PurchaseOrderDocumentSnapshot{}, err
	}
	locationBranding, err := NewLocationBrandingRepository(h.db, order.TenantID, order.LocationID).Get(ctx)
	if err != nil {
		return PurchaseOrderDocumentSnapshot{}, err
	}
	resolved := resolveLocationBranding(location, tenantBranding, locationBranding)

	snapshot := PurchaseOrderDocumentSnapshot{
		TenantID: order.TenantID, LocationID: order.LocationID, OrderID: order.ID,
		TenantName: tenant.Name, OrderNumber: order.OrderNumber, Status: order.Status,
		CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt, DeliveryDate: order.DeliveryDate,
		Notes: order.Notes, Supplier: PurchaseOrderSupplierSnapshot{Code: supplier.Code, Name: supplier.Name, ContactName: supplier.ContactName, Email: supplier.Email, Phone: supplier.Phone},
		Location: PurchaseOrderLocationSnapshot{Code: location.Code, Name: location.Name, Timezone: location.Timezone},
		Branding: PurchaseOrderBrandingSnapshot{DisplayName: resolved.DisplayName, PrimaryColor: resolved.PrimaryColor, AccentColor: resolved.AccentColor, Font: resolved.Font},
		Lines:    make([]PurchaseOrderLineSnapshot, 0, len(lines)),
	}
	for _, line := range lines {
		snapshot.Lines = append(snapshot.Lines, PurchaseOrderLineSnapshot{LineNumber: line.LineNumber, ItemSKU: line.ItemSKU, ItemName: line.ItemName, Description: line.Description, SupplierItemCode: line.SupplierItemCode, RequestedQuantityMicros: line.RequestedQuantityMicros, OrderedQuantityMicros: line.OrderedQuantityMicros, PackSizeMicros: line.PackSizeMicros, MOQ: line.MOQ, OrderedPacks: line.OrderedPacks, UnitPriceMinor: line.UnitPriceMinor, LineTotalMinor: line.LineTotalMinor, Currency: line.Currency, OrderedUnitCode: line.OrderedUnitCode, ItemBaseUnitCode: line.ItemBaseUnitCode})
		if snapshot.Currency == "" {
			snapshot.Currency = line.Currency
		}
		if line.LineTotalMinor < 0 || snapshot.SubtotalMinor > math.MaxInt64-line.LineTotalMinor {
			return PurchaseOrderDocumentSnapshot{}, errors.New("purchase order total overflow")
		}
		snapshot.SubtotalMinor += line.LineTotalMinor
	}
	snapshot.TotalMinor = snapshot.SubtotalMinor
	for _, kind := range []models.TenantBrandingAssetKind{models.TenantBrandingAssetPrimary, models.TenantBrandingAssetCompact} {
		var asset models.TenantBrandingAsset
		err := h.db.TenantBrandingAssets().FindOne(ctx, bson.M{"tenantId": order.TenantID, "kind": kind}).Decode(&asset)
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			return PurchaseOrderDocumentSnapshot{}, err
		}
		if logo, valid := validatedStoredPurchaseOrderLogo(&asset); valid {
			snapshot.Branding.Logo = logo
			break
		}
	}
	return snapshot, nil
}

func (h *productHandler) loadPurchaseDocumentLines(ctx context.Context, order models.PurchaseOrder) ([]models.PurchaseOrderLine, error) {
	cur, err := h.db.PurchaseOrderLines().Find(ctx, bson.M{"tenantId": order.TenantID, "locationId": order.LocationID, "purchaseOrderId": order.ID}, options.Find().SetSort(bson.D{{Key: "lineNumber", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	lines := []models.PurchaseOrderLine{}
	return lines, cur.All(ctx, &lines)
}

func (h *productHandler) getPurchaseOrderDocument(w http.ResponseWriter, r *http.Request) {
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
			apierror.Internal(w, r, "Failed to load purchase order")
		}
		return
	}
	if err := authorizePurchasingLocation(r.Context(), h.db, tenant.ID, order.LocationID); err != nil {
		apierror.NotFound(w, r, "Purchase order not found")
		return
	}
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
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeDocumentFilename(snapshot.OrderNumber)+`.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(pdf))
}

func safeDocumentFilename(value string) string {
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "purchase-order"
	}
	return value
}
