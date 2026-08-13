package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	masterimports "lastsaas/internal/imports"
	"lastsaas/internal/models"
	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
)

func TestImportsRoutePermissionsAndRunTenantScope(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "imports-routes")
	content := `{"target":"units","content":"code,name,symbol,dimension,precision,isActive\nkg,Kilogram,kg,mass,3,true\n","idempotencyKey":"route-key"}`

	member := testutil.CreateTestUser(t, env.DB, "imports-reader@test.com", "Test1234!@#$", "Reader")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/imports/dry-run", strings.NewReader(content), member, tenant.ID.Hex()), http.StatusForbidden, nil)
	if _, err := env.DB.StaffProfiles().UpdateOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": member.ID}, bson.M{"$set": bson.M{"permissionOverrides": bson.A{bson.M{"permission": models.PermissionCatalogRead, "allowed": true}}}}); err != nil {
		t.Fatal(err)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/imports/dry-run", strings.NewReader(content), member, tenant.ID.Hex()), http.StatusOK, nil)

	var applied struct {
		Run json.RawMessage `json:"run"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/imports", strings.NewReader(content), owner, tenant.ID.Hex()), http.StatusOK, &applied)
	var run models.ImportRun
	if err := json.Unmarshal(applied.Run, &run); err != nil {
		t.Fatal(err)
	}
	var runJSON map[string]any
	if err := json.Unmarshal(applied.Run, &runJSON); err != nil {
		t.Fatal(err)
	}
	if run.ID.IsZero() {
		t.Fatalf("unexpected import run: %#v", run)
	}
	if _, exposed := runJSON["tenantId"]; exposed {
		t.Fatal("import run response exposed tenantId")
	}
	otherOwner, otherTenant := setupProductTenant(t, env, "imports-other")
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/imports/"+run.ID.Hex(), nil, otherOwner, otherTenant.ID.Hex()), http.StatusNotFound, nil)
}

func TestImportsHTTPBoundaryRejectsJSONExpandedCSV(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "imports-limit")
	// A maximum-sized decoded CSV with JSON-expanding control characters still
	// fits through the 1 MiB HTTP body boundary under the conservative cap.
	prefix := "code,name,symbol,dimension,precision,isActive\nkg,\""
	suffix := "\",kg,mass,3,true\n"
	content := prefix + strings.Repeat("\x01", masterimports.MaxCSVBytes-len(prefix)-len(suffix)) + suffix
	body, err := json.Marshal(masterimports.Request{Target: models.ImportTargetUnits, Content: content, IdempotencyKey: "http-limit-key"})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) >= 1<<20 {
		t.Fatalf("maximum CSV request exceeds 1 MiB: %d bytes", len(body))
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/imports/dry-run", strings.NewReader(string(body)), owner, tenant.ID.Hex()), http.StatusOK, nil)
}
