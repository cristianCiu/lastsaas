package product

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"lastsaas/internal/apierror"
	"lastsaas/internal/db"
	"lastsaas/internal/email"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	"lastsaas/internal/syslog"
	"lastsaas/internal/validation"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type productHandler struct {
	db          *db.MongoDB
	logger      *syslog.Logger
	emailSender email.AttachmentSender
}

// RegisterRoutes installs all product routes beneath an already guarded API
// router. Production and integration tests call this same registration path.
func RegisterRoutes(guarded *mux.Router, database *db.MongoDB, auth *middleware.AuthMiddleware, tenant *middleware.TenantMiddleware, logger *syslog.Logger, senders ...email.AttachmentSender) {
	var sender email.AttachmentSender
	if len(senders) > 0 {
		sender = senders[0]
	}
	handler := &productHandler{db: database, logger: logger, emailSender: sender}
	requireProfile := RequireStaffProfile(database)
	onboardingAPI := guarded.PathPrefix("/product/onboarding").Subrouter()
	onboardingAPI.Use(auth.RequireAuth)
	onboardingAPI.Use(tenant.RequireTenant)
	onboardingAPI.Handle("", withProductMiddleware(http.HandlerFunc(handler.getOnboarding), requireProfile)).Methods(http.MethodGet)
	onboardingAPI.Handle("/complete", withProductMiddleware(http.HandlerFunc(handler.completeOnboarding), requireProfile, requireCoreRole(models.RoleOwner))).Methods(http.MethodPost)

	productAPI := guarded.PathPrefix("/product").Subrouter()
	productAPI.Use(auth.RequireAuth)
	productAPI.Use(tenant.RequireTenant)
	productAPI.Use(middleware.RequireActiveBilling())

	productAPI.Handle("/restaurant-settings", withProductMiddleware(http.HandlerFunc(handler.getRestaurantSettings), requireProfile)).Methods(http.MethodGet)
	productAPI.Handle("/branding", withProductMiddleware(http.HandlerFunc(handler.getTenantBranding), requireProfile)).Methods(http.MethodGet)
	productAPI.Handle("/branding/assets", withProductMiddleware(http.HandlerFunc(handler.listTenantBrandingAssets), requireProfile)).Methods(http.MethodGet)
	productAPI.Handle("/branding/assets/{kind}", withProductMiddleware(http.HandlerFunc(handler.serveTenantBrandingAsset), requireProfile)).Methods(http.MethodGet)
	productAPI.Handle("/locations", withProductMiddleware(http.HandlerFunc(handler.create), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodPost)
	productAPI.Handle("/restaurant-settings", withProductMiddleware(http.HandlerFunc(handler.putRestaurantSettings), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodPut)
	productAPI.Handle("/branding", withProductMiddleware(http.HandlerFunc(handler.putTenantBranding), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodPut)
	productAPI.Handle("/branding/assets/{kind}", withProductMiddleware(http.HandlerFunc(handler.putTenantBrandingAsset), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodPut)
	productAPI.Handle("/branding/assets/{kind}", withProductMiddleware(http.HandlerFunc(handler.deleteTenantBrandingAsset), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodDelete)
	productAPI.HandleFunc("/staff-profile", handler.getStaffProfile).Methods(http.MethodGet)
	productAPI.Handle("/locations", withProductMiddleware(http.HandlerFunc(handler.list), requireProfile)).Methods(http.MethodGet)
	productAPI.Handle("/staff-profiles", withProductMiddleware(http.HandlerFunc(handler.listStaffProfiles), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodGet)
	productAPI.Handle("/staff-profiles/{userId}", withProductMiddleware(http.HandlerFunc(handler.replaceStaffProfile), requireProfile, requireCoreRole(models.RoleAdmin))).Methods(http.MethodPut)
	productAPI.Handle("/units", withProductMiddleware(http.HandlerFunc(handler.listUnits), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/units", withProductMiddleware(http.HandlerFunc(handler.createUnit), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/units/{unitId}", withProductMiddleware(http.HandlerFunc(handler.updateUnit), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPatch)
	productAPI.Handle("/categories", withProductMiddleware(http.HandlerFunc(handler.listCategories), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/categories", withProductMiddleware(http.HandlerFunc(handler.createCategory), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/categories/{categoryId}", withProductMiddleware(http.HandlerFunc(handler.updateCategory), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPatch)
	productAPI.Handle("/items", withProductMiddleware(http.HandlerFunc(handler.listItems), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/items", withProductMiddleware(http.HandlerFunc(handler.createItem), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/items/{itemId}", withProductMiddleware(http.HandlerFunc(handler.updateItem), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPatch)
	productAPI.Handle("/items/{itemId}/conversions", withProductMiddleware(http.HandlerFunc(handler.listItemConversions), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/items/{itemId}/conversions", withProductMiddleware(http.HandlerFunc(handler.createItemConversion), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/items/{itemId}/conversions/{conversionId}", withProductMiddleware(http.HandlerFunc(handler.updateItemConversion), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPatch)
	productAPI.Handle("/suppliers", withProductMiddleware(http.HandlerFunc(handler.listSuppliers), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/suppliers", withProductMiddleware(http.HandlerFunc(handler.createSupplier), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/suppliers/{supplierId}", withProductMiddleware(http.HandlerFunc(handler.updateSupplier), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPatch)
	productAPI.Handle("/suppliers/{supplierId}/items", withProductMiddleware(http.HandlerFunc(handler.listSupplierItems), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/suppliers/{supplierId}/items", withProductMiddleware(http.HandlerFunc(handler.createSupplierItem), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/suppliers/{supplierId}/items/{supplierItemId}", withProductMiddleware(http.HandlerFunc(handler.updateSupplierItem), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPatch)
	productAPI.Handle("/recipes", withProductMiddleware(http.HandlerFunc(handler.listRecipes), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/recipes", withProductMiddleware(http.HandlerFunc(handler.createRecipe), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/recipes/{recipeId}", withProductMiddleware(http.HandlerFunc(handler.updateRecipe), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPatch)
	productAPI.Handle("/recipes/{recipeId}/versions", withProductMiddleware(http.HandlerFunc(handler.listRecipeVersions), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/recipes/{recipeId}/versions", withProductMiddleware(http.HandlerFunc(handler.createRecipeVersion), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/recipe-versions/{recipeVersionId}/components", withProductMiddleware(http.HandlerFunc(handler.listRecipeComponents), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/recipe-versions/{recipeVersionId}/release", withProductMiddleware(http.HandlerFunc(handler.releaseRecipeVersion), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/recipes/{recipeId}/versions/{recipeVersionId}/components", withProductMiddleware(http.HandlerFunc(handler.listRecipeComponents), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/recipes/{recipeId}/versions/{recipeVersionId}/release", withProductMiddleware(http.HandlerFunc(handler.releaseRecipeVersion), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/external-product-mappings", withProductMiddleware(http.HandlerFunc(handler.listMappings), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/external-product-mappings", withProductMiddleware(http.HandlerFunc(handler.createMapping), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/external-product-mappings/{mappingId}", withProductMiddleware(http.HandlerFunc(handler.updateMapping), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPatch)
	productAPI.Handle("/external-product-mappings/{mappingId}", withProductMiddleware(http.HandlerFunc(handler.deleteMapping), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodDelete)
	productAPI.Handle("/sales/import-template", withProductMiddleware(http.HandlerFunc(handler.getSalesImportTemplate), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/sales/imports/template", withProductMiddleware(http.HandlerFunc(handler.getSalesImportTemplate), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/sales/imports/dry-run", withProductMiddleware(http.HandlerFunc(handler.dryRunSalesImport), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodPost)
	productAPI.Handle("/sales/imports", withProductMiddleware(http.HandlerFunc(handler.applySalesImport), requireProfile, requireSalesManager())).Methods(http.MethodPost)
	productAPI.Handle("/sales/imports", withProductMiddleware(http.HandlerFunc(handler.listSalesImportRuns), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/sales/imports/{runId}", withProductMiddleware(http.HandlerFunc(handler.getSalesImportRun), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/sales/unmapped", withProductMiddleware(http.HandlerFunc(handler.listUnmappedSales), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/sales/{saleId}/cancel", withProductMiddleware(http.HandlerFunc(handler.cancelSale), requireProfile, requireSalesManager())).Methods(http.MethodPost)
	productAPI.Handle("/purchase-orders", withProductMiddleware(http.HandlerFunc(handler.listPurchaseOrders), requireProfile, RequireBusinessPermission(models.PermissionPurchasingRead))).Methods(http.MethodGet)
	productAPI.Handle("/purchase-orders", withProductMiddleware(http.HandlerFunc(handler.createPurchaseOrder), requireProfile, RequireBusinessPermission(models.PermissionPurchasingManage))).Methods(http.MethodPost)
	productAPI.Handle("/purchase-orders/{orderId}", withProductMiddleware(http.HandlerFunc(handler.getPurchaseOrder), requireProfile, RequireBusinessPermission(models.PermissionPurchasingRead))).Methods(http.MethodGet)
	productAPI.Handle("/purchase-orders/{orderId}/document", withProductMiddleware(http.HandlerFunc(handler.getPurchaseOrderDocument), requireProfile, RequireBusinessPermission(models.PermissionPurchasingRead))).Methods(http.MethodGet)
	productAPI.Handle("/purchase-orders/{orderId}/document/send", withProductMiddleware(http.HandlerFunc(handler.sendPurchaseOrderDocument), requireProfile, RequireBusinessPermission(models.PermissionPurchasingManage), requirePurchasingManager())).Methods(http.MethodPost)
	productAPI.Handle("/purchase-orders/{orderId}", withProductMiddleware(http.HandlerFunc(handler.updatePurchaseOrder), requireProfile, RequireBusinessPermission(models.PermissionPurchasingManage))).Methods(http.MethodPatch)
	productAPI.Handle("/purchase-orders/{orderId}/submit", withProductMiddleware(http.HandlerFunc(handler.submitPurchaseOrder), requireProfile, RequireBusinessPermission(models.PermissionPurchasingManage))).Methods(http.MethodPost)
	productAPI.Handle("/purchase-orders/{orderId}/approve", withProductMiddleware(http.HandlerFunc(handler.approvePurchaseOrder), requireProfile, RequireBusinessPermission(models.PermissionPurchasingApprove), requirePurchasingManager())).Methods(http.MethodPost)
	productAPI.Handle("/purchase-orders/{orderId}/supplier-confirm", withProductMiddleware(http.HandlerFunc(handler.confirmSupplierPurchaseOrder), requireProfile, RequireBusinessPermission(models.PermissionPurchasingManage))).Methods(http.MethodPost)
	productAPI.Handle("/purchase-orders/{orderId}/cancel", withProductMiddleware(http.HandlerFunc(handler.cancelPurchaseOrder), requireProfile, RequireBusinessPermission(models.PermissionPurchasingManage))).Methods(http.MethodPost)
	productAPI.Handle("/purchase-orders/{orderId}/receipts", withProductMiddleware(http.HandlerFunc(handler.createGoodsReceipt), requireProfile, RequireBusinessPermission(models.PermissionPurchasingReceive), RequireBusinessPermission(models.PermissionInventoryPost))).Methods(http.MethodPost)
	productAPI.Handle("/purchase-orders/{orderId}/receipts", withProductMiddleware(http.HandlerFunc(handler.listOrderGoodsReceipts), requireProfile, RequirePurchasingReceiveReadPermission())).Methods(http.MethodGet)
	productAPI.Handle("/purchase-orders/{orderId}/goods-receipts", withProductMiddleware(http.HandlerFunc(handler.listOrderGoodsReceipts), requireProfile, RequirePurchasingReceiveReadPermission())).Methods(http.MethodGet)
	productAPI.Handle("/goods-receipts", withProductMiddleware(http.HandlerFunc(handler.listGoodsReceipts), requireProfile, RequirePurchasingReceiveReadPermission())).Methods(http.MethodGet)
	productAPI.Handle("/goods-receipts/{receiptId}", withProductMiddleware(http.HandlerFunc(handler.getGoodsReceipt), requireProfile, RequirePurchasingReceiveReadPermission())).Methods(http.MethodGet)
	productAPI.Handle("/goods-receipts/{receiptId}/reverse", withProductMiddleware(http.HandlerFunc(handler.reverseGoodsReceipt), requireProfile, RequireBusinessPermission(models.PermissionPurchasingReceive), RequireBusinessPermission(models.PermissionInventoryPost))).Methods(http.MethodPost)
	productAPI.Handle("/goods-receipts/{receiptId}/cancel", withProductMiddleware(http.HandlerFunc(handler.reverseGoodsReceipt), requireProfile, RequireBusinessPermission(models.PermissionPurchasingReceive), RequireBusinessPermission(models.PermissionInventoryPost))).Methods(http.MethodPost)
	productAPI.Handle("/delivery-calendars", withProductMiddleware(http.HandlerFunc(handler.listDeliveryCalendars), requireProfile, RequireBusinessPermission(models.PermissionPurchasingRead))).Methods(http.MethodGet)
	productAPI.Handle("/delivery-calendars", withProductMiddleware(http.HandlerFunc(handler.createDeliveryCalendar), requireProfile, RequireBusinessPermission(models.PermissionPurchasingManage))).Methods(http.MethodPost)
	productAPI.Handle("/delivery-calendars/{calendarId}", withProductMiddleware(http.HandlerFunc(handler.updateDeliveryCalendar), requireProfile, RequireBusinessPermission(models.PermissionPurchasingManage))).Methods(http.MethodPatch)
	productAPI.Handle("/delivery-calendars/{calendarId}", withProductMiddleware(http.HandlerFunc(handler.deleteDeliveryCalendar), requireProfile, RequireBusinessPermission(models.PermissionPurchasingManage))).Methods(http.MethodDelete)
	forecastLocation := RequireAuthorizedLocation(database, "locationId")
	productAPI.Handle("/locations/{locationId}/forecast/datasets", withProductMiddleware(http.HandlerFunc(handler.listForecastDatasets), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/guest-plans", withProductMiddleware(http.HandlerFunc(handler.listGuestPlans), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/guest-plans", withProductMiddleware(http.HandlerFunc(handler.createGuestPlan), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastManage), requireForecastManager())).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/forecast/policies", withProductMiddleware(http.HandlerFunc(handler.listForecastPolicies), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/policies", withProductMiddleware(http.HandlerFunc(handler.createForecastPolicy), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastManage), requireForecastManager())).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/forecast/policies/{policyId}", withProductMiddleware(http.HandlerFunc(handler.updateForecastPolicy), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastManage), requireForecastManager())).Methods(http.MethodPatch)
	productAPI.Handle("/locations/{locationId}/forecast/runs", withProductMiddleware(http.HandlerFunc(handler.listForecastRuns), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/runs", withProductMiddleware(http.HandlerFunc(handler.createForecastRun), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRun), requireForecastManager())).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/forecast/jobs", withProductMiddleware(http.HandlerFunc(handler.listForecastJobs), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/runs/{runId}/points", withProductMiddleware(http.HandlerFunc(handler.listForecastPoints), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/runs/{runId}/maturity", withProductMiddleware(http.HandlerFunc(handler.listForecastMaturity), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/datasets/{datasetId}/inputs", withProductMiddleware(http.HandlerFunc(handler.listForecastInputs), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/recommendations", withProductMiddleware(http.HandlerFunc(handler.listReorderRecommendations), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/recommendations/{recommendationId}", withProductMiddleware(http.HandlerFunc(handler.getReorderRecommendation), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/reorder-recommendations/{id}/purchase-order-draft", withProductMiddleware(http.HandlerFunc(handler.createPurchaseOrderDraftFromRecommendation), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastManage), RequireBusinessPermission(models.PermissionPurchasingManage))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/forecast/coverage", withProductMiddleware(http.HandlerFunc(handler.listForecastCoverage), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/coverage/{coverageId}", withProductMiddleware(http.HandlerFunc(handler.getForecastCoverage), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/runs/{runId}/recommendations", withProductMiddleware(http.HandlerFunc(handler.listReorderRecommendations), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/runs/{runId}/recommendations/{recommendationId}", withProductMiddleware(http.HandlerFunc(handler.getReorderRecommendation), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/runs/{runId}/coverage", withProductMiddleware(http.HandlerFunc(handler.listForecastCoverage), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/forecast/runs/{runId}/coverage/{coverageId}", withProductMiddleware(http.HandlerFunc(handler.getForecastCoverage), requireProfile, forecastLocation, RequireBusinessPermission(models.PermissionForecastRead))).Methods(http.MethodGet)
	productAPI.Handle("/import-templates/{target}", withProductMiddleware(http.HandlerFunc(handler.getImportTemplate), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/imports/dry-run", withProductMiddleware(http.HandlerFunc(handler.dryRunImport), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodPost)
	productAPI.Handle("/imports", withProductMiddleware(http.HandlerFunc(handler.applyImport), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/imports/apply", withProductMiddleware(http.HandlerFunc(handler.applyImport), requireProfile, RequireBusinessPermission(models.PermissionCatalogManage))).Methods(http.MethodPost)
	productAPI.Handle("/imports", withProductMiddleware(http.HandlerFunc(handler.listImportRuns), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/imports/runs", withProductMiddleware(http.HandlerFunc(handler.listImportRuns), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/imports/{runId}", withProductMiddleware(http.HandlerFunc(handler.getImportRun), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)
	productAPI.Handle("/imports/runs/{runId}", withProductMiddleware(http.HandlerFunc(handler.getImportRun), requireProfile, RequireBusinessPermission(models.PermissionCatalogRead))).Methods(http.MethodGet)

	requireLocation := RequireAuthorizedLocation(database, "locationId")
	productAPI.Handle("/locations/{locationId}", withProductMiddleware(http.HandlerFunc(handler.update), requireProfile, requireCoreRole(models.RoleAdmin), requireLocation)).Methods(http.MethodPatch)
	productAPI.Handle("/locations/{locationId}/branding", withProductMiddleware(http.HandlerFunc(handler.getLocationBranding), requireProfile, requireLocation)).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/branding", withProductMiddleware(http.HandlerFunc(handler.putLocationBranding), requireProfile, requireLocation, requireCoreRole(models.RoleAdmin), middleware.RequireEntitlement(database, "location_branding"))).Methods(http.MethodPut)
	productAPI.Handle("/locations/{locationId}/branding", withProductMiddleware(http.HandlerFunc(handler.deleteLocationBranding), requireProfile, requireLocation, requireCoreRole(models.RoleAdmin), middleware.RequireEntitlement(database, "location_branding"))).Methods(http.MethodDelete)
	productAPI.Handle("/locations/{locationId}/storage-areas", withProductMiddleware(http.HandlerFunc(handler.listStorageAreas), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionStorageAreasRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/storage-areas", withProductMiddleware(http.HandlerFunc(handler.createStorageArea), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionStorageAreasManage))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/storage-areas/{storageAreaId}", withProductMiddleware(http.HandlerFunc(handler.updateStorageArea), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionStorageAreasManage))).Methods(http.MethodPatch)
	productAPI.Handle("/locations/{locationId}/inventory/opening-balance", withProductMiddleware(http.HandlerFunc(handler.postOpeningBalance), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryPost))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/adjustments", withProductMiddleware(http.HandlerFunc(handler.postAdjustment), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryPost))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/waste", withProductMiddleware(http.HandlerFunc(handler.postWaste), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryPost))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/transfers", withProductMiddleware(http.HandlerFunc(handler.postTransfer), requireProfile, requireLocation, RequireInventoryDestinationLocation(database), RequireBusinessPermission(models.PermissionInventoryPost))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/postings/{postingId}/reverse", withProductMiddleware(http.HandlerFunc(handler.reverseInventoryPosting), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryPost), RequireAuthorizedReversalLocations(database))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/journal", withProductMiddleware(http.HandlerFunc(handler.listInventoryJournal), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/inventory/balances", withProductMiddleware(http.HandlerFunc(handler.listInventoryBalances), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryRead))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/inventory/references", withProductMiddleware(http.HandlerFunc(handler.listInventoryReferences), requireProfile, requireLocation, RequireInventoryReferencePermission())).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/inventory/counts", withProductMiddleware(http.HandlerFunc(handler.createStockCount), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/counts", withProductMiddleware(http.HandlerFunc(handler.listStockCounts), requireProfile, requireLocation, RequireInventoryCountReadPermission())).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/inventory/counts/{countId}", withProductMiddleware(http.HandlerFunc(handler.getStockCount), requireProfile, requireLocation, RequireInventoryCountReadPermission())).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/inventory/counts/{countId}/lot-options", withProductMiddleware(http.HandlerFunc(handler.listStockCountLotOptions), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodGet)
	productAPI.Handle("/locations/{locationId}/inventory/counts/{countId}/freeze", withProductMiddleware(http.HandlerFunc(handler.freezeStockCount), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/counts/{countId}/lines/discover", withProductMiddleware(http.HandlerFunc(handler.discoverStockCountLine), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/counts/{countId}/lines/{lineId}", withProductMiddleware(http.HandlerFunc(handler.recordStockCountLine), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodPatch)
	productAPI.Handle("/locations/{locationId}/inventory/counts/{countId}/review", withProductMiddleware(http.HandlerFunc(handler.reviewStockCount), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/counts/{countId}/cancel", withProductMiddleware(http.HandlerFunc(handler.cancelStockCount), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/counts/{countId}/post", withProductMiddleware(http.HandlerFunc(handler.postStockCount), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryPost))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/reconciliation", withProductMiddleware(http.HandlerFunc(handler.runInventoryReconciliation), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodPost)
	productAPI.Handle("/locations/{locationId}/inventory/reconciliation/{runId}", withProductMiddleware(http.HandlerFunc(handler.getInventoryReconciliation), requireProfile, requireLocation, RequireBusinessPermission(models.PermissionInventoryRead))).Methods(http.MethodGet)
	productAPI.Handle("/inventory/references", withProductMiddleware(http.HandlerFunc(handler.listInventoryReferences), requireProfile, RequireInventoryReferencePermission())).Methods(http.MethodGet)
	productAPI.Handle("/items/{itemId}/lots", withProductMiddleware(http.HandlerFunc(handler.listInventoryLots), requireProfile, RequireBusinessPermission(models.PermissionInventoryRead))).Methods(http.MethodGet)
	productAPI.Handle("/items/{itemId}/lots", withProductMiddleware(http.HandlerFunc(handler.createInventoryLot), requireProfile, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodPost)
	productAPI.Handle("/items/{itemId}/lots/{lotId}", withProductMiddleware(http.HandlerFunc(handler.updateInventoryLot), requireProfile, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodPatch)
	productAPI.Handle("/items/{itemId}/lot-tracking", withProductMiddleware(http.HandlerFunc(handler.updateLotTrackingMode), requireProfile, RequireBusinessPermission(models.PermissionInventoryManage))).Methods(http.MethodPatch)
}

func withProductMiddleware(handler http.Handler, chain ...func(http.Handler) http.Handler) http.Handler {
	for i := len(chain) - 1; i >= 0; i-- {
		handler = chain[i](handler)
	}
	return handler
}

func (h *productHandler) update(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)["locationId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid location ID")
		return
	}

	var request struct {
		Version  int64   `json:"version" validate:"gte=1"`
		Name     *string `json:"name" validate:"omitempty,min=1,max=200"`
		Timezone *string `json:"timezone" validate:"omitempty,max=100,iana_timezone"`
		IsActive *bool   `json:"isActive"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		apierror.BadRequest(w, r, "Invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		apierror.BadRequest(w, r, "Invalid request body")
		return
	}
	if request.Name == nil && request.Timezone == nil && request.IsActive == nil {
		apierror.BadRequest(w, r, "At least one mutable field is required")
		return
	}
	if request.Name != nil {
		trimmed := strings.TrimSpace(*request.Name)
		request.Name = &trimmed
	}
	if err := validation.Validate(&request); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}

	location, err := NewLocationRepository(h.db, tenant).Update(r.Context(), locationID, request.Version, LocationUpdate{
		Name: request.Name, Timezone: request.Timezone, IsActive: request.IsActive,
	})
	switch {
	case err == nil:
		action := "location.updated"
		message := "Location updated"
		if request.IsActive != nil {
			if *request.IsActive {
				action = "location.reactivated"
				message = "Location reactivated"
			} else {
				action = "location.deactivated"
				message = "Location deactivated"
			}
		}
		if user, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, message, user.ID, tenant.ID, action, map[string]interface{}{
				"locationId": location.ID.Hex(),
				"code":       location.Code,
				"version":    location.Version,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"location": location})
	case errors.Is(err, ErrLocationNotFound):
		apierror.NotFound(w, r, "Location not found")
	case errors.Is(err, ErrLocationVersionConflict):
		apierror.VersionConflict(w, r, "Location was modified by another request")
	default:
		if strings.HasPrefix(err.Error(), "validation failed:") {
			apierror.Validation(w, r, err.Error())
			return
		}
		apierror.Internal(w, r, "Failed to update location")
	}
}

func (h *productHandler) list(w http.ResponseWriter, r *http.Request) {
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
	var assigned []primitive.ObjectID
	if !profile.AllLocations {
		assigned = make([]primitive.ObjectID, len(profile.LocationIDs))
		copy(assigned, profile.LocationIDs)
	}
	locations, err := NewLocationRepository(h.db, tenant).ListAssigned(r.Context(), assigned)
	if err != nil {
		apierror.Internal(w, r, "Failed to list locations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"locations": locations})
}

func (h *productHandler) create(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return
	}
	var request struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Timezone string `json:"timezone"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		apierror.BadRequest(w, r, "Invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		apierror.BadRequest(w, r, "Invalid request body")
		return
	}

	location, err := NewLocationRepository(h.db, tenant).Create(
		r.Context(), request.Code, strings.TrimSpace(request.Name), request.Timezone,
	)
	switch {
	case err == nil:
		if user, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, "Location created", user.ID, tenant.ID, "location.created", map[string]interface{}{
				"locationId": location.ID.Hex(),
				"code":       location.Code,
			})
		}
		writeJSON(w, http.StatusCreated, map[string]any{"location": location})
	case errors.Is(err, ErrDuplicateLocationCode):
		apierror.Conflict(w, r, "Location code already exists")
	case errors.Is(err, ErrLocationLimitReached):
		apierror.PlanLimit(w, r, "Maximum number of locations reached")
	default:
		if strings.HasPrefix(err.Error(), "validation failed:") {
			apierror.Validation(w, r, err.Error())
			return
		}
		apierror.Internal(w, r, "Failed to create location")
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
