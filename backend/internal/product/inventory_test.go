package product

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"lastsaas/internal/inventory"
	"lastsaas/internal/models"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestInventoryReferencesAllowInventoryOnlyPrincipals(t *testing.T) {
	inventoryOnly := &models.StaffProfile{
		Status:       models.StaffProfileActive,
		BusinessRole: models.BusinessRoleViewer,
		PermissionOverrides: []models.PermissionOverride{
			{Permission: models.PermissionInventoryRead, Allowed: true},
		},
	}
	if !hasInventoryReferencePermission(inventoryOnly) {
		t.Fatal("inventory-only principal was denied reference access")
	}
	viewer := &models.StaffProfile{Status: models.StaffProfileActive, BusinessRole: models.BusinessRoleViewer}
	if hasInventoryReferencePermission(viewer) {
		t.Fatal("viewer unexpectedly received inventory reference access")
	}
}

func TestReversalAuthorizationCoversEveryMovementLocation(t *testing.T) {
	source, destination := primitive.NewObjectID(), primitive.NewObjectID()
	profile := &models.StaffProfile{Status: models.StaffProfileActive, LocationIDs: []primitive.ObjectID{source}}
	movements := []models.StockMovement{{LocationID: source}, {LocationID: destination}}
	if allReversalLocationsAuthorized(profile, movements) {
		t.Fatal("reversal was authorized without destination location access")
	}
	profile.LocationIDs = append(profile.LocationIDs, destination)
	if !allReversalLocationsAuthorized(profile, movements) {
		t.Fatal("reversal was denied after all movement locations were authorized")
	}
}

func TestInventoryReferencesPermissionHandlerAllowsInventoryOnlyAccess(t *testing.T) {
	viewer := &models.StaffProfile{Status: models.StaffProfileActive, BusinessRole: models.BusinessRoleViewer}

	for _, test := range []struct {
		name    string
		profile *models.StaffProfile
		status  int
	}{
		{name: "inventory.read-only principal", profile: &models.StaffProfile{
			Status:       models.StaffProfileActive,
			BusinessRole: models.BusinessRoleViewer,
			PermissionOverrides: []models.PermissionOverride{
				{Permission: models.PermissionInventoryRead, Allowed: true},
				{Permission: models.PermissionCatalogRead, Allowed: false},
				{Permission: models.PermissionStorageAreasRead, Allowed: false},
			},
		}, status: http.StatusNoContent},
		{name: "inventory.manage principal", profile: &models.StaffProfile{
			Status:       models.StaffProfileActive,
			BusinessRole: models.BusinessRoleViewer,
			PermissionOverrides: []models.PermissionOverride{
				{Permission: models.PermissionInventoryManage, Allowed: true},
			},
		}, status: http.StatusNoContent},
		{name: "inventory.post principal", profile: &models.StaffProfile{
			Status:       models.StaffProfileActive,
			BusinessRole: models.BusinessRoleViewer,
			PermissionOverrides: []models.PermissionOverride{
				{Permission: models.PermissionInventoryPost, Allowed: true},
			},
		}, status: http.StatusNoContent},
		{name: "catalog-only viewer", profile: viewer, status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			reached := false
			handler := RequireInventoryReferencePermission()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/product/inventory/references", nil)
			request = request.WithContext(context.WithValue(request.Context(), staffProfileContextKey{}, test.profile))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if reached != (test.status == http.StatusNoContent) {
				t.Fatalf("next handler reached = %v, want %v", reached, test.status == http.StatusNoContent)
			}
		})
	}
}

func TestInventoryRoutesRegisterReferencesAndReversalAuthorization(t *testing.T) {
	router := mux.NewRouter()
	RegisterRoutes(router, nil, nil, nil, nil)

	wanted := map[string]bool{
		"/product/inventory/references":                                             false,
		"/product/locations/{locationId}/inventory/references":                      false,
		"/product/locations/{locationId}/inventory/postings/{postingId}/reverse":    false,
		"/product/locations/{locationId}/inventory/counts":                          false,
		"/product/locations/{locationId}/inventory/counts/{countId}":                false,
		"/product/locations/{locationId}/inventory/counts/{countId}/lot-options":    false,
		"/product/locations/{locationId}/inventory/counts/{countId}/freeze":         false,
		"/product/locations/{locationId}/inventory/counts/{countId}/review":         false,
		"/product/locations/{locationId}/inventory/counts/{countId}/lines/discover": false,
		"/product/locations/{locationId}/inventory/counts/{countId}/cancel":         false,
		"/product/locations/{locationId}/inventory/counts/{countId}/post":           false,
		"/product/locations/{locationId}/inventory/reconciliation":                  false,
	}
	if err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, err := route.GetPathTemplate()
		if err != nil {
			return err
		}
		methods, err := route.GetMethods()
		if err != nil {
			return nil
		}
		for _, method := range methods {
			if method == http.MethodGet && path == "/product/inventory/references" {
				wanted[path] = true
			}
			if method == http.MethodGet && path == "/product/locations/{locationId}/inventory/references" {
				wanted[path] = true
			}
			if method == http.MethodPost && path == "/product/locations/{locationId}/inventory/postings/{postingId}/reverse" {
				wanted[path] = true
			}
			if method == http.MethodPost && (path == "/product/locations/{locationId}/inventory/counts" || path == "/product/locations/{locationId}/inventory/counts/{countId}/freeze" || path == "/product/locations/{locationId}/inventory/counts/{countId}/review" || path == "/product/locations/{locationId}/inventory/counts/{countId}/lines/discover" || path == "/product/locations/{locationId}/inventory/counts/{countId}/cancel" || path == "/product/locations/{locationId}/inventory/counts/{countId}/post" || path == "/product/locations/{locationId}/inventory/reconciliation") {
				wanted[path] = true
			}
			if method == http.MethodGet && (path == "/product/locations/{locationId}/inventory/counts" || path == "/product/locations/{locationId}/inventory/counts/{countId}" || path == "/product/locations/{locationId}/inventory/counts/{countId}/lot-options") {
				wanted[path] = true
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	for path, found := range wanted {
		if !found {
			t.Errorf("required inventory route %q was not registered", path)
		}
	}
}

func TestInventoryAreaLockMapsToConflict(t *testing.T) {
	for _, err := range []error{inventory.ErrInventoryAreaLocked, inventory.ErrCountOwnershipRequired} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/product/locations/location/inventory/reconciliation", nil)
		writeInventoryError(response, request, err)
		if response.Code != http.StatusConflict {
			t.Fatalf("%v status = %d, want %d", err, response.Code, http.StatusConflict)
		}
		var body struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		if decodeErr := json.NewDecoder(response.Body).Decode(&body); decodeErr != nil {
			t.Fatalf("decode conflict: %v", decodeErr)
		}
		if body.Code != "CONFLICT" || body.Error != err.Error() {
			t.Fatalf("conflict body = %#v", body)
		}
	}
}

func TestActiveCountListQueryErrorsMapToValidationResponses(t *testing.T) {
	for _, err := range []error{inventory.ErrActiveCountCursorInvalid, inventory.ErrActiveCountLimitInvalid} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/product/locations/location/inventory/counts", nil)
		writeInventoryError(response, request, err)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%v status = %d, want %d", err, response.Code, http.StatusBadRequest)
		}
		var body struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		if decodeErr := json.NewDecoder(response.Body).Decode(&body); decodeErr != nil {
			t.Fatalf("decode validation response: %v", decodeErr)
		}
		if body.Code != "VALIDATION_ERROR" || body.Error != err.Error() {
			t.Fatalf("validation body = %#v", body)
		}
	}
}

func TestCountReadPermissionAllowsReadManageOrPostOnly(t *testing.T) {
	for _, permission := range []models.BusinessPermission{models.PermissionInventoryRead, models.PermissionInventoryManage, models.PermissionInventoryPost} {
		profile := &models.StaffProfile{Status: models.StaffProfileActive, BusinessRole: models.BusinessRoleViewer, PermissionOverrides: []models.PermissionOverride{{Permission: permission, Allowed: true}}}
		reached := false
		handler := RequireInventoryCountReadPermission()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached = true
			w.WriteHeader(http.StatusNoContent)
		}))
		request := httptest.NewRequest(http.MethodGet, "/product/locations/location/inventory/counts", nil).WithContext(context.WithValue(context.Background(), staffProfileContextKey{}, profile))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || !reached {
			t.Fatalf("permission %s status=%d reached=%t", permission, response.Code, reached)
		}
	}
	profile := &models.StaffProfile{Status: models.StaffProfileActive, BusinessRole: models.BusinessRoleViewer}
	handler := RequireInventoryCountReadPermission()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("viewer reached count read handler") }))
	request := httptest.NewRequest(http.MethodGet, "/product/locations/location/inventory/counts", nil).WithContext(context.WithValue(context.Background(), staffProfileContextKey{}, profile))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("viewer status=%d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestCountLotOptionsPermissionRemainsManageOnly(t *testing.T) {
	for _, permission := range []models.BusinessPermission{models.PermissionInventoryRead, models.PermissionInventoryPost} {
		profile := &models.StaffProfile{Status: models.StaffProfileActive, BusinessRole: models.BusinessRoleViewer, PermissionOverrides: []models.PermissionOverride{{Permission: permission, Allowed: true}}}
		reached := false
		handler := RequireBusinessPermission(models.PermissionInventoryManage)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
		request := httptest.NewRequest(http.MethodGet, "/product/locations/location/inventory/counts/count/lot-options", nil).WithContext(context.WithValue(context.Background(), staffProfileContextKey{}, profile))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || reached {
			t.Fatalf("permission %s status=%d reached=%t", permission, response.Code, reached)
		}
	}
}
