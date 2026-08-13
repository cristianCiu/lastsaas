package handlers

import (
	"net/http"
	"strings"
	"testing"

	"lastsaas/internal/apierror"
	"lastsaas/internal/models"
)

func TestItemConversionsNormalizeLifecycleAndBlockBaseUnitChange(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "item-conversions")
	category := createTestCategory(t, env, owner, tenant.ID.Hex(), `{"code":"food","name":"Food"}`)
	baseUnit := createTestUnit(t, env, owner, tenant.ID.Hex(), `{"code":"kg","name":"Kilogram","symbol":"kg","dimension":"mass","precision":3}`)
	sourceUnit := createTestUnit(t, env, owner, tenant.ID.Hex(), `{"code":"g","name":"Gram","symbol":"g","dimension":"mass","precision":0}`)

	var item struct {
		Item models.Item `json:"item"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/items", strings.NewReader(`{"sku":"flour","name":"Flour","categoryId":"`+category.ID.Hex()+`","baseUnitId":"`+baseUnit.ID.Hex()+`","stockable":true}`), owner, tenant.ID.Hex()), http.StatusCreated, &item)

	var created struct {
		Conversion models.ItemConversion `json:"conversion"`
	}
	conversionPath := "/api/product/items/" + item.Item.ID.Hex() + "/conversions"
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, conversionPath, strings.NewReader(`{"fromUnitId":"`+sourceUnit.ID.Hex()+`","numerator":1000,"denominator":100}`), owner, tenant.ID.Hex()), http.StatusCreated, &created)
	if created.Conversion.Numerator != 10 || created.Conversion.Denominator != 1 || created.Conversion.Version != 1 {
		t.Fatalf("conversion was not reduced: %#v", created.Conversion)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, conversionPath, strings.NewReader(`{"fromUnitId":"`+sourceUnit.ID.Hex()+`","numerator":1,"denominator":2}`), owner, tenant.ID.Hex()), http.StatusConflict, nil)

	var updated struct {
		Conversion models.ItemConversion `json:"conversion"`
	}
	conversionItemPath := conversionPath + "/" + created.Conversion.ID.Hex()
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, conversionItemPath, strings.NewReader(`{"version":1,"numerator":6,"denominator":8}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if updated.Conversion.Numerator != 3 || updated.Conversion.Denominator != 4 || updated.Conversion.Version != 2 {
		t.Fatalf("conversion update was not reduced: %#v", updated.Conversion)
	}

	// Existing conversions remain readable and mutable after their source unit is inactive.
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, "/api/product/units/"+sourceUnit.ID.Hex(), strings.NewReader(`{"version":1,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, nil)
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, conversionItemPath, strings.NewReader(`{"version":2,"isActive":false}`), owner, tenant.ID.Hex()), http.StatusOK, &updated)
	if updated.Conversion.IsActive || updated.Conversion.Version != 3 {
		t.Fatalf("unexpected conversion lifecycle update: %#v", updated.Conversion)
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, conversionPath, nil, owner, tenant.ID.Hex()), http.StatusOK, nil)

	// Base-unit changes are forbidden even when the conversion itself is inactive.
	newBase := createTestUnit(t, env, owner, tenant.ID.Hex(), `{"code":"lb","name":"Pound","symbol":"lb","dimension":"mass","precision":2}`)
	var conflict apierror.Response
	productResponse(t, env, env.tenantRequest(t, http.MethodPatch, "/api/product/items/"+item.Item.ID.Hex(), strings.NewReader(`{"version":1,"baseUnitId":"`+newBase.ID.Hex()+`"}`), owner, tenant.ID.Hex()), http.StatusConflict, &conflict)
	if conflict.Code != apierror.CodeConflict {
		t.Fatalf("unexpected base-unit conflict: %#v", conflict)
	}
}

func TestItemConversionsRejectInvalidReferencesAndTenantLeaks(t *testing.T) {
	env := setupTestServer(t)
	defer env.Cleanup()
	owner, tenant := setupProductTenant(t, env, "conversion-refs")
	category := createTestCategory(t, env, owner, tenant.ID.Hex(), `{"code":"food","name":"Food"}`)
	base := createTestUnit(t, env, owner, tenant.ID.Hex(), `{"code":"kg","name":"Kilogram","symbol":"kg","dimension":"mass","precision":3}`)
	volume := createTestUnit(t, env, owner, tenant.ID.Hex(), `{"code":"l","name":"Liter","symbol":"l","dimension":"volume","precision":2}`)
	var item struct {
		Item models.Item `json:"item"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/items", strings.NewReader(`{"sku":"sugar","name":"Sugar","categoryId":"`+category.ID.Hex()+`","baseUnitId":"`+base.ID.Hex()+`","stockable":true}`), owner, tenant.ID.Hex()), http.StatusCreated, &item)
	path := "/api/product/items/" + item.Item.ID.Hex() + "/conversions"
	for _, body := range []string{
		`{"fromUnitId":"` + base.ID.Hex() + `","numerator":1,"denominator":1}`,
		`{"fromUnitId":"` + volume.ID.Hex() + `","numerator":1,"denominator":1}`,
		`{"fromUnitId":"` + volume.ID.Hex() + `","numerator":0,"denominator":1}`,
	} {
		productResponse(t, env, env.tenantRequest(t, http.MethodPost, path, strings.NewReader(body), owner, tenant.ID.Hex()), http.StatusBadRequest, nil)
	}

	otherOwner, otherTenant := setupProductTenant(t, env, "conversion-other")
	otherCategory := createTestCategory(t, env, otherOwner, otherTenant.ID.Hex(), `{"code":"other","name":"Other"}`)
	otherUnit := createTestUnit(t, env, otherOwner, otherTenant.ID.Hex(), `{"code":"each","name":"Each","symbol":"ea","dimension":"count","precision":0}`)
	var otherItem struct {
		Item models.Item `json:"item"`
	}
	productResponse(t, env, env.tenantRequest(t, http.MethodPost, "/api/product/items", strings.NewReader(`{"sku":"other","name":"Other","categoryId":"`+otherCategory.ID.Hex()+`","baseUnitId":"`+otherUnit.ID.Hex()+`","stockable":true}`), otherOwner, otherTenant.ID.Hex()), http.StatusCreated, &otherItem)
	productResponse(t, env, env.tenantRequest(t, http.MethodGet, "/api/product/items/"+otherItem.Item.ID.Hex()+"/conversions", nil, owner, tenant.ID.Hex()), http.StatusNotFound, nil)
}
