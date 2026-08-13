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

func TestCategoriesLifecyclePermissionsAndTenantIsolation(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "categories")

	var created struct {
		Category models.Category `json:"category"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/categories", strings.NewReader(`{"code":"HOT-FOOD","name":" Hot Food "}`), owner, tenant.ID.Hex()), http.StatusCreated, &created)
	if created.Category.Code != "hot-food" || created.Category.Name != "Hot Food" || created.Category.Version != 1 || !created.Category.IsActive {
		t.Fatalf("unexpected category: %#v", created.Category)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/categories", strings.NewReader(`{"code":"hot-food","name":"Duplicate"}`), owner, tenant.ID.Hex()), http.StatusConflict, nil)

	member := testutil.CreateTestUser(t, env.DB, "category-reader@test.com", "Test1234!@#$", "Reader")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	if _, err := env.DB.StaffProfiles().UpdateOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": member.ID}, bson.M{"$set": bson.M{"permissionOverrides": bson.A{
		bson.M{"permission": models.PermissionCatalogRead, "allowed": true}, bson.M{"permission": models.PermissionCatalogManage, "allowed": false},
	}}}); err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Categories []models.Category `json:"categories"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/categories?includeInactive=true", nil, member, tenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.Categories) != 1 || listed.Categories[0].ID != created.Category.ID {
		t.Fatalf("unexpected category list: %#v", listed.Categories)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, "/api/product/categories/"+created.Category.ID.Hex(), strings.NewReader(`{"version":1,"name":"Denied"}`), member, tenant.ID.Hex()), http.StatusForbidden, nil)

	path := "/api/product/categories/" + created.Category.ID.Hex()
	var updated struct {
		Category models.Category `json:"category"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"name":"Prepared Food"}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if updated.Category.Name != "Prepared Food" || updated.Category.Version != 2 {
		t.Fatalf("unexpected update: %#v", updated.Category)
	}
	var conflict apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusConflict, &conflict)
	if conflict.Code != apierror.CodeVersionConflict {
		t.Fatalf("unexpected stale response: %#v", conflict)
	}

	otherOwner, otherTenant := setupProductTenant(t, env, "categories-other")
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":2,"name":"Cross tenant"}`), otherOwner, otherTenant.ID.Hex()), http.StatusNotFound, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/categories?includeInactive=true", nil, otherOwner, otherTenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.Categories) != 0 {
		t.Fatalf("categories leaked across tenants: %#v", listed.Categories)
	}
}
