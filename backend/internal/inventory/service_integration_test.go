package inventory_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/inventory"
	"lastsaas/internal/models"
	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type inventoryFixtures struct {
	tenant, user, location, area, item primitive.ObjectID
}

func seedInventory(t *testing.T, database *db.MongoDB, tenant primitive.ObjectID) inventoryFixtures {
	t.Helper()
	f := inventoryFixtures{tenant: tenant, user: primitive.NewObjectID(), location: primitive.NewObjectID(), area: primitive.NewObjectID(), item: primitive.NewObjectID()}
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := database.Locations().InsertOne(ctx, bson.M{"_id": f.location, "tenantId": tenant, "code": "inventory-location-" + f.location.Hex()[:8], "name": "Inventory location", "timezone": "UTC", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now, "limitSlot": int64(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.StorageAreas().InsertOne(ctx, bson.M{"_id": f.area, "tenantId": tenant, "locationId": f.location, "name": "Main", "type": "dry", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Items().InsertOne(ctx, bson.M{"_id": f.item, "tenantId": tenant, "sku": "inventory-item-" + f.item.Hex()[:8], "name": "Inventory item", "categoryId": primitive.NewObjectID(), "baseUnitId": primitive.NewObjectID(), "stockable": true, "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestLedgerKernelReplicaSetInvariants(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	tenantOne, tenantTwo := primitive.NewObjectID(), primitive.NewObjectID()
	one := seedInventory(t, database, tenantOne)
	two := seedInventory(t, database, tenantTwo)
	ctx := context.Background()
	service := inventory.NewService(database)
	command := inventory.Command{TenantID: one.tenant, UserID: one.user, LocationID: one.location, StorageAreaID: one.area, ItemID: one.item, QuantityMicros: 2_000_000, IdempotencyKey: "opening-0001"}
	first, err := service.OpeningBalance(ctx, command)
	if err != nil {
		t.Fatalf("opening balance: %v", err)
	}
	replay, err := service.OpeningBalance(ctx, command)
	if err != nil || replay.Posting.ID != first.Posting.ID {
		t.Fatalf("exact replay = %#v, %v", replay, err)
	}
	command.QuantityMicros = 3_000_000
	if _, err := service.OpeningBalance(ctx, command); !errors.Is(err, inventory.ErrIdempotencyConflict) {
		t.Fatalf("differing replay error = %v", err)
	}
	if _, err := service.ManualAdjustment(ctx, inventory.Command{TenantID: one.tenant, UserID: one.user, LocationID: one.location, StorageAreaID: one.area, ItemID: one.item, QuantityMicros: -3_000_000, IdempotencyKey: "negative-0001"}); !errors.Is(err, inventory.ErrInsufficientStock) {
		t.Fatalf("negative stock error = %v", err)
	}
	if count, _ := database.StockPostings().CountDocuments(ctx, bson.M{"tenantId": one.tenant}); count != 1 {
		t.Fatalf("negative rejection wrote %d postings", count)
	}
	// Historical postings remain reversible after their source references are
	// deactivated. Forward commands must still reject those same references.
	if _, err := database.Locations().UpdateOne(ctx, bson.M{"_id": one.location, "tenantId": one.tenant}, bson.M{"$set": bson.M{"isActive": false}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": one.area, "tenantId": one.tenant}, bson.M{"$set": bson.M{"isActive": false}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Items().UpdateOne(ctx, bson.M{"_id": one.item, "tenantId": one.tenant}, bson.M{"$set": bson.M{"isActive": false}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ManualAdjustment(ctx, inventory.Command{TenantID: one.tenant, UserID: one.user, LocationID: one.location, StorageAreaID: one.area, ItemID: one.item, QuantityMicros: 1_000_000, IdempotencyKey: "inactive-0001"}); !errors.Is(err, inventory.ErrInvalidReference) {
		t.Fatalf("forward inactive-reference error = %v", err)
	}
	reversed, err := service.Reverse(ctx, inventory.ReverseCommand{TenantID: one.tenant, UserID: one.user, LocationID: one.location, PostingID: first.Posting.ID, IdempotencyKey: "reverse-0001"})
	if err != nil {
		t.Fatalf("reverse posting: %v", err)
	}
	if reversed.Balance.QuantityMicros != 0 {
		t.Fatalf("reversed balance = %d", reversed.Balance.QuantityMicros)
	}
	if _, err := service.Reverse(ctx, inventory.ReverseCommand{TenantID: one.tenant, UserID: one.user, LocationID: one.location, PostingID: first.Posting.ID, IdempotencyKey: "reverse-0002"}); !errors.Is(err, inventory.ErrPostingAlreadyReversed) {
		t.Fatalf("second reversal error = %v", err)
	}
	journal, err := service.ListJournal(ctx, one.tenant, one.location, nil)
	if err != nil || len(journal) != 2 {
		t.Fatalf("journal = %d, %v", len(journal), err)
	}
	var sum int64
	for _, entry := range journal {
		sum += entry.Movement.QuantityMicros
	}
	balances, err := service.ListBalances(ctx, one.tenant, one.location)
	if err != nil || len(balances) != 1 || balances[0].QuantityMicros != sum {
		t.Fatalf("journal/balance invariant: sum=%d balances=%#v err=%v", sum, balances, err)
	}
	if _, err := service.OpeningBalance(ctx, inventory.Command{TenantID: two.tenant, UserID: two.user, LocationID: one.location, StorageAreaID: one.area, ItemID: one.item, QuantityMicros: 1_000_000, IdempotencyKey: "foreign-0001"}); !errors.Is(err, inventory.ErrInvalidReference) {
		t.Fatalf("tenant isolation error = %v", err)
	}
	if entries, err := service.ListJournal(ctx, two.tenant, two.location, nil); err != nil || len(entries) != 0 {
		t.Fatalf("tenant isolation journal = %d, %v", len(entries), err)
	}
	hooked := inventory.NewService(database)
	hooked.TestAfterPostingInsertHook = func(context.Context) error { return errors.New("forced rollback") }
	if _, err := hooked.OpeningBalance(ctx, inventory.Command{TenantID: one.tenant, UserID: one.user, LocationID: one.location, StorageAreaID: one.area, ItemID: one.item, QuantityMicros: 1_000_000, IdempotencyKey: "rollback-0001"}); err == nil {
		t.Fatal("forced transaction failure unexpectedly succeeded")
	}
	if count, _ := database.StockPostings().CountDocuments(ctx, bson.M{"tenantId": one.tenant}); count != 2 {
		t.Fatalf("rollback left %d postings", count)
	}
}

func TestFEFOAllocationCannotCommitAfterConcurrentQuarantine(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := database.Items().UpdateOne(ctx, bson.M{"_id": fixture.item, "tenantId": fixture.tenant}, bson.M{"$set": bson.M{"lotTracking": "required"}}); err != nil {
		t.Fatal(err)
	}
	service := inventory.NewService(database)
	lot, err := service.CreateLot(ctx, inventory.LotCommand{TenantID: fixture.tenant, UserID: fixture.user, ItemID: fixture.item, Code: "FEFO-RACE", ReceivedAt: now, Status: "available"})
	if err != nil {
		t.Fatalf("create lot: %v", err)
	}
	lotID := lot.ID
	if _, err := service.OpeningBalance(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, LotID: &lotID, QuantityMicros: 1_000_000, IdempotencyKey: "fefo-race-open"}); err != nil {
		t.Fatalf("opening balance: %v", err)
	}

	selected := make(chan struct{})
	quarantineResult := make(chan error, 1)
	var once sync.Once
	service.TestAfterLotAllocationHook = func(hookContext context.Context) error {
		var area struct {
			InventoryFence int64 `bson:"inventoryFence"`
		}
		if err := database.StorageAreas().FindOne(hookContext, bson.M{"_id": fixture.area, "tenantId": fixture.tenant}).Decode(&area); err != nil {
			return err
		}
		if area.InventoryFence != 2 {
			return errors.New("inventory fence was not acquired before lot guard")
		}
		once.Do(func() {
			close(selected)
			<-quarantineResult
		})
		return nil
	}
	go func() {
		<-selected
		_, statusErr := service.UpdateLotStatus(ctx, fixture.tenant, fixture.item, lotID, "quarantined")
		quarantineResult <- statusErr
	}()

	if _, err := service.Waste(ctx, inventory.WasteCommand{Command: inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 1_000_000, IdempotencyKey: "fefo-race-waste"}}); err == nil {
		t.Fatal("allocation committed after lot quarantine")
	}
	var quarantined struct {
		Status string `bson:"status"`
	}
	if err := database.StockLots().FindOne(ctx, bson.M{"_id": lotID, "tenantId": fixture.tenant}).Decode(&quarantined); err != nil {
		t.Fatalf("load lot status: %v", err)
	}
	if quarantined.Status != "quarantined" {
		t.Fatalf("lot status = %q, want quarantined", quarantined.Status)
	}
	if count, err := database.StockPostings().CountDocuments(ctx, bson.M{"tenantId": fixture.tenant, "idempotencyKey": "fefo-race-waste"}); err != nil || count != 0 {
		t.Fatalf("post-quarantine allocation postings = %d, %v", count, err)
	}
}

func TestFEFOGuardAdvancesLotState(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := database.Items().UpdateOne(ctx, bson.M{"_id": fixture.item, "tenantId": fixture.tenant}, bson.M{"$set": bson.M{"lotTracking": "required"}}); err != nil {
		t.Fatal(err)
	}
	service := inventory.NewService(database)
	lot, err := service.CreateLot(ctx, inventory.LotCommand{TenantID: fixture.tenant, UserID: fixture.user, ItemID: fixture.item, Code: "FEFO-GUARD", ReceivedAt: now, Status: "available"})
	if err != nil {
		t.Fatalf("create lot: %v", err)
	}
	lotID := lot.ID
	if _, err := service.OpeningBalance(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, LotID: &lotID, QuantityMicros: 1_000_000, IdempotencyKey: "fefo-guard-open"}); err != nil {
		t.Fatalf("opening balance: %v", err)
	}
	var before struct {
		UpdatedAt time.Time `bson:"updatedAt"`
	}
	if err := database.StockLots().FindOne(ctx, bson.M{"_id": lotID, "tenantId": fixture.tenant}).Decode(&before); err != nil {
		t.Fatalf("load lot before allocation: %v", err)
	}
	staleAt := time.Now().UTC().Add(-90 * 24 * time.Hour)
	if _, err := database.StockLots().UpdateOne(ctx, bson.M{"_id": lotID, "tenantId": fixture.tenant}, bson.M{"$set": bson.M{"updatedAt": staleAt}}); err != nil {
		t.Fatalf("make lot timestamp stale: %v", err)
	}
	before.UpdatedAt = staleAt
	startedAt := time.Now().UTC()
	if _, err := service.Waste(ctx, inventory.WasteCommand{Command: inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 1_000_000, IdempotencyKey: "fefo-guard-waste"}}); err != nil {
		t.Fatalf("waste: %v", err)
	}
	finishedAt := time.Now().UTC()
	var after struct {
		UpdatedAt time.Time `bson:"updatedAt"`
	}
	if err := database.StockLots().FindOne(ctx, bson.M{"_id": lotID, "tenantId": fixture.tenant}).Decode(&after); err != nil {
		t.Fatalf("load lot after allocation: %v", err)
	}
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("lot guard state did not advance: before=%s after=%s", before.UpdatedAt, after.UpdatedAt)
	}
	if after.UpdatedAt.Before(startedAt.Add(-5*time.Second)) || after.UpdatedAt.After(finishedAt.Add(5*time.Second)) {
		t.Fatalf("lot guard timestamp is not near operation time: started=%s finished=%s after=%s", startedAt, finishedAt, after.UpdatedAt)
	}
}

func TestTransferJournalUsesDestinationMovementLocation(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	destinationLocation, destinationArea := primitive.NewObjectID(), primitive.NewObjectID()
	now := time.Now().UTC()
	if _, err := database.Locations().InsertOne(context.Background(), bson.M{"_id": destinationLocation, "tenantId": fixture.tenant, "code": "inventory-destination-" + destinationLocation.Hex()[:8], "name": "Destination", "timezone": "UTC", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now, "limitSlot": int64(2)}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.StorageAreas().InsertOne(context.Background(), bson.M{"_id": destinationArea, "tenantId": fixture.tenant, "locationId": destinationLocation, "name": "Destination area", "type": "dry", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}); err != nil {
		t.Fatal(err)
	}
	service := inventory.NewService(database)
	if _, err := service.OpeningBalance(context.Background(), inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 1_000_000, IdempotencyKey: "journal-transfer-open"}); err != nil {
		t.Fatalf("opening balance: %v", err)
	}
	transfer, err := service.Transfer(context.Background(), inventory.TransferCommand{TenantID: fixture.tenant, UserID: fixture.user, SourceLocationID: fixture.location, SourceStorageAreaID: fixture.area, DestinationLocationID: destinationLocation, DestinationStorageAreaID: destinationArea, ItemID: fixture.item, QuantityMicros: 1_000_000, IdempotencyKey: "journal-transfer-post"})
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	entries, err := service.ListJournal(context.Background(), fixture.tenant, destinationLocation, nil)
	if err != nil {
		t.Fatalf("destination journal: %v", err)
	}
	if len(entries) != 1 || entries[0].Posting.ID != transfer.Posting.ID || entries[0].Movement.LocationID != destinationLocation || entries[0].Movement.QuantityMicros != 1_000_000 {
		t.Fatalf("destination journal = %#v", entries)
	}
	if entries[0].Posting.LocationID != fixture.location {
		t.Fatalf("transfer posting location = %s, want source %s", entries[0].Posting.LocationID.Hex(), fixture.location.Hex())
	}
}

func TestLockedSourceAreaRejectsOrdinaryPostingAndTransfer(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	now := time.Now().UTC()
	destinationLocation, destinationArea := primitive.NewObjectID(), primitive.NewObjectID()
	if _, err := database.Locations().InsertOne(ctx, bson.M{"_id": destinationLocation, "tenantId": fixture.tenant, "code": "inventory-locked-destination-" + destinationLocation.Hex()[:8], "name": "Destination", "timezone": "UTC", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now, "limitSlot": int64(2)}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.StorageAreas().InsertOne(ctx, bson.M{"_id": destinationArea, "tenantId": fixture.tenant, "locationId": destinationLocation, "name": "Destination area", "type": "dry", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}); err != nil {
		t.Fatal(err)
	}
	service := inventory.NewService(database)
	if _, err := service.OpeningBalance(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 1_000_000, IdempotencyKey: "locked-source-open"}); err != nil {
		t.Fatalf("opening balance: %v", err)
	}
	countID := primitive.NewObjectID()
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": fixture.area, "tenantId": fixture.tenant}, bson.M{"$set": bson.M{"activeStockCountId": countID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ManualAdjustment(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 1, IdempotencyKey: "locked-source-post"}); !errors.Is(err, inventory.ErrInventoryAreaLocked) {
		t.Fatalf("locked ordinary posting error = %v", err)
	}
	if _, err := service.Transfer(ctx, inventory.TransferCommand{TenantID: fixture.tenant, UserID: fixture.user, SourceLocationID: fixture.location, SourceStorageAreaID: fixture.area, DestinationLocationID: destinationLocation, DestinationStorageAreaID: destinationArea, ItemID: fixture.item, QuantityMicros: 1, IdempotencyKey: "locked-source-transfer"}); !errors.Is(err, inventory.ErrInventoryAreaLocked) {
		t.Fatalf("locked source transfer error = %v", err)
	}
}

func TestDeactivatedCountOwnedAreaRemainsLockedForReversal(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	service := inventory.NewService(database)
	opening, err := service.OpeningBalance(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 1_000_000, IdempotencyKey: "deactivated-count-open"})
	if err != nil {
		t.Fatalf("opening balance: %v", err)
	}
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": fixture.area, "tenantId": fixture.tenant}, bson.M{"$set": bson.M{"isActive": false, "activeStockCountId": primitive.NewObjectID()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reverse(ctx, inventory.ReverseCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, PostingID: opening.Posting.ID, IdempotencyKey: "deactivated-count-reverse"}); !errors.Is(err, inventory.ErrInventoryAreaLocked) {
		t.Fatalf("deactivated count-owned reversal error = %v", err)
	}
	if count, err := database.StockPostings().CountDocuments(ctx, bson.M{"tenantId": fixture.tenant}); err != nil || count != 1 {
		t.Fatalf("deactivated count-owned reversal postings = %d, %v", count, err)
	}
}

func TestMultiAreaTransferAndReversalFenceAllMovementAreas(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	now := time.Now().UTC()
	destinationLocation, destinationArea := primitive.NewObjectID(), primitive.NewObjectID()
	if _, err := database.Locations().InsertOne(ctx, bson.M{"_id": destinationLocation, "tenantId": fixture.tenant, "code": "inventory-fence-destination-" + destinationLocation.Hex()[:8], "name": "Destination", "timezone": "UTC", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now, "limitSlot": int64(2)}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.StorageAreas().InsertOne(ctx, bson.M{"_id": destinationArea, "tenantId": fixture.tenant, "locationId": destinationLocation, "name": "Destination area", "type": "dry", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}); err != nil {
		t.Fatal(err)
	}
	service := inventory.NewService(database)
	if _, err := service.OpeningBalance(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 1_000_000, IdempotencyKey: "fence-transfer-open"}); err != nil {
		t.Fatalf("opening balance: %v", err)
	}
	transfer, err := service.Transfer(ctx, inventory.TransferCommand{TenantID: fixture.tenant, UserID: fixture.user, SourceLocationID: fixture.location, SourceStorageAreaID: fixture.area, DestinationLocationID: destinationLocation, DestinationStorageAreaID: destinationArea, ItemID: fixture.item, QuantityMicros: 1, IdempotencyKey: "fence-transfer-post"})
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	assertInventoryFence(t, database, fixture.tenant, fixture.location, fixture.area, 2)
	assertInventoryFence(t, database, fixture.tenant, destinationLocation, destinationArea, 1)
	if _, err := service.Reverse(ctx, inventory.ReverseCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, PostingID: transfer.Posting.ID, IdempotencyKey: "fence-transfer-reverse"}); err != nil {
		t.Fatalf("reverse transfer: %v", err)
	}
	assertInventoryFence(t, database, fixture.tenant, fixture.location, fixture.area, 3)
	assertInventoryFence(t, database, fixture.tenant, destinationLocation, destinationArea, 2)
}

func assertInventoryFence(t *testing.T, database *db.MongoDB, tenant, location, area primitive.ObjectID, want int64) {
	t.Helper()
	var storageArea struct {
		InventoryFence int64 `bson:"inventoryFence"`
	}
	if err := database.StorageAreas().FindOne(context.Background(), bson.M{"_id": area, "tenantId": tenant, "locationId": location}).Decode(&storageArea); err != nil {
		t.Fatal(err)
	}
	if storageArea.InventoryFence != want {
		t.Fatalf("inventory fence for %s/%s = %d, want %d", location.Hex(), area.Hex(), storageArea.InventoryFence, want)
	}
}

func TestStockCountLifecycleLocksAndPostsZeroDifferenceReceipt(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	service := inventory.NewService(database)
	opening, err := service.OpeningBalance(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 2_000_000, IdempotencyKey: "count-lifecycle-open"})
	if err != nil {
		t.Fatalf("opening balance: %v", err)
	}
	if _, err := database.StockMovements().UpdateOne(ctx, bson.M{"_id": opening.Movement.ID, "tenantId": fixture.tenant}, bson.M{"$set": bson.M{"recordedAt": time.Now().UTC().Add(24 * time.Hour)}}); err != nil {
		t.Fatalf("move journal timestamp: %v", err)
	}
	count, err := service.CreateCount(ctx, inventory.CountCreateCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, IdempotencyKey: "count-lifecycle-create"})
	if err != nil {
		t.Fatalf("create count: %v", err)
	}
	frozen, err := service.FreezeCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, Version: 1})
	if err != nil || len(frozen.Lines) != 1 || frozen.Lines[0].SnapshotQuantityMicros != 2_000_000 {
		t.Fatalf("freeze = %#v, %v", frozen, err)
	}
	assertInventoryFence(t, database, fixture.tenant, fixture.location, fixture.area, 2)
	if _, err := service.ManualAdjustment(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 1, IdempotencyKey: "count-lifecycle-blocked"}); !errors.Is(err, inventory.ErrInventoryAreaLocked) {
		t.Fatalf("normal writer during count = %v", err)
	}
	line := frozen.Lines[0]
	if _, err := service.RecordCountLine(ctx, inventory.CountLineCommand{CountVersionCommand: inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, Version: 2}, LineID: line.ID, LineVersion: 1, CountedQuantityMicros: 2_000_000}); err != nil {
		t.Fatalf("record count line: %v", err)
	}
	if _, err := service.ReviewCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, Version: 3}); err != nil {
		t.Fatalf("review count: %v", err)
	}
	posted, err := service.PostCount(ctx, inventory.CountPostCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, IdempotencyKey: "count-lifecycle-post"})
	if err != nil || posted.Posting == nil || posted.Posting.Type != models.StockPostingCount || len(posted.Movements) != 0 {
		t.Fatalf("zero-difference post = %#v, %v", posted, err)
	}
	if _, err := service.PostCount(ctx, inventory.CountPostCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, IdempotencyKey: "count-lifecycle-other"}); !errors.Is(err, inventory.ErrIdempotencyConflict) {
		t.Fatalf("mismatched posted key = %v", err)
	}
	replay, err := service.PostCount(ctx, inventory.CountPostCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, IdempotencyKey: "count-lifecycle-post"})
	if err != nil || replay.Posting == nil || replay.Posting.ID != posted.Posting.ID || len(replay.Movements) != 0 {
		t.Fatalf("zero-difference replay = %#v, %v", replay, err)
	}
	assertInventoryFence(t, database, fixture.tenant, fixture.location, fixture.area, 2)
	var owner struct {
		ActiveStockCountID *primitive.ObjectID `bson:"activeStockCountId"`
	}
	if err := database.StorageAreas().FindOne(ctx, bson.M{"_id": fixture.area, "tenantId": fixture.tenant}).Decode(&owner); err != nil || owner.ActiveStockCountID != nil {
		t.Fatalf("posted count owner = %v, %v", owner.ActiveStockCountID, err)
	}
	if opening.Posting.ID.IsZero() {
		t.Fatal("opening posting was not created")
	}
}

func TestLegacyCountWithoutOwnershipLockMustBeCancelled(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	service := inventory.NewService(database)

	frozen, err := service.CreateCount(ctx, inventory.CountCreateCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, IdempotencyKey: "legacy-frozen-create"})
	if err != nil {
		t.Fatalf("create frozen legacy count: %v", err)
	}
	if _, err := service.FreezeCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: frozen.Count.ID, Version: 1}); err != nil {
		t.Fatalf("freeze legacy count: %v", err)
	}
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": fixture.area, "tenantId": fixture.tenant}, bson.M{"$unset": bson.M{"activeStockCountId": ""}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetCount(ctx, fixture.tenant, frozen.Count.ID, fixture.location); !errors.Is(err, inventory.ErrCountOwnershipRequired) {
		t.Fatalf("legacy frozen read error = %v", err)
	}
	if _, err := service.RecordCountLine(ctx, inventory.CountLineCommand{CountVersionCommand: inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: frozen.Count.ID, Version: 2}, LineID: primitive.NewObjectID(), LineVersion: 1, CountedQuantityMicros: 1}); !errors.Is(err, inventory.ErrCountOwnershipRequired) {
		t.Fatalf("legacy frozen record error = %v", err)
	}
	cancelled, err := service.CancelCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: frozen.Count.ID, Version: 2})
	if err != nil || cancelled.Count.Status != models.StockCountCancelled {
		t.Fatalf("cancel legacy frozen count = %#v, %v", cancelled, err)
	}

	reviewed, err := service.CreateCount(ctx, inventory.CountCreateCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, IdempotencyKey: "legacy-reviewed-create"})
	if err != nil {
		t.Fatalf("create reviewed legacy count: %v", err)
	}
	if _, err := service.FreezeCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: reviewed.Count.ID, Version: 1}); err != nil {
		t.Fatalf("freeze reviewed legacy count: %v", err)
	}
	if _, err := service.ReviewCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: reviewed.Count.ID, Version: 2}); err != nil {
		t.Fatalf("review count before legacy quarantine: %v", err)
	}
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": fixture.area, "tenantId": fixture.tenant}, bson.M{"$unset": bson.M{"activeStockCountId": ""}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetCount(ctx, fixture.tenant, reviewed.Count.ID, fixture.location); !errors.Is(err, inventory.ErrCountOwnershipRequired) {
		t.Fatalf("legacy reviewed read error = %v", err)
	}
	if _, err := service.PostCount(ctx, inventory.CountPostCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: reviewed.Count.ID, IdempotencyKey: "legacy-reviewed-post"}); !errors.Is(err, inventory.ErrCountOwnershipRequired) {
		t.Fatalf("legacy reviewed post error = %v", err)
	}
	if _, err := service.CancelCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: reviewed.Count.ID, Version: 3}); err != nil {
		t.Fatalf("cancel legacy reviewed count: %v", err)
	}

	draft, err := service.CreateCount(ctx, inventory.CountCreateCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, IdempotencyKey: "legacy-owner-draft"})
	if err != nil {
		t.Fatalf("create owner-preservation draft: %v", err)
	}
	otherOwner := primitive.NewObjectID()
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": fixture.area, "tenantId": fixture.tenant}, bson.M{"$set": bson.M{"activeStockCountId": otherOwner}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CancelCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: draft.Count.ID, Version: 1}); err != nil {
		t.Fatalf("cancel draft with other owner: %v", err)
	}
	var owner struct {
		ActiveStockCountID *primitive.ObjectID `bson:"activeStockCountId"`
	}
	if err := database.StorageAreas().FindOne(ctx, bson.M{"_id": fixture.area, "tenantId": fixture.tenant}).Decode(&owner); err != nil || owner.ActiveStockCountID == nil || *owner.ActiveStockCountID != otherOwner {
		t.Fatalf("different owner was cleared: %v, %v", owner.ActiveStockCountID, err)
	}
}

func TestStockCountDiscoveredLineAndCancelReleaseOwnership(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	now := time.Now().UTC()
	discoveredItem := primitive.NewObjectID()
	if _, err := database.Items().InsertOne(ctx, bson.M{"_id": discoveredItem, "tenantId": fixture.tenant, "sku": "discovered-" + discoveredItem.Hex()[:8], "name": "Discovered", "categoryId": primitive.NewObjectID(), "baseUnitId": primitive.NewObjectID(), "stockable": true, "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}); err != nil {
		t.Fatal(err)
	}
	service := inventory.NewService(database)
	count, err := service.CreateCount(ctx, inventory.CountCreateCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, IdempotencyKey: "discover-count-create"})
	if err != nil {
		t.Fatalf("create count: %v", err)
	}
	frozen, err := service.FreezeCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, Version: 1})
	if err != nil {
		t.Fatalf("freeze count: %v", err)
	}
	discovered, err := service.DiscoverCountLine(ctx, inventory.CountDiscoverLineCommand{CountVersionCommand: inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, Version: 2}, ItemID: discoveredItem})
	if err != nil || discovered.SnapshotQuantityMicros != 0 || discovered.ItemID != discoveredItem {
		t.Fatalf("discovered line = %#v, %v", discovered, err)
	}
	if _, err := service.DiscoverCountLine(ctx, inventory.CountDiscoverLineCommand{CountVersionCommand: inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, Version: 3}, ItemID: discoveredItem}); !errors.Is(err, inventory.ErrCountLineExists) {
		t.Fatalf("duplicate discovered line = %v", err)
	}
	cancelled, err := service.CancelCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, Version: 3})
	if err != nil || cancelled.Count.Status != models.StockCountCancelled {
		t.Fatalf("cancelled count = %#v, %v", cancelled, err)
	}
	var owner struct {
		ActiveStockCountID *primitive.ObjectID `bson:"activeStockCountId"`
	}
	if err := database.StorageAreas().FindOne(ctx, bson.M{"_id": fixture.area, "tenantId": fixture.tenant}).Decode(&owner); err != nil || owner.ActiveStockCountID != nil {
		t.Fatalf("cancelled count owner = %v, %v", owner.ActiveStockCountID, err)
	}
	if _, err := service.CancelCount(ctx, inventory.CountVersionCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: count.Count.ID, Version: 4}); !errors.Is(err, inventory.ErrCountState) {
		t.Fatalf("cancelled count repeat = %v", err)
	}
	if len(frozen.Lines) != 0 {
		t.Fatalf("unexpected frozen lines: %d", len(frozen.Lines))
	}
}

func TestPostCountRejectsPreExistingTenantWidePostingKeyReuse(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	now := time.Now().UTC()
	countID := primitive.NewObjectID()
	postingID := primitive.NewObjectID()
	key := "preexisting-count-key"
	count := models.StockCount{ID: countID, TenantID: fixture.tenant, LocationID: fixture.location, StorageAreaID: fixture.area, CreatedBy: fixture.user, Status: models.StockCountReviewed, Version: 1, IdempotencyKey: "preexisting-count-create", RequestHash: strings.Repeat("a", 64), CreatedAt: now, UpdatedAt: now}
	if _, err := database.StockCounts().InsertOne(ctx, count); err != nil {
		t.Fatalf("seed reviewed count: %v", err)
	}
	posting := models.StockPosting{ID: postingID, TenantID: fixture.tenant, LocationID: fixture.location, StorageAreaID: fixture.area, UserID: fixture.user, Type: models.StockPostingAdjustment, IdempotencyKey: key, RequestHash: strings.Repeat("b", 64), EffectiveAt: now, RecordedAt: now}
	if _, err := database.StockPostings().InsertOne(ctx, posting); err != nil {
		t.Fatalf("seed conflicting posting: %v", err)
	}
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": fixture.area, "tenantId": fixture.tenant}, bson.M{"$set": bson.M{"activeStockCountId": countID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.NewService(database).PostCount(ctx, inventory.CountPostCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: countID, IdempotencyKey: key}); !errors.Is(err, inventory.ErrIdempotencyConflict) {
		t.Fatalf("pre-existing tenant-wide key reuse error = %v", err)
	}
}

func TestActiveCountRecoveryListAndScopedLotEnrichment(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	ctx := context.Background()
	tenant, otherTenant := primitive.NewObjectID(), primitive.NewObjectID()
	fixture := seedInventory(t, database, tenant)
	other := seedInventory(t, database, otherTenant)
	now := time.Now().UTC()

	areaOwnerMissing, areaOwnerMismatch := primitive.NewObjectID(), primitive.NewObjectID()
	for _, areaID := range []primitive.ObjectID{areaOwnerMissing, areaOwnerMismatch} {
		if _, err := database.StorageAreas().InsertOne(ctx, bson.M{"_id": areaID, "tenantId": tenant, "locationId": fixture.location, "name": areaID.Hex(), "type": "dry", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}); err != nil {
			t.Fatal(err)
		}
	}
	missingArea := primitive.NewObjectID()
	ownerMismatch := primitive.NewObjectID()
	makeCount := func(id, area primitive.ObjectID, status models.StockCountStatus, createdAt time.Time) models.StockCount {
		return models.StockCount{ID: id, TenantID: tenant, LocationID: fixture.location, StorageAreaID: area, CreatedBy: fixture.user, Status: status, Version: 1, IdempotencyKey: "recovery-" + id.Hex(), RequestHash: strings.Repeat("a", 64), CreatedAt: createdAt, UpdatedAt: createdAt}
	}
	counts := []models.StockCount{
		makeCount(primitive.NewObjectID(), missingArea, models.StockCountDraft, now.Add(4*time.Minute)),
		makeCount(primitive.NewObjectID(), areaOwnerMismatch, models.StockCountFrozen, now.Add(3*time.Minute)),
		makeCount(primitive.NewObjectID(), areaOwnerMissing, models.StockCountReviewed, now.Add(2*time.Minute)),
		makeCount(primitive.NewObjectID(), fixture.area, models.StockCountDraft, now.Add(time.Minute)),
	}
	for _, count := range counts {
		if _, err := database.StockCounts().InsertOne(ctx, count); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": fixture.area, "tenantId": tenant}, bson.M{"$set": bson.M{"activeStockCountId": counts[3].ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": areaOwnerMismatch, "tenantId": tenant}, bson.M{"$set": bson.M{"activeStockCountId": ownerMismatch}}); err != nil {
		t.Fatal(err)
	}
	otherCount := makeCount(primitive.NewObjectID(), other.area, models.StockCountDraft, now.Add(5*time.Minute))
	otherCount.TenantID, otherCount.LocationID = otherTenant, other.location
	if _, err := database.StockCounts().InsertOne(ctx, otherCount); err != nil {
		t.Fatal(err)
	}

	service := inventory.NewService(database)
	first, err := service.ListActiveCounts(ctx, inventory.ActiveCountListCommand{TenantID: tenant, LocationID: fixture.location, Limit: 2})
	if err != nil || len(first.Counts) != 2 || first.Counts[0].Count.ID != counts[0].ID || first.Counts[1].Count.ID != counts[1].ID || first.NextCursor == "" {
		t.Fatalf("first recovery page = %#v, %v", first, err)
	}
	second, err := service.ListActiveCounts(ctx, inventory.ActiveCountListCommand{TenantID: tenant, LocationID: fixture.location, Cursor: first.NextCursor, Limit: 2})
	if err != nil || len(second.Counts) != 2 || second.Counts[0].Integrity != inventory.ActiveCountOwnerMissing || second.Counts[1].Integrity != inventory.ActiveCountHealthy {
		t.Fatalf("second recovery page = %#v, %v", second, err)
	}
	if second.Counts[0].Count.ID != counts[2].ID || second.Counts[1].Count.ID != counts[3].ID || second.NextCursor != "" {
		t.Fatalf("recovery keyset order = %#v", second)
	}
	if first.Counts[0].Integrity != inventory.ActiveCountHealthy || first.Counts[1].Integrity != inventory.ActiveCountOwnerMismatch {
		t.Fatalf("draft/frozen integrity classifications = %#v", first.Counts)
	}
	if isolated, err := service.ListActiveCounts(ctx, inventory.ActiveCountListCommand{TenantID: tenant, LocationID: other.location}); err != nil || len(isolated.Counts) != 0 {
		t.Fatalf("cross-tenant recovery list = %#v, %v", isolated, err)
	}

	lotID := primitive.NewObjectID()
	expires := now.Add(24 * time.Hour)
	if _, err := database.StockLots().InsertOne(ctx, bson.M{"_id": lotID, "tenantId": tenant, "itemId": fixture.item, "code": "RECOVERY-LOT", "status": "quarantined", "expiresAt": expires, "receivedAt": now, "createdAt": now, "updatedAt": now}); err != nil {
		t.Fatal(err)
	}
	lineID := primitive.NewObjectID()
	countID := primitive.NewObjectID()
	frozen := models.StockCount{ID: countID, TenantID: tenant, LocationID: fixture.location, StorageAreaID: fixture.area, CreatedBy: fixture.user, Status: models.StockCountFrozen, Version: 1, IdempotencyKey: "recovery-frozen", RequestHash: strings.Repeat("b", 64), CreatedAt: now, UpdatedAt: now}
	if _, err := database.StockCounts().InsertOne(ctx, frozen); err != nil {
		t.Fatal(err)
	}
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": fixture.area, "tenantId": tenant}, bson.M{"$set": bson.M{"activeStockCountId": countID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.StockCountLines().InsertOne(ctx, models.StockCountLine{ID: lineID, CountID: countID, TenantID: tenant, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, LotID: &lotID, SnapshotQuantityMicros: 1, Version: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	lots, err := service.ListCountLotOptions(ctx, inventory.CountLotOptionsCommand{TenantID: tenant, LocationID: fixture.location, CountID: countID, ItemID: fixture.item})
	if err != nil || len(lots.Lots) != 1 || lots.Lots[0].Code != "RECOVERY-LOT" || lots.Lots[0].Status != models.LotStatusQuarantined || lots.Lots[0].ExpiresAt == nil {
		t.Fatalf("scoped lot options = %#v, %v", lots, err)
	}
	result, err := service.GetCount(ctx, tenant, countID, fixture.location)
	if err != nil || len(result.Lines) != 1 || result.Lines[0].LotCode != "RECOVERY-LOT" || result.Lines[0].LotStatus != models.LotStatusQuarantined || result.Lines[0].LotExpiresAt == nil {
		t.Fatalf("enriched count result = %#v, %v", result, err)
	}
}

func TestPostCountRejectsEmptyPersistedPostHashReplay(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	now := time.Now().UTC()
	countID, postingID := primitive.NewObjectID(), primitive.NewObjectID()
	key := "empty-post-hash-key"
	posting := models.StockPosting{ID: postingID, TenantID: fixture.tenant, LocationID: fixture.location, StorageAreaID: fixture.area, UserID: fixture.user, Type: models.StockPostingCount, IdempotencyKey: key, RequestHash: strings.Repeat("a", 64), EffectiveAt: now, RecordedAt: now, Reason: "stock count " + countID.Hex()}
	if _, err := database.StockPostings().InsertOne(ctx, posting); err != nil {
		t.Fatalf("seed posted receipt: %v", err)
	}
	count := models.StockCount{ID: countID, TenantID: fixture.tenant, LocationID: fixture.location, StorageAreaID: fixture.area, CreatedBy: fixture.user, Status: models.StockCountPosted, Version: 2, IdempotencyKey: "empty-post-hash-create", RequestHash: strings.Repeat("b", 64), PostIdempotencyKey: key, PostPostingID: &postingID, CreatedAt: now, UpdatedAt: now, PostedAt: &now}
	if _, err := database.StockCounts().InsertOne(ctx, count); err != nil {
		t.Fatalf("seed malformed posted count: %v", err)
	}
	if _, err := inventory.NewService(database).PostCount(ctx, inventory.CountPostCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, CountID: countID, IdempotencyKey: key}); !errors.Is(err, inventory.ErrIdempotencyConflict) {
		t.Fatalf("empty persisted post hash replay error = %v", err)
	}
}

func TestReconciliationRepairDoesNotMutateJournal(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	service := inventory.NewService(database)
	if _, err := service.OpeningBalance(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 3_000_000, IdempotencyKey: "reconcile-open"}); err != nil {
		t.Fatal(err)
	}
	journalBefore, err := database.StockMovements().CountDocuments(ctx, bson.M{"tenantId": fixture.tenant})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.StockBalances().UpdateOne(ctx, bson.M{"tenantId": fixture.tenant, "locationId": fixture.location, "storageAreaId": fixture.area, "itemId": fixture.item, "lotId": nil}, bson.M{"$set": bson.M{"quantityMicros": int64(0)}}); err != nil {
		t.Fatal(err)
	}
	detected, err := service.RunReconciliation(ctx, inventory.ReconciliationCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, IdempotencyKey: "reconcile-detect", Repair: false})
	if err != nil || detected.Run.Status != "detected" || detected.Run.MismatchCount != 1 {
		t.Fatalf("detect result = %#v, %v", detected, err)
	}
	repaired, err := service.RunReconciliation(ctx, inventory.ReconciliationCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, IdempotencyKey: "reconcile-repair", Repair: true})
	if err != nil || repaired.Run.Status != "repaired" || repaired.Run.MismatchCount != 1 {
		t.Fatalf("repair result = %#v, %v", repaired, err)
	}
	journalAfter, err := database.StockMovements().CountDocuments(ctx, bson.M{"tenantId": fixture.tenant})
	if err != nil || journalAfter != journalBefore {
		t.Fatalf("repair mutated journal: before=%d after=%d err=%v", journalBefore, journalAfter, err)
	}
	var balance struct {
		QuantityMicros int64 `bson:"quantityMicros"`
	}
	if err := database.StockBalances().FindOne(ctx, bson.M{"tenantId": fixture.tenant, "locationId": fixture.location, "storageAreaId": fixture.area, "itemId": fixture.item, "lotId": nil}).Decode(&balance); err != nil || balance.QuantityMicros != 3_000_000 {
		t.Fatalf("repaired balance = %d, %v", balance.QuantityMicros, err)
	}
}

func TestReconciliationRepairRejectsLockedAreaWithoutBalanceMutation(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	now := time.Now().UTC()
	secondArea := primitive.NewObjectID()
	if _, err := database.StorageAreas().InsertOne(ctx, bson.M{"_id": secondArea, "tenantId": fixture.tenant, "locationId": fixture.location, "name": "Secondary", "type": "dry", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}); err != nil {
		t.Fatal(err)
	}
	service := inventory.NewService(database)
	if _, err := service.OpeningBalance(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 3_000_000, IdempotencyKey: "reconcile-lock-open-one"}); err != nil {
		t.Fatalf("first opening balance: %v", err)
	}
	if _, err := service.OpeningBalance(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: secondArea, ItemID: fixture.item, QuantityMicros: 2_000_000, IdempotencyKey: "reconcile-lock-open-two"}); err != nil {
		t.Fatalf("second opening balance: %v", err)
	}
	for _, area := range []primitive.ObjectID{fixture.area, secondArea} {
		if _, err := database.StockBalances().UpdateOne(ctx, bson.M{"tenantId": fixture.tenant, "locationId": fixture.location, "storageAreaId": area, "itemId": fixture.item, "lotId": nil}, bson.M{"$set": bson.M{"quantityMicros": int64(0)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.StorageAreas().UpdateOne(ctx, bson.M{"_id": secondArea, "tenantId": fixture.tenant}, bson.M{"$set": bson.M{"activeStockCountId": primitive.NewObjectID()}}); err != nil {
		t.Fatal(err)
	}
	beforeRuns, err := database.ReconciliationRuns().CountDocuments(ctx, bson.M{"tenantId": fixture.tenant})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunReconciliation(ctx, inventory.ReconciliationCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, IdempotencyKey: "reconcile-lock-repair", Repair: true}); !errors.Is(err, inventory.ErrInventoryAreaLocked) {
		t.Fatalf("locked reconciliation repair error = %v", err)
	}
	for _, area := range []primitive.ObjectID{fixture.area, secondArea} {
		var balance struct {
			QuantityMicros int64 `bson:"quantityMicros"`
		}
		if err := database.StockBalances().FindOne(ctx, bson.M{"tenantId": fixture.tenant, "locationId": fixture.location, "storageAreaId": area, "itemId": fixture.item, "lotId": nil}).Decode(&balance); err != nil {
			t.Fatal(err)
		}
		if balance.QuantityMicros != 0 {
			t.Fatalf("locked repair changed balance in area %s: %d", area.Hex(), balance.QuantityMicros)
		}
	}
	if afterRuns, _ := database.ReconciliationRuns().CountDocuments(ctx, bson.M{"tenantId": fixture.tenant}); afterRuns != beforeRuns {
		t.Fatalf("locked repair wrote %d reconciliation runs, want %d", afterRuns, beforeRuns)
	}
	assertInventoryFence(t, database, fixture.tenant, fixture.location, fixture.area, 1)
	assertInventoryFence(t, database, fixture.tenant, fixture.location, secondArea, 1)

	// Detection still reads the locked scope without taking an inventory fence.
	detected, err := service.RunReconciliation(ctx, inventory.ReconciliationCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, IdempotencyKey: "reconcile-lock-detect", Repair: false})
	if err != nil || detected.Run.MismatchCount != 2 {
		t.Fatalf("locked detection = %#v, %v", detected, err)
	}
	assertInventoryFence(t, database, fixture.tenant, fixture.location, fixture.area, 1)
	assertInventoryFence(t, database, fixture.tenant, fixture.location, secondArea, 1)
}

func TestReconciliationRepairPreservesPostCutoffMovements(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	service := inventory.NewService(database)
	if _, err := service.OpeningBalance(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 3_000_000, IdempotencyKey: "reconcile-cutoff-open"}); err != nil {
		t.Fatal(err)
	}
	later, err := service.ManualAdjustment(ctx, inventory.Command{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, StorageAreaID: fixture.area, ItemID: fixture.item, QuantityMicros: 2_000_000, IdempotencyKey: "reconcile-cutoff-later"})
	if err != nil {
		t.Fatal(err)
	}
	// Make the already-posted movement fall after the reconciliation cutoff
	// without changing the balance it established. This deterministically
	// exercises the same cutoff/retry boundary as a concurrent posting.
	future := time.Now().UTC().Add(time.Hour)
	if _, err := database.StockMovements().UpdateOne(ctx, bson.M{"_id": later.Movement.ID, "tenantId": fixture.tenant}, bson.M{"$set": bson.M{"recordedAt": future}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.StockBalances().UpdateOne(ctx, bson.M{"tenantId": fixture.tenant, "locationId": fixture.location, "storageAreaId": fixture.area, "itemId": fixture.item, "lotId": nil}, bson.M{"$set": bson.M{"quantityMicros": int64(0)}}); err != nil {
		t.Fatal(err)
	}

	repaired, err := service.RunReconciliation(ctx, inventory.ReconciliationCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, IdempotencyKey: "reconcile-cutoff-repair", Repair: true})
	if err != nil || repaired.Run.MismatchCount != 1 {
		t.Fatalf("repair result = %#v, %v", repaired, err)
	}
	var balance struct {
		QuantityMicros int64 `bson:"quantityMicros"`
	}
	if err := database.StockBalances().FindOne(ctx, bson.M{"tenantId": fixture.tenant, "locationId": fixture.location, "storageAreaId": fixture.area, "itemId": fixture.item, "lotId": nil}).Decode(&balance); err != nil || balance.QuantityMicros != 5_000_000 {
		t.Fatalf("repaired balance discarded later movement: %d, %v", balance.QuantityMicros, err)
	}
}

func TestReconciliationIdempotencyRaceReturnsExistingRun(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	fixture := seedInventory(t, database, primitive.NewObjectID())
	ctx := context.Background()
	commands := inventory.ReconciliationCommand{TenantID: fixture.tenant, UserID: fixture.user, LocationID: fixture.location, IdempotencyKey: "reconcile-race-key", Repair: false}
	services := []*inventory.Service{inventory.NewService(database), inventory.NewService(database)}
	results := make(chan *inventory.ReconciliationResult, len(services))
	errorsCh := make(chan error, len(services))
	var group sync.WaitGroup
	for _, service := range services {
		group.Add(1)
		go func(service *inventory.Service) {
			defer group.Done()
			result, err := service.RunReconciliation(ctx, commands)
			results <- result
			errorsCh <- err
		}(service)
	}
	group.Wait()
	close(results)
	close(errorsCh)
	var firstID primitive.ObjectID
	for result := range results {
		if result == nil {
			t.Fatal("reconciliation race returned nil result")
		}
		if firstID.IsZero() {
			firstID = result.Run.ID
		} else if result.Run.ID != firstID {
			t.Fatalf("reconciliation race returned different runs: %s and %s", firstID.Hex(), result.Run.ID.Hex())
		}
	}
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("reconciliation race error: %v", err)
		}
	}
	if count, err := database.ReconciliationRuns().CountDocuments(ctx, bson.M{"tenantId": fixture.tenant, "idempotencyKey": commands.IdempotencyKey}); err != nil || count != 1 {
		t.Fatalf("reconciliation race wrote %d runs, %v", count, err)
	}
}
