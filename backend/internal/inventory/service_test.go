package inventory

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestParseQuantityUsesExactFixedPoint(t *testing.T) {
	for input, want := range map[string]int64{"1": 1_000_000, "1.25": 1_250_000, "-0.000001": -1, "+2.5": 2_500_000} {
		got, err := ParseQuantity(input)
		if err != nil || got != want {
			t.Fatalf("ParseQuantity(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"0.0000001", "1e3", "1.", ".5", "not-a-number"} {
		if _, err := ParseQuantity(input); err == nil {
			t.Errorf("ParseQuantity(%q) should reject non-canonical quantity", input)
		}
	}
	if got := FormatQuantity(-1_250_000); got != "-1.25" {
		t.Fatalf("FormatQuantity = %q", got)
	}
}

func TestInventoryModelsAreStrictlyValidated(t *testing.T) {
	now := time.Now().UTC()
	tenant, location, area, user, item := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	posting := models.StockPosting{ID: primitive.NewObjectID(), TenantID: tenant, LocationID: location, StorageAreaID: area, UserID: user, Type: models.StockPostingAdjustment, IdempotencyKey: "inventory-test-key", RequestHash: strings.Repeat("a", 64), EffectiveAt: now, RecordedAt: now}
	if err := validation.Validate(&posting); err != nil {
		t.Fatalf("valid posting rejected: %v", err)
	}
	movement := models.StockMovement{ID: primitive.NewObjectID(), PostingID: posting.ID, TenantID: tenant, LocationID: location, StorageAreaID: area, ItemID: item, QuantityMicros: -1, EffectiveAt: now, RecordedAt: now}
	if err := validation.Validate(&movement); err != nil {
		t.Fatalf("valid movement rejected: %v", err)
	}
	posting.Type = "unknown"
	if err := validation.Validate(&posting); err == nil {
		t.Fatal("unknown posting type was accepted")
	}
	posting.Type = models.StockPostingAdjustment
	posting.RequestHash = strings.Repeat("A", 64)
	if err := validation.Validate(&posting); err == nil {
		t.Fatal("non-canonical request hash was accepted")
	}
	movement.QuantityMicros = 0
	if err := validation.Validate(&movement); err == nil {
		t.Fatal("zero movement was accepted")
	}
	movement.QuantityMicros = math.MinInt64
	if err := validation.Validate(&movement); err == nil {
		t.Fatal("unnegatable movement quantity was accepted")
	}
}

func TestLotTrackingAndLotModelsValidateStrictValues(t *testing.T) {
	now := time.Now().UTC()
	item := models.Item{ID: primitive.NewObjectID(), TenantID: primitive.NewObjectID(), SKU: "lot-item", Name: "Lot item", CategoryID: primitive.NewObjectID(), BaseUnitID: primitive.NewObjectID(), Stockable: true, LotTracking: models.LotTrackingRequired, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&item); err != nil {
		t.Fatalf("valid lot-tracking item rejected: %v", err)
	}
	lot := models.StockLot{ID: primitive.NewObjectID(), TenantID: item.TenantID, ItemID: item.ID, Code: "LOT-001", ReceivedAt: now, Status: models.LotStatusQuarantined, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&lot); err != nil {
		t.Fatalf("valid quarantined lot rejected: %v", err)
	}
	item.LotTracking = "unsupported"
	if err := validation.Validate(&item); err == nil {
		t.Fatal("unsupported lot-tracking mode accepted")
	}
	lot.Status = "released"
	if err := validation.Validate(&lot); err == nil {
		t.Fatal("unsupported lot status accepted")
	}
}

func TestFEFOOrderingIsDeterministicAndPutsUnboundedExpiryLast(t *testing.T) {
	now := time.Now().UTC()
	received := now.Add(-24 * time.Hour)
	firstID := primitive.NewObjectID()
	secondID := primitive.NewObjectID()
	thirdID := primitive.NewObjectID()
	expiry := now.Add(48 * time.Hour)
	laterExpiry := now.Add(72 * time.Hour)
	candidates := []allocationCandidate{
		{balance: models.StockBalance{ID: primitive.NewObjectID(), QuantityMicros: 1}, lot: &models.StockLot{ID: thirdID, ExpiresAt: nil, ReceivedAt: received}},
		{balance: models.StockBalance{ID: primitive.NewObjectID(), QuantityMicros: 1}, lot: &models.StockLot{ID: secondID, ExpiresAt: &laterExpiry, ReceivedAt: received}},
		{balance: models.StockBalance{ID: primitive.NewObjectID(), QuantityMicros: 1}, lot: &models.StockLot{ID: firstID, ExpiresAt: &expiry, ReceivedAt: received}},
	}
	sortAllocationCandidates(candidates)
	if candidates[0].lot.ID != firstID || candidates[1].lot.ID != secondID || candidates[2].lot.ID != thirdID {
		t.Fatalf("unexpected FEFO order: %#v", candidates)
	}
}

func TestInventoryRejectsUnrepresentableAbsoluteQuantity(t *testing.T) {
	if _, err := (&Service{}).Waste(nil, WasteCommand{Command: Command{QuantityMicros: math.MinInt64}}); err == nil {
		t.Fatal("MinInt64 waste quantity should be rejected before database access")
	}
}

func TestAllocatedLotGuardRequiresAvailableAndUnexpiredLot(t *testing.T) {
	tenant, item, lot := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	now := time.Now().UTC()
	filter := allocatedLotGuardFilter(tenant, item, lot, now)
	if filter["tenantId"] != tenant || filter["itemId"] != item || filter["_id"] != lot || filter["status"] != models.LotStatusAvailable {
		t.Fatalf("guard filter dimensions = %#v", filter)
	}
	wantExpiry := bson.A{
		bson.M{"expiresAt": bson.M{"$exists": false}},
		bson.M{"expiresAt": nil},
		bson.M{"expiresAt": bson.M{"$gt": now}},
	}
	if !reflect.DeepEqual(filter["$or"], wantExpiry) {
		t.Fatalf("guard expiry predicate = %#v, want %#v", filter["$or"], wantExpiry)
	}
}

func TestAllocatedLotGuardAdvancesDurableLotState(t *testing.T) {
	update := allocatedLotGuardUpdate()
	if len(update) != 1 {
		t.Fatalf("guard update stages = %#v", update)
	}
	set, ok := update[0][0].Value.(bson.M)
	if !ok {
		t.Fatalf("guard update is not a $set pipeline: %#v", update)
	}
	updatedAt, ok := set["updatedAt"]
	if !ok {
		t.Fatalf("guard update does not advance updatedAt: %#v", update)
	}
	max, ok := updatedAt.(bson.M)
	if !ok {
		t.Fatalf("guard update is not a max expression: %#v", update)
	}
	values, ok := max["$max"].(bson.A)
	if !ok || len(values) != 2 || values[0] != "$$NOW" {
		t.Fatalf("guard max expression = %#v", max["$max"])
	}
	dateAdd, ok := values[1].(bson.M)
	if !ok || dateAdd["$dateAdd"] == nil {
		t.Fatalf("guard max does not include a date increment: %#v", values)
	}
	dateAddSpec, ok := dateAdd["$dateAdd"].(bson.M)
	if !ok {
		t.Fatalf("date increment specification = %#v", dateAdd["$dateAdd"])
	}
	if start, ok := dateAddSpec["startDate"]; !ok || start != "$updatedAt" {
		t.Fatalf("date increment start = %#v", start)
	}
	if amount, ok := dateAddSpec["amount"]; !ok || amount != int64(1) {
		t.Fatalf("date increment amount = %#v", amount)
	}
}

func TestJournalPostingLookupDoesNotRestrictByHeaderLocation(t *testing.T) {
	tenant, posting := primitive.NewObjectID(), primitive.NewObjectID()
	filter := journalPostingFilter(tenant, posting)
	if _, exists := filter["locationId"]; exists {
		t.Fatal("journal posting lookup still restricts by posting-header location")
	}
	if filter["tenantId"] != tenant || filter["_id"] != posting {
		t.Fatalf("journal posting filter = %#v", filter)
	}
}

func TestPhase3WorkflowModelsRejectInvalidStates(t *testing.T) {
	now := time.Now().UTC()
	tenant, location, area, user := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	count := models.StockCount{ID: primitive.NewObjectID(), TenantID: tenant, LocationID: location, StorageAreaID: area, CreatedBy: user, Status: models.StockCountFrozen, Version: 1, IdempotencyKey: "count-model-1", RequestHash: strings.Repeat("a", 64), CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&count); err != nil {
		t.Fatalf("valid stock count rejected: %v", err)
	}
	count.Status = "discarded"
	if err := validation.Validate(&count); err == nil {
		t.Fatal("invalid stock count status accepted")
	}
	count.Status = models.StockCountFrozen
	line := models.StockCountLine{ID: primitive.NewObjectID(), CountID: count.ID, TenantID: tenant, LocationID: location, StorageAreaID: area, ItemID: primitive.NewObjectID(), SnapshotQuantityMicros: 1, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&line); err != nil {
		t.Fatalf("valid stock count line rejected: %v", err)
	}
	line.SnapshotQuantityMicros = -1
	if err := validation.Validate(&line); err == nil {
		t.Fatal("negative stock count snapshot accepted")
	}
}

func TestActiveCountCursorIsVersionedAndStrictlyValidated(t *testing.T) {
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 6, time.UTC)
	id := primitive.NewObjectID()
	cursor, err := EncodeActiveCountCursor(createdAt, id)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	decodedAt, decodedID, err := DecodeActiveCountCursor(cursor)
	if err != nil || !decodedAt.Equal(createdAt) || decodedID != id {
		t.Fatalf("decode cursor = %s/%s, %v", decodedAt, decodedID.Hex(), err)
	}
	for _, invalid := range []string{"not-base64", cursor + " ", ""} {
		if _, _, err := DecodeActiveCountCursor(invalid); err == nil {
			t.Errorf("DecodeActiveCountCursor(%q) unexpectedly succeeded", invalid)
		}
	}
	if _, err := EncodeActiveCountCursor(time.Time{}, id); err == nil {
		t.Fatal("zero timestamp cursor unexpectedly encoded")
	}
}

func TestActiveCountIntegrityClassificationDoesNotExposeOwner(t *testing.T) {
	countID, otherOwner := primitive.NewObjectID(), primitive.NewObjectID()
	owned := models.StorageArea{ActiveStockCountID: &countID}
	other := models.StorageArea{ActiveStockCountID: &otherOwner}
	for name, test := range map[string]struct {
		status models.StockCountStatus
		area   models.StorageArea
		exists bool
		want   ActiveCountIntegrity
	}{
		"draft ignores missing area": {status: models.StockCountDraft, exists: false, want: ActiveCountHealthy},
		"missing area":               {status: models.StockCountFrozen, exists: false, want: ActiveCountAreaMissing},
		"missing owner":              {status: models.StockCountReviewed, area: models.StorageArea{}, exists: true, want: ActiveCountOwnerMissing},
		"matching owner":             {status: models.StockCountFrozen, area: owned, exists: true, want: ActiveCountHealthy},
		"different owner":            {status: models.StockCountReviewed, area: other, exists: true, want: ActiveCountOwnerMismatch},
	} {
		if got := classifyActiveCountIntegrity(test.status, countID, test.area, test.exists); got != test.want {
			t.Errorf("%s classification = %q, want %q", name, got, test.want)
		}
	}
}
