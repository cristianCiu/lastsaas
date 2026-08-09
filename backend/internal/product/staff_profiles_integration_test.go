package product_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"lastsaas/internal/models"
	"lastsaas/internal/product"
	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestReconcileStaffProfilesInsertsOnlyMissingNonRootProfiles(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	testutil.CleanupCollections(t, database)
	ctx := context.Background()
	owner := testutil.CreateTestUser(t, database, "reconcile-owner@example.com", "password123", "Owner")
	tenant := testutil.CreateTestTenant(t, database, "Reconcile Tenant", owner.ID, false)
	member := testutil.CreateTestUser(t, database, "reconcile-member@example.com", "password123", "Member")
	now := time.Now()
	membership := models.TenantMembership{
		ID: primitive.NewObjectID(), TenantID: tenant.ID, UserID: member.ID, Role: models.RoleUser,
		JoinedAt: now, UpdatedAt: now,
	}
	if _, err := database.TenantMemberships().InsertOne(ctx, membership); err != nil {
		t.Fatalf("insert missing-profile membership: %v", err)
	}
	if _, err := database.StaffProfiles().UpdateOne(ctx,
		bson.M{"tenantId": tenant.ID, "userId": owner.ID},
		bson.M{"$set": bson.M{"businessRole": models.BusinessRoleController, "status": models.StaffProfileInactive}}); err != nil {
		t.Fatalf("customize existing profile: %v", err)
	}
	rootOwner := testutil.CreateTestUser(t, database, "reconcile-root@example.com", "password123", "Root")
	root := testutil.CreateTestTenant(t, database, "Reconcile Root", rootOwner.ID, true)

	if err := product.ReconcileStaffProfiles(ctx, database); err != nil {
		t.Fatalf("first reconciliation: %v", err)
	}
	if err := product.ReconcileStaffProfiles(ctx, database); err != nil {
		t.Fatalf("second reconciliation: %v", err)
	}
	var reconciled models.StaffProfile
	if err := database.StaffProfiles().FindOne(ctx, bson.M{"tenantId": tenant.ID, "userId": member.ID}).Decode(&reconciled); err != nil {
		t.Fatalf("missing profile was not inserted: %v", err)
	}
	if reconciled.BusinessRole != models.BusinessRoleViewer || reconciled.AllLocations || reconciled.Version != 1 {
		t.Fatalf("unexpected reconciled profile: %#v", reconciled)
	}
	var existing models.StaffProfile
	if err := database.StaffProfiles().FindOne(ctx, bson.M{"tenantId": tenant.ID, "userId": owner.ID}).Decode(&existing); err != nil {
		t.Fatal(err)
	}
	if existing.BusinessRole != models.BusinessRoleController || existing.Status != models.StaffProfileInactive || existing.Version != 1 {
		t.Fatalf("existing profile was changed: %#v", existing)
	}
	if count, err := database.StaffProfiles().CountDocuments(ctx, bson.M{"tenantId": root.ID}); err != nil || count != 0 {
		t.Fatalf("root profiles count = %d, err %v", count, err)
	}
}

func TestReconcileStaffProfilesRejectsAndPreservesInvalidExistingProfile(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	testutil.CleanupCollections(t, database)
	owner := testutil.CreateTestUser(t, database, "invalid-reconcile-owner@example.com", "password123", "Owner")
	tenant := testutil.CreateTestTenant(t, database, "Invalid Reconcile Tenant", owner.ID, false)
	foreignLocationID := primitive.NewObjectID()
	if _, err := database.StaffProfiles().UpdateOne(context.Background(),
		bson.M{"tenantId": tenant.ID, "userId": owner.ID},
		bson.M{"$set": bson.M{"allLocations": false, "locationIds": bson.A{foreignLocationID}}}); err != nil {
		t.Fatalf("make existing profile invalid: %v", err)
	}

	if err := product.ReconcileStaffProfiles(context.Background(), database); err == nil {
		t.Fatal("expected invalid existing profile to fail reconciliation")
	}
	var preserved models.StaffProfile
	if err := database.StaffProfiles().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": owner.ID}).Decode(&preserved); err != nil {
		t.Fatalf("load preserved profile: %v", err)
	}
	if preserved.AllLocations || len(preserved.LocationIDs) != 1 || preserved.LocationIDs[0] != foreignLocationID {
		t.Fatalf("invalid existing profile was changed: %#v", preserved)
	}
}

func TestConcurrentOwnershipTransfersLeaveExactlyOneOwner(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	testutil.CleanupCollections(t, database)
	owner := testutil.CreateTestUser(t, database, "concurrent-owner@example.com", "password123", "Owner")
	tenant := testutil.CreateTestTenant(t, database, "Concurrent Ownership Tenant", owner.ID, false)
	targets := []*models.User{
		testutil.CreateTestUser(t, database, "concurrent-one@example.com", "password123", "One"),
		testutil.CreateTestUser(t, database, "concurrent-two@example.com", "password123", "Two"),
	}
	for _, target := range targets {
		testutil.CreateTestMembership(t, database, target.ID, tenant.ID, models.RoleAdmin)
	}

	duplicateOwner := models.TenantMembership{
		ID: primitive.NewObjectID(), TenantID: tenant.ID, UserID: primitive.NewObjectID(), Role: models.RoleOwner,
		JoinedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := database.TenantMemberships().InsertOne(context.Background(), duplicateOwner); !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("owner partial unique index did not reject second owner: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, len(targets))
	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(targetID primitive.ObjectID) {
			defer wg.Done()
			session, err := database.Client.StartSession()
			if err != nil {
				results <- err
				return
			}
			defer session.EndSession(context.Background())
			<-start
			_, err = session.WithTransaction(context.Background(), func(sc mongo.SessionContext) (interface{}, error) {
				return nil, product.TransferTenantOwnership(sc, database, tenant.ID, owner.ID, targetID)
			})
			results <- err
		}(target.ID)
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent transfers = %d, want 1", successes)
	}
	if count, err := database.TenantMemberships().CountDocuments(context.Background(), bson.M{"tenantId": tenant.ID, "role": models.RoleOwner}); err != nil || count != 1 {
		t.Fatalf("owner count = %d, err %v", count, err)
	}
	var currentOwner models.TenantMembership
	if err := database.TenantMemberships().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "role": models.RoleOwner}).Decode(&currentOwner); err != nil {
		t.Fatalf("load resulting owner: %v", err)
	}
	var ownerProfile, formerOwnerProfile models.StaffProfile
	if err := database.StaffProfiles().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": currentOwner.UserID}).Decode(&ownerProfile); err != nil {
		t.Fatalf("load resulting owner profile: %v", err)
	}
	if err := database.StaffProfiles().FindOne(context.Background(), bson.M{"tenantId": tenant.ID, "userId": owner.ID}).Decode(&formerOwnerProfile); err != nil {
		t.Fatalf("load former owner profile: %v", err)
	}
	if ownerProfile.BusinessRole != models.BusinessRoleCompanyOwner || !ownerProfile.AllLocations || ownerProfile.Status != models.StaffProfileActive || len(ownerProfile.PermissionOverrides) != 0 {
		t.Fatalf("new owner profile invariant failed: %#v", ownerProfile)
	}
	if formerOwnerProfile.BusinessRole != models.BusinessRoleOperationsManager || !formerOwnerProfile.AllLocations || formerOwnerProfile.Status != models.StaffProfileActive || len(formerOwnerProfile.PermissionOverrides) != 0 {
		t.Fatalf("former owner defaults failed: %#v", formerOwnerProfile)
	}
}
