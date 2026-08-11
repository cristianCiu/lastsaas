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

func TestUnitsLifecyclePermissionsAndTenantIsolation(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "units")

	var created struct {
		Unit models.Unit `json:"unit"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/units", strings.NewReader(`{"code":"KG","name":" Kilogram ","symbol":" kg ","dimension":"mass","precision":3}`), owner, tenant.ID.Hex()), http.StatusCreated, &created)
	if created.Unit.Code != "kg" || created.Unit.Name != "Kilogram" || created.Unit.Symbol != "kg" || created.Unit.Version != 1 || !created.Unit.IsActive {
		t.Fatalf("unexpected unit: %#v", created.Unit)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/units", strings.NewReader(`{"code":"kg","name":"Duplicate","symbol":"kg","dimension":"mass","precision":3}`), owner, tenant.ID.Hex()), http.StatusConflict, nil)

	member := testutil.CreateTestUser(t, env.DB, "unit-reader@test.com", "Test1234!@#$", "Reader")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	if _, err := env.DB.StaffProfiles().UpdateOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": member.ID}, bson.M{"$set": bson.M{"permissionOverrides": bson.A{
		bson.M{"permission": models.PermissionCatalogRead, "allowed": true}, bson.M{"permission": models.PermissionCatalogManage, "allowed": false},
	}}}); err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Units []models.Unit `json:"units"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/units?includeInactive=true", nil, member, tenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.Units) != 1 || listed.Units[0].ID != created.Unit.ID {
		t.Fatalf("unexpected unit list: %#v", listed.Units)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, "/api/product/units/"+created.Unit.ID.Hex(), strings.NewReader(`{"version":1,"name":"Denied"}`), member, tenant.ID.Hex()), http.StatusForbidden, nil)

	var updated struct {
		Unit models.Unit `json:"unit"`
	}
	path := "/api/product/units/" + created.Unit.ID.Hex()
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"name":"Kilogram metric","precision":6}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if updated.Unit.Name != "Kilogram metric" || updated.Unit.Precision != 6 || updated.Unit.Version != 2 {
		t.Fatalf("unexpected update: %#v", updated.Unit)
	}
	var conflict apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusConflict, &conflict)
	if conflict.Code != apierror.CodeVersionConflict {
		t.Fatalf("unexpected stale response: %#v", conflict)
	}

	otherOwner, otherTenant := setupProductTenant(t, env, "units-other")
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":2,"name":"Cross tenant"}`), otherOwner, otherTenant.ID.Hex()), http.StatusNotFound, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/units?includeInactive=true", nil, otherOwner, otherTenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.Units) != 0 {
		t.Fatalf("units leaked across tenants: %#v", listed.Units)
	}
}
