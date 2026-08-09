package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"lastsaas/internal/apierror"
	"lastsaas/internal/models"
	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func insertStaffTestLocation(t *testing.T, env *testEnv, tenantID primitive.ObjectID, slot int64, code string) models.Location {
	t.Helper()
	now := time.Now()
	location := models.Location{
		ID: primitive.NewObjectID(), TenantID: tenantID, Code: code, Name: code, Timezone: "UTC",
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now, LimitSlot: slot,
	}
	if _, err := env.DB.Locations().InsertOne(context.Background(), location); err != nil {
		t.Fatalf("insert location: %v", err)
	}
	return location
}

func putStaffProfile(t *testing.T, env *testEnv, actor *models.User, tenantID, targetUserID primitive.ObjectID, body string, status int) map[string]any {
	t.Helper()
	req := env.tenantRequest(t, http.MethodPut, "/api/product/staff-profiles/"+targetUserID.Hex(), strings.NewReader(body), actor, tenantID.Hex())
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatalf("update staff profile: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		t.Fatalf("update staff profile status = %d, want %d: %#v", resp.StatusCode, status, failure)
	}
	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func getProductError(t *testing.T, env *testEnv, request *http.Request, wantStatus int) apierror.Response {
	t.Helper()
	var response apierror.Response
	productResponse(t, env, request, wantStatus, &response)
	return response
}

func TestStaffProfileManagementFilteringAndStorageAuthorization(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	testutil.MarkSystemInitialized(t, env.DB)
	owner := testutil.CreateTestUser(t, env.DB, "staff-owner@example.com", "password123", "Owner")
	tenant := testutil.CreateTestTenant(t, env.DB, "Staff Tenant", owner.ID, false)
	worker := testutil.CreateTestUser(t, env.DB, "worker@example.com", "password123", "Worker")
	testutil.CreateTestMembership(t, env.DB, worker.ID, tenant.ID, models.RoleUser)
	admin := testutil.CreateTestUser(t, env.DB, "staff-admin@example.com", "password123", "Admin")
	testutil.CreateTestMembership(t, env.DB, admin.ID, tenant.ID, models.RoleAdmin)
	assigned := insertStaffTestLocation(t, env, tenant.ID, 1, "assigned")
	unassigned := insertStaffTestLocation(t, env, tenant.ID, 2, "unassigned")

	body := fmt.Sprintf(`{"version":1,"businessRole":"head_chef","allLocations":false,"locationIds":[%q],"permissionOverrides":[{"permission":"storage_areas.read","allowed":true},{"permission":"storage_areas.manage","allowed":false}],"status":"active"}`, assigned.ID.Hex())
	updated := putStaffProfile(t, env, owner, tenant.ID, worker.ID, body, http.StatusOK)
	profile := updated["staffProfile"].(map[string]any)
	if profile["version"] != float64(2) || profile["tenantId"] != nil {
		t.Fatalf("unexpected profile response: %#v", profile)
	}
	effective := profile["effectivePermissions"].([]any)
	if len(effective) != 1 || effective[0] != string(models.PermissionStorageAreasRead) {
		t.Fatalf("effective permissions = %#v", effective)
	}
	var current struct {
		StaffProfile map[string]any `json:"staffProfile"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/staff-profile", nil, worker, tenant.ID.Hex()), http.StatusOK, &current)
	if current.StaffProfile["userId"] != worker.ID.Hex() || current.StaffProfile["tenantId"] != nil {
		t.Fatalf("unexpected current profile: %#v", current.StaffProfile)
	}
	var profiles struct {
		StaffProfiles []map[string]any `json:"staffProfiles"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/staff-profiles", nil, owner, tenant.ID.Hex()), http.StatusOK, &profiles)
	if len(profiles.StaffProfiles) != 3 {
		t.Fatalf("staff profile list length = %d, want 3", len(profiles.StaffProfiles))
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/staff-profiles", nil, worker, tenant.ID.Hex()), http.StatusForbidden, nil)

	listReq := env.tenantRequest(t, http.MethodGet, "/api/product/locations", nil, worker, tenant.ID.Hex())
	var listed struct {
		Locations []models.Location `json:"locations"`
	}
	productResponse(t, env, listReq, http.StatusOK, &listed)
	if len(listed.Locations) != 1 || listed.Locations[0].ID != assigned.ID {
		t.Fatalf("assigned locations = %#v", listed.Locations)
	}

	now := time.Now()
	area := models.StorageArea{
		ID: primitive.NewObjectID(), TenantID: tenant.ID, LocationID: assigned.ID, Name: "Dry", Type: models.StorageAreaDry,
		IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := env.DB.StorageAreas().InsertOne(context.Background(), area); err != nil {
		t.Fatalf("insert storage area: %v", err)
	}
	readPath := "/api/product/locations/" + assigned.ID.Hex() + "/storage-areas"
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, readPath, nil, worker, tenant.ID.Hex()), http.StatusOK, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, readPath, strings.NewReader(`{"name":"Denied","type":"dry"}`), worker, tenant.ID.Hex()), http.StatusForbidden, nil)
	unassignedPath := "/api/product/locations/" + unassigned.ID.Hex() + "/storage-areas"
	unassignedError := getProductError(t, env, env.tenantRequest(t, http.MethodGet, unassignedPath, nil, worker, tenant.ID.Hex()), http.StatusNotFound)
	if unassignedError.Code != apierror.CodeNotFound || unassignedError.Error != "Location not found" {
		t.Fatalf("unexpected unassigned location response: %#v", unassignedError)
	}

	manageBody := fmt.Sprintf(`{"version":2,"businessRole":"stock_service","allLocations":false,"locationIds":[%q],"permissionOverrides":[{"permission":"storage_areas.read","allowed":true},{"permission":"storage_areas.manage","allowed":true}],"status":"active"}`, assigned.ID.Hex())
	putStaffProfile(t, env, owner, tenant.ID, worker.ID, manageBody, http.StatusOK)
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, readPath, strings.NewReader(`{"name":"Managed","type":"dry"}`), worker, tenant.ID.Hex()), http.StatusCreated, nil)
	putStaffProfile(t, env, owner, tenant.ID, worker.ID, body, http.StatusConflict)
	duplicate := fmt.Sprintf(`{"version":3,"businessRole":"viewer","allLocations":false,"locationIds":[%q,%q],"permissionOverrides":[],"status":"active"}`, assigned.ID.Hex(), assigned.ID.Hex())
	putStaffProfile(t, env, owner, tenant.ID, worker.ID, duplicate, http.StatusBadRequest)
	unknown := fmt.Sprintf(`{"version":3,"businessRole":"viewer","allLocations":false,"locationIds":[%q],"permissionOverrides":[],"status":"active","unknown":true}`, assigned.ID.Hex())
	putStaffProfile(t, env, owner, tenant.ID, worker.ID, unknown, http.StatusBadRequest)
	adminDeny := `{"version":1,"businessRole":"operations_manager","allLocations":true,"locationIds":[],"permissionOverrides":[{"permission":"storage_areas.read","allowed":false}],"status":"active"}`
	putStaffProfile(t, env, owner, tenant.ID, admin.ID, adminDeny, http.StatusOK)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, readPath, nil, admin, tenant.ID.Hex()), http.StatusForbidden, nil)

	count, err := env.DB.SystemLogs().CountDocuments(context.Background(), bson.M{
		"tenantId": tenant.ID, "action": "staff_profile.updated", "severity": models.LogMedium,
	})
	if err != nil || count != 3 {
		t.Fatalf("staff profile audit count = %d, want 3, err %v", count, err)
	}
}

func TestStaffProfileHierarchyIsolationAndFailClosed(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	testutil.MarkSystemInitialized(t, env.DB)
	owner := testutil.CreateTestUser(t, env.DB, "hierarchy-owner@example.com", "password123", "Owner")
	tenant := testutil.CreateTestTenant(t, env.DB, "Hierarchy Tenant", owner.ID, false)
	admin := testutil.CreateTestUser(t, env.DB, "hierarchy-admin@example.com", "password123", "Admin")
	testutil.CreateTestMembership(t, env.DB, admin.ID, tenant.ID, models.RoleAdmin)
	otherAdmin := testutil.CreateTestUser(t, env.DB, "other-admin@example.com", "password123", "Other Admin")
	testutil.CreateTestMembership(t, env.DB, otherAdmin.ID, tenant.ID, models.RoleAdmin)
	location := insertStaffTestLocation(t, env, tenant.ID, 1, "hierarchy")
	body := `{"version":1,"businessRole":"operations_manager","allLocations":true,"locationIds":[],"permissionOverrides":[],"status":"active"}`
	putStaffProfile(t, env, admin, tenant.ID, otherAdmin.ID, body, http.StatusForbidden)
	putStaffProfile(t, env, owner, tenant.ID, owner.ID, body, http.StatusForbidden)

	foreignOwner := testutil.CreateTestUser(t, env.DB, "foreign-owner@example.com", "password123", "Foreign")
	foreignTenant := testutil.CreateTestTenant(t, env.DB, "Foreign Tenant", foreignOwner.ID, false)
	putStaffProfile(t, env, owner, tenant.ID, foreignOwner.ID, body, http.StatusNotFound)
	foreignLocationBody := fmt.Sprintf(`{"version":1,"businessRole":"viewer","allLocations":false,"locationIds":[%q],"permissionOverrides":[],"status":"active"}`, insertStaffTestLocation(t, env, foreignTenant.ID, 1, "foreign").ID.Hex())
	putStaffProfile(t, env, owner, tenant.ID, admin.ID, foreignLocationBody, http.StatusBadRequest)

	if _, err := env.DB.StaffProfiles().DeleteOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": admin.ID}); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	missingReq := env.tenantRequest(t, http.MethodGet, "/api/product/locations", nil, admin, tenant.ID.Hex())
	var missing apierror.Response
	productResponse(t, env, missingReq, http.StatusForbidden, &missing)
	if missing.Code != apierror.Code("STAFF_PROFILE_REQUIRED") {
		t.Fatalf("missing profile code = %s", missing.Code)
	}
	selfMissing := getProductError(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/staff-profile", nil, admin, tenant.ID.Hex()), http.StatusNotFound)
	if selfMissing.Code != apierror.CodeNotFound || selfMissing.Error != "Staff profile not found" {
		t.Fatalf("unexpected missing self profile response: %#v", selfMissing)
	}

	profile := models.StaffProfile{
		ID: primitive.NewObjectID(), TenantID: tenant.ID, UserID: admin.ID, BusinessRole: models.BusinessRoleOperationsManager,
		AllLocations: true, LocationIDs: []primitive.ObjectID{}, PermissionOverrides: []models.PermissionOverride{},
		Status: models.StaffProfileInactive, Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := env.DB.StaffProfiles().InsertOne(context.Background(), profile); err != nil {
		t.Fatalf("insert inactive profile: %v", err)
	}
	var inactiveSelf struct {
		StaffProfile struct {
			Status               models.StaffProfileStatus   `json:"status"`
			EffectivePermissions []models.BusinessPermission `json:"effectivePermissions"`
		} `json:"staffProfile"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/staff-profile", nil, admin, tenant.ID.Hex()), http.StatusOK, &inactiveSelf)
	if inactiveSelf.StaffProfile.Status != models.StaffProfileInactive || len(inactiveSelf.StaffProfile.EffectivePermissions) != 0 {
		t.Fatalf("unexpected inactive self profile: %#v", inactiveSelf.StaffProfile)
	}
	inactivePath := "/api/product/locations/" + location.ID.Hex() + "/storage-areas"
	var inactive apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, inactivePath, nil, admin, tenant.ID.Hex()), http.StatusForbidden, &inactive)
	if inactive.Code != apierror.Code("STAFF_PROFILE_INACTIVE") {
		t.Fatalf("inactive profile code = %s", inactive.Code)
	}
	unknownPath := "/api/product/locations/" + primitive.NewObjectID().Hex() + "/storage-areas"
	unknownError := getProductError(t, env, env.tenantRequest(t, http.MethodGet, unknownPath, nil, owner, tenant.ID.Hex()), http.StatusNotFound)
	foreignPath := "/api/product/locations/" + insertStaffTestLocation(t, env, foreignTenant.ID, 2, "foreign-denial").ID.Hex() + "/storage-areas"
	foreignError := getProductError(t, env, env.tenantRequest(t, http.MethodGet, foreignPath, nil, owner, tenant.ID.Hex()), http.StatusNotFound)
	for name, response := range map[string]apierror.Response{"unknown": unknownError, "foreign": foreignError} {
		if response.Code != apierror.CodeNotFound || response.Error != "Location not found" {
			t.Fatalf("%s location denial differs: %#v", name, response)
		}
	}
}
