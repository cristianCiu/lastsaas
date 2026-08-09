package handlers

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"testing"

	"lastsaas/internal/apierror"
	"lastsaas/internal/models"
	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
)

func tenantLogoPNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 128, 64))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func tenantLogoUploadRequest(t *testing.T, env *testEnv, user *models.User, tenantID, kind string, data []byte, contentType string, version int64) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="logo.png"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("version", strconv.FormatInt(version, 10)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := env.tenantRequest(t, http.MethodPut, "/api/product/branding/assets/"+kind, strings.NewReader(body.String()), user, tenantID)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestTenantBrandingAssetsSecureLifecycleAndIsolation(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "tenant-branding-assets")
	member := testutil.CreateTestUser(t, env.DB, "branding-assets-member@test.com", "Test1234!@#$", "Member")
	testutil.CreateTestMembership(t, env.DB, member.ID, tenant.ID, models.RoleUser)
	logo := tenantLogoPNG(t)

	productResponse(t, env, tenantLogoUploadRequest(t, env, member, tenant.ID.Hex(), "primary", logo, "image/png", 0), http.StatusForbidden, nil)
	productResponse(t, env, tenantLogoUploadRequest(t, env, owner, tenant.ID.Hex(), "primary", []byte("<svg><script/></svg>"), "image/png", 0), http.StatusBadRequest, nil)
	productResponse(t, env, tenantLogoUploadRequest(t, env, owner, tenant.ID.Hex(), "primary", logo, "image/jpeg", 0), http.StatusBadRequest, nil)

	var uploaded struct {
		Asset models.TenantBrandingAsset `json:"asset"`
	}
	productResponse(t, env, tenantLogoUploadRequest(t, env, owner, tenant.ID.Hex(), "primary", logo, "image/png", 0), http.StatusOK, &uploaded)
	if uploaded.Asset.Kind != models.TenantBrandingAssetPrimary || uploaded.Asset.ContentType != "image/png" || uploaded.Asset.Width != 128 || uploaded.Asset.Height != 64 || uploaded.Asset.Version != 1 {
		t.Fatalf("unexpected uploaded asset: %#v", uploaded.Asset)
	}

	var listed struct {
		Assets []map[string]any `json:"assets"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/branding/assets", nil, member, tenant.ID.Hex()), http.StatusOK, &listed)
	if len(listed.Assets) != 1 || listed.Assets[0]["kind"] != "primary" {
		t.Fatalf("unexpected asset list: %#v", listed.Assets)
	}
	if _, exposed := listed.Assets[0]["storageKey"]; exposed {
		t.Fatal("storage key exposed")
	}
	if _, exposed := listed.Assets[0]["data"]; exposed {
		t.Fatal("binary data exposed")
	}

	assetPath := "/api/product/branding/assets/primary"
	response, err := env.Client.Do(env.tenantRequest(t, http.MethodGet, assetPath, nil, member, tenant.ID.Hex()))
	if err != nil {
		t.Fatal(err)
	}
	etag := response.Header.Get("ETag")
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/png" || response.Header.Get("Cache-Control") != "private, no-cache" || etag == "" {
		response.Body.Close()
		t.Fatalf("unexpected asset response: status=%d headers=%v", response.StatusCode, response.Header)
	}
	response.Body.Close()
	conditional := env.tenantRequest(t, http.MethodGet, assetPath, nil, member, tenant.ID.Hex())
	conditional.Header.Set("If-None-Match", etag)
	productResponse(t, env, conditional, http.StatusNotModified, nil)

	var stale apierror.Response
	productResponse(t, env, tenantLogoUploadRequest(t, env, owner, tenant.ID.Hex(), "primary", logo, "image/png", 0), http.StatusConflict, &stale)
	if stale.Code != apierror.CodeVersionConflict {
		t.Fatalf("unexpected upload conflict: %#v", stale)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodDelete, assetPath+"?version=2", nil, owner, tenant.ID.Hex()), http.StatusConflict, nil)

	otherTenant := testutil.CreateTestTenant(t, env.DB, "Other Asset Tenant", owner.ID, false)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, assetPath, nil, owner, otherTenant.ID.Hex()), http.StatusNotFound, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodDelete, assetPath+"?version=1", nil, owner, tenant.ID.Hex()), http.StatusNoContent, nil)
	if env.DB.SystemLogs().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "action": "tenant_branding_asset.uploaded", "metadata.kind": "primary"}).Err() != nil {
		t.Fatal("asset upload audit missing")
	}
	if env.DB.SystemLogs().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "action": "tenant_branding_asset.deleted", "metadata.kind": "primary"}).Err() != nil {
		t.Fatal("asset delete audit missing")
	}
}
