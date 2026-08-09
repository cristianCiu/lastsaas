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

func TestTenantBrandingDefaultsAuthorizationIsolationAndVersioning(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "tenant-branding")

	var defaults struct {
		Branding map[string]any `json:"branding"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/branding", nil, owner, tenant.ID.Hex()), http.StatusOK, &defaults)
	if defaults.Branding["primaryColor"] != "" || defaults.Branding["accentColor"] != "" || defaults.Branding["font"] != "" || defaults.Branding["version"] != float64(0) {
		t.Fatalf("unexpected branding defaults: %#v", defaults.Branding)
	}
	if _, exposed := defaults.Branding["tenantId"]; exposed {
		t.Fatal("tenantId exposed")
	}
	if count, err := env.DB.TenantBranding().CountDocuments(context.Background(), bson.M{"tenantId": tenant.ID}); err != nil || count != 0 {
		t.Fatalf("GET persisted defaults: count=%d err=%v", count, err)
	}

	member := testutil.CreateTestUser(t, env.DB, "branding-member@test.com", "Test1234!@#$", "Member")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	valid := `{"primaryColor":" #AABBCC ","accentColor":"#123456","font":"HUMANIST","version":0}`
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/branding", nil, member, tenant.ID.Hex()), http.StatusOK, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/branding", strings.NewReader(valid), member, tenant.ID.Hex()), http.StatusForbidden, nil)
	for _, body := range []string{
		`{"primaryColor":"#aabbcc","accentColor":"","font":"system"}`,
		`{"primaryColor":"red","accentColor":"","font":"system","version":0}`,
		`{"primaryColor":"#aabbcc","accentColor":"","font":"remote","version":0}`,
		`{"primaryColor":"#aabbcc","accentColor":"","font":"system","version":0,"customCss":"body{}"}`,
		`{"primaryColor":"#aabbcc","accentColor":"","font":"system","version":0} {}`,
	} {
		productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/branding", strings.NewReader(body), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)
	}

	var created struct {
		Branding models.TenantBranding `json:"branding"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/branding", strings.NewReader(valid), owner, tenant.ID.Hex()), http.StatusOK, &created)
	if created.Branding.PrimaryColor != "#aabbcc" || created.Branding.AccentColor != "#123456" || created.Branding.Font != models.BrandingFontHumanist || created.Branding.Version != 1 {
		t.Fatalf("unexpected normalized branding: %#v", created.Branding)
	}
	var conflict apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/branding", strings.NewReader(valid), owner, tenant.ID.Hex()), http.StatusConflict, &conflict)
	if conflict.Code != apierror.CodeVersionConflict {
		t.Fatalf("unexpected conflict: %#v", conflict)
	}
	reset := `{"primaryColor":"","accentColor":"","font":"","version":1}`
	productResponse(t, env, env.tenantRequest(t, http.MethodPut, "/api/product/branding", strings.NewReader(reset), owner, tenant.ID.Hex()), http.StatusOK, nil)
	if env.DB.SystemLogs().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "action": "tenant_branding.updated", "metadata.version": int64(2)}).Err() != nil {
		t.Fatal("tenant branding audit missing")
	}

	otherTenant := testutil.CreateTestTenant(t, env.DB, "Other Branding Tenant", owner.ID, false)
	var isolated struct {
		Branding models.TenantBranding `json:"branding"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/branding", nil, owner, otherTenant.ID.Hex()), http.StatusOK, &isolated)
	if isolated.Branding.Version != 0 || isolated.Branding.PrimaryColor != "" {
		t.Fatalf("tenant branding leaked across tenants: %#v", isolated.Branding)
	}
}
