package forecast

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Scope struct {
	TenantID   primitive.ObjectID
	LocationID primitive.ObjectID
	ActorID    primitive.ObjectID
}

type SealedSnapshot struct {
	Dataset models.ForecastDataset
	Rows    []models.ForecastInputRow
}

type MongoStore struct{ DB *db.MongoDB }

func NewMongoStore(database *db.MongoDB) *MongoStore { return &MongoStore{DB: database} }

// MaterializeAndSeal reads immutable journal facts and guest-plan snapshots.
// It never reads sales.status and never writes outside forecast collections.
func (s *MongoStore) MaterializeAndSeal(ctx context.Context, scope Scope, req MaterializeRequest, name, source string) (SealedSnapshot, error) {
	if s == nil || s.DB == nil {
		return SealedSnapshot{}, errors.New("forecast mongo store is not configured")
	}
	journal, err := s.journal(ctx, scope, req.CutoffAt)
	if err != nil {
		return SealedSnapshot{}, err
	}
	plans, err := s.guestPlans(ctx, scope, req.CutoffAt)
	if err != nil {
		return SealedSnapshot{}, err
	}
	inbound, err := s.confirmedInbound(ctx, scope, req.CutoffAt, req.EffectiveFrom, req.EffectiveTo)
	if err != nil {
		return SealedSnapshot{}, err
	}
	req.Journal, req.GuestPlans, req.ConfirmedInbound = journal, plans, inbound
	sealed, err := Materialize(req)
	if err != nil {
		return SealedSnapshot{}, err
	}
	return s.persistSealed(ctx, scope, sealed, name, source, req.EffectiveFrom, req.EffectiveTo)
}

func (s *MongoStore) confirmedInbound(ctx context.Context, scope Scope, cutoff, from, to time.Time) ([]ConfirmedInboundRecord, error) {
	filter := bson.M{"tenantId": scope.TenantID, "locationId": scope.LocationID, "status": models.PurchaseOrderSupplierConfirmed, "supplierConfirmedAt": bson.M{"$lte": cutoff.UTC()}, "deliveryDate": bson.M{"$gt": cutoff.UTC(), "$lte": to.UTC()}}
	cur, err := s.DB.PurchaseOrders().Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1, "deliveryDate": 1, "supplierConfirmedAt": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var orders []models.PurchaseOrder
	if err := cur.All(ctx, &orders); err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, nil
	}
	ids := make([]primitive.ObjectID, len(orders))
	for i := range orders {
		ids[i] = orders[i].ID
	}
	lineCur, err := s.DB.PurchaseOrderLines().Find(ctx, bson.M{"tenantId": scope.TenantID, "locationId": scope.LocationID, "purchaseOrderId": bson.M{"$in": ids}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer lineCur.Close(ctx)
	var lines []models.PurchaseOrderLine
	if err := lineCur.All(ctx, &lines); err != nil {
		return nil, err
	}
	seen := make(map[primitive.ObjectID]struct{}, len(lines))
	out := make([]ConfirmedInboundRecord, 0, len(lines))
	for _, line := range lines {
		if _, exists := seen[line.ID]; exists {
			continue
		}
		seen[line.ID] = struct{}{}
		remaining := line.OrderedQuantityMicros - line.ReceivedQuantityMicros
		if remaining <= 0 {
			continue
		}
		// DeliveryDate is on the immutable order header. Loading it by ID keeps
		// this snapshot independent of any later purchasing edits.
		var order models.PurchaseOrder
		for _, candidate := range orders {
			if candidate.ID == line.PurchaseOrderID {
				order = candidate
				break
			}
		}
		if order.ID.IsZero() {
			continue
		}
		if order.SupplierConfirmedAt == nil || order.DeliveryDate.IsZero() {
			continue
		}
		out = append(out, ConfirmedInboundRecord{ID: line.ID.Hex(), TenantID: scope.TenantID.Hex(), LocationID: scope.LocationID.Hex(), ItemID: line.ItemID.Hex(), QuantityMicros: remaining, EffectiveAt: order.DeliveryDate, RecordedAt: *order.SupplierConfirmedAt})
	}
	return out, nil
}

func (s *MongoStore) journal(ctx context.Context, scope Scope, cutoff time.Time) ([]JournalRecord, error) {
	filter := bson.M{"tenantId": scope.TenantID, "locationId": scope.LocationID, "recordedAt": bson.M{"$lte": cutoff.UTC()}, "type": bson.M{"$in": bson.A{string(models.StockPostingSaleConsumption), string(models.StockPostingReversal)}}}
	cur, err := s.DB.StockPostings().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "recordedAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var postings []models.StockPosting
	if err := cur.All(ctx, &postings); err != nil {
		return nil, err
	}
	if len(postings) == 0 {
		return nil, nil
	}
	byPosting := make(map[primitive.ObjectID]models.StockPosting, len(postings))
	ids := make([]primitive.ObjectID, 0, len(postings))
	for _, p := range postings {
		byPosting[p.ID] = p
		ids = append(ids, p.ID)
	}
	mcur, err := s.DB.StockMovements().Find(ctx, bson.M{"tenantId": scope.TenantID, "locationId": scope.LocationID, "postingId": bson.M{"$in": ids}, "recordedAt": bson.M{"$lte": cutoff.UTC()}}, options.Find().SetSort(bson.D{{Key: "recordedAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer mcur.Close(ctx)
	var movements []models.StockMovement
	if err := mcur.All(ctx, &movements); err != nil {
		return nil, err
	}
	out := make([]JournalRecord, 0, len(movements))
	for _, m := range movements {
		p, ok := byPosting[m.PostingID]
		if !ok {
			continue
		}
		if m.QuantityMicros == math.MinInt64 {
			return nil, errors.New("forecast journal quantity overflows magnitude")
		}
		q := m.QuantityMicros
		if q < 0 {
			q = -q
		}
		r := JournalRecord{ID: m.ID.Hex(), PostingID: p.ID.Hex(), TenantID: scope.TenantID.Hex(), LocationID: scope.LocationID.Hex(), ItemID: m.ItemID.Hex(), QuantityMicros: q, EffectiveAt: m.EffectiveAt, RecordedAt: m.RecordedAt}
		if p.Type == models.StockPostingSaleConsumption {
			r.Kind = JournalSaleConsumption
		} else {
			r.Kind = JournalReversal
			if p.ReversalOf != nil {
				r.ReversalOf = p.ReversalOf.Hex()
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *MongoStore) guestPlans(ctx context.Context, scope Scope, cutoff time.Time) ([]GuestPlanRecord, error) {
	cur, err := s.DB.GuestPlans().Find(ctx, bson.M{"tenantId": scope.TenantID, "locationId": scope.LocationID, "updatedAt": bson.M{"$lte": cutoff.UTC()}, "isActual": false}, options.Find().SetSort(bson.D{{Key: "planDate", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var plans []models.GuestPlan
	if err := cur.All(ctx, &plans); err != nil {
		return nil, err
	}
	out := make([]GuestPlanRecord, 0, len(plans))
	for _, p := range plans {
		out = append(out, GuestPlanRecord{ID: p.ID.Hex(), TenantID: scope.TenantID.Hex(), LocationID: scope.LocationID.Hex(), PlanDate: p.PlanDate, ServicePeriod: p.ServicePeriod, GuestCount: p.GuestCount, RecordedAt: p.UpdatedAt, IsActual: p.IsActual})
	}
	return out, nil
}

func (s *MongoStore) persistSealed(ctx context.Context, scope Scope, sealed SealedDataset, name, source string, from, to time.Time) (SealedSnapshot, error) {
	if name == "" {
		name = "journal"
	}
	if source == "" {
		source = "journal"
	}
	var existing models.ForecastDataset
	err := s.DB.ForecastDatasets().FindOne(ctx, bson.M{"tenantId": scope.TenantID, "locationId": scope.LocationID, "contentHash": sealed.Manifest.ContentHash}).Decode(&existing)
	if err == nil {
		return s.loadSealed(ctx, scope, existing)
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return SealedSnapshot{}, err
	}
	if from.IsZero() {
		from = firstRowDate(sealed.Rows)
	}
	if to.IsZero() {
		to = lastRowDate(sealed.Rows).AddDate(0, 0, 1)
	}
	now := time.Now().UTC()
	sealedAt := now
	dataset := models.ForecastDataset{ID: primitive.NewObjectID(), TenantID: scope.TenantID, LocationID: scope.LocationID, Name: name, Source: source, Status: models.ForecastDatasetSealed, EffectiveFrom: day(from), EffectiveTo: day(to), CutoffAt: &sealed.CutoffAt, ManifestVersion: sealed.Manifest.Version, RowCount: int64(len(sealed.Rows)), ContentHash: sealed.Manifest.ContentHash, SealedBy: &scope.ActorID, SealedAt: &sealedAt, CreatedBy: scope.ActorID, Version: 1, CreatedAt: now, UpdatedAt: now}
	rows := make([]interface{}, 0, len(sealed.Rows))
	for i, row := range sealed.Rows {
		var itemID *primitive.ObjectID
		if row.ItemID != "" {
			id, e := primitive.ObjectIDFromHex(row.ItemID)
			if e != nil {
				return SealedSnapshot{}, fmt.Errorf("invalid materialized item id: %w", e)
			}
			itemID = &id
		}
		rows = append(rows, models.ForecastInputRow{ID: primitive.NewObjectID(), TenantID: scope.TenantID, LocationID: scope.LocationID, DatasetID: dataset.ID, RowNumber: int64(i + 1), Kind: models.ForecastInputKind(row.Kind), ItemID: itemID, ServicePeriod: row.ServicePeriod, SourceIDs: row.SourceIDs, QuantityMicros: row.QuantityMicros, GuestCount: row.GuestCount, EffectiveAt: row.EffectiveAt, RecordedAt: row.RecordedAt, CreatedAt: now})
	}
	session, err := s.DB.Client.StartSession()
	if err != nil {
		return SealedSnapshot{}, err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		if _, e := s.DB.ForecastDatasets().InsertOne(sc, dataset); e != nil {
			return nil, e
		}
		if len(rows) > 0 {
			if _, e := s.DB.ForecastInputRows().InsertMany(sc, rows); e != nil {
				return nil, e
			}
		}
		return nil, nil
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			if e := s.DB.ForecastDatasets().FindOne(ctx, bson.M{"tenantId": scope.TenantID, "locationId": scope.LocationID, "contentHash": sealed.Manifest.ContentHash}).Decode(&existing); e == nil {
				return s.loadSealed(ctx, scope, existing)
			}
		}
		return SealedSnapshot{}, err
	}
	out := make([]models.ForecastInputRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.(models.ForecastInputRow))
	}
	return SealedSnapshot{Dataset: dataset, Rows: out}, nil
}

func (s *MongoStore) loadSealed(ctx context.Context, scope Scope, dataset models.ForecastDataset) (SealedSnapshot, error) {
	cur, err := s.DB.ForecastInputRows().Find(ctx, bson.M{"tenantId": scope.TenantID, "locationId": scope.LocationID, "datasetId": dataset.ID}, options.Find().SetSort(bson.D{{Key: "rowNumber", Value: 1}}))
	if err != nil {
		return SealedSnapshot{}, err
	}
	defer cur.Close(ctx)
	var rows []models.ForecastInputRow
	if err := cur.All(ctx, &rows); err != nil {
		return SealedSnapshot{}, err
	}
	return SealedSnapshot{Dataset: dataset, Rows: rows}, nil
}

func firstRowDate(rows []MaterializedRow) time.Time {
	if len(rows) == 0 {
		return time.Now().UTC()
	}
	return rows[0].EffectiveAt
}
func lastRowDate(rows []MaterializedRow) time.Time {
	if len(rows) == 0 {
		return time.Now().UTC()
	}
	out := rows[0].EffectiveAt
	for _, r := range rows[1:] {
		if r.EffectiveAt.After(out) {
			out = r.EffectiveAt
		}
	}
	return out
}
