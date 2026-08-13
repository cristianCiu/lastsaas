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

func TestItemsLifecycleReferencesPermissionsAndTenantIsolation(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "items")

	category := createTestCategory(t, env, owner, tenant.ID.Hex(), `{"code":"hot-food","name":"Hot Food"}`)
	unit := createTestUnit(t, env, owner, tenant.ID.Hex(), `{"code":"kg","name":"Kilogram","symbol":"kg","dimension":"mass","precision":3}`)

	var created struct {
		Item models.Item `json:"item"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/items", strings.NewReader(`{"sku":"BURGER-001","name":" Burger ","description":"Classic","brand":"Acme","categoryId":"`+category.ID.Hex()+`","baseUnitId":"`+unit.ID.Hex()+`","allergens":["milk","milk","eggs"],"shelfLifeDays":30,"stockable":false}`), owner, tenant.ID.Hex()), http.StatusCreated, &created)
	if created.Item.SKU != "burger-001" || created.Item.Name != "Burger" || len(created.Item.Allergens) != 2 || created.Item.Version != 1 || !created.Item.IsActive || created.Item.Stockable {
		t.Fatalf("unexpected item: %#v", created.Item)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/items", strings.NewReader(`{"sku":"burger-001","name":"Duplicate","categoryId":"`+category.ID.Hex()+`","baseUnitId":"`+unit.ID.Hex()+`","stockable":true}`), owner, tenant.ID.Hex()), http.StatusConflict, nil)

	member := testutil.CreateTestUser(t, env.DB, "item-reader@test.com", "Test1234!@#$", "Reader")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	if _, err := env.DB.StaffProfiles().UpdateOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": member.ID}, bson.M{"$set": bson.M{"permissionOverrides": bson.A{
		bson.M{"permission": models.PermissionCatalogRead, "allowed": true}, bson.M{"permission": models.PermissionCatalogManage, "allowed": false},
	}}}); err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Items []models.Item `json:"items"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/items", nil, member, tenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.Items) != 1 || listed.Items[0].ID != created.Item.ID {
		t.Fatalf("unexpected item list: %#v", listed.Items)
	}
	path := "/api/product/items/" + created.Item.ID.Hex()
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"name":"Denied"}`), member, tenant.ID.Hex()), http.StatusForbidden, nil)

	var updated struct {
		Item models.Item `json:"item"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"stockable":true,"shelfLifeDays":null}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if !updated.Item.Stockable || updated.Item.Version != 2 || updated.Item.ShelfLifeDays != nil {
		t.Fatalf("unexpected update: %#v", updated.Item)
	}
	var conflict apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"name":"Stale"}`), owner, tenant.ID.Hex()), http.StatusConflict, &conflict)
	if conflict.Code != apierror.CodeVersionConflict {
		t.Fatalf("unexpected stale response: %#v", conflict)
	}

	otherOwner, otherTenant := setupProductTenant(t, env, "items-other")
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":2,"name":"Cross tenant"}`), otherOwner, otherTenant.ID.Hex()), http.StatusNotFound, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/items", nil, otherOwner, otherTenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.Items) != 0 {
		t.Fatalf("items leaked across tenants: %#v", listed.Items)
	}
}

func TestItemsRejectWrongAndInactiveReferences(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "item-refs")
	category := createTestCategory(t, env, owner, tenant.ID.Hex(), `{"code":"food","name":"Food"}`)
	unit := createTestUnit(t, env, owner, tenant.ID.Hex(), `{"code":"each","name":"Each","symbol":"ea","dimension":"count","precision":0}`)
	otherOwner, otherTenant := setupProductTenant(t, env, "item-refs-other")
	otherCategory := createTestCategory(t, env, otherOwner, otherTenant.ID.Hex(), `{"code":"other","name":"Other"}`)
	bad := `{"sku":"bad-ref","name":"Bad ref","categoryId":"` + otherCategory.ID.Hex() + `","baseUnitId":"` + unit.ID.Hex() + `","stockable":true}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/items", strings.NewReader(bad), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)
	categoryPath := "/api/product/categories/" + category.ID.Hex()
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, categoryPath, strings.NewReader(`{"version":1,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/items", strings.NewReader(`{"sku":"inactive-ref","name":"Inactive ref","categoryId":"`+category.ID.Hex()+`","baseUnitId":"`+unit.ID.Hex()+`","stockable":true}`), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)
}

func TestItemsRetainInactiveReferencesForNonReferenceUpdates(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "item-retain-inactive")
	category := createTestCategory(t, env, owner, tenant.ID.Hex(), `{"code":"food","name":"Food"}`)
	unit := createTestUnit(t, env, owner, tenant.ID.Hex(), `{"code":"each","name":"Each","symbol":"ea","dimension":"count","precision":0}`)
	var created struct {
		Item models.Item `json:"item"`
	}
	createBody := `{"sku":"retained-ref","name":"Retained","categoryId":"` + category.ID.Hex() + `","baseUnitId":"` + unit.ID.Hex() + `","stockable":true}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/items", strings.NewReader(createBody), owner, tenant.ID.Hex()), http.StatusCreated, &created)

	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, "/api/product/categories/"+category.ID.Hex(), strings.NewReader(`{"version":1,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, "/api/product/units/"+unit.ID.Hex(), strings.NewReader(`{"version":1,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, nil)

	path := "/api/product/items/" + created.Item.ID.Hex()
	var updated struct {
		Item models.Item `json:"item"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"name":"Still Retained"}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if updated.Item.Name != "Still Retained" || updated.Item.CategoryID != category.ID || updated.Item.BaseUnitID != unit.ID || updated.Item.Version != 2 {
		t.Fatalf("unexpected retained-reference update: %#v", updated.Item)
	}

	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":2,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if updated.Item.IsActive || updated.Item.Version != 3 {
		t.Fatalf("expected lifecycle update with inactive references: %#v", updated.Item)
	}

	// Supplying the same inactive references is still a non-change and remains valid.
	sameRefs := `{"version":3,"categoryId":"` + category.ID.Hex() + `","baseUnitId":"` + unit.ID.Hex() + `","isActive":true}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(sameRefs), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if !updated.Item.IsActive || updated.Item.Version != 4 {
		t.Fatalf("expected same-reference reactivation: %#v", updated.Item)
	}

	// A genuinely changed reference must still be active.
	newCategory := createTestCategory(t, env, owner, tenant.ID.Hex(), `{"code":"drinks","name":"Drinks"}`)
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":4,"categoryId":"`+newCategory.ID.Hex()+`"}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if updated.Item.CategoryID != newCategory.ID || updated.Item.Version != 5 {
		t.Fatalf("expected active reference change: %#v", updated.Item)
	}
}

func createTestCategory(t *testing.T, env *testEnv, user *models.User, tenantID, body string) models.Category {
	t.Helper()
	var response struct {
		Category models.Category `json:"category"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/categories", strings.NewReader(body), user, tenantID), http.StatusCreated, &response)
	return response.Category
}

func createTestUnit(t *testing.T, env *testEnv, user *models.User, tenantID, body string) models.Unit {
	t.Helper()
	var response struct {
		Unit models.Unit `json:"unit"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/units", strings.NewReader(body), user, tenantID), http.StatusCreated, &response)
	return response.Unit
}
