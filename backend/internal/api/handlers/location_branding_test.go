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

func TestLocationBrandingResolutionAuthorizationIsolationAndReset(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "location-branding")
	location := createLocation(t, env, owner, tenant.ID, `{"code":"berlin","name":"Berlin Mitte","timezone":"Europe/Berlin"}`)

	tenantBody := `{"primaryColor":"#111111","accentColor":"#222222","font":"humanist","version":0}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/branding", strings.NewReader(tenantBody), owner, tenant.ID.Hex()), http.StatusOK, nil)

	path := "/api/product/locations/" + location.ID.Hex() + "/branding"
	var defaults struct {
		Branding models.LocationBranding         `json:"branding"`
		Resolved models.ResolvedLocationBranding `json:"resolved"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, path, nil, owner, tenant.ID.Hex()), http.StatusOK, &defaults)
	if defaults.Branding.Version != 0 || defaults.Resolved.DisplayName != "Berlin Mitte" || defaults.Resolved.PrimaryColor != "#111111" || defaults.Resolved.Sources["primaryColor"] != "tenant" {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}
	if count, err := env.DB.LocationBranding().CountDocuments(context.Background(), bson.M{"tenantId": tenant.ID}); err != nil || count != 0 {
		t.Fatalf("GET persisted defaults: count=%d err=%v", count, err)
	}

	member := testutil.CreateTestUser(t, env.DB, "location-branding-member@test.com", "Test1234!@#$", "Member")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	if _, err := env.DB.StaffProfiles().UpdateOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": member.ID}, bson.M{"$set": bson.M{"allLocations": true, "locationIds": bson.A{}}}); err != nil {
		t.Fatal(err)
	}
	valid := `{"displayName":" Flagship ","primaryColor":" #AABBCC ","accentColor":"","font":"GEOMETRIC","version":0}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, path, strings.NewReader(valid), member, tenant.ID.Hex()), http.StatusForbidden, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, path, strings.NewReader(valid), owner, tenant.ID.Hex()), http.StatusForbidden, nil)

	plan := testutil.CreateTestPlan(t, env.DB, "Location Branding", 1000, false)
	if _, err := env.DB.Plans().UpdateOne(context.Background(), bson.M{"_id": plan.ID}, bson.M{"$set": bson.M{"entitlements.location_branding": models.EntitlementValue{Type: models.EntitlementTypeBool, BoolValue: true}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Tenants().UpdateOne(context.Background(), bson.M{"_id": tenant.ID}, bson.M{"$set": bson.M{"planId": plan.ID}}); err != nil {
		t.Fatal(err)
	}

	var created struct {
		Branding models.LocationBranding `json:"branding"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, path, strings.NewReader(valid), owner, tenant.ID.Hex()), http.StatusOK, &created)
	if created.Branding.DisplayName != "Flagship" || created.Branding.PrimaryColor != "#aabbcc" || created.Branding.Font != models.BrandingFontGeometric || created.Branding.Version != 1 {
		t.Fatalf("unexpected location branding: %#v", created.Branding)
	}
	var resolved struct {
		Resolved models.ResolvedLocationBranding `json:"resolved"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, path, nil, owner, tenant.ID.Hex()), http.StatusOK, &resolved)
	if resolved.Resolved.PrimaryColor != "#aabbcc" || resolved.Resolved.AccentColor != "#222222" || resolved.Resolved.Sources["accentColor"] != "tenant" {
		t.Fatalf("unexpected resolved override: %#v", resolved.Resolved)
	}
	if _, err := env.DB.Plans().UpdateOne(context.Background(), bson.M{"_id": plan.ID}, bson.M{"$set": bson.M{"entitlements.location_branding": models.EntitlementValue{Type: models.EntitlementTypeNumeric, NumericValue: 1}}}); err != nil {
		t.Fatal(err)
	}
	var downgraded struct {
		Branding models.LocationBranding         `json:"branding"`
		Resolved models.ResolvedLocationBranding `json:"resolved"`
		Entitled bool                            `json:"entitled"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, path, nil, owner, tenant.ID.Hex()), http.StatusOK, &downgraded)
	if downgraded.Entitled || downgraded.Branding.Version != 1 || downgraded.Resolved.PrimaryColor != "#111111" || downgraded.Resolved.Sources["primaryColor"] != "tenant" {
		t.Fatalf("downgrade did not preserve stored override while removing it from resolution: %#v", downgraded)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodDelete, path+"?version=1", nil, owner, tenant.ID.Hex()), http.StatusForbidden, nil)
	if _, err := env.DB.Plans().UpdateOne(context.Background(), bson.M{"_id": plan.ID}, bson.M{"$set": bson.M{"entitlements.location_branding": models.EntitlementValue{Type: models.EntitlementTypeBool, BoolValue: true}}}); err != nil {
		t.Fatal(err)
	}

	var conflict apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, path, strings.NewReader(valid), owner, tenant.ID.Hex()), http.StatusConflict, &conflict)
	if conflict.Code != apierror.CodeVersionConflict {
		t.Fatalf("unexpected conflict: %#v", conflict)
	}

	otherOwner, otherTenant := setupProductTenant(t, env, "location-branding-other")
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, path, nil, otherOwner, otherTenant.ID.Hex()), http.StatusNotFound, nil)

	productResponse(t, env, env.tenantRequest(t, http.MethodDelete, path+"?version=1", nil, owner, tenant.ID.Hex()), http.StatusNoContent, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, path, nil, owner, tenant.ID.Hex()), http.StatusOK, &defaults)
	if defaults.Branding.Version != 0 || defaults.Resolved.PrimaryColor != "#111111" {
		t.Fatalf("reset did not restore tenant fallback: %#v", defaults)
	}
	if env.DB.SystemLogs().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "action": "location_branding.reset", "metadata.locationId": location.ID.Hex()}).Err() != nil {
		t.Fatal("location branding reset audit missing")
	}
}
