package inventory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"lastsaas/internal/models"
	"lastsaas/internal/syslog"
	"lastsaas/internal/validation"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const reconciliationSummaryLimit int64 = 1000

var (
	ErrCountNotFound              = errors.New("stock count not found")
	ErrCountState                 = errors.New("stock count is not in the required state")
	ErrCountVersionConflict       = errors.New("stock count version conflict")
	ErrCountLineNotFound          = errors.New("stock count line not found")
	ErrCountLineExists            = errors.New("stock count line already exists")
	ErrCountDiscoveryNotZero      = errors.New("discovered stock has existing journal quantity")
	ErrCountOwnershipRequired     = errors.New("stock count ownership lock is missing; cancel this count before continuing")
	ErrCountSnapshotInvalid       = errors.New("stock count snapshot is invalid")
	ErrReconciliationNotFound     = errors.New("reconciliation run not found")
	ErrReconciliationTooLarge     = errors.New("reconciliation scope exceeds the repair bound")
	ErrReconciliationNegative     = errors.New("journal contains a negative reconciliation balance")
	ErrReconciliationRepairNeeded = errors.New("reconciliation repair must be explicit")
	ErrActiveCountCursorInvalid   = errors.New("active count cursor is invalid")
	ErrActiveCountLimitInvalid    = errors.New("active count page size is invalid")
)

const (
	activeCountDefaultLimit  int64 = 25
	activeCountMaximumLimit  int64 = 100
	activeCountCursorVersion       = 1
)

type CountCreateCommand struct {
	TenantID       primitive.ObjectID
	UserID         primitive.ObjectID
	LocationID     primitive.ObjectID
	StorageAreaID  primitive.ObjectID
	IdempotencyKey string
}

type CountVersionCommand struct {
	TenantID primitive.ObjectID
	UserID   primitive.ObjectID
	// LocationID is supplied by location-scoped handlers.  Keeping it in the
	// command prevents a count from one authorized location being addressed
	// through another location's route.
	LocationID primitive.ObjectID
	CountID    primitive.ObjectID
	Version    int64
}

type CountLineCommand struct {
	CountVersionCommand
	LineID                primitive.ObjectID
	CountedQuantityMicros int64
	Note                  string
	LineVersion           int64
}

type CountDiscoverLineCommand struct {
	CountVersionCommand
	ItemID primitive.ObjectID
	LotID  *primitive.ObjectID
}

type CountPostCommand struct {
	TenantID       primitive.ObjectID
	UserID         primitive.ObjectID
	LocationID     primitive.ObjectID
	CountID        primitive.ObjectID
	IdempotencyKey string
}

type ReconciliationCommand struct {
	TenantID       primitive.ObjectID
	UserID         primitive.ObjectID
	LocationID     primitive.ObjectID
	StorageAreaID  *primitive.ObjectID
	IdempotencyKey string
	Repair         bool
}

type CountResult struct {
	Count     models.StockCount      `json:"count"`
	Lines     []CountLineResult      `json:"lines"`
	Posting   *models.StockPosting   `json:"posting,omitempty"`
	Movements []models.StockMovement `json:"movements,omitempty"`
}

// CountLineResult is the read DTO for a stock-count line.  Lot metadata is
// deliberately not part of models.StockCountLine: it is current catalog data,
// rather than part of the persisted count observation.
type CountLineResult struct {
	models.StockCountLine
	LotCode      string           `json:"lotCode,omitempty"`
	LotStatus    models.LotStatus `json:"lotStatus,omitempty"`
	LotExpiresAt *time.Time       `json:"lotExpiresAt,omitempty"`
}

type ActiveCountIntegrity string

const (
	ActiveCountHealthy       ActiveCountIntegrity = "healthy"
	ActiveCountOwnerMissing  ActiveCountIntegrity = "owner_missing"
	ActiveCountOwnerMismatch ActiveCountIntegrity = "owner_mismatch"
	ActiveCountAreaMissing   ActiveCountIntegrity = "area_missing"
)

// ActiveCountListCommand is the service-level input for the active count
// recovery list. Cursor is an opaque, versioned base64url keyset cursor.
type ActiveCountListCommand struct {
	TenantID   primitive.ObjectID
	LocationID primitive.ObjectID
	Cursor     string
	Limit      int64
}

// CountListCommand is kept as a concise alias for callers that use the count
// terminology rather than the recovery-list terminology.
type CountListCommand = ActiveCountListCommand
type ActiveCountCommand = ActiveCountListCommand

type ActiveCountSummary struct {
	Count     models.StockCount    `json:"count"`
	Integrity ActiveCountIntegrity `json:"integrity"`
}

type ActiveCountListResult struct {
	Counts     []ActiveCountSummary `json:"counts"`
	NextCursor string               `json:"nextCursor,omitempty"`
}

type CountListResult = ActiveCountListResult
type ActiveCountsResult = ActiveCountListResult

type activeCountCursor struct {
	Version   int                `json:"v"`
	CreatedAt time.Time          `json:"createdAt"`
	ID        primitive.ObjectID `json:"id"`
}

// EncodeActiveCountCursor and DecodeActiveCountCursor are exported so an HTTP
// adapter can treat the cursor as opaque without duplicating its validation.
func EncodeActiveCountCursor(createdAt time.Time, id primitive.ObjectID) (string, error) {
	if createdAt.IsZero() || id.IsZero() {
		return "", ErrActiveCountCursorInvalid
	}
	payload, err := json.Marshal(activeCountCursor{Version: activeCountCursorVersion, CreatedAt: createdAt.UTC(), ID: id})
	if err != nil {
		return "", ErrActiveCountCursorInvalid
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecodeActiveCountCursor(value string) (time.Time, primitive.ObjectID, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, primitive.NilObjectID, ErrActiveCountCursorInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != value {
		return time.Time{}, primitive.NilObjectID, ErrActiveCountCursorInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cursor activeCountCursor
	if err := decoder.Decode(&cursor); err != nil || cursor.Version != activeCountCursorVersion || cursor.CreatedAt.IsZero() || cursor.ID.IsZero() {
		return time.Time{}, primitive.NilObjectID, ErrActiveCountCursorInvalid
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return time.Time{}, primitive.NilObjectID, ErrActiveCountCursorInvalid
	}
	return cursor.CreatedAt.UTC(), cursor.ID, nil
}

type CountLotOptionsCommand struct {
	TenantID   primitive.ObjectID
	UserID     primitive.ObjectID
	LocationID primitive.ObjectID
	CountID    primitive.ObjectID
	ItemID     primitive.ObjectID
}

type CountLotOption struct {
	ID        primitive.ObjectID `json:"id"`
	Code      string             `json:"code"`
	Status    models.LotStatus   `json:"status"`
	ExpiresAt *time.Time         `json:"expiresAt,omitempty"`
}

type CountLotOptionsResult struct {
	Lots []CountLotOption `json:"lots"`
}

type ReconciliationResult struct {
	Run models.ReconciliationRun `json:"run"`
}

type countBalanceKey struct {
	LocationID    primitive.ObjectID
	StorageAreaID primitive.ObjectID
	ItemID        primitive.ObjectID
	LotID         primitive.ObjectID
	HasLot        bool
}

func newCountBalanceKey(locationID, areaID, itemID primitive.ObjectID, lotID *primitive.ObjectID) countBalanceKey {
	key := countBalanceKey{LocationID: locationID, StorageAreaID: areaID, ItemID: itemID}
	if lotID != nil {
		key.LotID, key.HasLot = *lotID, true
	}
	return key
}

func (k countBalanceKey) filter() bson.M {
	filter := bson.M{"tenantId": nil, "locationId": k.LocationID, "storageAreaId": k.StorageAreaID, "itemId": k.ItemID}
	if k.HasLot {
		filter["lotId"] = k.LotID
	} else {
		filter["lotId"] = nil
	}
	return filter
}

func (k countBalanceKey) less(other countBalanceKey) bool {
	if k.StorageAreaID != other.StorageAreaID {
		return k.StorageAreaID.Hex() < other.StorageAreaID.Hex()
	}
	if k.ItemID != other.ItemID {
		return k.ItemID.Hex() < other.ItemID.Hex()
	}
	if k.HasLot != other.HasLot {
		return !k.HasLot
	}
	return k.LotID.Hex() < other.LotID.Hex()
}

func (s *Service) SetLogger(logger *syslog.Logger) *Service {
	s.logger = logger
	return s
}

func (s *Service) CreateCount(ctx context.Context, command CountCreateCommand) (*CountResult, error) {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.LocationID.IsZero() || command.StorageAreaID.IsZero() || len(strings.TrimSpace(command.IdempotencyKey)) < 8 {
		return nil, ErrInvalidReference
	}
	if err := s.verifyCountScope(ctx, command.TenantID, command.LocationID, command.StorageAreaID); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(command.IdempotencyKey)
	requestHash := countCreateHash(command)
	if existing, err := s.findCountByIdempotency(ctx, command.TenantID, key); err != nil {
		return nil, err
	} else if existing != nil {
		if existing.RequestHash != requestHash {
			return nil, ErrIdempotencyConflict
		}
		return s.countResult(ctx, existing)
	}
	now := time.Now().UTC()
	count := models.StockCount{ID: primitive.NewObjectID(), TenantID: command.TenantID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, CreatedBy: command.UserID, Status: models.StockCountDraft, Version: 1, IdempotencyKey: key, RequestHash: requestHash, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&count); err != nil {
		return nil, err
	}
	if _, err := s.db.StockCounts().InsertOne(ctx, count); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			if existing, lookupErr := s.findCountByIdempotency(ctx, command.TenantID, key); lookupErr == nil && existing != nil {
				if existing.RequestHash != requestHash {
					return nil, ErrIdempotencyConflict
				}
				return s.countResult(ctx, existing)
			}
		}
		return nil, err
	}
	return s.countResult(ctx, &count)
}

func (s *Service) CreateStockCount(ctx context.Context, command CountCreateCommand) (*CountResult, error) {
	return s.CreateCount(ctx, command)
}

func (s *Service) FreezeCount(ctx context.Context, command CountVersionCommand) (*CountResult, error) {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.CountID.IsZero() || command.Version < 1 {
		return nil, ErrInvalidReference
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)
	var result *CountResult
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		var count models.StockCount
		if err := s.db.StockCounts().FindOne(sc, bson.M{"_id": command.CountID, "tenantId": command.TenantID}).Decode(&count); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountNotFound
			}
			return nil, err
		}
		if !command.LocationID.IsZero() && count.LocationID != command.LocationID {
			return nil, ErrCountNotFound
		}
		if count.Status != models.StockCountDraft {
			return nil, ErrCountState
		}
		if count.Version != command.Version {
			return nil, ErrCountVersionConflict
		}
		if err := s.claimCountArea(sc, count); err != nil {
			return nil, err
		}
		sums, err := s.sumAllMovements(sc, command.TenantID, count.LocationID, &count.StorageAreaID)
		if err != nil {
			return nil, err
		}
		keys := make([]countBalanceKey, 0, len(sums))
		for key, quantity := range sums {
			if quantity < 0 {
				return nil, ErrCountSnapshotInvalid
			}
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].less(keys[j]) })
		now := time.Now().UTC()
		for _, key := range keys {
			line := models.StockCountLine{ID: primitive.NewObjectID(), CountID: count.ID, TenantID: command.TenantID, LocationID: key.LocationID, StorageAreaID: key.StorageAreaID, ItemID: key.ItemID, SnapshotQuantityMicros: sums[key], Version: 1, CreatedAt: now, UpdatedAt: now}
			if key.HasLot {
				lotID := key.LotID
				line.LotID = &lotID
			}
			if err := validation.Validate(&line); err != nil {
				return nil, err
			}
			if _, err := s.db.StockCountLines().InsertOne(sc, line); err != nil {
				return nil, err
			}
		}
		cutoff := time.Now().UTC()
		updated := count
		if err := s.db.StockCounts().FindOneAndUpdate(sc, bson.M{"_id": count.ID, "tenantId": command.TenantID, "status": models.StockCountDraft, "version": command.Version}, bson.M{"$set": bson.M{"status": models.StockCountFrozen, "cutoffAt": cutoff, "frozenAt": now, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updated); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountVersionConflict
			}
			return nil, err
		}
		result, err = s.countResult(sc, &updated)
		return nil, err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) FreezeStockCount(ctx context.Context, command CountVersionCommand) (*CountResult, error) {
	return s.FreezeCount(ctx, command)
}

func (s *Service) RecordCountLine(ctx context.Context, command CountLineCommand) (*models.StockCountLine, error) {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.CountID.IsZero() || command.LineID.IsZero() || command.Version < 1 || command.LineVersion < 1 || command.CountedQuantityMicros < 0 {
		return nil, ErrInvalidReference
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)
	var updated *models.StockCountLine
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		var count models.StockCount
		if err := s.db.StockCounts().FindOne(sc, bson.M{"_id": command.CountID, "tenantId": command.TenantID}).Decode(&count); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountNotFound
			}
			return nil, err
		}
		if !command.LocationID.IsZero() && count.LocationID != command.LocationID {
			return nil, ErrCountNotFound
		}
		if count.Status != models.StockCountFrozen {
			return nil, ErrCountState
		}
		if count.Version != command.Version {
			return nil, ErrCountVersionConflict
		}
		if err := s.verifyCountOwner(sc, count); err != nil {
			return nil, err
		}
		var line models.StockCountLine
		if err := s.db.StockCountLines().FindOne(sc, bson.M{"_id": command.LineID, "countId": command.CountID, "tenantId": command.TenantID}).Decode(&line); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountLineNotFound
			}
			return nil, err
		}
		if line.Version != command.LineVersion {
			return nil, ErrCountVersionConflict
		}
		now := time.Now().UTC()
		if err := s.db.StockCountLines().FindOneAndUpdate(sc, bson.M{"_id": line.ID, "countId": count.ID, "tenantId": command.TenantID, "version": command.LineVersion}, bson.M{"$set": bson.M{"countedQuantityMicros": command.CountedQuantityMicros, "note": strings.TrimSpace(command.Note), "updatedAt": now}, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&line); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountVersionConflict
			}
			return nil, err
		}
		if err := s.db.StockCounts().FindOneAndUpdate(sc, bson.M{"_id": count.ID, "tenantId": command.TenantID, "status": models.StockCountFrozen, "version": command.Version}, bson.M{"$set": bson.M{"updatedAt": now}, "$inc": bson.M{"version": int64(1)}}).Err(); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountVersionConflict
			}
			return nil, err
		}
		updated = &line
		return nil, nil
	})
	return updated, err
}

func (s *Service) RecordStockCountLine(ctx context.Context, command CountLineCommand) (*models.StockCountLine, error) {
	return s.RecordCountLine(ctx, command)
}

func (s *Service) DiscoverCountLine(ctx context.Context, command CountDiscoverLineCommand) (*models.StockCountLine, error) {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.CountID.IsZero() || command.Version < 1 || command.ItemID.IsZero() {
		return nil, ErrInvalidReference
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)
	var result *models.StockCountLine
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		var count models.StockCount
		if err := s.db.StockCounts().FindOne(sc, bson.M{"_id": command.CountID, "tenantId": command.TenantID}).Decode(&count); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountNotFound
			}
			return nil, err
		}
		if !command.LocationID.IsZero() && count.LocationID != command.LocationID {
			return nil, ErrCountNotFound
		}
		if count.Status != models.StockCountFrozen {
			return nil, ErrCountState
		}
		if count.Version != command.Version {
			return nil, ErrCountVersionConflict
		}
		if err := s.verifyCountOwner(sc, count); err != nil {
			return nil, err
		}

		var item models.Item
		if err := s.db.Items().FindOne(sc, bson.M{"_id": command.ItemID, "tenantId": command.TenantID, "isActive": true, "stockable": true}).Decode(&item); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrInvalidReference
			}
			return nil, err
		}
		if command.LotID == nil && item.LotTracking == models.LotTrackingRequired {
			return nil, ErrLotRequired
		}
		if command.LotID != nil {
			if item.LotTracking == models.LotTrackingDisabled {
				return nil, ErrLotTrackingDisabled
			}
			if err := s.db.StockLots().FindOne(sc, bson.M{"_id": *command.LotID, "tenantId": command.TenantID, "itemId": command.ItemID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					return nil, ErrLotNotFound
				}
				return nil, err
			}
		}
		journalQuantity, err := s.exactMovementQuantity(sc, command.TenantID, count.LocationID, count.StorageAreaID, command.ItemID, command.LotID)
		if err != nil {
			return nil, err
		}
		if journalQuantity != 0 {
			return nil, ErrCountDiscoveryNotZero
		}
		lineFilter := bson.M{"tenantId": command.TenantID, "countId": count.ID, "locationId": count.LocationID, "storageAreaId": count.StorageAreaID, "itemId": command.ItemID, "lotId": nil}
		if command.LotID != nil {
			lineFilter["lotId"] = *command.LotID
		}
		if err := s.db.StockCountLines().FindOne(sc, lineFilter, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err == nil {
			return nil, ErrCountLineExists
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		now := time.Now().UTC()
		line := models.StockCountLine{ID: primitive.NewObjectID(), CountID: count.ID, TenantID: command.TenantID, LocationID: count.LocationID, StorageAreaID: count.StorageAreaID, ItemID: command.ItemID, LotID: command.LotID, SnapshotQuantityMicros: 0, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := validation.Validate(&line); err != nil {
			return nil, err
		}
		if _, err := s.db.StockCountLines().InsertOne(sc, line); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return nil, ErrCountLineExists
			}
			return nil, err
		}
		if err := s.db.StockCounts().FindOneAndUpdate(sc, bson.M{"_id": count.ID, "tenantId": command.TenantID, "status": models.StockCountFrozen, "version": command.Version}, bson.M{"$set": bson.M{"updatedAt": now}, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Err(); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountVersionConflict
			}
			return nil, err
		}
		result = &line
		return nil, nil
	})
	return result, err
}

func (s *Service) DiscoverStockCountLine(ctx context.Context, command CountDiscoverLineCommand) (*models.StockCountLine, error) {
	return s.DiscoverCountLine(ctx, command)
}

func (s *Service) CancelCount(ctx context.Context, command CountVersionCommand) (*CountResult, error) {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.CountID.IsZero() || command.Version < 1 {
		return nil, ErrInvalidReference
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)
	var result *CountResult
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		var count models.StockCount
		if err := s.db.StockCounts().FindOne(sc, bson.M{"_id": command.CountID, "tenantId": command.TenantID}).Decode(&count); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountNotFound
			}
			return nil, err
		}
		if !command.LocationID.IsZero() && count.LocationID != command.LocationID {
			return nil, ErrCountNotFound
		}
		if count.Version != command.Version {
			return nil, ErrCountVersionConflict
		}
		if count.Status != models.StockCountDraft && count.Status != models.StockCountFrozen && count.Status != models.StockCountReviewed {
			return nil, ErrCountState
		}
		now := time.Now().UTC()
		var updated models.StockCount
		if err := s.db.StockCounts().FindOneAndUpdate(sc, bson.M{"_id": count.ID, "tenantId": command.TenantID, "status": bson.M{"$in": bson.A{models.StockCountDraft, models.StockCountFrozen, models.StockCountReviewed}}, "version": command.Version}, bson.M{"$set": bson.M{"status": models.StockCountCancelled, "cancelledAt": now, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updated); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountVersionConflict
			}
			return nil, err
		}
		_, err := s.releaseCountAreaIfOwned(sc, count)
		if err != nil {
			return nil, err
		}
		result, err = s.countResult(sc, &updated)
		return nil, err
	})
	return result, err
}

func (s *Service) CancelStockCount(ctx context.Context, command CountVersionCommand) (*CountResult, error) {
	return s.CancelCount(ctx, command)
}

func (s *Service) ReviewCount(ctx context.Context, command CountVersionCommand) (*CountResult, error) {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.CountID.IsZero() || command.Version < 1 {
		return nil, ErrInvalidReference
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)
	var result *CountResult
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		var count models.StockCount
		if err := s.db.StockCounts().FindOne(sc, bson.M{"_id": command.CountID, "tenantId": command.TenantID}).Decode(&count); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountNotFound
			}
			return nil, err
		}
		if !command.LocationID.IsZero() && count.LocationID != command.LocationID {
			return nil, ErrCountNotFound
		}
		if count.Status != models.StockCountFrozen {
			return nil, ErrCountState
		}
		if count.Version != command.Version {
			return nil, ErrCountVersionConflict
		}
		if err := s.verifyCountOwner(sc, count); err != nil {
			return nil, err
		}
		cursor, err := s.db.StockCountLines().Find(sc, bson.M{"tenantId": command.TenantID, "countId": count.ID})
		if err != nil {
			return nil, err
		}
		var lines []models.StockCountLine
		if err := cursor.All(sc, &lines); err != nil {
			return nil, err
		}
		for _, line := range lines {
			if line.CountedQuantityMicros == nil {
				return nil, ErrCountState
			}
		}
		now := time.Now().UTC()
		var updated models.StockCount
		if err := s.db.StockCounts().FindOneAndUpdate(sc, bson.M{"_id": count.ID, "tenantId": command.TenantID, "status": models.StockCountFrozen, "version": command.Version}, bson.M{"$set": bson.M{"status": models.StockCountReviewed, "reviewedAt": now, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updated); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountVersionConflict
			}
			return nil, err
		}
		result, err = s.countResult(sc, &updated)
		return nil, err
	})
	return result, err
}

func (s *Service) ReviewStockCount(ctx context.Context, command CountVersionCommand) (*CountResult, error) {
	return s.ReviewCount(ctx, command)
}

func (s *Service) PostCount(ctx context.Context, command CountPostCommand) (*CountResult, error) {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.CountID.IsZero() || len(strings.TrimSpace(command.IdempotencyKey)) < 8 {
		return nil, ErrInvalidReference
	}
	key := strings.TrimSpace(command.IdempotencyKey)
	if count, err := s.loadCount(ctx, command.TenantID, command.CountID); err != nil {
		return nil, err
	} else if count.Status == models.StockCountPosted {
		if count.LocationID != command.LocationID && !command.LocationID.IsZero() {
			return nil, ErrCountNotFound
		}
		if count.PostIdempotencyKey != key || count.PostRequestHash == "" || count.PostRequestHash != countPostHash(command, *count) {
			return nil, ErrIdempotencyConflict
		}
		return s.countResult(ctx, count)
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)
	var result *CountResult
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		var count models.StockCount
		if err := s.db.StockCounts().FindOne(sc, bson.M{"_id": command.CountID, "tenantId": command.TenantID}).Decode(&count); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountNotFound
			}
			return nil, err
		}
		if !command.LocationID.IsZero() && count.LocationID != command.LocationID {
			return nil, ErrCountNotFound
		}
		if count.Status == models.StockCountPosted {
			if count.PostIdempotencyKey != key || count.PostRequestHash == "" || count.PostRequestHash != countPostHash(command, count) {
				return nil, ErrIdempotencyConflict
			}
			var postResult error
			result, postResult = s.countResult(sc, &count)
			return nil, postResult
		}
		if count.Status != models.StockCountReviewed {
			return nil, ErrCountState
		}
		if err := s.verifyCountOwner(sc, count); err != nil {
			return nil, err
		}
		postHash := countPostHash(command, count)
		var sameKey models.StockPosting
		if err := s.db.StockPostings().FindOne(sc, bson.M{"tenantId": command.TenantID, "idempotencyKey": key}).Decode(&sameKey); err == nil {
			if sameKey.RequestHash != postHash {
				return nil, ErrIdempotencyConflict
			}
			// A posting with this key is never valid without the count being
			// marked posted in the same transaction. Do not attach or mutate it.
			return nil, ErrIdempotencyConflict
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		cursor, err := s.db.StockCountLines().Find(sc, bson.M{"tenantId": command.TenantID, "countId": count.ID}, options.Find().SetSort(bson.D{{Key: "storageAreaId", Value: 1}, {Key: "itemId", Value: 1}, {Key: "_id", Value: 1}}))
		if err != nil {
			return nil, err
		}
		var lines []models.StockCountLine
		if err := cursor.All(sc, &lines); err != nil {
			return nil, err
		}
		differences := make([]lineSpec, 0, len(lines))
		for _, line := range lines {
			if line.CountedQuantityMicros == nil {
				return nil, ErrCountState
			}
			delta, err := safeSub(*line.CountedQuantityMicros, line.SnapshotQuantityMicros)
			if err != nil {
				return nil, ErrQuantityOverflow
			}
			if delta != 0 {
				differences = append(differences, lineSpec{TenantID: command.TenantID, LocationID: line.LocationID, StorageAreaID: line.StorageAreaID, ItemID: line.ItemID, LotID: line.LotID, QuantityMicros: delta})
			}
		}
		now := time.Now().UTC()
		posted := models.StockPosting{ID: primitive.NewObjectID(), TenantID: command.TenantID, LocationID: count.LocationID, StorageAreaID: count.StorageAreaID, UserID: command.UserID, Type: models.StockPostingCount, IdempotencyKey: key, RequestHash: postHash, EffectiveAt: now, RecordedAt: now, Reason: "stock count " + count.ID.Hex()}
		if err := validation.Validate(&posted); err != nil {
			return nil, err
		}
		if _, err := s.db.StockPostings().InsertOne(sc, posted); err != nil {
			return nil, err
		}
		movements := make([]models.StockMovement, 0, len(differences))
		for number, difference := range differences {
			movement := models.StockMovement{ID: primitive.NewObjectID(), PostingID: posted.ID, TenantID: command.TenantID, LocationID: difference.LocationID, StorageAreaID: difference.StorageAreaID, ItemID: difference.ItemID, LotID: difference.LotID, LineNumber: int32(number), QuantityMicros: difference.QuantityMicros, EffectiveAt: now, RecordedAt: now}
			if err := validation.Validate(&movement); err != nil {
				return nil, err
			}
			if _, err := s.db.StockMovements().InsertOne(sc, movement); err != nil {
				return nil, err
			}
			if _, err := s.applyBalanceLine(sc, difference); err != nil {
				return nil, err
			}
			movements = append(movements, movement)
		}
		updated := count
		update := bson.M{"$set": bson.M{"status": models.StockCountPosted, "postIdempotencyKey": key, "postRequestHash": postHash, "postedAt": now, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}}
		update["$set"].(bson.M)["postPostingId"] = posted.ID
		if err := s.db.StockCounts().FindOneAndUpdate(sc, bson.M{"_id": count.ID, "tenantId": command.TenantID, "status": models.StockCountReviewed, "version": count.Version}, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updated); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, ErrCountVersionConflict
			}
			return nil, err
		}
		if err := s.releaseCountArea(sc, count); err != nil {
			return nil, err
		}
		enriched, err := s.enrichCountLines(sc, command.TenantID, lines)
		if err != nil {
			return nil, err
		}
		result = &CountResult{Count: updated, Lines: enriched, Posting: &posted, Movements: movements}
		return nil, nil
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return s.recoverPostedCountDuplicate(ctx, command, key)
		}
		return nil, err
	}
	return result, nil
}

func (s *Service) recoverPostedCountDuplicate(ctx context.Context, command CountPostCommand, key string) (*CountResult, error) {
	var posting models.StockPosting
	if err := s.db.StockPostings().FindOne(ctx, bson.M{"tenantId": command.TenantID, "idempotencyKey": key}).Decode(&posting); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrIdempotencyConflict
		}
		return nil, err
	}
	var count models.StockCount
	if err := s.db.StockCounts().FindOne(ctx, bson.M{"tenantId": command.TenantID, "postPostingId": posting.ID}).Decode(&count); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrIdempotencyConflict
		}
		return nil, err
	}
	if count.ID != command.CountID || count.Status != models.StockCountPosted || count.PostIdempotencyKey != key || count.PostPostingID == nil || *count.PostPostingID != posting.ID {
		return nil, ErrIdempotencyConflict
	}
	expectedHash := countPostHash(command, count)
	if posting.RequestHash != expectedHash || count.PostRequestHash != expectedHash {
		return nil, ErrIdempotencyConflict
	}
	return s.countResult(ctx, &count)
}

func (s *Service) PostStockCount(ctx context.Context, command CountPostCommand) (*CountResult, error) {
	return s.PostCount(ctx, command)
}

func (s *Service) GetCount(ctx context.Context, tenantID, countID primitive.ObjectID, locationIDs ...primitive.ObjectID) (*CountResult, error) {
	count, err := s.loadCount(ctx, tenantID, countID)
	if err != nil {
		return nil, err
	}
	if len(locationIDs) > 0 && !locationIDs[0].IsZero() && count.LocationID != locationIDs[0] {
		return nil, ErrCountNotFound
	}
	if count.Status == models.StockCountFrozen || count.Status == models.StockCountReviewed {
		if err := s.verifyCountOwner(ctx, *count); err != nil {
			return nil, err
		}
	}
	return s.countResult(ctx, count)
}

// ListActiveCounts returns only counts which can still own an area or need
// recovery: draft, frozen, and reviewed.  The owner is read solely for
// classification and is never copied into the returned summary.
func (s *Service) ListActiveCounts(ctx context.Context, command ActiveCountListCommand) (*ActiveCountListResult, error) {
	if command.TenantID.IsZero() || command.LocationID.IsZero() {
		return nil, ErrInvalidReference
	}
	limit := command.Limit
	if limit == 0 {
		limit = activeCountDefaultLimit
	}
	if limit < 1 || limit > activeCountMaximumLimit {
		return nil, ErrActiveCountLimitInvalid
	}

	filter := bson.M{
		"tenantId":   command.TenantID,
		"locationId": command.LocationID,
		"status":     bson.M{"$in": bson.A{models.StockCountDraft, models.StockCountFrozen, models.StockCountReviewed}},
	}
	if command.Cursor != "" {
		createdAt, id, err := DecodeActiveCountCursor(command.Cursor)
		if err != nil {
			return nil, err
		}
		filter["$or"] = bson.A{
			bson.M{"createdAt": bson.M{"$lt": createdAt}},
			bson.M{"createdAt": createdAt, "_id": bson.M{"$lt": id}},
		}
	}
	cursor, err := s.db.StockCounts().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit+1))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	counts := make([]models.StockCount, 0, limit+1)
	if err := cursor.All(ctx, &counts); err != nil {
		return nil, err
	}
	hasNext := int64(len(counts)) > limit
	if hasNext {
		counts = counts[:limit]
	}

	areas := make(map[primitive.ObjectID]models.StorageArea, len(counts))
	ownedCountIDs := make([]primitive.ObjectID, 0, len(counts))
	for _, count := range counts {
		if count.Status != models.StockCountDraft {
			ownedCountIDs = append(ownedCountIDs, count.StorageAreaID)
		}
	}
	if len(ownedCountIDs) > 0 {
		areaIDs := make([]primitive.ObjectID, 0, len(counts))
		for _, count := range counts {
			if count.Status != models.StockCountDraft {
				areaIDs = append(areaIDs, count.StorageAreaID)
			}
		}
		areaCursor, err := s.db.StorageAreas().Find(ctx, bson.M{"tenantId": command.TenantID, "locationId": command.LocationID, "_id": bson.M{"$in": areaIDs}})
		if err != nil {
			return nil, err
		}
		defer areaCursor.Close(ctx)
		var found []models.StorageArea
		if err := areaCursor.All(ctx, &found); err != nil {
			return nil, err
		}
		for _, area := range found {
			areas[area.ID] = area
		}
	}

	result := &ActiveCountListResult{Counts: make([]ActiveCountSummary, 0, len(counts))}
	for _, count := range counts {
		area, exists := areas[count.StorageAreaID]
		result.Counts = append(result.Counts, ActiveCountSummary{Count: count, Integrity: classifyActiveCountIntegrity(count.Status, count.ID, area, exists)})
	}
	if hasNext {
		last := counts[len(counts)-1]
		result.NextCursor, err = EncodeActiveCountCursor(last.CreatedAt, last.ID)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func classifyActiveCountIntegrity(status models.StockCountStatus, countID primitive.ObjectID, area models.StorageArea, exists bool) ActiveCountIntegrity {
	if status == models.StockCountDraft {
		return ActiveCountHealthy
	}
	if !exists {
		return ActiveCountAreaMissing
	}
	if area.ActiveStockCountID == nil {
		return ActiveCountOwnerMissing
	}
	if *area.ActiveStockCountID == countID {
		return ActiveCountHealthy
	}
	return ActiveCountOwnerMismatch
}

func (s *Service) ListActiveStockCounts(ctx context.Context, command ActiveCountListCommand) (*ActiveCountListResult, error) {
	return s.ListActiveCounts(ctx, command)
}

func (s *Service) ListCounts(ctx context.Context, command CountListCommand) (*CountListResult, error) {
	return s.ListActiveCounts(ctx, command)
}

// ListCountLotOptions returns every lot for an item, including quarantined
// lots.  Availability is a detail for the count UI; filtering it here would
// hide information needed to recover a frozen count safely.
func (s *Service) ListCountLotOptions(ctx context.Context, command CountLotOptionsCommand) (*CountLotOptionsResult, error) {
	if command.TenantID.IsZero() || command.LocationID.IsZero() || command.CountID.IsZero() || command.ItemID.IsZero() {
		return nil, ErrInvalidReference
	}
	count, err := s.loadCount(ctx, command.TenantID, command.CountID)
	if err != nil {
		return nil, err
	}
	if count.LocationID != command.LocationID {
		return nil, ErrCountNotFound
	}
	if count.Status != models.StockCountFrozen {
		return nil, ErrCountState
	}
	if err := s.verifyCountOwner(ctx, *count); err != nil {
		return nil, err
	}
	if err := s.db.Items().FindOne(ctx, bson.M{"_id": command.ItemID, "tenantId": command.TenantID, "isActive": true, "stockable": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrInvalidReference
		}
		return nil, err
	}
	lotsCursor, err := s.db.StockLots().Find(ctx, bson.M{"tenantId": command.TenantID, "itemId": command.ItemID}, options.Find().SetSort(bson.D{{Key: "receivedAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer lotsCursor.Close(ctx)
	var lots []models.StockLot
	if err := lotsCursor.All(ctx, &lots); err != nil {
		return nil, err
	}
	result := &CountLotOptionsResult{Lots: make([]CountLotOption, 0, len(lots))}
	for _, lot := range lots {
		result.Lots = append(result.Lots, CountLotOption{ID: lot.ID, Code: lot.Code, Status: lot.Status, ExpiresAt: lot.ExpiresAt})
	}
	return result, nil
}

func (s *Service) GetCountLotOptions(ctx context.Context, command CountLotOptionsCommand) (*CountLotOptionsResult, error) {
	return s.ListCountLotOptions(ctx, command)
}

func (s *Service) ListStockCountLotOptions(ctx context.Context, command CountLotOptionsCommand) (*CountLotOptionsResult, error) {
	return s.ListCountLotOptions(ctx, command)
}

func (s *Service) ListCountLots(ctx context.Context, command CountLotOptionsCommand) (*CountLotOptionsResult, error) {
	return s.ListCountLotOptions(ctx, command)
}

func (s *Service) RunReconciliation(ctx context.Context, command ReconciliationCommand) (*ReconciliationResult, error) {
	if command.TenantID.IsZero() || command.UserID.IsZero() || command.LocationID.IsZero() || len(strings.TrimSpace(command.IdempotencyKey)) < 8 {
		return nil, ErrInvalidReference
	}
	if command.Repair && command.UserID.IsZero() {
		return nil, ErrReconciliationRepairNeeded
	}
	requestHash := reconciliationHash(command)
	if err := s.verifyReconciliationScope(ctx, command); err != nil {
		return nil, err
	}
	if existing, err := s.findReconciliation(ctx, command.TenantID, command.IdempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		if existing.RequestHash != requestHash {
			return nil, ErrIdempotencyConflict
		}
		return &ReconciliationResult{Run: *existing}, nil
	}
	session, err := s.db.Client.StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)
	cutoff := time.Now().UTC()
	var run *models.ReconciliationRun
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		// Recheck idempotency inside the transaction. The preflight lookup above
		// is only an optimization; concurrent callers can both pass it, and a
		// transaction retry must also be able to return a run committed by an
		// earlier attempt.
		if existing, lookupErr := s.findReconciliation(sc, command.TenantID, command.IdempotencyKey); lookupErr != nil {
			return nil, lookupErr
		} else if existing != nil {
			if existing.RequestHash != requestHash {
				return nil, ErrIdempotencyConflict
			}
			run = existing
			return nil, nil
		}
		expected, err := s.sumMovements(sc, command.TenantID, command.LocationID, command.StorageAreaID, cutoff)
		if err != nil {
			return nil, err
		}
		actual, err := s.loadBalances(sc, command.TenantID, command.LocationID, command.StorageAreaID)
		if err != nil {
			return nil, err
		}
		var later map[countBalanceKey]int64
		if command.Repair {
			// A repair corrects the balance at the cutoff while retaining every
			// journal movement recorded after that cutoff. The transaction may be
			// retried after a concurrent posting; recomputing this on each attempt
			// prevents a retry from resetting that newer movement to the cutoff
			// balance.
			later, err = s.sumMovementsAfter(sc, command.TenantID, command.LocationID, command.StorageAreaID, cutoff)
			if err != nil {
				return nil, err
			}
		}
		keys := make(map[countBalanceKey]struct{}, len(expected)+len(actual))
		for key := range expected {
			keys[key] = struct{}{}
		}
		for key := range actual {
			keys[key] = struct{}{}
		}
		ordered := make([]countBalanceKey, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].less(ordered[j]) })
		mismatches := make([]models.ReconciliationMismatch, 0)
		var mismatchCount int64
		for _, key := range ordered {
			expectedQuantity := expected[key]
			actualQuantity := actual[key]
			if expectedQuantity == actualQuantity {
				continue
			}
			mismatchCount++
			if int64(len(mismatches)) < reconciliationSummaryLimit {
				delta, err := safeSub(expectedQuantity, actualQuantity)
				if err != nil {
					return nil, ErrQuantityOverflow
				}
				mismatch := models.ReconciliationMismatch{LocationID: key.LocationID, StorageAreaID: key.StorageAreaID, ItemID: key.ItemID, ExpectedQuantityMicros: expectedQuantity, ActualQuantityMicros: actualQuantity, DeltaMicros: delta}
				if key.HasLot {
					lotID := key.LotID
					mismatch.LotID = &lotID
				}
				mismatches = append(mismatches, mismatch)
			}
		}
		if command.Repair {
			if mismatchCount > reconciliationSummaryLimit {
				return nil, ErrReconciliationTooLarge
			}
			scopes := make([]inventoryScope, 0, len(mismatches))
			for _, mismatch := range mismatches {
				if mismatch.ExpectedQuantityMicros < 0 {
					return nil, ErrReconciliationNegative
				}
				scopes = append(scopes, inventoryScope{TenantID: command.TenantID, LocationID: mismatch.LocationID, StorageAreaID: mismatch.StorageAreaID})
			}
			if err := s.fenceInventoryScopes(sc, scopes...); err != nil {
				return nil, err
			}
			for _, mismatch := range mismatches {
				key := newCountBalanceKey(mismatch.LocationID, mismatch.StorageAreaID, mismatch.ItemID, mismatch.LotID)
				filter := key.filter()
				filter["tenantId"] = command.TenantID
				setOnInsert := bson.M{"_id": primitive.NewObjectID(), "tenantId": command.TenantID, "locationId": key.LocationID, "storageAreaId": key.StorageAreaID, "itemId": key.ItemID}
				if key.HasLot {
					setOnInsert["lotId"] = key.LotID
				}
				targetQuantity := mismatch.ExpectedQuantityMicros
				if laterQuantity, ok := later[key]; ok {
					targetQuantity, err = safeAdd(targetQuantity, laterQuantity)
					if err != nil {
						return nil, ErrQuantityOverflow
					}
				}
				if _, err := s.db.StockBalances().UpdateOne(sc, filter, bson.M{"$set": bson.M{"quantityMicros": targetQuantity, "updatedAt": time.Now().UTC()}, "$setOnInsert": setOnInsert}, options.Update().SetUpsert(true)); err != nil {
					return nil, err
				}
			}
		}
		now := time.Now().UTC()
		status := models.ReconciliationDetected
		if command.Repair {
			status = models.ReconciliationRepaired
		}
		candidate := models.ReconciliationRun{ID: primitive.NewObjectID(), TenantID: command.TenantID, UserID: command.UserID, LocationID: command.LocationID, StorageAreaID: command.StorageAreaID, CutoffAt: cutoff, IdempotencyKey: strings.TrimSpace(command.IdempotencyKey), RequestHash: requestHash, Status: status, MismatchCount: mismatchCount, Truncated: mismatchCount > int64(len(mismatches)), Mismatches: mismatches, CreatedAt: now, CompletedAt: now}
		if err := validation.Validate(&candidate); err != nil {
			return nil, err
		}
		if _, err := s.db.ReconciliationRuns().InsertOne(sc, candidate); err != nil {
			return nil, err
		}
		run = &candidate
		return nil, nil
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			existing, lookupErr := s.findReconciliation(ctx, command.TenantID, command.IdempotencyKey)
			if lookupErr != nil {
				return nil, lookupErr
			}
			if existing != nil {
				if existing.RequestHash != requestHash {
					return nil, ErrIdempotencyConflict
				}
				return &ReconciliationResult{Run: *existing}, nil
			}
		}
		return nil, err
	}
	if s.logger != nil {
		if run.MismatchCount > 0 {
			s.logger.High(ctx, fmt.Sprintf("Inventory reconciliation mismatch: tenant=%s location=%s mismatches=%d repaired=%t", run.TenantID.Hex(), run.LocationID.Hex(), run.MismatchCount, command.Repair))
		} else {
			s.logger.Medium(ctx, fmt.Sprintf("Inventory reconciliation completed: tenant=%s location=%s repaired=%t", run.TenantID.Hex(), run.LocationID.Hex(), command.Repair))
		}
	}
	return &ReconciliationResult{Run: *run}, nil
}

func (s *Service) Reconcile(ctx context.Context, command ReconciliationCommand) (*ReconciliationResult, error) {
	return s.RunReconciliation(ctx, command)
}

func (s *Service) GetReconciliation(ctx context.Context, tenantID, runID primitive.ObjectID, locationIDs ...primitive.ObjectID) (*models.ReconciliationRun, error) {
	var run models.ReconciliationRun
	if err := s.db.ReconciliationRuns().FindOne(ctx, bson.M{"_id": runID, "tenantId": tenantID}).Decode(&run); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrReconciliationNotFound
		}
		return nil, err
	}
	if len(locationIDs) > 0 && !locationIDs[0].IsZero() && run.LocationID != locationIDs[0] {
		return nil, ErrReconciliationNotFound
	}
	return &run, nil
}

func (s *Service) verifyCountScope(ctx context.Context, tenantID, locationID, areaID primitive.ObjectID) error {
	if err := s.db.Locations().FindOne(ctx, bson.M{"_id": locationID, "tenantId": tenantID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		return ErrInvalidReference
	}
	if err := s.db.StorageAreas().FindOne(ctx, bson.M{"_id": areaID, "tenantId": tenantID, "locationId": locationID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		return ErrInvalidReference
	}
	return nil
}

func (s *Service) claimCountArea(ctx mongo.SessionContext, count models.StockCount) error {
	filter := bson.M{
		"_id":        count.StorageAreaID,
		"tenantId":   count.TenantID,
		"locationId": count.LocationID,
		"isActive":   true,
		"$or": bson.A{
			bson.M{"activeStockCountId": bson.M{"$exists": false}},
			bson.M{"activeStockCountId": nil},
		},
	}
	updated, err := s.db.StorageAreas().UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"activeStockCountId": count.ID},
		"$inc": bson.M{"inventoryFence": int64(1)},
	})
	if err != nil {
		return err
	}
	if updated.MatchedCount != 0 {
		return nil
	}
	var area struct {
		IsActive           bool                `bson:"isActive"`
		ActiveStockCountID *primitive.ObjectID `bson:"activeStockCountId"`
	}
	err = s.db.StorageAreas().FindOne(ctx, bson.M{"_id": count.StorageAreaID, "tenantId": count.TenantID, "locationId": count.LocationID}).Decode(&area)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrInvalidReference
	}
	if err != nil {
		return err
	}
	if area.ActiveStockCountID != nil {
		return ErrInventoryAreaLocked
	}
	if !area.IsActive {
		return ErrInvalidReference
	}
	return ErrInventoryAreaLocked
}

func (s *Service) verifyCountOwner(ctx context.Context, count models.StockCount) error {
	var area struct {
		ActiveStockCountID *primitive.ObjectID `bson:"activeStockCountId"`
	}
	err := s.db.StorageAreas().FindOne(ctx, bson.M{"_id": count.StorageAreaID, "tenantId": count.TenantID, "locationId": count.LocationID}, options.FindOne().SetProjection(bson.M{"activeStockCountId": 1})).Decode(&area)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrInvalidReference
	}
	if err != nil {
		return err
	}
	if area.ActiveStockCountID == nil {
		return ErrCountOwnershipRequired
	}
	if *area.ActiveStockCountID != count.ID {
		return ErrInventoryAreaLocked
	}
	return nil
}

func (s *Service) releaseCountArea(ctx mongo.SessionContext, count models.StockCount) error {
	released, err := s.releaseCountAreaIfOwned(ctx, count)
	if err != nil {
		return err
	}
	if !released {
		return ErrInventoryAreaLocked
	}
	return nil
}

func (s *Service) releaseCountAreaIfOwned(ctx mongo.SessionContext, count models.StockCount) (bool, error) {
	updated, err := s.db.StorageAreas().UpdateOne(ctx, bson.M{
		"_id":                count.StorageAreaID,
		"tenantId":           count.TenantID,
		"locationId":         count.LocationID,
		"activeStockCountId": count.ID,
	}, bson.M{"$unset": bson.M{"activeStockCountId": ""}})
	if err != nil {
		return false, err
	}
	return updated.MatchedCount != 0, nil
}

func (s *Service) verifyReconciliationScope(ctx context.Context, command ReconciliationCommand) error {
	if err := s.db.Locations().FindOne(ctx, bson.M{"_id": command.LocationID, "tenantId": command.TenantID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		return ErrInvalidReference
	}
	if command.StorageAreaID != nil {
		if err := s.db.StorageAreas().FindOne(ctx, bson.M{"_id": *command.StorageAreaID, "tenantId": command.TenantID, "locationId": command.LocationID, "isActive": true}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
			return ErrInvalidReference
		}
	}
	return nil
}

func (s *Service) loadCount(ctx context.Context, tenantID, countID primitive.ObjectID) (*models.StockCount, error) {
	var count models.StockCount
	if err := s.db.StockCounts().FindOne(ctx, bson.M{"_id": countID, "tenantId": tenantID}).Decode(&count); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrCountNotFound
		}
		return nil, err
	}
	return &count, nil
}

func (s *Service) enrichCountLines(ctx context.Context, tenantID primitive.ObjectID, lines []models.StockCountLine) ([]CountLineResult, error) {
	result := make([]CountLineResult, len(lines))
	lotIDs := make([]primitive.ObjectID, 0, len(lines))
	for index, line := range lines {
		result[index].StockCountLine = line
		if line.LotID != nil && !line.LotID.IsZero() {
			lotIDs = append(lotIDs, *line.LotID)
		}
	}
	if len(lotIDs) == 0 {
		return result, nil
	}
	cursor, err := s.db.StockLots().Find(ctx, bson.M{"tenantId": tenantID, "_id": bson.M{"$in": lotIDs}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var lots []models.StockLot
	if err := cursor.All(ctx, &lots); err != nil {
		return nil, err
	}
	byID := make(map[primitive.ObjectID]models.StockLot, len(lots))
	for _, lot := range lots {
		byID[lot.ID] = lot
	}
	for index, line := range lines {
		if line.LotID == nil {
			continue
		}
		lot, exists := byID[*line.LotID]
		// The item check is intentional. A malformed or cross-item lot document
		// must not turn into misleading line metadata.
		if !exists || lot.ItemID != line.ItemID {
			continue
		}
		result[index].LotCode = lot.Code
		result[index].LotStatus = lot.Status
		result[index].LotExpiresAt = lot.ExpiresAt
	}
	return result, nil
}

func (s *Service) countResult(ctx context.Context, count *models.StockCount) (*CountResult, error) {
	cursor, err := s.db.StockCountLines().Find(ctx, bson.M{"tenantId": count.TenantID, "countId": count.ID}, options.Find().SetSort(bson.D{{Key: "storageAreaId", Value: 1}, {Key: "itemId", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var lines []models.StockCountLine
	if err := cursor.All(ctx, &lines); err != nil {
		return nil, err
	}
	enriched, err := s.enrichCountLines(ctx, count.TenantID, lines)
	if err != nil {
		return nil, err
	}
	result := &CountResult{Count: *count, Lines: enriched}
	if count.PostPostingID != nil {
		var posting models.StockPosting
		if err := s.db.StockPostings().FindOne(ctx, bson.M{"_id": *count.PostPostingID, "tenantId": count.TenantID}).Decode(&posting); err != nil {
			return nil, err
		}
		result.Posting = &posting
		cursor, err := s.db.StockMovements().Find(ctx, bson.M{"tenantId": count.TenantID, "postingId": posting.ID}, options.Find().SetSort(bson.D{{Key: "lineNumber", Value: 1}, {Key: "_id", Value: 1}}))
		if err != nil {
			return nil, err
		}
		defer cursor.Close(ctx)
		movements := make([]models.StockMovement, 0)
		if err := cursor.All(ctx, &movements); err != nil {
			return nil, err
		}
		result.Movements = movements
	}
	return result, nil
}

func (s *Service) findCountByIdempotency(ctx context.Context, tenantID primitive.ObjectID, key string) (*models.StockCount, error) {
	var count models.StockCount
	err := s.db.StockCounts().FindOne(ctx, bson.M{"tenantId": tenantID, "idempotencyKey": strings.TrimSpace(key)}).Decode(&count)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &count, nil
}

func (s *Service) findReconciliation(ctx context.Context, tenantID primitive.ObjectID, key string) (*models.ReconciliationRun, error) {
	var run models.ReconciliationRun
	err := s.db.ReconciliationRuns().FindOne(ctx, bson.M{"tenantId": tenantID, "idempotencyKey": strings.TrimSpace(key)}).Decode(&run)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *Service) sumAllMovements(ctx context.Context, tenantID, locationID primitive.ObjectID, areaID *primitive.ObjectID) (map[countBalanceKey]int64, error) {
	filter := bson.M{"tenantId": tenantID, "locationId": locationID}
	if areaID != nil {
		filter["storageAreaId"] = *areaID
	}
	cursor, err := s.db.StockMovements().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var movements []models.StockMovement
	if err := cursor.All(ctx, &movements); err != nil {
		return nil, err
	}
	sums := make(map[countBalanceKey]int64, len(movements))
	for _, movement := range movements {
		key := newCountBalanceKey(movement.LocationID, movement.StorageAreaID, movement.ItemID, movement.LotID)
		value, err := safeAdd(sums[key], movement.QuantityMicros)
		if err != nil {
			return nil, ErrQuantityOverflow
		}
		sums[key] = value
	}
	return sums, nil
}

func (s *Service) exactMovementQuantity(ctx mongo.SessionContext, tenantID, locationID, areaID, itemID primitive.ObjectID, lotID *primitive.ObjectID) (int64, error) {
	filter := bson.M{"tenantId": tenantID, "locationId": locationID, "storageAreaId": areaID, "itemId": itemID, "lotId": nil}
	if lotID != nil {
		filter["lotId"] = *lotID
	}
	cursor, err := s.db.StockMovements().Find(ctx, filter)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)
	var movements []models.StockMovement
	if err := cursor.All(ctx, &movements); err != nil {
		return 0, err
	}
	var total int64
	for _, movement := range movements {
		total, err = safeAdd(total, movement.QuantityMicros)
		if err != nil {
			return 0, ErrQuantityOverflow
		}
	}
	return total, nil
}

func (s *Service) sumMovements(ctx context.Context, tenantID, locationID primitive.ObjectID, areaID *primitive.ObjectID, cutoff time.Time) (map[countBalanceKey]int64, error) {
	filter := bson.M{"tenantId": tenantID, "locationId": locationID, "recordedAt": bson.M{"$lte": cutoff}}
	if areaID != nil {
		filter["storageAreaId"] = *areaID
	}
	cursor, err := s.db.StockMovements().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var movements []models.StockMovement
	if err := cursor.All(ctx, &movements); err != nil {
		return nil, err
	}
	sums := make(map[countBalanceKey]int64, len(movements))
	for _, movement := range movements {
		key := newCountBalanceKey(movement.LocationID, movement.StorageAreaID, movement.ItemID, movement.LotID)
		value, err := safeAdd(sums[key], movement.QuantityMicros)
		if err != nil {
			return nil, ErrQuantityOverflow
		}
		sums[key] = value
	}
	return sums, nil
}

func (s *Service) sumMovementsAfter(ctx context.Context, tenantID, locationID primitive.ObjectID, areaID *primitive.ObjectID, cutoff time.Time) (map[countBalanceKey]int64, error) {
	filter := bson.M{"tenantId": tenantID, "locationId": locationID, "recordedAt": bson.M{"$gt": cutoff}}
	if areaID != nil {
		filter["storageAreaId"] = *areaID
	}
	cursor, err := s.db.StockMovements().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var movements []models.StockMovement
	if err := cursor.All(ctx, &movements); err != nil {
		return nil, err
	}
	sums := make(map[countBalanceKey]int64, len(movements))
	for _, movement := range movements {
		key := newCountBalanceKey(movement.LocationID, movement.StorageAreaID, movement.ItemID, movement.LotID)
		value, err := safeAdd(sums[key], movement.QuantityMicros)
		if err != nil {
			return nil, ErrQuantityOverflow
		}
		sums[key] = value
	}
	return sums, nil
}

func (s *Service) loadBalances(ctx context.Context, tenantID, locationID primitive.ObjectID, areaID *primitive.ObjectID) (map[countBalanceKey]int64, error) {
	filter := bson.M{"tenantId": tenantID, "locationId": locationID}
	if areaID != nil {
		filter["storageAreaId"] = *areaID
	}
	cursor, err := s.db.StockBalances().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var balances []models.StockBalance
	if err := cursor.All(ctx, &balances); err != nil {
		return nil, err
	}
	result := make(map[countBalanceKey]int64, len(balances))
	for _, balance := range balances {
		result[newCountBalanceKey(balance.LocationID, balance.StorageAreaID, balance.ItemID, balance.LotID)] = balance.QuantityMicros
	}
	return result, nil
}

func safeAdd(left, right int64) (int64, error) {
	if right > 0 && left > math.MaxInt64-right || right < 0 && left < math.MinInt64-right {
		return 0, ErrQuantityOverflow
	}
	return left + right, nil
}

func safeSub(left, right int64) (int64, error) {
	if right == math.MinInt64 {
		return 0, ErrQuantityOverflow
	}
	return safeAdd(left, -right)
}

func countPostHash(command CountPostCommand, count models.StockCount) string {
	value := struct {
		TenantID, UserID, CountID primitive.ObjectID
		Key                       string
	}{command.TenantID, command.UserID, command.CountID, command.IdempotencyKey}
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func countCreateHash(command CountCreateCommand) string {
	value := struct {
		TenantID, UserID, LocationID, StorageAreaID primitive.ObjectID
	}{command.TenantID, command.UserID, command.LocationID, command.StorageAreaID}
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func reconciliationHash(command ReconciliationCommand) string {
	value := struct {
		TenantID, UserID, LocationID primitive.ObjectID
		StorageAreaID                *primitive.ObjectID
		Repair                       bool
	}{command.TenantID, command.UserID, command.LocationID, command.StorageAreaID, command.Repair}
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
