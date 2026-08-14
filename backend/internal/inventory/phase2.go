package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
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
	ErrLotNotFound                = errors.New("stock lot not found")
	ErrDuplicateLot               = errors.New("stock lot code already exists")
	ErrLotUnavailable             = errors.New("stock lot is not available for allocation")
	ErrLotRequired                = errors.New("a lot is required for this item")
	ErrLotOverrideRequired        = errors.New("explicit lot selection requires elevated permission and a reason")
	ErrLotTrackingUnsafe          = errors.New("lot tracking mode change is unsafe with existing balances")
	ErrLotTrackingDisabled        = errors.New("lot tracking is disabled for this item")
	ErrLotTrackingVersionConflict = errors.New("lot tracking version conflict")
	ErrTransferInvalid            = errors.New("transfer source and destination are invalid")

	lotNow = time.Now
)

type lineSpec struct {
	TenantID       primitive.ObjectID
	LocationID     primitive.ObjectID
	StorageAreaID  primitive.ObjectID
	ItemID         primitive.ObjectID
	LotID          *primitive.ObjectID
	QuantityMicros int64
}

type TransferCommand struct {
	TenantID                 primitive.ObjectID
	UserID                   primitive.ObjectID
	SourceLocationID         primitive.ObjectID
	SourceStorageAreaID      primitive.ObjectID
	DestinationLocationID    primitive.ObjectID
	DestinationStorageAreaID primitive.ObjectID
	ItemID                   primitive.ObjectID
	QuantityMicros           int64
	LotID                    *primitive.ObjectID
	LotOverride              bool
	Reason                   string
	IdempotencyKey           string
	EffectiveAt              time.Time
}

type WasteCommand struct {
	Command
}

type LotCommand struct {
	TenantID   primitive.ObjectID
	UserID     primitive.ObjectID
	ItemID     primitive.ObjectID
	Code       string
	ExpiresAt  *time.Time
	ReceivedAt time.Time
	Status     models.LotStatus
}

type TransferResult struct {
	Posting   models.StockPosting    `json:"posting"`
	Movements []models.StockMovement `json:"movements"`
	Balances  []models.StockBalance  `json:"balances"`
}

func (s *Service) needsLotHandling(ctx context.Context, command Command) bool {
	if command.LotID != nil {
		return true
	}
	var item models.Item
	if err := s.db.Items().FindOne(ctx, bson.M{"_id": command.ItemID, "tenantId": command.TenantID}, options.FindOne().SetProjection(bson.M{"lotTracking": 1})).Decode(&item); err != nil {
		return false
	}
	return item.LotTracking != models.LotTrackingDisabled && (command.QuantityMicros < 0 || item.LotTracking == models.LotTrackingRequired)
}

func (s *Service) postLotAware(ctx context.Context, command Command, kind models.StockPostingType) (*Result, error) {
	if err := validateCommand(command, kind, nil); err != nil {
		return nil, err
	}
	hash := commandHash(command, kind, nil)
	if existing, err := s.findIdempotentMany(ctx, command.TenantID, command.IdempotencyKey, hash); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}
	var lines []lineSpec
	if command.QuantityMicros >= 0 {
		lines = []lineSpec{{TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, ItemID: command.ItemID, LotID: command.LotID, QuantityMicros: command.QuantityMicros}}
	}
	return s.postLines(ctx, command.TenantID, command.UserID, command.LocationID, command.StorageAreaID, nil, nil, command.IdempotencyKey, command.EffectiveAt, kind, lines, &command, hash)
}

func (s *Service) Waste(ctx context.Context, command WasteCommand) (*Result, error) {
	if command.QuantityMicros == math.MinInt64 {
		return nil, ErrQuantityOverflow
	}
	command.QuantityMicros = -absQuantity(command.QuantityMicros)
	if command.QuantityMicros == 0 {
		return nil, fmt.Errorf("waste quantity must be positive")
	}
	if err := validateCommand(command.Command, models.StockPostingWaste, nil); err != nil {
		return nil, err
	}
	hash := commandHash(command.Command, models.StockPostingWaste, nil)
	if existing, err := s.findIdempotentMany(ctx, command.TenantID, command.IdempotencyKey, hash); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}
	return s.postLines(ctx, command.TenantID, command.UserID, command.LocationID, command.StorageAreaID, nil, nil, command.IdempotencyKey, command.EffectiveAt, models.StockPostingWaste, nil, &command.Command, hash)
}

func (s *Service) Transfer(ctx context.Context, command TransferCommand) (*TransferResult, error) {
	if command.QuantityMicros <= 0 || command.SourceLocationID.IsZero() || command.SourceStorageAreaID.IsZero() || command.DestinationLocationID.IsZero() || command.DestinationStorageAreaID.IsZero() || command.ItemID.IsZero() || command.SourceLocationID == command.DestinationLocationID && command.SourceStorageAreaID == command.DestinationStorageAreaID {
		return nil, ErrTransferInvalid
	}
	base := Command{TenantID: command.TenantID, UserID: command.UserID, LocationID: command.SourceLocationID, StorageAreaID: command.SourceStorageAreaID, ItemID: command.ItemID, QuantityMicros: -command.QuantityMicros, IdempotencyKey: command.IdempotencyKey, EffectiveAt: command.EffectiveAt, LotID: command.LotID, LotOverride: command.LotOverride, Reason: command.Reason}
	if err := validateCommand(base, models.StockPostingTransfer, nil); err != nil {
		return nil, err
	}
	hash := hashTransfer(command)
	if existing, err := s.findIdempotentMany(ctx, command.TenantID, command.IdempotencyKey, hash); err != nil {
		return nil, err
	} else if existing != nil {
		return &TransferResult{Posting: existing.Posting, Movements: existing.Movements, Balances: existing.Balances}, nil
	}
	result, err := s.postLines(ctx, command.TenantID, command.UserID, command.SourceLocationID, command.SourceStorageAreaID, &command.DestinationLocationID, &command.DestinationStorageAreaID, command.IdempotencyKey, command.EffectiveAt, models.StockPostingTransfer, nil, &base, hash)
	if err != nil {
		return nil, err
	}
	return &TransferResult{Posting: result.Posting, Movements: result.Movements, Balances: result.Balances}, nil
}

func absQuantity(value int64) int64 {
	// Callers reject MinInt64 before reaching this helper because its positive
	// magnitude cannot be represented as an int64.
	if value < 0 {
		return -value
	}
	return value
}

func (s *Service) itemLotMode(ctx context.Context, tenantID, itemID primitive.ObjectID) (models.LotTrackingMode, error) {
	var item models.Item
	if err := s.db.Items().FindOne(ctx, bson.M{"_id": itemID, "tenantId": tenantID}).Decode(&item); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return models.LotTrackingDisabled, ErrInvalidReference
		}
		return models.LotTrackingDisabled, err
	}
	return item.LotTracking, nil
}

func (s *Service) validateLotForQuantity(ctx context.Context, command Command, mode models.LotTrackingMode, quantity int64) error {
	if command.LotID == nil {
		if mode == models.LotTrackingRequired && quantity > 0 {
			return ErrLotRequired
		}
		return nil
	}
	if mode == models.LotTrackingDisabled {
		return ErrLotTrackingDisabled
	}
	var lot models.StockLot
	if err := s.db.StockLots().FindOne(ctx, bson.M{"_id": *command.LotID, "tenantId": command.TenantID, "itemId": command.ItemID}).Decode(&lot); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrLotNotFound
		}
		return err
	}
	if quantity < 0 {
		if !command.LotOverride || strings.TrimSpace(command.Reason) == "" {
			return ErrLotOverrideRequired
		}
		if lot.Status != models.LotStatusAvailable || lot.ExpiresAt != nil && !lot.ExpiresAt.After(lotNow().UTC()) {
			return ErrLotUnavailable
		}
	}
	return nil
}

type allocationCandidate struct {
	balance models.StockBalance
	lot     *models.StockLot
}

func (s *Service) allocateOutbound(ctx context.Context, command Command, mode models.LotTrackingMode) ([]lineSpec, error) {
	if command.QuantityMicros >= 0 {
		return nil, fmt.Errorf("outbound quantity must be negative")
	}
	if command.QuantityMicros == math.MinInt64 {
		return nil, ErrQuantityOverflow
	}
	if err := s.validateLotForQuantity(ctx, command, mode, command.QuantityMicros); err != nil {
		return nil, err
	}
	want := absQuantity(command.QuantityMicros)
	if command.LotID != nil {
		return []lineSpec{{TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, ItemID: command.ItemID, LotID: command.LotID, QuantityMicros: -want}}, nil
	}
	if mode == models.LotTrackingDisabled {
		return []lineSpec{{TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, ItemID: command.ItemID, QuantityMicros: -want}}, nil
	}
	cursor, err := s.db.StockBalances().Find(ctx, bson.M{"tenantId": command.TenantID, "locationId": command.LocationID, "storageAreaId": command.StorageAreaID, "itemId": command.ItemID, "quantityMicros": bson.M{"$gt": int64(0)}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var balances []models.StockBalance
	if err := cursor.All(ctx, &balances); err != nil {
		return nil, err
	}
	lotIDs := make([]primitive.ObjectID, 0)
	for _, balance := range balances {
		if balance.LotID != nil {
			lotIDs = append(lotIDs, *balance.LotID)
		}
	}
	lots := map[primitive.ObjectID]models.StockLot{}
	if len(lotIDs) > 0 {
		lotCursor, err := s.db.StockLots().Find(ctx, bson.M{"tenantId": command.TenantID, "itemId": command.ItemID, "_id": bson.M{"$in": lotIDs}})
		if err != nil {
			return nil, err
		}
		var found []models.StockLot
		if err := lotCursor.All(ctx, &found); err != nil {
			return nil, err
		}
		for _, lot := range found {
			lots[lot.ID] = lot
		}
	}
	candidates := make([]allocationCandidate, 0, len(balances))
	for _, balance := range balances {
		if balance.LotID == nil {
			if mode == models.LotTrackingRequired {
				continue
			}
			candidates = append(candidates, allocationCandidate{balance: balance})
			continue
		}
		lot, ok := lots[*balance.LotID]
		if !ok || lot.Status != models.LotStatusAvailable || lot.ExpiresAt != nil && !lot.ExpiresAt.After(lotNow().UTC()) {
			continue
		}
		lotCopy := lot
		candidates = append(candidates, allocationCandidate{balance: balance, lot: &lotCopy})
	}
	sortAllocationCandidates(candidates)
	lines := make([]lineSpec, 0, len(candidates))
	for _, candidate := range candidates {
		if want <= 0 {
			break
		}
		amount := candidate.balance.QuantityMicros
		if amount > want {
			amount = want
		}
		var lotID *primitive.ObjectID
		if candidate.lot != nil {
			id := candidate.lot.ID
			lotID = &id
		}
		lines = append(lines, lineSpec{TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, ItemID: command.ItemID, LotID: lotID, QuantityMicros: -amount})
		want -= amount
	}
	if want != 0 {
		return nil, ErrInsufficientStock
	}
	return lines, nil
}

func sortAllocationCandidates(candidates []allocationCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i].lot, candidates[j].lot
		if a == nil && b != nil {
			return false
		}
		if a != nil && b == nil {
			return true
		}
		if a != nil && b != nil {
			if (a.ExpiresAt == nil) != (b.ExpiresAt == nil) {
				return a.ExpiresAt != nil
			}
			if a.ExpiresAt != nil && !a.ExpiresAt.Equal(*b.ExpiresAt) {
				return a.ExpiresAt.Before(*b.ExpiresAt)
			}
			if !a.ReceivedAt.Equal(b.ReceivedAt) {
				return a.ReceivedAt.Before(b.ReceivedAt)
			}
			return a.ID.Hex() < b.ID.Hex()
		}
		return a == nil && b == nil && candidates[i].balance.ID.Hex() < candidates[j].balance.ID.Hex()
	})
}

func (s *Service) postLines(ctx context.Context, tenantID, userID, locationID, areaID primitive.ObjectID, destinationLocationID, destinationAreaID *primitive.ObjectID, key string, effective time.Time, kind models.StockPostingType, lines []lineSpec, command *Command, hash string) (*Result, error) {
	if existing, err := s.findIdempotentMany(ctx, tenantID, key, hash); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, fmt.Errorf("start inventory transaction: %w", err)
	}
	defer session.EndSession(ctx)
	var result *Result
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		if existing, lookupErr := s.findIdempotentMany(sc, tenantID, key, hash); lookupErr != nil {
			return nil, lookupErr
		} else if existing != nil {
			result = existing
			return nil, nil
		}
		if kind == models.StockPostingTransfer {
			if destinationLocationID == nil || destinationAreaID == nil {
				return nil, ErrTransferInvalid
			}
		}
		var allocated []lineSpec
		if command != nil && command.QuantityMicros < 0 {
			mode, err := s.itemLotMode(sc, tenantID, command.ItemID)
			if err != nil {
				return nil, err
			}
			allocated, err = s.allocateOutbound(sc, *command, mode)
			if err != nil {
				return nil, err
			}
			if kind == models.StockPostingTransfer {
				lines = make([]lineSpec, 0, len(allocated)*2)
				for _, line := range allocated {
					lines = append(lines, lineSpec{TenantID: tenantID, LocationID: line.LocationID, StorageAreaID: line.StorageAreaID, ItemID: line.ItemID, LotID: line.LotID, QuantityMicros: line.QuantityMicros})
					lines = append(lines, lineSpec{TenantID: tenantID, LocationID: *destinationLocationID, StorageAreaID: *destinationAreaID, ItemID: line.ItemID, LotID: line.LotID, QuantityMicros: -line.QuantityMicros})
				}
			} else {
				lines = allocated
			}
		} else if command != nil {
			mode, err := s.itemLotMode(sc, tenantID, command.ItemID)
			if err != nil {
				return nil, err
			}
			if err := s.validateLotForQuantity(sc, *command, mode, command.QuantityMicros); err != nil {
				return nil, err
			}
		}
		if command != nil {
			for _, line := range lines {
				if err := s.verifyReferences(sc, Command{TenantID: tenantID, LocationID: line.LocationID, StorageAreaID: line.StorageAreaID, ItemID: line.ItemID}); err != nil {
					return nil, err
				}
			}
		}
		if len(lines) == 0 {
			return nil, ErrInsufficientStock
		}
		scopes := make([]inventoryScope, 0, len(lines)+2)
		scopes = append(scopes, inventoryScope{TenantID: tenantID, LocationID: locationID, StorageAreaID: areaID})
		if destinationLocationID != nil && destinationAreaID != nil {
			scopes = append(scopes, inventoryScope{TenantID: tenantID, LocationID: *destinationLocationID, StorageAreaID: *destinationAreaID})
		}
		for _, line := range lines {
			scopes = append(scopes, inventoryScope{TenantID: line.TenantID, LocationID: line.LocationID, StorageAreaID: line.StorageAreaID})
		}
		if err := s.fenceInventoryScopes(sc, scopes...); err != nil {
			return nil, err
		}
		if command != nil && command.QuantityMicros < 0 {
			if s.TestAfterLotAllocationHook != nil {
				if err := s.TestAfterLotAllocationHook(sc); err != nil {
					return nil, err
				}
			}
			// Allocation reads lot status as part of FEFO selection. Turn that
			// read into a conditional write in the same transaction so a lot
			// quarantine cannot commit between selection and consumption. The
			// storage-area fence is acquired before this mutation.
			if err := s.guardAllocatedLots(sc, tenantID, command.ItemID, allocated); err != nil {
				return nil, err
			}
		}
		now := time.Now().UTC()
		if effective.IsZero() {
			effective = now
		}
		reason := ""
		if command != nil {
			reason = strings.TrimSpace(command.Reason)
		}
		posting := models.StockPosting{ID: primitive.NewObjectID(), TenantID: tenantID, LocationID: locationID, StorageAreaID: areaID, UserID: userID, Type: kind, IdempotencyKey: key, RequestHash: hash, EffectiveAt: effective.UTC(), RecordedAt: now, Reason: reason, DestinationLocationID: destinationLocationID, DestinationStorageAreaID: destinationAreaID}
		if err := validation.Validate(&posting); err != nil {
			return nil, err
		}
		if _, err := s.db.StockPostings().InsertOne(sc, posting); err != nil {
			return nil, err
		}
		movements := make([]models.StockMovement, 0, len(lines))
		balances := make([]models.StockBalance, 0, len(lines))
		for number, line := range lines {
			movement := models.StockMovement{ID: primitive.NewObjectID(), PostingID: posting.ID, TenantID: tenantID, LocationID: line.LocationID, StorageAreaID: line.StorageAreaID, ItemID: line.ItemID, LotID: line.LotID, LineNumber: int32(number), QuantityMicros: line.QuantityMicros, EffectiveAt: effective.UTC(), RecordedAt: now}
			if err := validation.Validate(&movement); err != nil {
				return nil, err
			}
			if _, err := s.db.StockMovements().InsertOne(sc, movement); err != nil {
				return nil, err
			}
			balance, err := s.applyBalanceLine(sc, line)
			if err != nil {
				return nil, err
			}
			movements = append(movements, movement)
			balances = append(balances, *balance)
		}
		// Posting, movements, and projection updates are all in the same
		// transaction; a failure rolls back every line and balance.
		if s.TestAfterPostingInsertHook != nil {
			if err := s.TestAfterPostingInsertHook(sc); err != nil {
				return nil, err
			}
		}
		result = &Result{Posting: posting, Movement: movements[0], Balance: balances[0], Movements: movements, Balances: balances}
		return nil, nil
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			if existing, lookupErr := s.findIdempotentMany(ctx, tenantID, key, hash); lookupErr == nil && existing != nil {
				return existing, nil
			}
		}
		return nil, err
	}
	return result, nil
}

func (s *Service) guardAllocatedLots(ctx mongo.SessionContext, tenantID, itemID primitive.ObjectID, lines []lineSpec) error {
	now := lotNow().UTC()
	for _, line := range lines {
		if line.LotID == nil {
			continue
		}
		filter := allocatedLotGuardFilter(tenantID, itemID, *line.LotID, now)
		// BSON dates have millisecond precision. Assigning the wall clock here
		// can therefore be a no-op, which would not establish a write/write
		// conflict with a concurrent lot status update. Advance the durable lot
		// state from the later of server time and its stored value plus 1ms; the
		// pipeline always changes it without leaving a stale timestamp behind.
		updated, err := s.db.StockLots().UpdateOne(ctx, filter, allocatedLotGuardUpdate())
		if err != nil {
			return err
		}
		if updated.MatchedCount == 0 {
			return ErrLotUnavailable
		}
	}
	return nil
}

func allocatedLotGuardUpdate() mongo.Pipeline {
	return mongo.Pipeline{
		bson.D{{
			Key: "$set",
			Value: bson.M{
				"updatedAt": bson.M{"$max": bson.A{
					"$$NOW",
					bson.M{"$dateAdd": bson.M{
						"startDate": "$updatedAt",
						"unit":      "millisecond",
						"amount":    int64(1),
					}},
				}},
			},
		}},
	}
}

func allocatedLotGuardFilter(tenantID, itemID, lotID primitive.ObjectID, now time.Time) bson.M {
	return bson.M{
		"_id":      lotID,
		"tenantId": tenantID,
		"itemId":   itemID,
		"status":   models.LotStatusAvailable,
		"$or": bson.A{
			bson.M{"expiresAt": bson.M{"$exists": false}},
			bson.M{"expiresAt": nil},
			bson.M{"expiresAt": bson.M{"$gt": now}},
		},
	}
}

func (s *Service) applyBalanceLine(ctx mongo.SessionContext, line lineSpec) (*models.StockBalance, error) {
	key := bson.M{"tenantId": line.TenantID, "locationId": line.LocationID, "storageAreaId": line.StorageAreaID, "itemId": line.ItemID}
	if line.LotID == nil {
		key["lotId"] = nil
	} else {
		key["lotId"] = *line.LotID
	}
	now := time.Now().UTC()
	if line.QuantityMicros < 0 {
		if line.QuantityMicros == -1<<63 {
			return nil, ErrQuantityOverflow
		}
		filter := bson.M{"tenantId": line.TenantID, "locationId": line.LocationID, "storageAreaId": line.StorageAreaID, "itemId": line.ItemID, "quantityMicros": bson.M{"$gte": -line.QuantityMicros}}
		if line.LotID == nil {
			filter["lotId"] = nil
		} else {
			filter["lotId"] = *line.LotID
		}
		updated, err := s.db.StockBalances().UpdateOne(ctx, filter, bson.M{"$inc": bson.M{"quantityMicros": line.QuantityMicros}, "$set": bson.M{"updatedAt": now}})
		if err != nil {
			return nil, err
		}
		if updated.MatchedCount == 0 {
			return nil, ErrInsufficientStock
		}
	} else {
		setOnInsert := bson.M{"_id": primitive.NewObjectID(), "tenantId": line.TenantID, "locationId": line.LocationID, "storageAreaId": line.StorageAreaID, "itemId": line.ItemID}
		if line.LotID != nil {
			setOnInsert["lotId"] = *line.LotID
		}
		if _, err := s.db.StockBalances().UpdateOne(ctx, key, bson.M{"$inc": bson.M{"quantityMicros": line.QuantityMicros}, "$set": bson.M{"updatedAt": now}, "$setOnInsert": setOnInsert}, options.Update().SetUpsert(true)); err != nil {
			return nil, err
		}
	}
	return s.currentBalanceForLot(ctx, line.TenantID, line.LocationID, line.StorageAreaID, line.ItemID, line.LotID)
}

func (s *Service) currentBalanceForLot(ctx context.Context, tenantID, locationID, areaID, itemID primitive.ObjectID, lotID *primitive.ObjectID) (*models.StockBalance, error) {
	filter := bson.M{"tenantId": tenantID, "locationId": locationID, "storageAreaId": areaID, "itemId": itemID}
	if lotID == nil {
		filter["lotId"] = nil
	} else {
		filter["lotId"] = *lotID
	}
	var balance models.StockBalance
	if err := s.db.StockBalances().FindOne(ctx, filter).Decode(&balance); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return &models.StockBalance{TenantID: tenantID, LocationID: locationID, StorageAreaID: areaID, ItemID: itemID, LotID: lotID}, nil
		}
		return nil, err
	}
	return &balance, nil
}

func (s *Service) findIdempotentMany(ctx context.Context, tenantID primitive.ObjectID, key, hash string) (*Result, error) {
	var posting models.StockPosting
	if err := s.db.StockPostings().FindOne(ctx, bson.M{"tenantId": tenantID, "idempotencyKey": key}).Decode(&posting); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	if posting.RequestHash != hash {
		return nil, ErrIdempotencyConflict
	}
	cursor, err := s.db.StockMovements().Find(ctx, bson.M{"tenantId": tenantID, "postingId": posting.ID}, options.Find().SetSort(bson.D{{Key: "lineNumber", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var movements []models.StockMovement
	if err := cursor.All(ctx, &movements); err != nil {
		return nil, err
	}
	if len(movements) == 0 {
		return nil, ErrPostingNotFound
	}
	balances := make([]models.StockBalance, 0, len(movements))
	for _, movement := range movements {
		balance, err := s.currentBalanceForLot(ctx, tenantID, movement.LocationID, movement.StorageAreaID, movement.ItemID, movement.LotID)
		if err != nil {
			return nil, err
		}
		balances = append(balances, *balance)
	}
	return &Result{Posting: posting, Movement: movements[0], Balance: balances[0], Movements: movements, Balances: balances}, nil
}

func hashTransfer(command TransferCommand) string {
	encoded, _ := json.Marshal(command)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (s *Service) reverseMany(ctx context.Context, command ReverseCommand) (*Result, error) {
	hash := commandHash(Command{TenantID: command.TenantID, UserID: command.UserID, LocationID: command.LocationID}, models.StockPostingReversal, &command.PostingID)
	if existing, err := s.findIdempotentMany(ctx, command.TenantID, command.IdempotencyKey, hash); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)
	var result *Result
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		if existing, lookupErr := s.findIdempotentMany(sc, command.TenantID, command.IdempotencyKey, hash); lookupErr != nil {
			return nil, lookupErr
		} else if existing != nil {
			result = existing
			return nil, nil
		}
		var original models.StockPosting
		if err := s.db.StockPostings().FindOne(sc, bson.M{"_id": command.PostingID, "tenantId": command.TenantID}).Decode(&original); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrPostingNotFound
			}
			return nil, err
		}
		if !command.LocationID.IsZero() && original.LocationID != command.LocationID {
			return nil, ErrPostingNotFound
		}
		if original.Type == models.StockPostingReversal {
			return nil, ErrInvalidReference
		}
		var prior models.StockPosting
		if err := s.db.StockPostings().FindOne(sc, bson.M{"tenantId": command.TenantID, "reversalOf": original.ID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Decode(&prior); err == nil {
			return nil, ErrPostingAlreadyReversed
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		cursor, err := s.db.StockMovements().Find(sc, bson.M{"tenantId": command.TenantID, "postingId": original.ID}, options.Find().SetSort(bson.D{{Key: "lineNumber", Value: 1}, {Key: "_id", Value: 1}}))
		if err != nil {
			return nil, err
		}
		var originals []models.StockMovement
		if err := cursor.All(sc, &originals); err != nil {
			return nil, err
		}
		if len(originals) == 0 {
			return nil, ErrPostingNotFound
		}
		lines := make([]lineSpec, 0, len(originals))
		for _, movement := range originals {
			if movement.TenantID != command.TenantID {
				return nil, ErrInvalidReference
			}
			if movement.QuantityMicros == -1<<63 {
				return nil, ErrQuantityOverflow
			}
			lines = append(lines, lineSpec{TenantID: command.TenantID, LocationID: movement.LocationID, StorageAreaID: movement.StorageAreaID, ItemID: movement.ItemID, LotID: movement.LotID, QuantityMicros: -movement.QuantityMicros})
		}
		scopes := make([]inventoryScope, 0, len(lines))
		for _, line := range lines {
			scopes = append(scopes, inventoryScope{TenantID: line.TenantID, LocationID: line.LocationID, StorageAreaID: line.StorageAreaID})
		}
		if err := s.fenceInventoryScopes(sc, scopes...); err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		posting := models.StockPosting{ID: primitive.NewObjectID(), TenantID: command.TenantID, LocationID: original.LocationID, StorageAreaID: original.StorageAreaID, UserID: command.UserID, Type: models.StockPostingReversal, IdempotencyKey: command.IdempotencyKey, RequestHash: hash, EffectiveAt: now, RecordedAt: now, ReversalOf: &original.ID, DestinationLocationID: original.DestinationLocationID, DestinationStorageAreaID: original.DestinationStorageAreaID}
		if err := validation.Validate(&posting); err != nil {
			return nil, err
		}
		if _, err := s.db.StockPostings().InsertOne(sc, posting); err != nil {
			return nil, err
		}
		movements := make([]models.StockMovement, 0, len(lines))
		balances := make([]models.StockBalance, 0, len(lines))
		for number, line := range lines {
			movement := models.StockMovement{ID: primitive.NewObjectID(), PostingID: posting.ID, TenantID: command.TenantID, LocationID: line.LocationID, StorageAreaID: line.StorageAreaID, ItemID: line.ItemID, LotID: line.LotID, LineNumber: int32(number), QuantityMicros: line.QuantityMicros, EffectiveAt: now, RecordedAt: now}
			if err := validation.Validate(&movement); err != nil {
				return nil, err
			}
			if _, err := s.db.StockMovements().InsertOne(sc, movement); err != nil {
				return nil, err
			}
			balance, err := s.applyBalanceLine(sc, line)
			if err != nil {
				return nil, err
			}
			movements = append(movements, movement)
			balances = append(balances, *balance)
		}
		result = &Result{Posting: posting, Movement: movements[0], Balance: balances[0], Movements: movements, Balances: balances}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) CreateLot(ctx context.Context, command LotCommand) (*models.StockLot, error) {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.ItemID.IsZero() {
		return nil, ErrInvalidReference
	}
	var item models.Item
	if err := s.db.Items().FindOne(ctx, bson.M{"_id": command.ItemID, "tenantId": command.TenantID, "isActive": true, "stockable": true}).Decode(&item); err != nil {
		return nil, ErrInvalidReference
	}
	now := time.Now().UTC()
	received := command.ReceivedAt.UTC()
	if received.IsZero() {
		received = now
	}
	status := command.Status
	if status == "" {
		status = models.LotStatusAvailable
	}
	lot := models.StockLot{ID: primitive.NewObjectID(), TenantID: command.TenantID, ItemID: command.ItemID, Code: strings.TrimSpace(command.Code), ExpiresAt: command.ExpiresAt, ReceivedAt: received, Status: status, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&lot); err != nil {
		return nil, err
	}
	if _, err := s.db.StockLots().InsertOne(ctx, lot); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrDuplicateLot
		}
		return nil, err
	}
	return &lot, nil
}

func (s *Service) ListLots(ctx context.Context, tenantID, itemID primitive.ObjectID) ([]models.StockLot, error) {
	cursor, err := s.db.StockLots().Find(ctx, bson.M{"tenantId": tenantID, "itemId": itemID}, options.Find().SetSort(bson.D{{Key: "receivedAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var lots []models.StockLot
	if err := cursor.All(ctx, &lots); err != nil {
		return nil, err
	}
	return lots, nil
}

func (s *Service) UpdateLotStatus(ctx context.Context, tenantID, itemID, lotID primitive.ObjectID, status models.LotStatus) (*models.StockLot, error) {
	if !models.ValidLotStatus(status) {
		return nil, fmt.Errorf("validation failed: invalid lot status")
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, fmt.Errorf("start lot-status transaction: %w", err)
	}
	defer session.EndSession(ctx)
	var updated *models.StockLot
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		var lot models.StockLot
		err := s.db.StockLots().FindOneAndUpdate(sc,
			bson.M{"_id": lotID, "tenantId": tenantID, "itemId": itemID},
			bson.M{"$set": bson.M{"status": status, "updatedAt": lotNow().UTC()}},
			options.FindOneAndUpdate().SetReturnDocument(options.After),
		).Decode(&lot)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrLotNotFound
		}
		if err != nil {
			return nil, err
		}
		updated = &lot
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) SetLotTrackingMode(ctx context.Context, tenantID, itemID primitive.ObjectID, mode models.LotTrackingMode, version int64) (*models.Item, error) {
	if !models.ValidLotTrackingMode(mode) {
		return nil, fmt.Errorf("validation failed: invalid lot tracking mode")
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, fmt.Errorf("start lot-tracking transaction: %w", err)
	}
	defer session.EndSession(ctx)
	var updated *models.Item
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		var item models.Item
		if err := s.db.Items().FindOne(sc, bson.M{"_id": itemID, "tenantId": tenantID}).Decode(&item); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrInvalidReference
			}
			return nil, err
		}
		if !item.Stockable {
			return nil, ErrInvalidReference
		}
		if item.LotTracking == mode {
			if item.Version != version {
				return nil, ErrLotTrackingVersionConflict
			}
			updated = &item
			return nil, nil
		}
		positive := bson.M{"tenantId": tenantID, "itemId": itemID, "quantityMicros": bson.M{"$gt": int64(0)}}
		if mode == models.LotTrackingRequired && item.LotTracking != models.LotTrackingRequired {
			positive["$or"] = bson.A{bson.M{"lotId": bson.M{"$exists": false}}, bson.M{"lotId": nil}}
			if count, err := s.db.StockBalances().CountDocuments(sc, positive); err != nil {
				return nil, err
			} else if count > 0 {
				return nil, ErrLotTrackingUnsafe
			}
		}
		if mode == models.LotTrackingDisabled && item.LotTracking != models.LotTrackingDisabled {
			positive["lotId"] = bson.M{"$exists": true, "$ne": nil}
			delete(positive, "$or")
			if count, err := s.db.StockBalances().CountDocuments(sc, positive); err != nil {
				return nil, err
			} else if count > 0 {
				return nil, ErrLotTrackingUnsafe
			}
		}
		var result models.Item
		err := s.db.Items().FindOneAndUpdate(sc, bson.M{"_id": itemID, "tenantId": tenantID, "version": version}, bson.M{"$set": bson.M{"lotTracking": mode, "updatedAt": time.Now().UTC()}, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&result)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrLotTrackingVersionConflict
		}
		if err != nil {
			return nil, err
		}
		updated = &result
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}
