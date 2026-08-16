package product

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestForecastExecutionAndReadRoutesRegister(t *testing.T) {
	router := mux.NewRouter()
	RegisterRoutes(router, nil, nil, nil, nil)
	want := map[string]string{
		"/product/locations/{locationId}/forecast/runs":                                              http.MethodPost,
		"/product/locations/{locationId}/forecast/jobs":                                              http.MethodGet,
		"/product/locations/{locationId}/forecast/runs/{runId}/points":                               http.MethodGet,
		"/product/locations/{locationId}/forecast/datasets/{datasetId}/inputs":                       http.MethodGet,
		"/product/locations/{locationId}/forecast/runs/{runId}/maturity":                             http.MethodGet,
		"/product/locations/{locationId}/forecast/recommendations":                                   http.MethodGet,
		"/product/locations/{locationId}/forecast/recommendations/{recommendationId}":                http.MethodGet,
		"/product/locations/{locationId}/forecast/reorder-recommendations/{id}/purchase-order-draft": http.MethodPost,
		"/product/locations/{locationId}/forecast/coverage":                                          http.MethodGet,
		"/product/locations/{locationId}/forecast/coverage/{coverageId}":                             http.MethodGet,
		"/product/locations/{locationId}/forecast/runs/{runId}/recommendations":                      http.MethodGet,
		"/product/locations/{locationId}/forecast/runs/{runId}/recommendations/{recommendationId}":   http.MethodGet,
		"/product/locations/{locationId}/forecast/runs/{runId}/coverage":                             http.MethodGet,
		"/product/locations/{locationId}/forecast/runs/{runId}/coverage/{coverageId}":                http.MethodGet,
		"/product/locations/{locationId}/forecast/shadow-kpis":                                       http.MethodGet,
		"/product/locations/{locationId}/forecast/shadow-kpis/{reportId}":                            http.MethodGet,
	}
	seen := map[string]bool{}
	if err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, err := route.GetPathTemplate()
		if err != nil {
			return err
		}
		methods, _ := route.GetMethods()
		for _, method := range methods {
			if want[path] == method {
				seen[path] = true
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for path := range want {
		if !seen[path] {
			t.Errorf("route %s was not registered", path)
		}
	}
}

func TestShadowKPIReportFilterIsTenantAndLocationScoped(t *testing.T) {
	tenant := primitive.NewObjectID()
	location := primitive.NewObjectID()
	filter := forecastTenantLocationFilter(tenant, location)
	if filter["tenantId"] != tenant || filter["locationId"] != location {
		t.Fatalf("shadow KPI report filter lost scope: %#v", filter)
	}
}

func TestForecastReadFilterIsTenantAndLocationScoped(t *testing.T) {
	tenant := primitive.NewObjectID()
	location := primitive.NewObjectID()
	filter := forecastTenantLocationFilter(tenant, location)
	if filter["tenantId"] != tenant || filter["locationId"] != location {
		t.Fatalf("forecast read filter lost scope: %#v", filter)
	}
}

func TestForecastRunRequiresManagerRole(t *testing.T) {
	for _, tc := range []struct {
		name   string
		role   models.BusinessRole
		status int
	}{
		{"operations manager", models.BusinessRoleOperationsManager, http.StatusNoContent},
		{"viewer", models.BusinessRoleViewer, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hit := false
			h := requireForecastManager()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true; w.WriteHeader(http.StatusNoContent) }))
			profile := &models.StaffProfile{Status: models.StaffProfileActive, BusinessRole: tc.role}
			req := httptest.NewRequest(http.MethodPost, "/product/locations/location/forecast/runs", nil).WithContext(context.WithValue(context.Background(), staffProfileContextKey{}, profile))
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != tc.status || hit != (tc.status == http.StatusNoContent) {
				t.Fatalf("status=%d hit=%v", res.Code, hit)
			}
		})
	}
}

func TestRecommendationDraftRequiresForecastAndPurchasingManage(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := withProductMiddleware(next, RequireBusinessPermission(models.PermissionForecastManage), RequireBusinessPermission(models.PermissionPurchasingManage))
	for _, tc := range []struct {
		name    string
		profile *models.StaffProfile
		status  int
	}{
		{name: "owner has both permissions", profile: &models.StaffProfile{Status: models.StaffProfileActive, BusinessRole: models.BusinessRoleCompanyOwner}, status: http.StatusNoContent},
		{name: "purchasing role lacks forecast manage", profile: &models.StaffProfile{Status: models.StaffProfileActive, BusinessRole: models.BusinessRolePurchasing}, status: http.StatusForbidden},
		{name: "forecast-only override lacks purchasing manage", profile: &models.StaffProfile{Status: models.StaffProfileActive, BusinessRole: models.BusinessRoleViewer, PermissionOverrides: []models.PermissionOverride{{Permission: models.PermissionForecastManage, Allowed: true}}}, status: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/product/locations/location/forecast/reorder-recommendations/id/purchase-order-draft", nil).WithContext(context.WithValue(context.Background(), staffProfileContextKey{}, tc.profile))
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("status=%d, want %d", res.Code, tc.status)
			}
		})
	}
}

func TestRecommendationDraftRequestRequiresExplicitSupplierAndIdempotency(t *testing.T) {
	valid := purchaseOrderDraftFromRecommendationRequest{SupplierItemID: primitive.NewObjectID(), IdempotencyKey: "draft-key-1"}
	if err := validation.Validate(&valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	for _, invalid := range []purchaseOrderDraftFromRecommendationRequest{
		{IdempotencyKey: "draft-key-1"},
		{SupplierItemID: primitive.NewObjectID(), IdempotencyKey: "short"},
	} {
		if err := validation.Validate(&invalid); err == nil {
			t.Fatalf("invalid request accepted: %+v", invalid)
		}
	}
}
