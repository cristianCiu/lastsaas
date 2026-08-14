package validation

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"lastsaas/internal/models"

	"github.com/go-playground/validator/v10"
)

var v *validator.Validate

var (
	locationCodePattern  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	currencyCodePattern  = regexp.MustCompile(`^[A-Z]{3}$`)
	languageTagPattern   = regexp.MustCompile(`^[a-z]{2}(?:-[A-Z]{2})?$`)
	supplierEmailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	hexColorPattern      = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	sha256HexPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	euAllergenCodes      = map[string]struct{}{
		"celery": {}, "cereals-gluten": {}, "crustaceans": {}, "eggs": {}, "fish": {},
		"lupin": {}, "milk": {}, "molluscs": {}, "mustard": {}, "nuts": {},
		"peanuts": {}, "sesame": {}, "soy": {}, "sulphites": {},
	}
)

func init() {
	v = validator.New()

	v.RegisterValidation("valid_role", func(fl validator.FieldLevel) bool {
		return models.ValidRole(models.MemberRole(fl.Field().String()))
	})
	v.RegisterValidation("valid_api_authority", func(fl validator.FieldLevel) bool {
		return models.ValidAPIKeyAuthority(models.APIKeyAuthority(fl.Field().String()))
	})
	v.RegisterValidation("valid_config_type", func(fl validator.FieldLevel) bool {
		return models.ValidConfigVarType(models.ConfigVarType(fl.Field().String()))
	})
	v.RegisterValidation("valid_webhook_event", func(fl validator.FieldLevel) bool {
		return models.ValidWebhookEventType(models.WebhookEventType(fl.Field().String()))
	})
	v.RegisterValidation("valid_billing_status", func(fl validator.FieldLevel) bool {
		s := models.BillingStatus(fl.Field().String())
		return s == "" || s == models.BillingStatusNone || s == models.BillingStatusActive ||
			s == models.BillingStatusPastDue || s == models.BillingStatusCanceled
	})
	v.RegisterValidation("valid_pricing_model", func(fl validator.FieldLevel) bool {
		s := models.PricingModel(fl.Field().String())
		return s == models.PricingModelFlat || s == models.PricingModelPerSeat
	})
	v.RegisterValidation("valid_credit_reset", func(fl validator.FieldLevel) bool {
		s := models.CreditResetPolicy(fl.Field().String())
		return s == models.CreditResetPolicyReset || s == models.CreditResetPolicyAccrue
	})
	v.RegisterValidation("valid_auth_method", func(fl validator.FieldLevel) bool {
		switch models.AuthMethod(fl.Field().String()) {
		case models.AuthMethodPassword, models.AuthMethodGoogle, models.AuthMethodGitHub,
			models.AuthMethodMicrosoft, models.AuthMethodMagicLink, models.AuthMethodPasskey:
			return true
		}
		return false
	})
	v.RegisterValidation("valid_invitation_status", func(fl validator.FieldLevel) bool {
		s := models.InvitationStatus(fl.Field().String())
		return s == models.InvitationPending || s == models.InvitationAccepted
	})
	v.RegisterValidation("valid_logo_mode", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return s == "" || s == "text" || s == "image" || s == "both"
	})
	v.RegisterValidation("location_code", func(fl validator.FieldLevel) bool {
		return locationCodePattern.MatchString(fl.Field().String())
	})
	v.RegisterValidation("not_blank", func(fl validator.FieldLevel) bool {
		return strings.TrimSpace(fl.Field().String()) != ""
	})
	v.RegisterValidation("eu_allergen", func(fl validator.FieldLevel) bool {
		_, ok := euAllergenCodes[fl.Field().String()]
		return ok
	})
	v.RegisterValidation("currency_code", func(fl validator.FieldLevel) bool {
		return currencyCodePattern.MatchString(fl.Field().String())
	})
	v.RegisterValidation("language_tag", func(fl validator.FieldLevel) bool {
		return languageTagPattern.MatchString(fl.Field().String())
	})
	v.RegisterValidation("storage_area_type", func(fl validator.FieldLevel) bool {
		return models.ValidStorageAreaType(models.StorageAreaType(fl.Field().String()))
	})
	v.RegisterValidation("unit_dimension", func(fl validator.FieldLevel) bool {
		return models.ValidUnitDimension(models.UnitDimension(fl.Field().String()))
	})
	v.RegisterValidation("supplier_email", func(fl validator.FieldLevel) bool {
		return supplierEmailPattern.MatchString(fl.Field().String())
	})
	v.RegisterValidation("business_role", func(fl validator.FieldLevel) bool {
		return models.ValidBusinessRole(models.BusinessRole(fl.Field().String()))
	})
	v.RegisterValidation("business_permission", func(fl validator.FieldLevel) bool {
		return models.ValidBusinessPermission(models.BusinessPermission(fl.Field().String()))
	})
	v.RegisterValidation("staff_profile_status", func(fl validator.FieldLevel) bool {
		return models.ValidStaffProfileStatus(models.StaffProfileStatus(fl.Field().String()))
	})
	v.RegisterValidation("stock_posting_type", func(fl validator.FieldLevel) bool {
		return models.ValidStockPostingType(models.StockPostingType(fl.Field().String()))
	})
	v.RegisterValidation("lot_tracking_mode", func(fl validator.FieldLevel) bool {
		return models.ValidLotTrackingMode(models.LotTrackingMode(fl.Field().String()))
	})
	v.RegisterValidation("lot_status", func(fl validator.FieldLevel) bool {
		return models.ValidLotStatus(models.LotStatus(fl.Field().String()))
	})
	v.RegisterValidation("stock_count_status", func(fl validator.FieldLevel) bool {
		return models.ValidStockCountStatus(models.StockCountStatus(fl.Field().String()))
	})
	v.RegisterValidation("reconciliation_status", func(fl validator.FieldLevel) bool {
		return models.ValidReconciliationStatus(models.ReconciliationStatus(fl.Field().String()))
	})
	v.RegisterValidation("sha256_hex", func(fl validator.FieldLevel) bool {
		return sha256HexPattern.MatchString(fl.Field().String())
	})
	v.RegisterValidation("quantity_micros", func(fl validator.FieldLevel) bool {
		return fl.Field().Int() != math.MinInt64
	})
	v.RegisterValidation("hex_color", func(fl validator.FieldLevel) bool {
		return hexColorPattern.MatchString(fl.Field().String())
	})
	v.RegisterValidation("branding_font", func(fl validator.FieldLevel) bool {
		return models.ValidBrandingFont(models.BrandingFont(fl.Field().String()))
	})
	v.RegisterValidation("tenant_branding_asset_kind", func(fl validator.FieldLevel) bool {
		return models.ValidTenantBrandingAssetKind(models.TenantBrandingAssetKind(fl.Field().String()))
	})
	v.RegisterValidation("iana_timezone", func(fl validator.FieldLevel) bool {
		name := fl.Field().String()
		if name == "Local" {
			return false
		}
		_, err := time.LoadLocation(name)
		return err == nil
	})
	v.RegisterStructValidation(validateStaffProfile, models.StaffProfile{})
	v.RegisterStructValidation(validateItem, models.Item{})
	v.RegisterStructValidation(validateSupplier, models.Supplier{})
	v.RegisterValidation("import_target", func(fl validator.FieldLevel) bool {
		return models.ValidImportTarget(models.ImportTarget(fl.Field().String()))
	})
	v.RegisterValidation("import_run_status", func(fl validator.FieldLevel) bool {
		return models.ImportRunStatus(fl.Field().String()) == models.ImportRunCompleted ||
			models.ImportRunStatus(fl.Field().String()) == models.ImportRunPending ||
			models.ImportRunStatus(fl.Field().String()) == models.ImportRunFailed
	})
	v.RegisterValidation("recipe_version_status", func(fl validator.FieldLevel) bool {
		return models.ValidRecipeVersionStatus(models.RecipeVersionStatus(fl.Field().String()))
	})
	v.RegisterValidation("recipe_component_type", func(fl validator.FieldLevel) bool {
		return models.ValidRecipeComponentType(models.RecipeComponentType(fl.Field().String()))
	})
	v.RegisterValidation("sale_status", func(fl validator.FieldLevel) bool {
		return models.ValidSaleStatus(models.SaleStatus(fl.Field().String()))
	})
	v.RegisterValidation("sales_import_status", func(fl validator.FieldLevel) bool {
		return models.ValidSalesImportStatus(models.SalesImportStatus(fl.Field().String()))
	})
	v.RegisterValidation("purchase_order_status", func(fl validator.FieldLevel) bool {
		return models.ValidPurchaseOrderStatus(models.PurchaseOrderStatus(fl.Field().String()))
	})
	v.RegisterValidation("goods_receipt_status", func(fl validator.FieldLevel) bool {
		return models.ValidGoodsReceiptStatus(models.GoodsReceiptStatus(fl.Field().String()))
	})
	v.RegisterStructValidation(validateRecipeVersion, models.RecipeVersion{})
	v.RegisterStructValidation(validateRecipeComponent, models.RecipeComponent{})
	v.RegisterStructValidation(validateExternalProductMapping, models.ExternalProductMapping{})
	v.RegisterStructValidation(validateSale, models.Sale{})
	v.RegisterStructValidation(validatePurchaseOrder, models.PurchaseOrder{})
	v.RegisterStructValidation(validatePurchaseOrderLine, models.PurchaseOrderLine{})
	v.RegisterStructValidation(validateGoodsReceiptLine, models.GoodsReceiptLine{})
	v.RegisterStructValidation(validateGoodsReceipt, models.GoodsReceipt{})
	v.RegisterStructValidation(validatePurchaseOrderEmailDelivery, models.PurchaseOrderEmailDelivery{})
}

func validateRecipeVersion(sl validator.StructLevel) {
	version := sl.Current().Interface().(models.RecipeVersion)
	if version.EffectiveTo != nil && !version.EffectiveTo.After(version.EffectiveFrom) {
		sl.ReportError(version.EffectiveTo, "EffectiveTo", "effectiveTo", "after_effective_from", "")
	}
}

func validateRecipeComponent(sl validator.StructLevel) {
	component := sl.Current().Interface().(models.RecipeComponent)
	itemSet := component.ItemID != nil && !component.ItemID.IsZero()
	subrecipeSet := component.SubrecipeID != nil && !component.SubrecipeID.IsZero()
	if itemSet == subrecipeSet ||
		(component.ComponentType == models.RecipeComponentItem && !itemSet) ||
		(component.ComponentType == models.RecipeComponentSubrecipe && !subrecipeSet) {
		sl.ReportError(component.ComponentType, "ComponentType", "componentType", "component_reference", "")
	}
}

func validateExternalProductMapping(sl validator.StructLevel) {
	mapping := sl.Current().Interface().(models.ExternalProductMapping)
	if mapping.EffectiveTo != nil && !mapping.EffectiveTo.After(mapping.EffectiveFrom) {
		sl.ReportError(mapping.EffectiveTo, "EffectiveTo", "effectiveTo", "after_effective_from", "")
	}
}

func validateSale(sl validator.StructLevel) {
	sale := sl.Current().Interface().(models.Sale)
	if sale.Status == models.SaleStatusCancelled && sale.CancelledAt == nil {
		sl.ReportError(sale.CancelledAt, "CancelledAt", "cancelledAt", "required_for_cancelled", "")
	}
	if sale.Status == models.SaleStatusCompleted && sale.CancelledAt != nil {
		sl.ReportError(sale.CancelledAt, "CancelledAt", "cancelledAt", "forbidden_for_completed", "")
	}
}

func validatePurchaseOrder(sl validator.StructLevel) {
	order := sl.Current().Interface().(models.PurchaseOrder)
	if order.Status == models.PurchaseOrderSubmitted && (order.SubmittedBy == nil || order.SubmittedAt == nil) {
		sl.ReportError(order.SubmittedAt, "SubmittedAt", "submittedAt", "required_for_submitted", "")
	}
	if order.Status == models.PurchaseOrderApproved && (order.ApprovedBy == nil || order.ApprovedAt == nil) {
		sl.ReportError(order.ApprovedAt, "ApprovedAt", "approvedAt", "required_for_approved", "")
	}
	if order.Status == models.PurchaseOrderCancelled && (order.CancelledBy == nil || order.CancelledAt == nil) {
		sl.ReportError(order.CancelledAt, "CancelledAt", "cancelledAt", "required_for_cancelled", "")
	}
}

func validatePurchaseOrderLine(sl validator.StructLevel) {
	line := sl.Current().Interface().(models.PurchaseOrderLine)
	if line.OrderedQuantityMicros < line.RequestedQuantityMicros || line.OrderedQuantityMicros-line.RequestedQuantityMicros != line.RoundingDeltaMicros {
		sl.ReportError(line.OrderedQuantityMicros, "OrderedQuantityMicros", "orderedQuantityMicros", "rounding_result", "")
	}
	if line.OrderedPacks > 0 && line.PackSizeMicros > 0 && line.OrderedPacks > math.MaxInt64/line.PackSizeMicros {
		sl.ReportError(line.OrderedPacks, "OrderedPacks", "orderedPacks", "rounding_overflow", "")
	}
	if line.OrderedPacks > 0 && line.UnitPriceMinor > 0 && line.OrderedPacks > math.MaxInt64/line.UnitPriceMinor {
		sl.ReportError(line.LineTotalMinor, "LineTotalMinor", "lineTotalMinor", "rounding_overflow", "")
	}
}

func validateGoodsReceiptLine(sl validator.StructLevel) {
	line := sl.Current().Interface().(models.GoodsReceiptLine)
	if line.ReceivedQuantityMicros > 0 && line.OrderedQuantityMicros <= math.MaxInt64-line.ReceivedQuantityMicros {
		expected := line.ReceivedQuantityMicros - line.OrderedQuantityMicros
		if line.QuantityVarianceMicros != expected {
			sl.ReportError(line.QuantityVarianceMicros, "QuantityVarianceMicros", "quantityVarianceMicros", "exact_variance", "")
		}
	}
	if line.ActualUnitPriceMinor <= math.MaxInt64-line.OrderedUnitPriceMinor {
		if line.PriceVarianceMinor != line.ActualUnitPriceMinor-line.OrderedUnitPriceMinor {
			sl.ReportError(line.PriceVarianceMinor, "PriceVarianceMinor", "priceVarianceMinor", "exact_variance", "")
		}
	}
}

func validateGoodsReceipt(sl validator.StructLevel) {
	receipt := sl.Current().Interface().(models.GoodsReceipt)
	if receipt.Status == models.GoodsReceiptCancelled && receipt.ReversalPostingID == nil {
		sl.ReportError(receipt.ReversalPostingID, "ReversalPostingID", "reversalPostingId", "required_for_cancelled", "")
	}
}

func validatePurchaseOrderEmailDelivery(sl validator.StructLevel) {
	delivery := sl.Current().Interface().(models.PurchaseOrderEmailDelivery)
	if delivery.Status == "sent" && delivery.SentAt == nil {
		sl.ReportError(delivery.SentAt, "SentAt", "sentAt", "required_for_sent", "")
	}
	if delivery.Status == "pending" && delivery.SentAt != nil {
		sl.ReportError(delivery.SentAt, "SentAt", "sentAt", "forbidden_for_pending", "")
	}
}

func validateSupplier(sl validator.StructLevel) {
	supplier := sl.Current().Interface().(models.Supplier)
	seen := make(map[int32]struct{}, len(supplier.OrderingDays))
	for _, day := range supplier.OrderingDays {
		if _, exists := seen[day]; exists {
			sl.ReportError(supplier.OrderingDays, "OrderingDays", "orderingDays", "unique", "")
			return
		}
		seen[day] = struct{}{}
	}
}

func validateItem(sl validator.StructLevel) {
	item := sl.Current().Interface().(models.Item)
	seen := make(map[string]struct{}, len(item.Allergens))
	for _, allergen := range item.Allergens {
		if _, exists := seen[allergen]; exists {
			sl.ReportError(item.Allergens, "Allergens", "allergens", "unique", "")
			return
		}
		seen[allergen] = struct{}{}
	}
}

func validateStaffProfile(sl validator.StructLevel) {
	profile := sl.Current().Interface().(models.StaffProfile)
	if profile.AllLocations && len(profile.LocationIDs) > 0 {
		sl.ReportError(profile.LocationIDs, "LocationIDs", "locationIds", "all_locations_scope", "")
	}
	locations := make(map[string]struct{}, len(profile.LocationIDs))
	for _, locationID := range profile.LocationIDs {
		key := locationID.Hex()
		if locationID.IsZero() {
			sl.ReportError(profile.LocationIDs, "LocationIDs", "locationIds", "valid_object_id", "")
		}
		if _, exists := locations[key]; exists {
			sl.ReportError(profile.LocationIDs, "LocationIDs", "locationIds", "unique", "")
			break
		}
		locations[key] = struct{}{}
	}
	overrides := make(map[models.BusinessPermission]struct{}, len(profile.PermissionOverrides))
	for _, override := range profile.PermissionOverrides {
		if _, exists := overrides[override.Permission]; exists {
			sl.ReportError(profile.PermissionOverrides, "PermissionOverrides", "permissionOverrides", "unique_permissions", "")
			break
		}
		overrides[override.Permission] = struct{}{}
	}
}

// Validate validates a struct using go-playground/validator tags.
func Validate(s interface{}) error {
	err := v.Struct(s)
	if err == nil {
		return nil
	}
	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return err
	}
	var msgs []string
	for _, fe := range validationErrors {
		msgs = append(msgs, formatFieldError(fe))
	}
	return fmt.Errorf("validation failed: %s", strings.Join(msgs, "; "))
}

func formatFieldError(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", fe.Field())
	case "email":
		return fmt.Sprintf("%s must be a valid email", fe.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s", fe.Field(), fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s", fe.Field(), fe.Param())
	case "gte":
		return fmt.Sprintf("%s must be >= %s", fe.Field(), fe.Param())
	case "gt":
		return fmt.Sprintf("%s must be > %s", fe.Field(), fe.Param())
	case "len":
		return fmt.Sprintf("%s must be exactly %s characters", fe.Field(), fe.Param())
	case "url":
		return fmt.Sprintf("%s must be a valid URL", fe.Field())
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", fe.Field(), fe.Param())
	default:
		return fmt.Sprintf("%s failed %s validation", fe.Field(), fe.Tag())
	}
}
