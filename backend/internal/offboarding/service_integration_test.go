package offboarding_test

import (
	"context"
	"testing"
	"time"

	"lastsaas/internal/models"
	"lastsaas/internal/offboarding"
	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestOffboardTenantIsolatedRetryRevokesAndRetains(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	ctx := context.Background()

	owner := testutil.CreateTestUser(t, database, "offboard-owner@example.com", "password123", "Owner")
	member := testutil.CreateTestUser(t, database, "offboard-member@example.com", "password123", "Member")
	otherOwner := testutil.CreateTestUser(t, database, "offboard-other@example.com", "password123", "Other")
	tenant := testutil.CreateTestTenant(t, database, "Offboard Tenant", owner.ID, false)
	testutil.CreateTestMembership(t, database, member.ID, tenant.ID, models.RoleUser)
	otherTenant := testutil.CreateTestTenant(t, database, "Other Tenant", otherOwner.ID, false)

	now := time.Now()
	refresh := models.RefreshToken{ID: primitive.NewObjectID(), UserID: owner.ID, TokenHash: "offboard-refresh", ExpiresAt: now.Add(time.Hour), CreatedAt: now, LastActiveAt: now}
	if _, err := database.RefreshTokens().InsertOne(ctx, refresh); err != nil {
		t.Fatal(err)
	}
	retainedID := primitive.NewObjectID()
	if _, err := database.Sales().InsertOne(ctx, bson.M{
		"_id": retainedID, "tenantId": tenant.ID, "userId": owner.ID, "source": "test", "externalSaleId": "sale-1",
	}); err != nil {
		t.Fatal(err)
	}
	foreignSaleID := primitive.NewObjectID()
	if _, err := database.Sales().InsertOne(ctx, bson.M{
		"_id": foreignSaleID, "tenantId": otherTenant.ID, "userId": otherOwner.ID, "source": "test", "externalSaleId": "sale-2",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := offboarding.NewService(database).Offboard(ctx, tenant.ID, owner.ID)
	if err != nil {
		t.Fatalf("offboard: %v", err)
	}
	if result.Status != models.TenantOffboardingCompleted || result.TombstoneID.IsZero() {
		t.Fatalf("unexpected result: %#v", result)
	}
	if retry, err := offboarding.NewService(database).Offboard(ctx, tenant.ID, owner.ID); err != nil || retry.TombstoneID != result.TombstoneID {
		t.Fatalf("offboarding retry was not idempotent: %#v, %v", retry, err)
	}

	var tenantAfter models.Tenant
	if err := database.Tenants().FindOne(ctx, bson.M{"_id": tenant.ID}).Decode(&tenantAfter); err != nil || tenantAfter.IsActive || tenantAfter.OffboardingStatus != models.TenantOffboardingCompleted {
		t.Fatalf("tenant was not fenced: %#v, %v", tenantAfter, err)
	}
	var revoked models.RefreshToken
	if err := database.RefreshTokens().FindOne(ctx, bson.M{"_id": refresh.ID}).Decode(&revoked); err != nil || !revoked.IsRevoked {
		t.Fatalf("refresh token was not revoked: %#v, %v", revoked, err)
	}
	var retained bson.M
	if err := database.Sales().FindOne(ctx, bson.M{"_id": retainedID}).Decode(&retained); err != nil {
		t.Fatal("retained sale was erased")
	}
	pseudo, ok := retained["userId"].(primitive.ObjectID)
	if !ok || pseudo == owner.ID || pseudo.IsZero() {
		t.Fatalf("retained actor was not pseudonymized: %#v", retained["userId"])
	}
	if count, _ := database.Sales().CountDocuments(ctx, bson.M{"_id": foreignSaleID, "userId": otherOwner.ID}); count != 1 {
		t.Fatal("offboarding crossed tenant boundary")
	}
	if count, _ := database.StaffProfiles().CountDocuments(ctx, bson.M{"tenantId": tenant.ID}); count != 0 {
		t.Fatal("tenant profiles were not erased")
	}
	if count, _ := database.TenantMemberships().CountDocuments(ctx, bson.M{"tenantId": tenant.ID}); count != 0 {
		t.Fatal("tenant memberships were not erased")
	}
}
