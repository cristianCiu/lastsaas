package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"lastsaas/internal/apierror"
	"lastsaas/internal/models"
	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func productResponse(t *testing.T, env *testEnv, request *http.Request, wantStatus int, target any) {
	t.Helper()
	response, err := env.Client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("expected %d, got %d", wantStatus, response.StatusCode)
	}
	if target != nil {
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	}
}

func TestRestaurantSettingsDefaultsRolesStrictnessAndVersioning(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "restaurant-settings")

	var defaults struct {
		Settings map[string]any `json:"settings"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/restaurant-settings", nil, owner, tenant.ID.Hex()), http.StatusOK, &defaults)
	settings := defaults.Settings
	if settings["currency"] != "EUR" || settings["language"] != "de" || settings["defaultTimezone"] != "Europe/Berlin" || settings["version"] != float64(0) {
		t.Fatalf("unexpected defaults: %#v", settings)
	}
	count, err := env.DB.RestaurantSettings().CountDocuments(context.Background(), bson.M{"tenantId": tenant.ID})
	if err != nil || count != 0 {
		t.Fatalf("GET persisted defaults: count=%d err=%v", count, err)
	}

	member := testutil.CreateTestUser(t, env.DB, "settings-member@test.com", "Test1234!@#$", "Member")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/restaurant-settings", nil, member, tenant.ID.Hex()), http.StatusOK, nil)
	valid := `{"currency":"usd","language":"EN-us","defaultTimezone":"America/New_York","version":0}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/restaurant-settings", strings.NewReader(valid), member, tenant.ID.Hex()), http.StatusForbidden, nil)
	for _, body := range []string{`{"currency":"EUR","language":"de","defaultTimezone":"UTC"}`, `{"currency":"EUR","language":"de","defaultTimezone":"UTC","version":0,"tenantId":"x"}`, `{"currency":"EURO","language":"de","defaultTimezone":"UTC","version":0}`} {
		productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/restaurant-settings", strings.NewReader(body), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)
	}
	var created struct {
		Settings models.RestaurantSettings `json:"settings"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/restaurant-settings", strings.NewReader(valid), owner, tenant.ID.Hex()), http.StatusOK, &created)
	if created.Settings.Currency != "USD" || created.Settings.Language != "en-US" || created.Settings.Version != 1 {
		t.Fatalf("unexpected normalized settings: %#v", created.Settings)
	}
	var conflict apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/restaurant-settings", strings.NewReader(valid), owner, tenant.ID.Hex()), http.StatusConflict, &conflict)
	if conflict.Code != apierror.CodeVersionConflict {
		t.Fatalf("unexpected conflict: %#v", conflict)
	}
	update := `{"currency":"eur","language":"de-DE","defaultTimezone":"Europe/Berlin","version":1}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/restaurant-settings", strings.NewReader(update), owner, tenant.ID.Hex()), http.StatusOK, nil)
	if env.DB.SystemLogs().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "action": "restaurant_settings.updated"}).Err() != nil {
		t.Fatal("restaurant settings audit missing")
	}

	otherTenant := testutil.CreateTestTenant(t, env.DB, "Other Settings Tenant", owner.ID, false)
	var isolated struct {
		Settings models.RestaurantSettings `json:"settings"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/restaurant-settings", nil, owner, otherTenant.ID.Hex()), http.StatusOK, &isolated)
	if isolated.Settings.Version != 0 || isolated.Settings.Currency != "EUR" {
		t.Fatalf("restaurant settings leaked across tenants: %#v", isolated.Settings)
	}
}

func createStorageAreaHTTP(t *testing.T, env *testEnv, user *models.User, tenantID, locationID primitive.ObjectID, body string) models.StorageArea {
	t.Helper()
	var payload struct {
		StorageArea models.StorageArea `json:"storageArea"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/locations/"+locationID.Hex()+"/storage-areas", strings.NewReader(body), user, tenantID.Hex()), http.StatusCreated, &payload)
	return payload.StorageArea
}

func TestStorageAreasScopeValidationLifecycleAndAudit(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "storage-areas")
	location := createLocation(t, env, owner, tenant.ID, validLocationBody)
	member := testutil.CreateTestUser(t, env.DB, "storage-member@test.com", "Test1234!@#$", "Member")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	admin := testutil.CreateTestUser(t, env.DB, "storage-admin@test.com", "Test1234!@#$", "Admin")
	testutil.CreateTestMembership(t, env.DB, admin.ID, tenant.ID, models.RoleAdmin)
	path := "/api/product/locations/" + location.ID.Hex() + "/storage-areas"
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, path, nil, member, tenant.ID.Hex()), http.StatusForbidden, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, path, nil, admin, tenant.ID.Hex()), http.StatusOK, nil)
	for _, body := range []string{`{"name":"","type":"dry"}`, `{"name":"Store","type":"warehouse"}`, `{"name":"Store","type":"dry","locationId":"x"}`} {
		productResponse(t, env, env.tenantRequest(t, http.MethodPost, path, strings.NewReader(body), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)
	}
	area := createStorageAreaHTTP(t, env, owner, tenant.ID, location.ID, `{"name":"  Walk-in  ","type":"refrigerated"}`)
	if area.Name != "Walk-in" || !area.IsActive || area.Version != 1 || area.LocationID != location.ID {
		t.Fatalf("unexpected area: %#v", area)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, path, strings.NewReader(`{"name":"Walk-in","type":"frozen"}`), owner, tenant.ID.Hex()), http.StatusConflict, nil)
	var listed struct {
		StorageAreas []map[string]any `json:"storageAreas"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, path, nil, owner, tenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.StorageAreas) != 1 {
		t.Fatalf("unexpected list: %#v", listed)
	}
	if _, exposed := listed.StorageAreas[0]["tenantId"]; exposed {
		t.Fatal("tenantId exposed")
	}

	patchPath := path + "/" + area.ID.Hex()
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, patchPath, strings.NewReader(`{"name":"Missing version"}`), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, patchPath, strings.NewReader(`{"version":1,"name":"Trailing"} {}`), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, patchPath, strings.NewReader(`{"version":1,"isActive":true}`), owner, tenant.ID.Hex()), http.StatusOK, nil)
	reactivated, err := env.DB.SystemLogs().CountDocuments(context.Background(), bson.M{"tenantId": tenant.ID, "action": "storage_area.reactivated", "metadata.storageAreaId": area.ID.Hex()})
	if err != nil || reactivated != 0 {
		t.Fatalf("no-op active assignment emitted lifecycle audit: count=%d err=%v", reactivated, err)
	}
	if env.DB.SystemLogs().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "action": "storage_area.updated", "metadata.storageAreaId": area.ID.Hex(), "metadata.version": int64(2)}).Err() != nil {
		t.Fatal("no-op active assignment should be audited as an update")
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, patchPath, strings.NewReader(`{"version":2,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, nil)
	var stale apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, patchPath, strings.NewReader(`{"version":2,"name":"Stale"}`), owner, tenant.ID.Hex()), http.StatusConflict, &stale)
	if stale.Code != apierror.CodeVersionConflict {
		t.Fatalf("unexpected stale response: %#v", stale)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, patchPath, strings.NewReader(`{"version":3,"isActive":true,"name":"Cold room","type":"frozen"}`), owner, tenant.ID.Hex()), http.StatusOK, nil)
	for _, action := range []string{"storage_area.created", "storage_area.deactivated", "storage_area.reactivated"} {
		if env.DB.SystemLogs().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "action": action, "metadata.storageAreaId": area.ID.Hex()}).Err() != nil {
			t.Errorf("audit %s missing", action)
		}
	}
}

func TestStorageAreasTenantAndLocationIsolation(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	testutil.MarkSystemInitialized(t, env.DB)
	owner := testutil.CreateTestUser(t, env.DB, "storage-isolation@test.com", "Test1234!@#$", "Owner")
	tenantA := testutil.CreateTestTenant(t, env.DB, "Storage A", owner.ID, false)
	tenantB := testutil.CreateTestTenant(t, env.DB, "Storage B", owner.ID, false)
	locationA := createLocation(t, env, owner, tenantA.ID, validLocationBody)
	if _, err := env.DB.Tenants().UpdateOne(context.Background(), bson.M{"_id": tenantA.ID}, bson.M{"$set": bson.M{"billingWaived": true}}); err != nil {
		t.Fatal(err)
	}
	locationA2 := createLocation(t, env, owner, tenantA.ID, `{"code":"hamburg","name":"Hamburg","timezone":"Europe/Berlin"}`)
	locationB := createLocation(t, env, owner, tenantB.ID, validLocationBody)
	crossTenantArea := createStorageAreaHTTP(t, env, owner, tenantB.ID, locationB.ID, `{"name":"Private","type":"dry"}`)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/locations/"+locationB.ID.Hex()+"/storage-areas", nil, owner, tenantA.ID.Hex()), http.StatusNotFound, nil)
	wrongLocationPatch := "/api/product/locations/" + locationA.ID.Hex() + "/storage-areas/" + crossTenantArea.ID.Hex()
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, wrongLocationPatch, strings.NewReader(`{"version":1,"name":"Leaked"}`), owner, tenantA.ID.Hex()), http.StatusNotFound, nil)

	createStorageAreaHTTP(t, env, owner, tenantA.ID, locationA.ID, `{"name":"Shared name","type":"dry"}`)
	areaA2 := createStorageAreaHTTP(t, env, owner, tenantA.ID, locationA2.ID, `{"name":"Shared name","type":"bar"}`)
	sameTenantWrongLocation := "/api/product/locations/" + locationA.ID.Hex() + "/storage-areas/" + areaA2.ID.Hex()
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, sameTenantWrongLocation, strings.NewReader(`{"version":1,"name":"Cross-location"}`), owner, tenantA.ID.Hex()), http.StatusNotFound, nil)
}
