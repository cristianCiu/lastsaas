package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"lastsaas/internal/apierror"
	"lastsaas/internal/models"
	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const validLocationBody = `{"code":"berlin","name":"Berlin","timezone":"Europe/Berlin"}`

func createLocation(t *testing.T, env *testEnv, user *models.User, tenantID primitive.ObjectID, body string) models.Location {
	t.Helper()
	req := env.tenantRequest(t, http.MethodPost, "/api/product/locations", strings.NewReader(body), user, tenantID.Hex())
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected location create to return 201, got %d", resp.StatusCode)
	}
	var payload struct {
		Location models.Location `json:"location"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload.Location
}

func setupProductTenant(t *testing.T, env *testEnv, suffix string) (*models.User, *models.Tenant) {
	t.Helper()
	testutil.MarkSystemInitialized(t, env.DB)
	user := testutil.CreateTestUser(t, env.DB, suffix+"@test.com", "Test1234!@#$", "Product User")
	tenant := testutil.CreateTestTenant(t, env.DB, "Product "+suffix, user.ID, false)
	return user, tenant
}

func TestLocationsRequireAuthentication(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	testutil.MarkSystemInitialized(t, env.DB)

	resp, err := env.Client.Get(env.Server.URL + "/api/product/locations")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestLocationsPOSTRequiresAdminOrOwner(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "roles")
	_, err := env.DB.Tenants().UpdateOne(context.Background(), bson.M{"_id": tenant.ID}, bson.M{"$set": bson.M{"billingWaived": true}})
	if err != nil {
		t.Fatal(err)
	}
	user := testutil.CreateTestUser(t, env.DB, "member@test.com", "Test1234!@#$", "Member")
	testutil.CreateTestMembership(t, env.DB, user.ID, tenant.ID, models.RoleUser)

	denied := env.tenantRequest(t, http.MethodPost, "/api/product/locations", strings.NewReader(validLocationBody), user, tenant.ID.Hex())
	resp, err := env.Client.Do(denied)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected user POST to return 403, got %d", resp.StatusCode)
	}

	admin := testutil.CreateTestUser(t, env.DB, "location-admin@test.com", "Test1234!@#$", "Admin")
	testutil.CreateTestMembership(t, env.DB, admin.ID, tenant.ID, models.RoleAdmin)
	ownerRequest := env.tenantRequest(t, http.MethodPost, "/api/product/locations", strings.NewReader(validLocationBody), owner, tenant.ID.Hex())
	resp, err = env.Client.Do(ownerRequest)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected owner POST to return 201, got %d", resp.StatusCode)
	}

	adminBody := `{"code":"hamburg","name":"Hamburg","timezone":"Europe/Berlin"}`
	allowed := env.tenantRequest(t, http.MethodPost, "/api/product/locations", strings.NewReader(adminBody), admin, tenant.ID.Hex())
	resp, err = env.Client.Do(allowed)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected admin POST to return 201, got %d", resp.StatusCode)
	}
}

func TestLocationsAreTenantIsolatedAndHideInternalFields(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	testutil.MarkSystemInitialized(t, env.DB)
	user := testutil.CreateTestUser(t, env.DB, "isolation@test.com", "Test1234!@#$", "Owner")
	tenantA := testutil.CreateTestTenant(t, env.DB, "Location Tenant A", user.ID, false)
	tenantB := testutil.CreateTestTenant(t, env.DB, "Location Tenant B", user.ID, false)

	for _, tenant := range []*models.Tenant{tenantA, tenantB} {
		req := env.tenantRequest(t, http.MethodPost, "/api/product/locations", strings.NewReader(validLocationBody), user, tenant.ID.Hex())
		resp, err := env.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusCreated {
			resp.Body.Close()
			t.Fatalf("expected same code in separate tenants to return 201, got %d", resp.StatusCode)
		}
		var payload map[string]map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if _, exists := payload["location"]["tenantId"]; exists {
			t.Fatal("response exposed tenantId")
		}
		if _, exists := payload["location"]["limitSlot"]; exists {
			t.Fatal("response exposed limitSlot")
		}
	}

	listReq := env.tenantRequest(t, http.MethodGet, "/api/product/locations", nil, user, tenantA.ID.Hex())
	resp, err := env.Client.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result struct {
		Locations []models.Location `json:"locations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Locations) != 1 || result.Locations[0].Code != "berlin" {
		t.Fatalf("unexpected tenant-scoped locations: %#v", result.Locations)
	}
}

func TestLocationsDuplicateCodeAndDefaultLimit(t *testing.T) {
	t.Run("duplicate code", func(t *testing.T) {
		env := setupTestServer(t)
		defer env.Cleanup()
		user, tenant := setupProductTenant(t, env, "duplicate")

		for attempt := 0; attempt < 2; attempt++ {
			req := env.tenantRequest(t, http.MethodPost, "/api/product/locations", strings.NewReader(validLocationBody), user, tenant.ID.Hex())
			resp, err := env.Client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if attempt == 0 && resp.StatusCode != http.StatusCreated {
				resp.Body.Close()
				t.Fatalf("expected first create to return 201, got %d", resp.StatusCode)
			}
			if attempt == 1 {
				var response apierror.Response
				if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != http.StatusConflict || response.Code != apierror.CodeConflict {
					t.Fatalf("expected duplicate conflict, got %d %#v", resp.StatusCode, response)
				}
			}
			resp.Body.Close()
		}
	})

	t.Run("no plan defaults to one", func(t *testing.T) {
		env := setupTestServer(t)
		defer env.Cleanup()
		user, tenant := setupProductTenant(t, env, "default-limit")

		first := env.tenantRequest(t, http.MethodPost, "/api/product/locations", strings.NewReader(validLocationBody), user, tenant.ID.Hex())
		resp, err := env.Client.Do(first)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		secondBody := `{"code":"hamburg","name":"Hamburg","timezone":"Europe/Berlin"}`
		second := env.tenantRequest(t, http.MethodPost, "/api/product/locations", strings.NewReader(secondBody), user, tenant.ID.Hex())
		resp, err = env.Client.Do(second)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var response apierror.Response
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusConflict || response.Code != apierror.CodePlanLimit {
			t.Fatalf("expected plan limit, got %d %#v", resp.StatusCode, response)
		}
	})
}

func TestLocationsNumericLimitIsExact(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	user, tenant := setupProductTenant(t, env, "numeric-limit")
	plan := testutil.CreateTestPlan(t, env.DB, "Two Locations", 1000, false)
	_, err := env.DB.Plans().UpdateOne(context.Background(), bson.M{"_id": plan.ID}, bson.M{"$set": bson.M{
		"entitlements.max_locations": models.EntitlementValue{Type: models.EntitlementTypeNumeric, NumericValue: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.DB.Tenants().UpdateOne(context.Background(), bson.M{"_id": tenant.ID}, bson.M{"$set": bson.M{"planId": plan.ID}})
	if err != nil {
		t.Fatal(err)
	}

	bodies := []string{
		validLocationBody,
		`{"code":"hamburg","name":"Hamburg","timezone":"Europe/Berlin"}`,
		`{"code":"munich","name":"Munich","timezone":"Europe/Berlin"}`,
	}
	for i, body := range bodies {
		req := env.tenantRequest(t, http.MethodPost, "/api/product/locations", strings.NewReader(body), user, tenant.ID.Hex())
		resp, err := env.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		expected := http.StatusCreated
		if i == 2 {
			expected = http.StatusConflict
		}
		if resp.StatusCode != expected {
			t.Fatalf("request %d: expected %d, got %d", i+1, expected, resp.StatusCode)
		}
	}
}

func TestLocationsConcurrentCreatesRespectExactLimit(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	user, tenant := setupProductTenant(t, env, "concurrent-limit")
	plan := testutil.CreateTestPlan(t, env.DB, "Concurrent Two Locations", 1000, false)
	_, err := env.DB.Plans().UpdateOne(context.Background(), bson.M{"_id": plan.ID}, bson.M{"$set": bson.M{
		"entitlements.max_locations": models.EntitlementValue{Type: models.EntitlementTypeNumeric, NumericValue: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.DB.Tenants().UpdateOne(context.Background(), bson.M{"_id": tenant.ID}, bson.M{"$set": bson.M{"planId": plan.ID}})
	if err != nil {
		t.Fatal(err)
	}

	const requestCount = 8
	requests := make([]*http.Request, requestCount)
	for i := range requests {
		body := `{"code":"location-` + string(rune('a'+i)) + `","name":"Concurrent","timezone":"UTC"}`
		requests[i] = env.tenantRequest(t, http.MethodPost, "/api/product/locations", strings.NewReader(body), user, tenant.ID.Hex())
	}

	statuses := make(chan int, requestCount)
	var wg sync.WaitGroup
	for _, request := range requests {
		wg.Add(1)
		go func(req *http.Request) {
			defer wg.Done()
			resp, requestErr := env.Client.Do(req)
			if requestErr != nil {
				statuses <- 0
				return
			}
			resp.Body.Close()
			statuses <- resp.StatusCode
		}(request)
	}
	wg.Wait()
	close(statuses)

	created := 0
	conflicts := 0
	for status := range statuses {
		switch status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicts++
		default:
			t.Errorf("unexpected concurrent response status: %d", status)
		}
	}
	if created != 2 || conflicts != requestCount-2 {
		t.Fatalf("expected 2 creates and %d conflicts, got %d and %d", requestCount-2, created, conflicts)
	}
}

func TestLocationsPATCHOwnerAndAdminSuccess(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "patch-roles")
	if _, err := env.DB.Tenants().UpdateOne(context.Background(), bson.M{"_id": tenant.ID}, bson.M{"$set": bson.M{"billingWaived": true}}); err != nil {
		t.Fatal(err)
	}
	admin := testutil.CreateTestUser(t, env.DB, "patch-admin@test.com", "Test1234!@#$", "Admin")
	testutil.CreateTestMembership(t, env.DB, admin.ID, tenant.ID, models.RoleAdmin)

	tests := []struct {
		name string
		user *models.User
		body string
	}{
		{name: "owner", user: owner, body: validLocationBody},
		{name: "admin", user: admin, body: `{"code":"hamburg","name":"Hamburg","timezone":"Europe/Berlin"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			location := createLocation(t, env, owner, tenant.ID, tt.body)
			patch := `{"version":1,"name":"  Updated location  ","timezone":"UTC"}`
			req := env.tenantRequest(t, http.MethodPatch, "/api/product/locations/"+location.ID.Hex(), strings.NewReader(patch), tt.user, tenant.ID.Hex())
			resp, err := env.Client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var payload struct {
				Location map[string]any `json:"location"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK || payload.Location["name"] != "Updated location" || payload.Location["timezone"] != "UTC" || payload.Location["version"] != float64(2) {
				t.Fatalf("unexpected PATCH response: %d %#v", resp.StatusCode, payload)
			}
			if payload.Location["code"] != location.Code {
				t.Fatalf("location code changed: %#v", payload.Location)
			}
			if _, exists := payload.Location["tenantId"]; exists {
				t.Fatal("PATCH response exposed tenantId")
			}
			if _, exists := payload.Location["limitSlot"]; exists {
				t.Fatal("PATCH response exposed limitSlot")
			}
		})
	}
}

func TestLocationsPATCHRequiresAdminOrOwner(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "patch-user")
	location := createLocation(t, env, owner, tenant.ID, validLocationBody)
	user := testutil.CreateTestUser(t, env.DB, "patch-member@test.com", "Test1234!@#$", "Member")
	testutil.CreateTestMembership(t, env.DB, user.ID, tenant.ID, models.RoleUser)

	req := env.tenantRequest(t, http.MethodPatch, "/api/product/locations/"+location.ID.Hex(), strings.NewReader(`{"version":1,"name":"Denied"}`), user, tenant.ID.Hex())
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected user PATCH to return 403, got %d", resp.StatusCode)
	}
}

func TestLocationsPATCHRequiresAssignedActiveStaffProfile(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "patch-assignment")
	location := createLocation(t, env, owner, tenant.ID, validLocationBody)
	admin := testutil.CreateTestUser(t, env.DB, "patch-restricted-admin@test.com", "Test1234!@#$", "Restricted Admin")
	testutil.CreateTestMembership(t, env.DB, admin.ID, tenant.ID, models.RoleAdmin)
	if _, err := env.DB.StaffProfiles().UpdateOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": admin.ID}, bson.M{"$set": bson.M{"allLocations": false, "locationIds": bson.A{}}}); err != nil {
		t.Fatal(err)
	}

	path := "/api/product/locations/" + location.ID.Hex()
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"name":"Denied"}`), admin, tenant.ID.Hex()), http.StatusNotFound, nil)
	if _, err := env.DB.StaffProfiles().UpdateOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": admin.ID}, bson.M{"$set": bson.M{"status": models.StaffProfileInactive}}); err != nil {
		t.Fatal(err)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, path, strings.NewReader(`{"version":1,"name":"Still denied"}`), admin, tenant.ID.Hex()), http.StatusForbidden, nil)

	var stored models.Location
	if err := env.DB.Locations().FindOne(context.Background(), bson.M{"_id": location.ID}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if stored.Name != location.Name || stored.Version != location.Version {
		t.Fatalf("unauthorized location update changed target: %#v", stored)
	}
}

func TestLocationsPATCHIsTenantScoped(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	testutil.MarkSystemInitialized(t, env.DB)
	owner := testutil.CreateTestUser(t, env.DB, "patch-isolation@test.com", "Test1234!@#$", "Owner")
	tenantA := testutil.CreateTestTenant(t, env.DB, "Patch Tenant A", owner.ID, false)
	tenantB := testutil.CreateTestTenant(t, env.DB, "Patch Tenant B", owner.ID, false)
	target := createLocation(t, env, owner, tenantB.ID, validLocationBody)

	req := env.tenantRequest(t, http.MethodPatch, "/api/product/locations/"+target.ID.Hex(), strings.NewReader(`{"version":1,"name":"Cross tenant"}`), owner, tenantA.ID.Hex())
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var apiErr apierror.Response
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound || apiErr.Code != apierror.CodeNotFound {
		t.Fatalf("expected cross-tenant PATCH to return 404, got %d %#v", resp.StatusCode, apiErr)
	}

	var stored models.Location
	if err := env.DB.Locations().FindOne(context.Background(), bson.M{"_id": target.ID}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if stored.Name != target.Name || stored.Version != 1 {
		t.Fatalf("cross-tenant PATCH changed target: %#v", stored)
	}
}

func TestLocationsPATCHRejectsMalformedIDAndInvalidBodies(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "patch-invalid")
	location := createLocation(t, env, owner, tenant.ID, validLocationBody)

	tests := []struct {
		name string
		id   string
		body string
	}{
		{name: "malformed ID", id: "not-an-object-id", body: `{"version":1,"name":"Valid"}`},
		{name: "missing mutable field", id: location.ID.Hex(), body: `{"version":1}`},
		{name: "missing version", id: location.ID.Hex(), body: `{"name":"Valid"}`},
		{name: "unknown field", id: location.ID.Hex(), body: `{"version":1,"code":"immutable"}`},
		{name: "empty trimmed name", id: location.ID.Hex(), body: `{"version":1,"name":"   "}`},
		{name: "invalid timezone", id: location.ID.Hex(), body: `{"version":1,"timezone":"Europe/Not_A_City"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := env.tenantRequest(t, http.MethodPatch, "/api/product/locations/"+tt.id, strings.NewReader(tt.body), owner, tenant.ID.Hex())
			resp, err := env.Client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestLocationsPATCHStaleVersionConflict(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "patch-conflict")
	location := createLocation(t, env, owner, tenant.ID, validLocationBody)

	for attempt := 0; attempt < 2; attempt++ {
		req := env.tenantRequest(t, http.MethodPatch, "/api/product/locations/"+location.ID.Hex(), strings.NewReader(`{"version":1,"name":"Versioned"}`), owner, tenant.ID.Hex())
		resp, err := env.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if attempt == 0 && resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("expected initial PATCH to return 200, got %d", resp.StatusCode)
		}
		if attempt == 1 {
			var apiErr apierror.Response
			if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusConflict || apiErr.Code != apierror.CodeVersionConflict {
				t.Fatalf("expected VERSION_CONFLICT, got %d %#v", resp.StatusCode, apiErr)
			}
		}
		resp.Body.Close()
	}
}

func TestLocationsPATCHDeactivationIsAudited(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "patch-deactivate")
	location := createLocation(t, env, owner, tenant.ID, validLocationBody)

	req := env.tenantRequest(t, http.MethodPatch, "/api/product/locations/"+location.ID.Hex(), strings.NewReader(`{"version":1,"isActive":false}`), owner, tenant.ID.Hex())
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var payload struct {
		Location models.Location `json:"location"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || payload.Location.IsActive || payload.Location.Version != 2 {
		t.Fatalf("unexpected deactivation response: %d %#v", resp.StatusCode, payload.Location)
	}

	var audit models.SystemLog
	if err := env.DB.SystemLogs().FindOne(context.Background(), bson.M{
		"tenantId": tenant.ID, "action": "location.deactivated", "metadata.locationId": location.ID.Hex(),
	}).Decode(&audit); err != nil {
		t.Fatalf("location deactivation audit missing: %v", err)
	}
}
