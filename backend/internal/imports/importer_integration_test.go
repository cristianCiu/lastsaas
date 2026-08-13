package imports

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"lastsaas/internal/models"
	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestEngineReplicaSetDryRunDoesNotWrite(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if !isReplicaSet(t, database.Client) {
		return
	}
	testutil.CleanupCollections(t, database)
	tenant := primitive.NewObjectID()
	user := primitive.NewObjectID()
	engine := &Engine{DB: database}
	req := Request{Target: models.ImportTargetUnits, IdempotencyKey: "dry-run-key", Content: "code,name,symbol,dimension,precision,isActive\nkg,Kilogram,kg,mass,3,true\n"}

	report, err := engine.DryRun(context.Background(), tenant, user, req)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !report.DryRun || report.TotalRows != 1 || report.CreatedRows != 1 || len(report.Errors) != 0 {
		t.Fatalf("unexpected dry-run report: %#v", report)
	}
	for _, collection := range []*mongo.Collection{database.Units(), database.Categories(), database.Items(), database.Suppliers(), database.SupplierItems(), database.ImportRuns()} {
		if count, err := collection.CountDocuments(context.Background(), bson.M{}); err != nil || count != 0 {
			t.Fatalf("dry run wrote to %s: count=%d err=%v", collection.Name(), count, err)
		}
	}
}

func TestEngineReplicaSetAppliesAllTargetsAndScopesKeys(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if !isReplicaSet(t, database.Client) {
		return
	}
	testutil.CleanupCollections(t, database)
	ctx := context.Background()
	tenant := primitive.NewObjectID()
	otherTenant := primitive.NewObjectID()
	user := primitive.NewObjectID()
	engine := &Engine{DB: database}

	apply := func(req Request) *Report {
		t.Helper()
		report, err := engine.Apply(ctx, tenant, user, req)
		if err != nil {
			t.Fatalf("apply %s: %v", req.Target, err)
		}
		if len(report.Errors) != 0 {
			t.Fatalf("apply %s errors: %#v", req.Target, report.Errors)
		}
		return report
	}
	apply(Request{Target: models.ImportTargetUnits, IdempotencyKey: "all-targets-unit", Content: "code,name,symbol,dimension,precision,isActive\nkg,Kilogram,kg,mass,3,true\n"})
	apply(Request{Target: models.ImportTargetCategories, IdempotencyKey: "all-targets-category", Content: "code,name,isActive\nfood,Food,true\n"})
	apply(Request{Target: models.ImportTargetSuppliers, IdempotencyKey: "all-targets-supplier", Content: "code,name,contactName,email,phone,orderingDays,defaultLeadTimeDays,isActive\nacme,Acme,,, ,1|3,2,true\n"})
	apply(Request{Target: models.ImportTargetItems, IdempotencyKey: "all-targets-item", Content: "sku,name,description,brand,categoryCode,baseUnitCode,allergens,shelfLifeDays,stockable,isActive\nburger,Burger,,,food,kg,,30,true,true\n"})
	apply(Request{Target: models.ImportTargetSupplierItems, IdempotencyKey: "all-targets-supplier-item", Content: "supplierCode,itemSKU,supplierSKU,packSizeMicros,moq,unitPriceMinor,currency,leadTimeDays,isActive\nacme,burger,acme-burger,1000000,1,250,EUR,2,true\n"})

	for _, check := range []struct {
		collection *mongo.Collection
		want       int64
	}{
		{database.Units(), 1}, {database.Categories(), 1}, {database.Items(), 1},
		{database.Suppliers(), 1}, {database.SupplierItems(), 1}, {database.ImportRuns(), 5},
	} {
		if count, err := check.collection.CountDocuments(ctx, bson.M{"tenantId": tenant}); err != nil || count != check.want {
			t.Fatalf("%s count=%d err=%v, want %d tenant records", check.collection.Name(), count, err, check.want)
		}
	}
	// Target is part of the idempotency scope: the same key can be used for a
	// different target without colliding with the completed unit run.
	apply(Request{Target: models.ImportTargetCategories, IdempotencyKey: "shared-key", Content: "code,name,isActive\ndrinks,Drinks,true\n"})
	if report := apply(Request{Target: models.ImportTargetCategories, IdempotencyKey: "shared-key", Content: "code,name,isActive\ndifferent,Different,true\n"}); report.CreatedRows != 1 {
		t.Fatalf("replay did not return the original result: %#v", report)
	}
	otherReport, err := engine.Apply(ctx, otherTenant, user, Request{Target: models.ImportTargetCategories, IdempotencyKey: "shared-key", Content: "code,name,isActive\nfood,Other food,true\n"})
	if err != nil || len(otherReport.Errors) != 0 {
		t.Fatalf("same key cross-tenant apply failed: report=%#v err=%v", otherReport, err)
	}
}

func TestEngineReplicaSetRollbackAndConcurrentReplay(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if !isReplicaSet(t, database.Client) {
		return
	}
	testutil.CleanupCollections(t, database)
	ctx := context.Background()
	tenant := primitive.NewObjectID()
	user := primitive.NewObjectID()
	engine := &Engine{DB: database}

	// A late invalid row aborts the complete request; the valid first row is
	// never persisted because planning and writes share the transaction.
	rollback, err := engine.Apply(ctx, tenant, user, Request{Target: models.ImportTargetUnits, IdempotencyKey: "rollback-key", Content: "code,name,symbol,dimension,precision,isActive\nkg,Kilogram,kg,mass,3,true\nkg2,Invalid,g,unknown,0,true\n"})
	if err != nil || len(rollback.Errors) == 0 {
		t.Fatalf("expected transactional validation rollback: report=%#v err=%v", rollback, err)
	}
	if count := testutil.CountDocuments(t, database, "units", bson.M{"tenantId": tenant}); count != 0 {
		t.Fatalf("rollback left %d units", count)
	}

	req := Request{Target: models.ImportTargetUnits, IdempotencyKey: "concurrent-key", Content: "code,name,symbol,dimension,precision,isActive\nkg,Kilogram,kg,mass,3,true\n"}
	const attempts = 8
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			report, applyErr := engine.Apply(ctx, tenant, user, req)
			if applyErr == nil && len(report.Errors) != 0 {
				applyErr = fmt.Errorf("concurrent replay returned errors: %v", report.Errors)
			}
			results <- applyErr
		}()
	}
	wg.Wait()
	close(results)
	for applyErr := range results {
		if applyErr != nil {
			t.Fatalf("concurrent replay: %v", applyErr)
		}
	}
	if count := testutil.CountDocuments(t, database, "units", bson.M{"tenantId": tenant, "code": "kg"}); count != 1 {
		t.Fatalf("concurrent replay created %d units", count)
	}
	if count := testutil.CountDocuments(t, database, "import_runs", bson.M{"tenantId": tenant, "target": models.ImportTargetUnits, "idempotencyKey": req.IdempotencyKey}); count != 1 {
		t.Fatalf("concurrent replay created %d runs", count)
	}
}

func TestEngineReplicaSetPostWriteFailureRollsBackCatalogAndRun(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if !isReplicaSet(t, database.Client) {
		return
	}
	testutil.CleanupCollections(t, database)
	tenant := primitive.NewObjectID()
	engine := &Engine{DB: database}
	engine.testAfterWriteHook = func(writeNumber int) error {
		if writeNumber == 1 {
			return errors.New("controlled post-write test failure")
		}
		return nil
	}
	report, err := engine.Apply(context.Background(), tenant, primitive.NewObjectID(), Request{
		Target:         models.ImportTargetUnits,
		IdempotencyKey: "post-write-failure",
		Content:        "code,name,symbol,dimension,precision,isActive\nkg,Kilogram,kg,mass,3,true\ng,Gram,g,mass,0,true\n",
	})
	if err == nil || report != nil || !strings.Contains(err.Error(), "controlled post-write test failure") {
		t.Fatalf("expected controlled failure, report=%#v err=%v", report, err)
	}
	if count := testutil.CountDocuments(t, database, "units", bson.M{"tenantId": tenant}); count != 0 {
		t.Fatalf("post-write rollback left %d catalog rows", count)
	}
	if count := testutil.CountDocuments(t, database, "import_runs", bson.M{"tenantId": tenant}); count != 0 {
		t.Fatalf("post-write rollback left %d import runs", count)
	}
}

func TestEngineReplicaSetRejectsCrossTenantAndImmutableReferences(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if !isReplicaSet(t, database.Client) {
		return
	}
	testutil.CleanupCollections(t, database)
	ctx := context.Background()
	tenant, foreign := primitive.NewObjectID(), primitive.NewObjectID()
	user := primitive.NewObjectID()
	engine := &Engine{DB: database}
	now := time.Now().UTC()
	foreignCategory := models.Category{ID: primitive.NewObjectID(), TenantID: foreign, Code: "food", Name: "Food", IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	foreignUnit := models.Unit{ID: primitive.NewObjectID(), TenantID: foreign, Code: "kg", Name: "Kilogram", Symbol: "kg", Dimension: models.UnitDimensionMass, Precision: 3, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if _, err := database.Categories().InsertOne(ctx, foreignCategory); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Units().InsertOne(ctx, foreignUnit); err != nil {
		t.Fatal(err)
	}
	crossTenant, err := engine.Apply(ctx, tenant, user, Request{Target: models.ImportTargetItems, IdempotencyKey: "cross-tenant-key", Content: "sku,name,description,brand,categoryCode,baseUnitCode,allergens,shelfLifeDays,stockable,isActive\nitem,Item,,,food,kg,,1,true,true\n"})
	if err != nil || len(crossTenant.Errors) == 0 {
		t.Fatalf("cross-tenant references were accepted: report=%#v err=%v", crossTenant, err)
	}

	if _, err := database.Units().InsertOne(ctx, models.Unit{ID: primitive.NewObjectID(), TenantID: tenant, Code: "kg", Name: "Kilogram", Symbol: "kg", Dimension: models.UnitDimensionMass, Precision: 3, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Categories().InsertOne(ctx, models.Category{ID: primitive.NewObjectID(), TenantID: tenant, Code: "food", Name: "Food", IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Units().InsertOne(ctx, models.Unit{ID: primitive.NewObjectID(), TenantID: tenant, Code: "other", Name: "Other", Symbol: "o", Dimension: models.UnitDimensionMass, Precision: 3, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if report, err := engine.Apply(ctx, tenant, user, Request{Target: models.ImportTargetUnits, IdempotencyKey: "immutable-unit-key", Content: "code,name,symbol,dimension,precision,isActive\nkg,Kilogram,kg,volume,3,true\n"}); err != nil || len(report.Errors) == 0 {
		t.Fatalf("unit dimension changed: report=%#v err=%v", report, err)
	}
	item, err := engine.Apply(ctx, tenant, user, Request{Target: models.ImportTargetItems, IdempotencyKey: "immutable-item-key", Content: "sku,name,description,brand,categoryCode,baseUnitCode,allergens,shelfLifeDays,stockable,isActive\nitem,Item,,,food,kg,,1,true,true\n"})
	if err != nil || len(item.Errors) != 0 {
		t.Fatalf("seed item: report=%#v err=%v", item, err)
	}
	var stored models.Item
	if err := database.Items().FindOne(ctx, bson.M{"tenantId": tenant, "sku": "item"}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ItemConversions().InsertOne(ctx, models.ItemConversion{ID: primitive.NewObjectID(), TenantID: tenant, ItemID: stored.ID, FromUnitID: foreignUnit.ID, Numerator: 1, Denominator: 1, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	changed, err := engine.Apply(ctx, tenant, user, Request{Target: models.ImportTargetItems, IdempotencyKey: "immutable-item-change-key", Content: "sku,name,description,brand,categoryCode,baseUnitCode,allergens,shelfLifeDays,stockable,isActive\nitem,Item,,,food,other,,1,true,true\n"})
	if err != nil || len(changed.Errors) == 0 {
		t.Fatalf("item base unit changed with conversion: report=%#v err=%v", changed, err)
	}
}

func isReplicaSet(t *testing.T, client *mongo.Client) bool {
	t.Helper()
	var hello bson.M
	err := client.Database("admin").RunCommand(context.Background(), bson.D{{Key: "hello", Value: 1}}).Decode(&hello)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			t.Skip("skipping: MongoDB hello unavailable")
		}
		t.Skipf("skipping: MongoDB does not support transactions: %v", err)
	}
	if _, ok := hello["setName"]; !ok {
		t.Skip("skipping: MongoDB replica set not configured")
	}
	return true
}
