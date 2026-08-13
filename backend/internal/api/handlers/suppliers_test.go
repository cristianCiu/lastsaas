package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"lastsaas/internal/apierror"
	"lastsaas/internal/models"
	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
)

func TestSuppliersLifecyclePermissionsDuplicateAndTenantIsolation(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "suppliers")

	var created struct {
		Supplier models.Supplier `json:"supplier"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/suppliers", strings.NewReader(`{"code":"ACME","name":" Acme Foods ","contactName":"Buyer","email":"buyer@example.com","orderingDays":[1,3,3],"defaultLeadTimeDays":5}`), owner, tenant.ID.Hex()), http.StatusCreated, &created)
	if created.Supplier.Code != "acme" || created.Supplier.Name != "Acme Foods" || len(created.Supplier.OrderingDays) != 2 || created.Supplier.Version != 1 || !created.Supplier.IsActive {
		t.Fatalf("unexpected supplier: %#v", created.Supplier)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/suppliers", strings.NewReader(`{"code":"acme","name":"Duplicate","defaultLeadTimeDays":1}`), owner, tenant.ID.Hex()), http.StatusConflict, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/suppliers", strings.NewReader(`{"code":"bad-email","name":"Bad","email":"not-an-email","defaultLeadTimeDays":1}`), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)

	member := testutil.CreateTestUser(t, env.DB, "supplier-reader@test.com", "Test1234!@#$", "Supplier Reader")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	if _, err := env.DB.StaffProfiles().UpdateOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": member.ID}, bson.M{"$set": bson.M{"permissionOverrides": bson.A{
		bson.M{"permission": models.PermissionCatalogRead, "allowed": true},
		bson.M{"permission": models.PermissionCatalogManage, "allowed": false},
	}}}); err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Suppliers []models.Supplier `json:"suppliers"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/suppliers", nil, member, tenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.Suppliers) != 1 || listed.Suppliers[0].ID != created.Supplier.ID {
		t.Fatalf("unexpected supplier list: %#v", listed.Suppliers)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, "/api/product/suppliers/"+created.Supplier.ID.Hex(), strings.NewReader(`{"version":1,"name":"Denied"}`), member, tenant.ID.Hex()), http.StatusForbidden, nil)

	path := "/api/product/suppliers/" + created.Supplier.ID.Hex()
	var updated struct {
		Supplier models.Supplier `json:"supplier"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"name":"Acme Updated","isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if updated.Supplier.Name != "Acme Updated" || updated.Supplier.IsActive || updated.Supplier.Version != 2 {
		t.Fatalf("unexpected supplier update: %#v", updated.Supplier)
	}
	var conflict apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"name":"Stale"}`), owner, tenant.ID.Hex()), http.StatusConflict, &conflict)
	if conflict.Code != apierror.CodeVersionConflict {
		t.Fatalf("unexpected supplier stale response: %#v", conflict)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/suppliers", nil, owner, tenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.Suppliers) != 0 {
		t.Fatalf("inactive supplier should be hidden: %#v", listed.Suppliers)
	}

	otherOwner, otherTenant := setupProductTenant(t, env, "suppliers-other")
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/suppliers", nil, otherOwner, otherTenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.Suppliers) != 0 {
		t.Fatalf("suppliers leaked across tenants: %#v", listed.Suppliers)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":2,"name":"Cross tenant"}`), otherOwner, otherTenant.ID.Hex()), http.StatusNotFound, nil)
}

func TestSupplierItemsTermsReferencesExactStringsAndLifecycle(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "supplier-terms")
	supplier := createTestSupplier(t, env, owner, tenant.ID.Hex(), `{"code":"fresh","name":"Fresh Supplier","defaultLeadTimeDays":4}`)
	category := createTestCategory(t, env, owner, tenant.ID.Hex(), `{"code":"food","name":"Food"}`)
	unit := createTestUnit(t, env, owner, tenant.ID.Hex(), `{"code":"kg","name":"Kilogram","symbol":"kg","dimension":"mass","precision":3}`)
	item := createTestItemForSupplierTest(t, env, owner, tenant.ID.Hex(), category.ID.Hex(), unit.ID.Hex(), "flour")

	path := "/api/product/suppliers/" + supplier.ID.Hex() + "/items"
	var created struct {
		SupplierItem models.SupplierItem `json:"supplierItem"`
	}
	body := `{"itemId":"` + item.ID.Hex() + `","supplierSKU":"FLOUR-1","packSizeMicros":"2500000","moq":2,"unitPriceMinor":"9007199254740993","currency":"EUR","leadTimeDays":7}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, path, strings.NewReader(body), owner, tenant.ID.Hex()), http.StatusCreated, &created)
	if created.SupplierItem.PackSizeMicros != 2500000 || created.SupplierItem.UnitPriceMinor != 9007199254740993 || created.SupplierItem.Currency != "EUR" || created.SupplierItem.Version != 1 || !created.SupplierItem.IsActive {
		t.Fatalf("unexpected supplier item: %#v", created.SupplierItem)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, path, strings.NewReader(body), owner, tenant.ID.Hex()), http.StatusConflict, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, path, strings.NewReader(`{"itemId":"`+item.ID.Hex()+`","packSizeMicros":2500000,"moq":1,"unitPriceMinor":"1","currency":"EUR"}`), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, path, strings.NewReader(`{"itemId":"`+item.ID.Hex()+`","packSizeMicros":"1","moq":1,"unitPriceMinor":"1","currency":"EURO"}`), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)

	itemPath := path + "/" + created.SupplierItem.ID.Hex()
	var updated struct {
		SupplierItem models.SupplierItem `json:"supplierItem"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, itemPath, strings.NewReader(`{"version":1,"moq":3,"unitPriceMinor":"42","leadTimeDays":null}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if updated.SupplierItem.MOQ != 3 || updated.SupplierItem.UnitPriceMinor != 42 || updated.SupplierItem.LeadTimeDays != nil || updated.SupplierItem.Version != 2 {
		t.Fatalf("unexpected supplier item update: %#v", updated.SupplierItem)
	}
	var conflict apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, itemPath, strings.NewReader(`{"version":1,"moq":4}`), owner, tenant.ID.Hex()), http.StatusConflict, &conflict)
	if conflict.Code != apierror.CodeVersionConflict {
		t.Fatalf("unexpected supplier item stale response: %#v", conflict)
	}

	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, "/api/product/suppliers/"+supplier.ID.Hex(), strings.NewReader(`{"version":1,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, "/api/product/items/"+item.ID.Hex(), strings.NewReader(`{"version":1,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, itemPath, strings.NewReader(`{"version":2,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if updated.SupplierItem.IsActive || updated.SupplierItem.Version != 3 {
		t.Fatalf("existing term should remain updateable: %#v", updated.SupplierItem)
	}

	otherOwner, otherTenant := setupProductTenant(t, env, "supplier-terms-other")
	otherSupplier := createTestSupplier(t, env, otherOwner, otherTenant.ID.Hex(), `{"code":"other","name":"Other Supplier","defaultLeadTimeDays":1}`)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/suppliers/"+otherSupplier.ID.Hex()+"/items", nil, owner, tenant.ID.Hex()), http.StatusNotFound, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, path, strings.NewReader(`{"itemId":"`+item.ID.Hex()+`","packSizeMicros":"1","moq":1,"unitPriceMinor":"1","currency":"EUR"}`), owner, tenant.ID.Hex()), http.StatusNotFound, nil)
}

func createTestSupplier(t *testing.T, env *testEnv, user *models.User, tenantID, body string) models.Supplier {
	t.Helper()
	var response struct {
		Supplier models.Supplier `json:"supplier"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/suppliers", strings.NewReader(body), user, tenantID), http.StatusCreated, &response)
	return response.Supplier
}

func createTestItemForSupplierTest(t *testing.T, env *testEnv, user *models.User, tenantID, categoryID, unitID, sku string) models.Item {
	t.Helper()
	var response struct {
		Item models.Item `json:"item"`
	}
	body := `{"sku":"` + sku + `","name":"` + sku + `","categoryId":"` + categoryID + `","baseUnitId":"` + unitID + `","stockable":true}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/items", strings.NewReader(body), user, tenantID), http.StatusCreated, &response)
	return response.Item
}
