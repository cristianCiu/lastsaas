package product

import (
	"testing"
	"time"

	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStaffProfileDefaultsAndOverrides(t *testing.T) {
	tenantID, userID := primitive.NewObjectID(), primitive.NewObjectID()
	tests := []struct {
		role          models.MemberRole
		businessRole  models.BusinessRole
		allLocations  bool
		permissionLen int
	}{
		{models.RoleOwner, models.BusinessRoleCompanyOwner, true, 8},
		{models.RoleAdmin, models.BusinessRoleOperationsManager, true, 8},
		{models.RoleUser, models.BusinessRoleViewer, false, 0},
	}
	for _, test := range tests {
		profile := NewDefaultStaffProfile(tenantID, userID, test.role, time.Now())
		if profile.BusinessRole != test.businessRole || profile.AllLocations != test.allLocations {
			t.Errorf("default for %s = role %s allLocations %v", test.role, profile.BusinessRole, profile.AllLocations)
		}
		if got := len(EffectivePermissions(&profile)); got != test.permissionLen {
			t.Errorf("default permissions for %s = %d, want %d", test.role, got, test.permissionLen)
		}
	}

	profile := NewDefaultStaffProfile(tenantID, userID, models.RoleOwner, time.Now())
	profile.PermissionOverrides = []models.PermissionOverride{
		{Permission: models.PermissionStorageAreasRead, Allowed: false},
		{Permission: models.PermissionStorageAreasManage, Allowed: true},
		{Permission: models.PermissionCatalogManage, Allowed: false},
	}
	if HasBusinessPermission(&profile, models.PermissionStorageAreasRead) {
		t.Fatal("explicit deny must override role default")
	}
	if !HasBusinessPermission(&profile, models.PermissionStorageAreasManage) {
		t.Fatal("explicit allow must override role default")
	}
	if HasBusinessPermission(&profile, models.PermissionCatalogManage) {
		t.Fatal("explicit catalog deny must override role default")
	}
}

func TestValidateStaffProfileRejectsAmbiguousScopeAndDuplicates(t *testing.T) {
	profile := NewDefaultStaffProfile(primitive.NewObjectID(), primitive.NewObjectID(), models.RoleUser, time.Now())
	locationID := primitive.NewObjectID()
	profile.AllLocations = true
	profile.LocationIDs = []primitive.ObjectID{locationID}
	if err := ValidateStaffProfile(&profile); err == nil {
		t.Fatal("expected allLocations with locationIds to fail")
	}

	profile.AllLocations = false
	profile.LocationIDs = []primitive.ObjectID{locationID, locationID}
	if err := ValidateStaffProfile(&profile); err == nil {
		t.Fatal("expected duplicate location IDs to fail")
	}

	profile.LocationIDs = nil
	profile.PermissionOverrides = []models.PermissionOverride{
		{Permission: models.PermissionStorageAreasRead, Allowed: true},
		{Permission: models.PermissionStorageAreasRead, Allowed: false},
	}
	if err := ValidateStaffProfile(&profile); err == nil {
		t.Fatal("expected duplicate permission overrides to fail")
	}

	profile.PermissionOverrides = []models.PermissionOverride{{Permission: "unknown", Allowed: true}}
	if err := ValidateStaffProfile(&profile); err == nil {
		t.Fatal("expected unknown permission to fail")
	}
	profile.PermissionOverrides = nil
	profile.BusinessRole = "unknown"
	if err := ValidateStaffProfile(&profile); err == nil {
		t.Fatal("expected unknown role to fail")
	}
}

func TestInventoryPermissionDefaultsAndOverrides(t *testing.T) {
	owner := NewDefaultStaffProfile(primitive.NewObjectID(), primitive.NewObjectID(), models.RoleOwner, time.Now())
	if !HasBusinessPermission(&owner, models.PermissionInventoryPost) || !HasBusinessPermission(&owner, models.PermissionInventoryRead) {
		t.Fatal("owner should be able to read and post inventory")
	}
	viewer := NewDefaultStaffProfile(primitive.NewObjectID(), primitive.NewObjectID(), models.RoleUser, time.Now())
	if HasBusinessPermission(&viewer, models.PermissionInventoryRead) {
		t.Fatal("viewer should not read inventory by default")
	}
	viewer.PermissionOverrides = []models.PermissionOverride{{Permission: models.PermissionInventoryRead, Allowed: true}}
	if !HasBusinessPermission(&viewer, models.PermissionInventoryRead) {
		t.Fatal("inventory read override was ignored")
	}
}
