package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrGoodsReceiptNotFound  = errors.New("goods receipt not found")
	ErrGoodsReceiptState     = errors.New("goods receipt state does not allow this operation")
	ErrPurchaseOrderNotReady = errors.New("purchase order is not approved or open for receiving")
	ErrReceiptOverage        = errors.New("receipt would exceed the ordered quantity")
	ErrReceiptVarianceReason = errors.New("variance reason is required")
	ErrReceiptLineNotFound   = errors.New("purchase order line not found")
)

type GoodsReceiptLineCommand struct {
	PurchaseOrderLineID    primitive.ObjectID
	ReceivedQuantityMicros int64
	ActualUnitPriceMinor   int64
	VarianceReason         string
	LotCode                string
	ExpiresAt              *time.Time
}

type GoodsReceiptCommand struct {
	TenantID        primitive.ObjectID
	UserID          primitive.ObjectID
	LocationID      primitive.ObjectID
	StorageAreaID   primitive.ObjectID
	PurchaseOrderID primitive.ObjectID
	ReceivedAt      time.Time
	IdempotencyKey  string
	Lines           []GoodsReceiptLineCommand
	// VarianceApproved is set by the HTTP authorization layer only after it has
	// established that the receiver is a purchasing manager.
	VarianceApproved bool
}

type GoodsReceiptResult struct {
	Receipt   models.GoodsReceipt       `json:"receipt"`
	Lines     []models.GoodsReceiptLine `json:"lines"`
	Inventory *Result                   `json:"inventory"`
}

func receiptHash(command GoodsReceiptCommand) string {
	encoded, _ := json.Marshal(command)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func receiptReverseHash(tenantID, userID, locationID, receiptID primitive.ObjectID) string {
	encoded, _ := json.Marshal(struct{ Tenant, User, Location, Receipt primitive.ObjectID }{tenantID, userID, locationID, receiptID})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func validateGoodsReceiptCommand(command GoodsReceiptCommand) error {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.LocationID.IsZero() || command.StorageAreaID.IsZero() || command.PurchaseOrderID.IsZero() || len(strings.TrimSpace(command.IdempotencyKey)) < 8 || len(command.IdempotencyKey) > 128 || len(command.Lines) == 0 || len(command.Lines) > 1000 {
		return ErrInvalidReference
	}
	seen := make(map[primitive.ObjectID]struct{}, len(command.Lines))
	for _, line := range command.Lines {
		if line.PurchaseOrderLineID.IsZero() || line.ReceivedQuantityMicros <= 0 || line.ActualUnitPriceMinor < 0 {
			return ErrInvalidReference
		}
		if _, ok := seen[line.PurchaseOrderLineID]; ok {
			return ErrInvalidReference
		}
		seen[line.PurchaseOrderLineID] = struct{}{}
	}
	return nil
}

func (s *Service) ReceiveGoodsReceipt(ctx context.Context, command GoodsReceiptCommand) (*GoodsReceiptResult, error) {
	if err := validateGoodsReceiptCommand(command); err != nil {
		return nil, err
	}
	hash := receiptHash(command)
	if existing, err := s.findGoodsReceipt(ctx, command.TenantID, command.IdempotencyKey, hash); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, ErrTransactionRequired
	}
	defer session.EndSession(ctx)
	var result *GoodsReceiptResult
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		if existing, lookupErr := s.findGoodsReceipt(sc, command.TenantID, command.IdempotencyKey, hash); lookupErr != nil {
			return nil, lookupErr
		} else if existing != nil {
			result = existing
			return nil, nil
		}
		result, err = s.receiveGoodsReceiptInTransaction(sc, command, hash)
		return nil, err
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			if recovered, lookupErr := s.findGoodsReceipt(ctx, command.TenantID, command.IdempotencyKey, hash); lookupErr == nil && recovered != nil {
				return recovered, nil
			}
		}
		return nil, err
	}
	return result, nil
}

func (s *Service) ApplyGoodsReceipt(ctx context.Context, command GoodsReceiptCommand) (*GoodsReceiptResult, error) {
	return s.ReceiveGoodsReceipt(ctx, command)
}

func (s *Service) receiveGoodsReceiptInTransaction(ctx mongo.SessionContext, command GoodsReceiptCommand, hash string) (*GoodsReceiptResult, error) {
	var order models.PurchaseOrder
	if err := s.db.PurchaseOrders().FindOne(ctx, bson.M{"_id": command.PurchaseOrderID, "tenantId": command.TenantID, "locationId": command.LocationID}).Decode(&order); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrPurchaseOrderNotReady
		}
		return nil, err
	}
	if order.ApprovedBy == nil || order.ApprovedAt == nil || (order.Status != models.PurchaseOrderApproved && order.Status != models.PurchaseOrderOrdered && order.Status != models.PurchaseOrderPartiallyReceived) {
		return nil, ErrPurchaseOrderNotReady
	}
	cur, err := s.db.PurchaseOrderLines().Find(ctx, bson.M{"tenantId": command.TenantID, "purchaseOrderId": order.ID, "locationId": order.LocationID})
	if err != nil {
		return nil, err
	}
	var orderLines []models.PurchaseOrderLine
	if err := cur.All(ctx, &orderLines); err != nil {
		return nil, err
	}
	byID := make(map[primitive.ObjectID]models.PurchaseOrderLine, len(orderLines))
	for _, line := range orderLines {
		byID[line.ID] = line
	}
	now := time.Now().UTC()
	receivedAt := command.ReceivedAt.UTC()
	if receivedAt.IsZero() {
		receivedAt = now
	}
	receipt := models.GoodsReceipt{ID: primitive.NewObjectID(), TenantID: command.TenantID, LocationID: order.LocationID, StorageAreaID: command.StorageAreaID, PurchaseOrderID: order.ID, OrderNumberSnapshot: order.OrderNumber, OrderVersionSnapshot: order.Version, SupplierID: order.SupplierID, ReceivedBy: command.UserID, ReceivedAt: receivedAt, Status: models.GoodsReceiptPosted, Audit: []models.PurchaseOrderAuditEntry{{Action: "posted", UserID: command.UserID, At: now}}, IdempotencyKey: command.IdempotencyKey, RequestHash: hash, CreatedAt: now, UpdatedAt: now}
	if err := s.fenceInventoryScopes(ctx, inventoryScope{TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID}); err != nil {
		return nil, err
	}
	posting := models.StockPosting{ID: primitive.NewObjectID(), TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, UserID: command.UserID, Type: models.StockPostingGoodsReceipt, IdempotencyKey: command.IdempotencyKey, RequestHash: hash, EffectiveAt: receivedAt, RecordedAt: now, Reason: "goods receipt " + order.OrderNumber}
	if err := validation.Validate(&posting); err != nil {
		return nil, err
	}
	if _, err := s.db.StockPostings().InsertOne(ctx, posting); err != nil {
		return nil, err
	}
	receipt.PostingID = posting.ID
	lines := make([]models.GoodsReceiptLine, 0, len(command.Lines))
	postLines := make([]lineSpec, 0, len(command.Lines))
	movements := make([]models.StockMovement, 0, len(command.Lines))
	balances := make([]models.StockBalance, 0, len(command.Lines))
	for _, input := range command.Lines {
		orderLine, ok := byID[input.PurchaseOrderLineID]
		if !ok {
			return nil, ErrReceiptLineNotFound
		}
		if orderLine.ReceivedQuantityMicros > math.MaxInt64-input.ReceivedQuantityMicros {
			return nil, ErrQuantityOverflow
		}
		newTotal := orderLine.ReceivedQuantityMicros + input.ReceivedQuantityMicros
		overage := newTotal > orderLine.OrderedQuantityMicros
		priceVariance, subErr := safeSub(input.ActualUnitPriceMinor, orderLine.UnitPriceMinor)
		if subErr != nil {
			return nil, ErrQuantityOverflow
		}
		qtyVariance, subErr := safeSub(input.ReceivedQuantityMicros, orderLine.OrderedQuantityMicros)
		if subErr != nil {
			return nil, ErrQuantityOverflow
		}
		reason := strings.TrimSpace(input.VarianceReason)
		if overage && !command.VarianceApproved {
			return nil, ErrReceiptOverage
		}
		if (overage && reason == "") || (priceVariance != 0 && reason == "") {
			return nil, ErrReceiptVarianceReason
		}
		lotID, err := s.ensureReceiptLot(ctx, command, orderLine, input, receivedAt)
		if err != nil {
			return nil, err
		}
		line := models.GoodsReceiptLine{ID: primitive.NewObjectID(), TenantID: command.TenantID, LocationID: command.LocationID, GoodsReceiptID: receipt.ID, PurchaseOrderID: order.ID, PurchaseOrderLineID: orderLine.ID, LineNumber: orderLine.LineNumber, ItemID: orderLine.ItemID, ItemSKU: orderLine.ItemSKU, ItemName: orderLine.ItemName, Description: orderLine.Description, SupplierCode: orderLine.SupplierCode, SupplierName: orderLine.SupplierName, SupplierItemCode: orderLine.SupplierItemCode, SupplierItemVersion: orderLine.SupplierItemVersion, ItemBaseUnitCode: orderLine.ItemBaseUnitCode, OrderedUnitCode: orderLine.OrderedUnitCode, OrderedQuantityMicros: orderLine.OrderedQuantityMicros, PreviouslyReceivedMicros: orderLine.ReceivedQuantityMicros, ReceivedQuantityMicros: input.ReceivedQuantityMicros, QuantityVarianceMicros: qtyVariance, OrderedUnitPriceMinor: orderLine.UnitPriceMinor, ActualUnitPriceMinor: input.ActualUnitPriceMinor, PriceVarianceMinor: priceVariance, Currency: orderLine.Currency, VarianceReason: reason, StorageAreaID: command.StorageAreaID, LotCode: strings.TrimSpace(input.LotCode), LotID: lotID, ExpiresAt: input.ExpiresAt, CreatedAt: now, UpdatedAt: now}
		if err := validation.Validate(&line); err != nil {
			return nil, err
		}
		if _, err := s.db.GoodsReceiptLines().InsertOne(ctx, line); err != nil {
			return nil, err
		}
		lines = append(lines, line)
		postLines = append(postLines, lineSpec{TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, ItemID: orderLine.ItemID, LotID: lotID, QuantityMicros: input.ReceivedQuantityMicros})
		updatedLine, err := s.db.PurchaseOrderLines().UpdateOne(ctx, bson.M{"_id": orderLine.ID, "tenantId": command.TenantID, "purchaseOrderId": order.ID, "receivedQuantityMicros": orderLine.ReceivedQuantityMicros}, bson.M{"$inc": bson.M{"receivedQuantityMicros": input.ReceivedQuantityMicros}, "$set": bson.M{"updatedAt": now}})
		if err != nil {
			return nil, err
		}
		if updatedLine.MatchedCount != 1 {
			return nil, ErrPurchaseOrderNotReady
		}
	}
	for number, line := range postLines {
		mode, err := s.itemLotMode(ctx, command.TenantID, line.ItemID)
		if err != nil {
			return nil, err
		}
		if err := s.validateLotForQuantity(ctx, Command{TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, ItemID: line.ItemID, LotID: line.LotID, QuantityMicros: line.QuantityMicros}, mode, line.QuantityMicros); err != nil {
			return nil, err
		}
		if err := s.verifyReferences(ctx, Command{TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, ItemID: line.ItemID}); err != nil {
			return nil, err
		}
		movement := models.StockMovement{ID: primitive.NewObjectID(), PostingID: posting.ID, TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, ItemID: line.ItemID, LotID: line.LotID, LineNumber: int32(number), QuantityMicros: line.QuantityMicros, EffectiveAt: receivedAt, RecordedAt: now}
		if err := validation.Validate(&movement); err != nil {
			return nil, err
		}
		if _, err := s.db.StockMovements().InsertOne(ctx, movement); err != nil {
			return nil, err
		}
		balance, err := s.applyBalanceLine(ctx, line)
		if err != nil {
			return nil, err
		}
		movements = append(movements, movement)
		balances = append(balances, *balance)
	}
	allReceived := true
	anyReceived := false
	for _, line := range orderLines {
		n := line.ReceivedQuantityMicros
		for _, input := range command.Lines {
			if input.PurchaseOrderLineID == line.ID {
				n += input.ReceivedQuantityMicros
			}
		}
		if n > 0 {
			anyReceived = true
		}
		if n < line.OrderedQuantityMicros {
			allReceived = false
		}
	}
	status := models.PurchaseOrderPartiallyReceived
	if allReceived && anyReceived {
		status = models.PurchaseOrderReceived
	}
	updatedOrder := order
	if err := s.db.PurchaseOrders().FindOneAndUpdate(ctx, bson.M{"_id": order.ID, "tenantId": command.TenantID, "version": order.Version, "status": bson.M{"$in": bson.A{models.PurchaseOrderApproved, models.PurchaseOrderOrdered, models.PurchaseOrderPartiallyReceived}}}, bson.M{"$set": bson.M{"status": status, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}, "$push": bson.M{"audit": models.PurchaseOrderAuditEntry{Action: "received", UserID: command.UserID, At: now}}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updatedOrder); err != nil {
		return nil, ErrPurchaseOrderNotReady
	}
	if err := validation.Validate(&receipt); err != nil {
		return nil, err
	}
	if _, err := s.db.GoodsReceipts().InsertOne(ctx, receipt); err != nil {
		return nil, err
	}
	return &GoodsReceiptResult{Receipt: receipt, Lines: lines, Inventory: &Result{Posting: posting, Movement: movements[0], Balance: balances[0], Movements: movements, Balances: balances}}, nil
}

func (s *Service) ensureReceiptLot(ctx mongo.SessionContext, command GoodsReceiptCommand, line models.PurchaseOrderLine, input GoodsReceiptLineCommand, receivedAt time.Time) (*primitive.ObjectID, error) {
	itemMode, err := s.itemLotMode(ctx, command.TenantID, line.ItemID)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(input.LotCode)
	if itemMode == models.LotTrackingRequired && code == "" {
		return nil, ErrLotRequired
	}
	if code == "" {
		return nil, nil
	}
	var item models.Item
	if err := s.db.Items().FindOne(ctx, bson.M{"_id": line.ItemID, "tenantId": command.TenantID, "isActive": true, "stockable": true}).Decode(&item); err != nil {
		return nil, ErrInvalidReference
	}
	expires := input.ExpiresAt
	if expires == nil && item.ShelfLifeDays != nil && *item.ShelfLifeDays > 0 {
		value := receivedAt.AddDate(0, 0, int(*item.ShelfLifeDays))
		expires = &value
	}
	var existing models.StockLot
	err = s.db.StockLots().FindOne(ctx, bson.M{"tenantId": command.TenantID, "itemId": line.ItemID, "code": code}).Decode(&existing)
	if err == nil {
		if (existing.ExpiresAt == nil) != (expires == nil) || (existing.ExpiresAt != nil && !existing.ExpiresAt.Equal(*expires)) {
			return nil, ErrInvalidReference
		}
		return &existing.ID, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	lot := models.StockLot{ID: primitive.NewObjectID(), TenantID: command.TenantID, ItemID: line.ItemID, Code: code, ExpiresAt: expires, ReceivedAt: receivedAt, Status: models.LotStatusAvailable, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := validation.Validate(&lot); err != nil {
		return nil, err
	}
	if _, err := s.db.StockLots().InsertOne(ctx, lot); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDuplicateLot
		}
		return nil, err
	}
	return &lot.ID, nil
}

func (s *Service) findGoodsReceipt(ctx context.Context, tenantID primitive.ObjectID, key, hash string) (*GoodsReceiptResult, error) {
	var receipt models.GoodsReceipt
	err := s.db.GoodsReceipts().FindOne(ctx, bson.M{"tenantId": tenantID, "idempotencyKey": key}).Decode(&receipt)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if receipt.RequestHash != hash {
		return nil, ErrIdempotencyConflict
	}
	postingResult, err := s.findIdempotentMany(ctx, tenantID, receipt.IdempotencyKey, receipt.RequestHash)
	if err != nil {
		return nil, err
	}
	lines, err := loadGoodsReceiptLines(ctx, s, tenantID, receipt.ID)
	if err != nil {
		return nil, err
	}
	return &GoodsReceiptResult{Receipt: receipt, Lines: lines, Inventory: postingResult}, nil
}

func loadGoodsReceiptLines(ctx context.Context, s *Service, tenantID, receiptID primitive.ObjectID) ([]models.GoodsReceiptLine, error) {
	cur, err := s.db.GoodsReceiptLines().Find(ctx, bson.M{"tenantId": tenantID, "goodsReceiptId": receiptID}, options.Find().SetSort(bson.D{{Key: "lineNumber", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var lines []models.GoodsReceiptLine
	if err := cur.All(ctx, &lines); err != nil {
		return nil, err
	}
	return lines, nil
}

func (s *Service) ReverseGoodsReceipt(ctx context.Context, tenantID, userID, locationID, receiptID primitive.ObjectID, key string) (*GoodsReceiptResult, error) {
	if tenantID.IsZero() || userID.IsZero() || receiptID.IsZero() || len(strings.TrimSpace(key)) < 8 || len(key) > 128 {
		return nil, ErrInvalidReference
	}
	hash := receiptReverseHash(tenantID, userID, locationID, receiptID)
	var receipt models.GoodsReceipt
	if err := s.db.GoodsReceipts().FindOne(ctx, bson.M{"_id": receiptID, "tenantId": tenantID, "locationId": locationID}).Decode(&receipt); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrGoodsReceiptNotFound
		}
		return nil, err
	}
	if receipt.Status == models.GoodsReceiptCancelled {
		if receipt.ReversalIdempotencyKey == key && receipt.ReversalRequestHash == hash {
			posting, err := s.findIdempotentMany(ctx, tenantID, key, hash)
			if err != nil {
				return nil, err
			}
			lines, err := loadGoodsReceiptLines(ctx, s, tenantID, receipt.ID)
			if err != nil {
				return nil, err
			}
			return &GoodsReceiptResult{Receipt: receipt, Lines: lines, Inventory: posting}, nil
		}
		return nil, ErrPostingAlreadyReversed
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, ErrTransactionRequired
	}
	defer session.EndSession(ctx)
	var result *GoodsReceiptResult
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		if err := s.db.GoodsReceipts().FindOne(sc, bson.M{"_id": receiptID, "tenantId": tenantID, "status": models.GoodsReceiptPosted}).Decode(&receipt); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrGoodsReceiptState
			}
			return nil, err
		}
		postingResult, err := s.ReverseInTransaction(sc, ReverseCommand{TenantID: tenantID, UserID: userID, LocationID: locationID, PostingID: receipt.PostingID, IdempotencyKey: key})
		if err != nil {
			return nil, err
		}
		lines, err := loadGoodsReceiptLines(sc, s, tenantID, receipt.ID)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			updated, err := s.db.PurchaseOrderLines().UpdateOne(sc, bson.M{"_id": line.PurchaseOrderLineID, "tenantId": tenantID, "purchaseOrderId": receipt.PurchaseOrderID, "receivedQuantityMicros": bson.M{"$gte": line.ReceivedQuantityMicros}}, bson.M{"$inc": bson.M{"receivedQuantityMicros": -line.ReceivedQuantityMicros}, "$set": bson.M{"updatedAt": time.Now().UTC()}})
			if err != nil {
				return nil, err
			}
			if updated.MatchedCount != 1 {
				return nil, ErrGoodsReceiptState
			}
		}
		var order models.PurchaseOrder
		if err := s.db.PurchaseOrders().FindOne(sc, bson.M{"_id": receipt.PurchaseOrderID, "tenantId": tenantID}).Decode(&order); err != nil {
			return nil, err
		}
		cur, err := s.db.PurchaseOrderLines().Find(sc, bson.M{"tenantId": tenantID, "purchaseOrderId": order.ID})
		if err != nil {
			return nil, err
		}
		var orderLines []models.PurchaseOrderLine
		if err := cur.All(sc, &orderLines); err != nil {
			return nil, err
		}
		status := models.PurchaseOrderApproved
		any := false
		all := true
		for _, line := range orderLines {
			if line.ReceivedQuantityMicros > 0 {
				any = true
			}
			if line.ReceivedQuantityMicros < line.OrderedQuantityMicros {
				all = false
			}
		}
		if any {
			status = models.PurchaseOrderPartiallyReceived
		}
		if all && any {
			status = models.PurchaseOrderReceived
		}
		now := time.Now().UTC()
		if _, err := s.db.PurchaseOrders().UpdateOne(sc, bson.M{"_id": order.ID, "tenantId": tenantID, "version": order.Version}, bson.M{"$set": bson.M{"status": status, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}}); err != nil {
			return nil, err
		}
		receipt.Status = models.GoodsReceiptCancelled
		receipt.ReversalPostingID = &postingResult.Posting.ID
		receipt.ReversalIdempotencyKey = key
		receipt.ReversalRequestHash = hash
		receipt.UpdatedAt = now
		receipt.Audit = append(receipt.Audit, models.PurchaseOrderAuditEntry{Action: "cancelled", UserID: userID, At: now})
		if err := validation.Validate(&receipt); err != nil {
			return nil, err
		}
		updatedReceipt, err := s.db.GoodsReceipts().UpdateOne(sc, bson.M{"_id": receipt.ID, "tenantId": tenantID, "status": models.GoodsReceiptPosted}, bson.M{"$set": bson.M{"status": receipt.Status, "reversalPostingId": receipt.ReversalPostingID, "reversalIdempotencyKey": key, "reversalRequestHash": hash, "updatedAt": now}, "$push": bson.M{"audit": models.PurchaseOrderAuditEntry{Action: "cancelled", UserID: userID, At: now}}})
		if err != nil {
			return nil, err
		}
		if updatedReceipt.MatchedCount != 1 {
			return nil, ErrGoodsReceiptState
		}
		result = &GoodsReceiptResult{Receipt: receipt, Lines: lines, Inventory: postingResult}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) ListGoodsReceipts(ctx context.Context, tenantID, locationID, orderID primitive.ObjectID) ([]models.GoodsReceipt, error) {
	filter := bson.M{"tenantId": tenantID}
	if !locationID.IsZero() {
		filter["locationId"] = locationID
	}
	if !orderID.IsZero() {
		filter["purchaseOrderId"] = orderID
	}
	cur, err := s.db.GoodsReceipts().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "receivedAt", Value: -1}, {Key: "_id", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var receipts []models.GoodsReceipt
	if err := cur.All(ctx, &receipts); err != nil {
		return nil, err
	}
	return receipts, nil
}

func (s *Service) GetGoodsReceipt(ctx context.Context, tenantID, locationID, receiptID primitive.ObjectID) (*GoodsReceiptResult, error) {
	var receipt models.GoodsReceipt
	if err := s.db.GoodsReceipts().FindOne(ctx, bson.M{"_id": receiptID, "tenantId": tenantID, "locationId": locationID}).Decode(&receipt); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrGoodsReceiptNotFound
		}
		return nil, err
	}
	postingResult, err := s.findIdempotentMany(ctx, tenantID, receipt.IdempotencyKey, receipt.RequestHash)
	if err != nil {
		return nil, err
	}
	lines, err := loadGoodsReceiptLines(ctx, s, tenantID, receipt.ID)
	if err != nil {
		return nil, err
	}
	return &GoodsReceiptResult{Receipt: receipt, Lines: lines, Inventory: postingResult}, nil
}
