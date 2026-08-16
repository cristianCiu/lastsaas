package offboarding

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrTenantNotFound      = errors.New("tenant not found")
	ErrOwnerAuthorization  = errors.New("tenant owner authorization required")
	ErrRootTenant          = errors.New("the root tenant cannot be offboarded")
	ErrOffboardingTooLarge = errors.New("tenant has too many actors to offboard")
)

type Result struct {
	TombstoneID            primitive.ObjectID             `json:"tombstoneId"`
	Status                 models.TenantOffboardingStatus `json:"status"`
	RevokedRefreshTokens   int64                          `json:"revokedRefreshTokens"`
	DeletedProfiles        int64                          `json:"deletedProfiles"`
	DeletedAccounts        int64                          `json:"deletedAccounts"`
	PseudonymizedDocuments int64                          `json:"pseudonymizedDocuments"`
}

type Service struct {
	db *db.MongoDB
}

func NewService(database *db.MongoDB) *Service { return &Service{db: database} }

// Offboard starts or resumes the one-way offboarding operation for tenantID.
// The tombstone is created before any identity data is removed, making retries
// safe after a process interruption. Operational and journal documents are
// never deleted by this service.
func (s *Service) Offboard(ctx context.Context, tenantID, requesterID primitive.ObjectID) (Result, error) {
	var tenant models.Tenant
	if err := s.db.Tenants().FindOne(ctx, bson.M{"_id": tenantID}).Decode(&tenant); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return Result{}, ErrTenantNotFound
		}
		return Result{}, fmt.Errorf("load tenant: %w", err)
	}
	if tenant.IsRoot {
		return Result{}, ErrRootTenant
	}
	// A completed tombstone is an idempotent no-op. It is checked before the
	// membership lookup because completion intentionally erases memberships.
	var existing models.TenantOffboardingTombstone
	if err := s.db.TenantOffboardingTombstones().FindOne(ctx, bson.M{"tenantId": tenantID, "status": models.TenantOffboardingCompleted}).Decode(&existing); err == nil {
		if existing.OwnerID != requesterID {
			return Result{}, ErrOwnerAuthorization
		}
		return s.completedResult(ctx, existing)
	}

	var membership models.TenantMembership
	if err := s.db.TenantMemberships().FindOne(ctx, bson.M{
		"tenantId": tenantID, "userId": requesterID, "role": models.RoleOwner,
	}).Decode(&membership); err != nil {
		return Result{}, ErrOwnerAuthorization
	}

	tombstone, err := s.ensureTombstone(ctx, tenantID, requesterID)
	if err != nil {
		return Result{}, err
	}
	if tombstone.Status == models.TenantOffboardingCompleted {
		if tombstone.OwnerID != requesterID {
			return Result{}, ErrOwnerAuthorization
		}
		return s.completedResult(ctx, tombstone)
	}

	// This update is deliberately separate from the eventual cleanup. Once it
	// succeeds RequireTenant can no longer admit new tenant requests.
	if _, err := s.db.Tenants().UpdateOne(ctx, bson.M{"_id": tenantID}, bson.M{"$set": bson.M{
		"isActive": false, "offboardingStatus": models.TenantOffboardingStarted,
		"offboardingTombstoneId": tombstone.ID, "updatedAt": time.Now(),
	}}); err != nil {
		return Result{}, fmt.Errorf("fence tenant: %w", err)
	}

	result := Result{TombstoneID: tombstone.ID, Status: models.TenantOffboardingStarted}
	result.RevokedRefreshTokens, err = s.revokeCredentials(ctx, tombstone)
	if err != nil {
		return Result{}, err
	}
	result.DeletedProfiles, err = s.eraseProfiles(ctx, tenantID)
	if err != nil {
		return Result{}, err
	}
	result.PseudonymizedDocuments, err = s.pseudonymizeRetained(ctx, tenantID, tombstone)
	if err != nil {
		return Result{}, err
	}
	result.DeletedAccounts, err = s.eraseAccounts(ctx, tenantID, tombstone)
	if err != nil {
		return Result{}, err
	}

	// Memberships are removed last. Until this point the owner can retry a
	// partially completed request without any alternate authorization path.
	if _, err := s.db.TenantMemberships().DeleteMany(ctx, bson.M{"tenantId": tenantID}); err != nil {
		return Result{}, fmt.Errorf("delete tenant memberships: %w", err)
	}

	now := time.Now()
	if _, err := s.db.TenantOffboardingTombstones().UpdateOne(ctx,
		bson.M{"_id": tombstone.ID, "status": models.TenantOffboardingStarted},
		bson.M{"$set": bson.M{"status": models.TenantOffboardingCompleted, "completedAt": now, "updatedAt": now}, "$inc": bson.M{"version": 1}},
	); err != nil {
		return Result{}, fmt.Errorf("complete tombstone: %w", err)
	}
	if _, err := s.db.Tenants().UpdateOne(ctx, bson.M{"_id": tenantID}, bson.M{"$set": bson.M{
		"isActive": false, "offboardingStatus": models.TenantOffboardingCompleted,
		"offboardedAt": now, "updatedAt": now,
		// Tenant identity is no longer useful after offboarding. Keep only an
		// opaque, stable label so the inaccessible marker remains addressable.
		"name": "offboarded-" + tenantID.Hex(), "slug": "offboarded-" + tenantID.Hex(),
		"stripeCustomerId": "", "stripeSubscriptionId": "", "billingInterval": "",
	}}); err != nil {
		return Result{}, fmt.Errorf("complete tenant: %w", err)
	}

	result.Status = models.TenantOffboardingCompleted
	if err := s.recordAudit(ctx, tombstone, models.TenantOffboardingAudit{
		Event: string(models.TenantOffboardingCompleted), RevokedRefreshTokens: result.RevokedRefreshTokens,
		DeletedProfiles: result.DeletedProfiles, DeletedAccounts: result.DeletedAccounts,
		PseudonymizedDocuments: result.PseudonymizedDocuments,
	}); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (s *Service) completedResult(ctx context.Context, tombstone models.TenantOffboardingTombstone) (Result, error) {
	result := Result{TombstoneID: tombstone.ID, Status: tombstone.Status}
	var audit models.TenantOffboardingAudit
	if err := s.db.TenantOffboardingAudit().FindOne(ctx, bson.M{"tombstoneId": tombstone.ID, "event": models.TenantOffboardingCompleted}).Decode(&audit); err == nil {
		result.RevokedRefreshTokens = audit.RevokedRefreshTokens
		result.DeletedProfiles = audit.DeletedProfiles
		result.DeletedAccounts = audit.DeletedAccounts
		result.PseudonymizedDocuments = audit.PseudonymizedDocuments
	}
	return result, nil
}

func (s *Service) recordAudit(ctx context.Context, tombstone models.TenantOffboardingTombstone, audit models.TenantOffboardingAudit) error {
	audit.ID = primitive.NewObjectID()
	audit.TenantID = tombstone.TenantID
	audit.TombstoneID = tombstone.ID
	audit.CreatedAt = time.Now()
	if _, err := s.db.TenantOffboardingAudit().InsertOne(ctx, audit); err != nil && !mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("write offboarding audit: %w", err)
	}
	return nil
}

func (s *Service) ensureTombstone(ctx context.Context, tenantID, ownerID primitive.ObjectID) (models.TenantOffboardingTombstone, error) {
	var existing models.TenantOffboardingTombstone
	err := s.db.TenantOffboardingTombstones().FindOne(ctx, bson.M{"tenantId": tenantID}).Decode(&existing)
	if err == nil {
		if !existing.OwnerID.IsZero() && existing.OwnerID != ownerID {
			return models.TenantOffboardingTombstone{}, ErrOwnerAuthorization
		}
		if existing.OwnerID.IsZero() {
			if _, updateErr := s.db.TenantOffboardingTombstones().UpdateOne(ctx, bson.M{"_id": existing.ID, "ownerId": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"ownerId": ownerID, "updatedAt": time.Now()}}); updateErr != nil {
				return models.TenantOffboardingTombstone{}, fmt.Errorf("bind tombstone owner: %w", updateErr)
			}
			existing.OwnerID = ownerID
		}
		return existing, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return models.TenantOffboardingTombstone{}, fmt.Errorf("load tombstone: %w", err)
	}

	actors, err := s.actorIDs(ctx, tenantID)
	if err != nil {
		return models.TenantOffboardingTombstone{}, err
	}
	if len(actors) > 10000 {
		return models.TenantOffboardingTombstone{}, ErrOffboardingTooLarge
	}
	now := time.Now()
	tombstone := models.TenantOffboardingTombstone{
		ID: primitive.NewObjectID(), TenantID: tenantID, Status: models.TenantOffboardingStarted,
		OwnerID:         ownerID,
		ActorPseudonyms: make([]models.TenantActorPseudonym, 0, len(actors)), Version: 1,
		StartedAt: now, UpdatedAt: now,
	}
	for _, actor := range actors {
		tombstone.ActorPseudonyms = append(tombstone.ActorPseudonyms, models.TenantActorPseudonym{
			ActorID: actor, Pseudonym: pseudonym(tenantID, actor),
		})
	}
	if _, err := s.db.TenantOffboardingTombstones().InsertOne(ctx, tombstone); err != nil {
		// A concurrent request owns the unique tenant tombstone. Re-read it and
		// continue rather than creating a second pseudonym mapping.
		if mongo.IsDuplicateKeyError(err) {
			if readErr := s.db.TenantOffboardingTombstones().FindOne(ctx, bson.M{"tenantId": tenantID}).Decode(&existing); readErr == nil {
				return existing, nil
			}
		}
		return models.TenantOffboardingTombstone{}, fmt.Errorf("create tombstone: %w", err)
	}
	if err := s.recordAudit(ctx, tombstone, models.TenantOffboardingAudit{Event: string(models.TenantOffboardingStarted)}); err != nil {
		return models.TenantOffboardingTombstone{}, err
	}
	return tombstone, nil
}

func pseudonym(tenantID, actorID primitive.ObjectID) string {
	h := sha256.Sum256([]byte("lastsaas/offboarding/v1/" + tenantID.Hex() + "/" + actorID.Hex()))
	var id primitive.ObjectID
	copy(id[:], h[:12])
	if id.IsZero() || id == actorID {
		h[0] ^= 0xff
		copy(id[:], h[:12])
	}
	return id.Hex()
}

func (s *Service) actorIDs(ctx context.Context, tenantID primitive.ObjectID) ([]primitive.ObjectID, error) {
	seen := make(map[primitive.ObjectID]struct{})
	add := func(values []interface{}) {
		for _, value := range values {
			if id, ok := value.(primitive.ObjectID); ok && !id.IsZero() {
				seen[id] = struct{}{}
			}
		}
	}
	values, err := s.db.TenantMemberships().Distinct(ctx, "userId", bson.M{"tenantId": tenantID})
	if err != nil {
		return nil, fmt.Errorf("find tenant actors: %w", err)
	}
	add(values)
	for _, collection := range retainedCollections {
		for _, field := range actorFields {
			values, err = s.db.Database.Collection(collection).Distinct(ctx, field, bson.M{"tenantId": tenantID})
			if err != nil {
				return nil, fmt.Errorf("find actors in %s: %w", collection, err)
			}
			add(values)
		}
	}
	actors := make([]primitive.ObjectID, 0, len(seen))
	for id := range seen {
		actors = append(actors, id)
	}
	return actors, nil
}

func (s *Service) revokeCredentials(ctx context.Context, tombstone models.TenantOffboardingTombstone) (int64, error) {
	ids := actorIDsFromTombstone(tombstone)
	if len(ids) == 0 {
		return 0, nil
	}
	refresh, err := s.db.RefreshTokens().UpdateMany(ctx, bson.M{"userId": bson.M{"$in": ids}}, bson.M{"$set": bson.M{"isRevoked": true}})
	if err != nil {
		return 0, fmt.Errorf("revoke refresh tokens: %w", err)
	}
	for _, collection := range []*mongo.Collection{s.db.VerificationTokens(), s.db.AuthCodes(), s.db.WebAuthnCredentials(), s.db.WebAuthnSessions(), s.db.Messages()} {
		if _, err := collection.DeleteMany(ctx, bson.M{"userId": bson.M{"$in": ids}}); err != nil {
			return 0, fmt.Errorf("erase account credentials: %w", err)
		}
	}
	if _, err := s.db.APIKeys().UpdateMany(ctx, bson.M{"createdBy": bson.M{"$in": ids}}, bson.M{"$set": bson.M{"isActive": false}}); err != nil {
		return 0, fmt.Errorf("revoke api keys: %w", err)
	}
	return refresh.ModifiedCount, nil
}

func (s *Service) eraseProfiles(ctx context.Context, tenantID primitive.ObjectID) (int64, error) {
	result, err := s.db.StaffProfiles().DeleteMany(ctx, bson.M{"tenantId": tenantID})
	if err != nil {
		return 0, fmt.Errorf("erase staff profiles: %w", err)
	}
	if _, err := s.db.Invitations().DeleteMany(ctx, bson.M{"tenantId": tenantID}); err != nil {
		return 0, fmt.Errorf("erase invitations: %w", err)
	}
	if _, err := s.db.SSOConnections().DeleteMany(ctx, bson.M{"tenantId": tenantID}); err != nil {
		return 0, fmt.Errorf("erase sso credentials: %w", err)
	}
	return result.DeletedCount, nil
}

var retainedCollections = []string{
	"audit_log", "financial_transactions", "goods_receipts", "import_runs", "purchase_orders",
	"reconciliation_runs", "sales", "sales_import_runs", "stock_counts", "stock_postings",
	"system_logs", "telemetry_events", "usage_events", "forecast_datasets", "forecast_jobs",
	"forecast_runs", "forecast_points", "forecast_metrics", "forecast_policies", "guest_plans",
	"forecast_overrides", "reorder_recommendations", "forecast_coverages", "purchase_order_lines",
	"goods_receipt_lines", "sales_lines", "unresolved_sale_lines", "stock_movements", "stock_balances",
	"stock_lots", "stock_count_lines", "purchase_order_email_deliveries",
}

var actorFields = []string{
	"userId", "createdBy", "invitedBy", "submittedBy", "approvedBy", "supplierConfirmedBy",
	"cancelledBy", "sealedBy", "performedBy", "recordedBy",
}

func (s *Service) pseudonymizeRetained(ctx context.Context, tenantID primitive.ObjectID, tombstone models.TenantOffboardingTombstone) (int64, error) {
	total := int64(0)
	for _, actor := range tombstone.ActorPseudonyms {
		pseudo, err := primitive.ObjectIDFromHex(actor.Pseudonym)
		if err != nil {
			return 0, fmt.Errorf("invalid tombstone pseudonym: %w", err)
		}
		for _, collection := range retainedCollections {
			coll := s.db.Database.Collection(collection)
			for _, field := range actorFields {
				result, err := coll.UpdateMany(ctx, bson.M{"tenantId": tenantID, field: actor.ActorID}, bson.M{"$set": bson.M{field: pseudo}})
				if err != nil {
					return 0, fmt.Errorf("pseudonymize %s.%s: %w", collection, field, err)
				}
				total += result.ModifiedCount
			}
			// Purchase order audit entries are immutable nested actor fields.
			result, err := coll.UpdateMany(ctx, bson.M{"tenantId": tenantID, "audit.userId": actor.ActorID}, bson.M{"$set": bson.M{"audit.$[entry].userId": pseudo}}, optionsArrayFilter(actor.ActorID))
			if err != nil {
				return 0, fmt.Errorf("pseudonymize %s audit: %w", collection, err)
			}
			total += result.ModifiedCount
		}
	}
	return total, nil
}

func optionsArrayFilter(actor primitive.ObjectID) *options.UpdateOptions {
	return mongoOptionsWithArrayFilter(bson.M{"entry.userId": actor})
}

// Kept in a helper to make the update options construction easy to audit.
func mongoOptionsWithArrayFilter(filter bson.M) *options.UpdateOptions {
	return mongoOptions(filter)
}

func mongoOptions(filter bson.M) *options.UpdateOptions {
	return options.Update().SetArrayFilters(options.ArrayFilters{Filters: []interface{}{filter}})
}

func actorIDsFromTombstone(tombstone models.TenantOffboardingTombstone) []primitive.ObjectID {
	ids := make([]primitive.ObjectID, 0, len(tombstone.ActorPseudonyms))
	for _, actor := range tombstone.ActorPseudonyms {
		ids = append(ids, actor.ActorID)
	}
	return ids
}

func (s *Service) eraseAccounts(ctx context.Context, tenantID primitive.ObjectID, tombstone models.TenantOffboardingTombstone) (int64, error) {
	deleted := int64(0)
	for _, actor := range tombstone.ActorPseudonyms {
		count, err := s.db.TenantMemberships().CountDocuments(ctx, bson.M{"userId": actor.ActorID, "tenantId": bson.M{"$ne": tenantID}})
		if err != nil {
			return 0, fmt.Errorf("check shared account: %w", err)
		}
		if count != 0 {
			continue
		}
		result, err := s.db.Users().DeleteOne(ctx, bson.M{"_id": actor.ActorID})
		if err != nil {
			return 0, fmt.Errorf("erase account: %w", err)
		}
		deleted += result.DeletedCount
	}
	return deleted, nil
}
