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

func TestRestaurantOnboardingRequiresPersistedSetupAndIsTenantScoped(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "onboarding")
	if _, err := env.DB.Tenants().UpdateOne(context.Background(), bson.M{"_id": tenant.ID}, bson.M{"$set": bson.M{"billingStatus": models.BillingStatusPastDue}}); err != nil {
		t.Fatal(err)
	}

	path := "/api/product/onboarding"
	var initial struct {
		Onboarding struct {
			Completed                  bool `json:"completed"`
			RestaurantSettingsComplete bool `json:"restaurantSettingsComplete"`
			FirstLocationComplete      bool `json:"firstLocationComplete"`
		} `json:"onboarding"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, path, nil, owner, tenant.ID.Hex()), http.StatusOK, &initial)
	if initial.Onboarding.Completed || initial.Onboarding.RestaurantSettingsComplete || initial.Onboarding.FirstLocationComplete {
		t.Fatalf("unexpected initial onboarding status: %#v", initial)
	}
	var validation apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, path+"/complete", nil, owner, tenant.ID.Hex()), http.StatusBadRequest, &validation)
	if validation.Code != apierror.CodeValidation {
		t.Fatalf("unexpected incomplete setup response: %#v", validation)
	}
	if _, err := env.DB.Tenants().UpdateOne(context.Background(), bson.M{"_id": tenant.ID}, bson.M{"$set": bson.M{"billingStatus": models.BillingStatusNone}}); err != nil {
		t.Fatal(err)
	}

	settings := `{"currency":"EUR","language":"de-DE","defaultTimezone":"Europe/Berlin","version":0}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/restaurant-settings", strings.NewReader(settings), owner, tenant.ID.Hex()), http.StatusOK, nil)
	createLocation(t, env, owner, tenant.ID, `{"code":"main","name":"Main Restaurant","timezone":"Europe/Berlin"}`)

	member := testutil.CreateTestUser(t, env.DB, "onboarding-member@test.com", "Test1234!@#$", "Member")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, path+"/complete", nil, member, tenant.ID.Hex()), http.StatusForbidden, nil)

	productResponse(t, env, env.tenantRequest(t, http.MethodPost, path+"/complete", nil, owner, tenant.ID.Hex()), http.StatusOK, &initial)
	if !initial.Onboarding.Completed || !initial.Onboarding.RestaurantSettingsComplete || !initial.Onboarding.FirstLocationComplete {
		t.Fatalf("onboarding did not complete: %#v", initial)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, path+"/complete", nil, owner, tenant.ID.Hex()), http.StatusOK, &initial)
	if count, err := env.DB.SystemLogs().CountDocuments(context.Background(), bson.M{"tenantId": tenant.ID, "action": "restaurant.onboarding.completed"}); err != nil || count != 1 {
		t.Fatalf("expected one idempotent completion audit, count=%d err=%v", count, err)
	}

	otherOwner, otherTenant := setupProductTenant(t, env, "onboarding-other")
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, path, nil, otherOwner, otherTenant.ID.Hex()), http.StatusOK, &initial)
	if initial.Onboarding.Completed || initial.Onboarding.RestaurantSettingsComplete || initial.Onboarding.FirstLocationComplete {
		t.Fatalf("tenant onboarding status leaked across tenants: %#v", initial)
	}
}
