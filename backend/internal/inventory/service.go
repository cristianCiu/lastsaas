// Package inventory contains the Phase 1 append-only inventory ledger kernel.
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
	"strconv"
	"strings"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/models"
	"lastsaas/internal/syslog"
	"lastsaas/internal/validation"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const QuantityScale int64 = 1_000_000

var (
	ErrIdempotencyConflict    = errors.New("idempotency key was already used for a different command")
	ErrInsufficientStock      = errors.New("insufficient stock")
	ErrPostingNotFound        = errors.New("stock posting not found")
	ErrPostingAlreadyReversed = errors.New("stock posting is already reversed")
	ErrInvalidReference       = errors.New("inventory reference is invalid")
	ErrQuantityOverflow       = errors.New("inventory quantity overflow")
	ErrTransactionRequired    = errors.New("inventory writes require MongoDB transactions")
	ErrInventoryAreaLocked    = errors.New("inventory storage area is locked by an active stock count")
)

// ParseQuantity parses the public decimal-string quantity into millionths of
// a base unit. Exponents, NaN, infinities, and more than six fractional digits
// are deliberately rejected rather than rounded.
func ParseQuantity(value string) (int64, error) {
	s := strings.TrimSpace(value)
	if s == "" {
		return 0, fmt.Errorf("quantity is required")
	}
	negative := false
	if s[0] == '+' || s[0] == '-' {
		negative = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return 0, fmt.Errorf("quantity must be a decimal")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("quantity must be a decimal")
	}
	if len(parts) == 2 && parts[1] == "" {
		return 0, fmt.Errorf("quantity must be a decimal")
	}
	if len(parts) == 1 {
		parts = append(parts, "")
	}
	if len(parts[1]) > 6 {
		return 0, fmt.Errorf("quantity supports at most six fractional digits")
	}
	for _, part := range parts {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("quantity must be a decimal")
			}
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > math.MaxInt64/QuantityScale {
		return 0, ErrQuantityOverflow
	}
	fraction := parts[1] + strings.Repeat("0", 6-len(parts[1]))
	frac, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, ErrQuantityOverflow
	}
	if whole == math.MaxInt64/QuantityScale && frac > math.MaxInt64%QuantityScale {
		return 0, ErrQuantityOverflow
	}
	result := whole*QuantityScale + frac
	if result < 0 || (negative && result != 0) && result == math.MinInt64 {
		return 0, ErrQuantityOverflow
	}
	if negative {
		result = -result
	}
	return result, nil
}

func FormatQuantity(quantityMicros int64) string {
	if quantityMicros == math.MinInt64 {
		return "-9223372036854.775808"
	}
	negative := quantityMicros < 0
	if negative {
		quantityMicros = -quantityMicros
	}
	whole := quantityMicros / QuantityScale
	frac := quantityMicros % QuantityScale
	if frac == 0 {
		if negative {
			return "-" + strconv.FormatInt(whole, 10)
		}
		return strconv.FormatInt(whole, 10)
	}
	result := fmt.Sprintf("%d.%06d", whole, frac)
	result = strings.TrimRight(strings.TrimRight(result, "0"), ".")
	if negative {
		return "-" + result
	}
	return result
}

type Command struct {
	TenantID       primitive.ObjectID
	UserID         primitive.ObjectID
	LocationID     primitive.ObjectID
	StorageAreaID  primitive.ObjectID
	ItemID         primitive.ObjectID
	QuantityMicros int64
	IdempotencyKey string
	EffectiveAt    time.Time
	LotID          *primitive.ObjectID
	LotOverride    bool
	Reason         string
}

type ReverseCommand struct {
	TenantID       primitive.ObjectID
	UserID         primitive.ObjectID
	LocationID     primitive.ObjectID
	PostingID      primitive.ObjectID
	IdempotencyKey string
}

type OpeningBalanceCommand = Command
type AdjustmentCommand = Command
type ReversalCommand = ReverseCommand

type Result struct {
	Posting   models.StockPosting    `json:"posting"`
	Movement  models.StockMovement   `json:"movement"`
	Balance   models.StockBalance    `json:"balance"`
	Movements []models.StockMovement `json:"movements,omitempty"`
	Balances  []models.StockBalance  `json:"balances,omitempty"`
}

type JournalEntry struct {
	Posting  models.StockPosting  `json:"posting"`
	Movement models.StockMovement `json:"movement"`
}

type Service struct {
	db     *db.MongoDB
	logger *syslog.Logger

	// TestAfterLotAllocationHook is intentionally inert in production. It lets
	// package integration tests arrange a lot-status write after FEFO selection
	// and before the guarded lot write.
	TestAfterLotAllocationHook func(context.Context) error

	// TestAfterPostingInsertHook is intentionally inert in production. It lets
	// package tests force an error after immutable rows are inserted and prove
	// that the transaction rolls them back.
	TestAfterPostingInsertHook func(context.Context) error
}

type inventoryScope struct {
	TenantID      primitive.ObjectID
	LocationID    primitive.ObjectID
	StorageAreaID primitive.ObjectID
}

// fenceInventoryScopes establishes a transaction-local write conflict for each
// distinct storage area. Active, unowned areas are incremented; any existing
// stock-count owner is rejected even if the area has since been deactivated.
// The stable order prevents multi-area writers from taking these locks in
// different orders.
func (s *Service) fenceInventoryScopes(ctx mongo.SessionContext, scopes ...inventoryScope) error {
	if len(scopes) == 0 {
		return nil
	}
	unique := make(map[inventoryScope]struct{}, len(scopes))
	distinct := make([]inventoryScope, 0, len(scopes))
	for _, scope := range scopes {
		if _, exists := unique[scope]; exists {
			continue
		}
		unique[scope] = struct{}{}
		distinct = append(distinct, scope)
	}
	sort.Slice(distinct, func(i, j int) bool {
		if distinct[i].LocationID != distinct[j].LocationID {
			return distinct[i].LocationID.Hex() < distinct[j].LocationID.Hex()
		}
		if distinct[i].StorageAreaID != distinct[j].StorageAreaID {
			return distinct[i].StorageAreaID.Hex() < distinct[j].StorageAreaID.Hex()
		}
		return distinct[i].TenantID.Hex() < distinct[j].TenantID.Hex()
	})

	for _, scope := range distinct {
		filter := bson.M{
			"_id":        scope.StorageAreaID,
			"tenantId":   scope.TenantID,
			"locationId": scope.LocationID,
			"isActive":   true,
			"$or": bson.A{
				bson.M{"activeStockCountId": bson.M{"$exists": false}},
				bson.M{"activeStockCountId": nil},
			},
		}
		updated, err := s.db.StorageAreas().UpdateOne(ctx, filter, bson.M{"$inc": bson.M{"inventoryFence": int64(1)}})
		if err != nil {
			return err
		}
		if updated.MatchedCount != 0 {
			continue
		}

		// An inactive area without an owner is intentionally ignored here:
		// reversals retain their existing ability to restore historical postings.
		// An owner always wins, including on an area that was deactivated after
		// the count claimed it.
		var area struct {
			IsActive           bool                `bson:"isActive"`
			ActiveStockCountID *primitive.ObjectID `bson:"activeStockCountId"`
		}
		err = s.db.StorageAreas().FindOne(ctx, bson.M{"_id": scope.StorageAreaID, "tenantId": scope.TenantID, "locationId": scope.LocationID}).Decode(&area)
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			return err
		}
		if area.ActiveStockCountID != nil {
			return ErrInventoryAreaLocked
		}
		if !area.IsActive {
			continue
		}
	}
	return nil
}

func NewService(database *db.MongoDB) *Service { return &Service{db: database} }

func (s *Service) OpeningBalance(ctx context.Context, command Command) (*Result, error) {
	if command.QuantityMicros <= 0 {
		return nil, fmt.Errorf("opening balance quantity must be positive")
	}
	// Route every new posting through the lot-aware transactional path. It also
	// handles disabled lot tracking, while resolving the current item mode in
	// the transaction so a mode change cannot bypass lot requirements.
	return s.postLotAware(ctx, command, models.StockPostingOpeningBalance)
}

func (s *Service) PostOpeningBalance(ctx context.Context, command OpeningBalanceCommand) (*Result, error) {
	return s.OpeningBalance(ctx, command)
}

func (s *Service) ManualAdjustment(ctx context.Context, command Command) (*Result, error) {
	if command.QuantityMicros == 0 {
		return nil, fmt.Errorf("adjustment quantity must not be zero")
	}
	return s.postLotAware(ctx, command, models.StockPostingAdjustment)
}

func (s *Service) PostManualAdjustment(ctx context.Context, command AdjustmentCommand) (*Result, error) {
	return s.ManualAdjustment(ctx, command)
}

func (s *Service) Reverse(ctx context.Context, command ReverseCommand) (*Result, error) {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.PostingID.IsZero() {
		return nil, ErrInvalidReference
	}
	if len(command.IdempotencyKey) < 8 || len(command.IdempotencyKey) > 128 {
		return nil, fmt.Errorf("invalid idempotency key")
	}
	return s.reverseMany(ctx, command)
}

func (s *Service) ReversePosting(ctx context.Context, command ReversalCommand) (*Result, error) {
	return s.Reverse(ctx, command)
}

func (s *Service) post(ctx context.Context, command Command, kind models.StockPostingType, reversalOf *primitive.ObjectID) (*Result, error) {
	if err := validateCommand(command, kind, reversalOf); err != nil {
		return nil, err
	}
	hash := commandHash(command, kind, reversalOf)
	if existing, err := s.findIdempotent(ctx, command.TenantID, command.IdempotencyKey, hash); err != nil {
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
		if existing, lookupErr := s.findIdempotent(sc, command.TenantID, command.IdempotencyKey, hash); lookupErr != nil {
			return nil, lookupErr
		} else if existing != nil {
			result = existing
			return nil, nil
		}

		var quantity int64
		var posting models.StockPosting
		var movement models.StockMovement
		now := time.Now().UTC()
		if kind == models.StockPostingReversal {
			var original models.StockPosting
			if err := s.db.StockPostings().FindOne(sc, bson.M{"_id": *reversalOf, "tenantId": command.TenantID}).Decode(&original); err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, ErrPostingNotFound
				}
				return nil, fmt.Errorf("load posting for reversal: %w", err)
			}
			if !command.LocationID.IsZero() && original.LocationID != command.LocationID {
				return nil, ErrPostingNotFound
			}
			if original.Type == models.StockPostingReversal {
				return nil, fmt.Errorf("%w: reversal postings cannot be reversed", ErrInvalidReference)
			}
			var prior models.StockPosting
			err := s.db.StockPostings().FindOne(sc, bson.M{"tenantId": command.TenantID, "reversalOf": original.ID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Decode(&prior)
			if err == nil {
				return nil, ErrPostingAlreadyReversed
			}
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return nil, fmt.Errorf("check posting reversal: %w", err)
			}
			var originalMovement models.StockMovement
			if err := s.db.StockMovements().FindOne(sc, bson.M{"postingId": original.ID, "tenantId": command.TenantID}).Decode(&originalMovement); err != nil {
				return nil, fmt.Errorf("load movement for reversal: %w", err)
			}
			if originalMovement.LocationID != original.LocationID || originalMovement.StorageAreaID != original.StorageAreaID {
				return nil, fmt.Errorf("%w: posting and movement scope mismatch", ErrInvalidReference)
			}
			command.LocationID, command.StorageAreaID, command.ItemID = original.LocationID, original.StorageAreaID, originalMovement.ItemID
			quantity = -originalMovement.QuantityMicros
			if originalMovement.QuantityMicros == math.MinInt64 {
				return nil, ErrQuantityOverflow
			}
		} else {
			quantity = command.QuantityMicros
			if err := s.verifyReferences(sc, command); err != nil {
				return nil, err
			}
		}
		if err := s.fenceInventoryScopes(sc, inventoryScope{TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID}); err != nil {
			return nil, err
		}
		effective := command.EffectiveAt.UTC()
		if effective.IsZero() {
			effective = now
		}
		posting = models.StockPosting{ID: primitive.NewObjectID(), TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, UserID: command.UserID, Type: kind, IdempotencyKey: command.IdempotencyKey, RequestHash: hash, EffectiveAt: effective, RecordedAt: now, Reason: strings.TrimSpace(command.Reason), ReversalOf: reversalOf}
		movement = models.StockMovement{ID: primitive.NewObjectID(), PostingID: posting.ID, TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, ItemID: command.ItemID, LineNumber: 0, QuantityMicros: quantity, EffectiveAt: effective, RecordedAt: now}
		if err := validation.Validate(&posting); err != nil {
			return nil, err
		}
		if err := validation.Validate(&movement); err != nil {
			return nil, err
		}
		if _, err := s.db.StockPostings().InsertOne(sc, posting); err != nil {
			return nil, err
		}
		if _, err := s.db.StockMovements().InsertOne(sc, movement); err != nil {
			return nil, err
		}
		if s.TestAfterPostingInsertHook != nil {
			if err := s.TestAfterPostingInsertHook(sc); err != nil {
				return nil, err
			}
		}
		balance, err := s.applyBalance(sc, command, quantity)
		if err != nil {
			return nil, err
		}
		result = &Result{Posting: posting, Movement: movement, Balance: *balance}
		return nil, nil
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			if existing, lookupErr := s.findIdempotent(ctx, command.TenantID, command.IdempotencyKey, hash); lookupErr == nil && existing != nil {
				return existing, nil
			}
		}
		return nil, err
	}
	return result, nil
}

func validateCommand(command Command, kind models.StockPostingType, reversalOf *primitive.ObjectID) error {
	if !models.ValidStockPostingType(kind) || command.TenantID.IsZero() || command.UserID.IsZero() || len(command.IdempotencyKey) < 8 || len(command.IdempotencyKey) > 128 {
		return ErrInvalidReference
	}
	if kind == models.StockPostingReversal {
		return nil
	}
	if command.LocationID.IsZero() || command.StorageAreaID.IsZero() || command.ItemID.IsZero() {
		return ErrInvalidReference
	}
	if reversalOf != nil {
		return ErrInvalidReference
	}
	return nil
}

func commandHash(command Command, kind models.StockPostingType, reversalOf *primitive.ObjectID) string {
	if command.LotID == nil && command.Reason == "" {
		legacy := struct {
			Kind                               models.StockPostingType
			Tenant, User, Location, Area, Item primitive.ObjectID
			Quantity                           int64
			Effective                          time.Time
			Reversal                           *primitive.ObjectID
		}{kind, command.TenantID, command.UserID, command.LocationID, command.StorageAreaID, command.ItemID, command.QuantityMicros, command.EffectiveAt.UTC(), reversalOf}
		encoded, _ := json.Marshal(legacy)
		digest := sha256.Sum256(encoded)
		return hex.EncodeToString(digest[:])
	}
	value := struct {
		Kind                               models.StockPostingType
		Tenant, User, Location, Area, Item primitive.ObjectID
		Quantity                           int64
		Effective                          time.Time
		Reversal                           *primitive.ObjectID
		Lot                                *primitive.ObjectID
		Reason                             string
	}{kind, command.TenantID, command.UserID, command.LocationID, command.StorageAreaID, command.ItemID, command.QuantityMicros, command.EffectiveAt.UTC(), reversalOf, command.LotID, command.Reason}
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (s *Service) findIdempotent(ctx context.Context, tenantID primitive.ObjectID, key, hash string) (*Result, error) {
	var posting models.StockPosting
	err := s.db.StockPostings().FindOne(ctx, bson.M{"tenantId": tenantID, "idempotencyKey": key}).Decode(&posting)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find inventory idempotency key: %w", err)
	}
	if posting.RequestHash != hash {
		return nil, ErrIdempotencyConflict
	}
	var movement models.StockMovement
	if err := s.db.StockMovements().FindOne(ctx, bson.M{"postingId": posting.ID, "tenantId": tenantID}).Decode(&movement); err != nil {
		return nil, fmt.Errorf("load idempotent movement: %w", err)
	}
	balance, err := s.currentBalance(ctx, posting.TenantID, posting.LocationID, posting.StorageAreaID, movement.ItemID)
	if err != nil {
		return nil, err
	}
	return &Result{Posting: posting, Movement: movement, Balance: *balance}, nil
}

func (s *Service) verifyReferences(ctx context.Context, command Command) error {
	if err := s.db.Locations().FindOne(ctx, bson.M{"_id": command.LocationID, "tenantId": command.TenantID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		return ErrInvalidReference
	}
	if err := s.db.StorageAreas().FindOne(ctx, bson.M{"_id": command.StorageAreaID, "tenantId": command.TenantID, "locationId": command.LocationID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		return ErrInvalidReference
	}
	if err := s.db.Items().FindOne(ctx, bson.M{"_id": command.ItemID, "tenantId": command.TenantID, "isActive": true, "stockable": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		return ErrInvalidReference
	}
	return nil
}

func (s *Service) applyBalance(ctx mongo.SessionContext, command Command, delta int64) (*models.StockBalance, error) {
	key := bson.M{"tenantId": command.TenantID, "locationId": command.LocationID, "storageAreaId": command.StorageAreaID, "itemId": command.ItemID}
	key["lotId"] = nil
	now := time.Now().UTC()
	if delta < 0 {
		if delta == math.MinInt64 {
			return nil, ErrQuantityOverflow
		}
		filter := bson.M{"tenantId": command.TenantID, "locationId": command.LocationID, "storageAreaId": command.StorageAreaID, "itemId": command.ItemID, "lotId": nil, "quantityMicros": bson.M{"$gte": -delta}}
		updated, err := s.db.StockBalances().UpdateOne(ctx, filter, bson.M{"$inc": bson.M{"quantityMicros": delta}, "$set": bson.M{"updatedAt": now}})
		if err != nil {
			return nil, fmt.Errorf("decrease stock balance: %w", err)
		}
		if updated.MatchedCount == 0 {
			return nil, ErrInsufficientStock
		}
	} else {
		update := bson.M{"$inc": bson.M{"quantityMicros": delta}, "$set": bson.M{"updatedAt": now}, "$setOnInsert": bson.M{"_id": primitive.NewObjectID(), "tenantId": command.TenantID, "locationId": command.LocationID, "storageAreaId": command.StorageAreaID, "itemId": command.ItemID}}
		if _, err := s.db.StockBalances().UpdateOne(ctx, key, update, options.Update().SetUpsert(true)); err != nil {
			return nil, fmt.Errorf("increase stock balance: %w", err)
		}
	}
	return s.currentBalance(ctx, command.TenantID, command.LocationID, command.StorageAreaID, command.ItemID)
}

func (s *Service) currentBalance(ctx context.Context, tenantID, locationID, storageAreaID, itemID primitive.ObjectID) (*models.StockBalance, error) {
	var balance models.StockBalance
	err := s.db.StockBalances().FindOne(ctx, bson.M{"tenantId": tenantID, "locationId": locationID, "storageAreaId": storageAreaID, "itemId": itemID, "lotId": nil}).Decode(&balance)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &models.StockBalance{TenantID: tenantID, LocationID: locationID, StorageAreaID: storageAreaID, ItemID: itemID}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load stock balance: %w", err)
	}
	return &balance, nil
}

func (s *Service) ListJournal(ctx context.Context, tenantID, locationID primitive.ObjectID, itemIDs ...*primitive.ObjectID) ([]JournalEntry, error) {
	filter := bson.M{"tenantId": tenantID, "locationId": locationID}
	if len(itemIDs) > 0 && itemIDs[0] != nil {
		filter["itemId"] = *itemIDs[0]
	}
	cursor, err := s.db.StockMovements().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "recordedAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	entries := make([]JournalEntry, 0)
	for cursor.Next(ctx) {
		var movement models.StockMovement
		if err := cursor.Decode(&movement); err != nil {
			return nil, err
		}
		var posting models.StockPosting
		if err := s.db.StockPostings().FindOne(ctx, journalPostingFilter(tenantID, movement.PostingID)).Decode(&posting); err != nil {
			return nil, err
		}
		entries = append(entries, JournalEntry{Posting: posting, Movement: movement})
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func journalPostingFilter(tenantID, postingID primitive.ObjectID) bson.M {
	return bson.M{"_id": postingID, "tenantId": tenantID}
}

func (s *Service) ListBalances(ctx context.Context, tenantID, locationID primitive.ObjectID) ([]models.StockBalance, error) {
	cursor, err := s.db.StockBalances().Find(ctx, bson.M{"tenantId": tenantID, "locationId": locationID}, options.Find().SetSort(bson.D{{Key: "storageAreaId", Value: 1}, {Key: "itemId", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	balances := make([]models.StockBalance, 0)
	if err := cursor.All(ctx, &balances); err != nil {
		return nil, err
	}
	return balances, nil
}
