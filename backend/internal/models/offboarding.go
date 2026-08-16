package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TenantActorPseudonym is the durable re-application record for one actor.
// ActorID is only an internal identifier; no account/profile attributes are
// stored in a tombstone. OwnerID is only an opaque authorization handle.
// Pseudonym is a deterministic opaque ObjectID encoded
// as lower-case hex so retained ObjectID actor fields remain type-safe.
type TenantActorPseudonym struct {
	ActorID   primitive.ObjectID `json:"actorId" bson:"actorId" validate:"required"`
	Pseudonym string             `json:"pseudonym" bson:"pseudonym" validate:"required,len=24,hexadecimal"`
}

type TenantOffboardingTombstone struct {
	ID              primitive.ObjectID      `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID        primitive.ObjectID      `json:"tenantId" bson:"tenantId" validate:"required"`
	OwnerID         primitive.ObjectID      `json:"-" bson:"ownerId" validate:"required"`
	Status          TenantOffboardingStatus `json:"status" bson:"status" validate:"required,oneof=started completed"`
	ActorPseudonyms []TenantActorPseudonym  `json:"actorPseudonyms" bson:"actorPseudonyms" validate:"required,max=10000,dive"`
	Version         int64                   `json:"version" bson:"version" validate:"gte=1"`
	StartedAt       time.Time               `json:"startedAt" bson:"startedAt" validate:"required"`
	CompletedAt     *time.Time              `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
	UpdatedAt       time.Time               `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

// TenantOffboardingAudit contains only opaque tenant/tombstone identifiers,
// event state, and bounded counters. It intentionally has no user, email,
// name, address, URL, or free-form metadata fields.
type TenantOffboardingAudit struct {
	ID                     primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID               primitive.ObjectID `json:"tenantId" bson:"tenantId" validate:"required"`
	TombstoneID            primitive.ObjectID `json:"tombstoneId" bson:"tombstoneId" validate:"required"`
	Event                  string             `json:"event" bson:"event" validate:"required,oneof=started completed"`
	RevokedRefreshTokens   int64              `json:"revokedRefreshTokens" bson:"revokedRefreshTokens" validate:"gte=0"`
	DeletedProfiles        int64              `json:"deletedProfiles" bson:"deletedProfiles" validate:"gte=0"`
	DeletedAccounts        int64              `json:"deletedAccounts" bson:"deletedAccounts" validate:"gte=0"`
	PseudonymizedDocuments int64              `json:"pseudonymizedDocuments" bson:"pseudonymizedDocuments" validate:"gte=0"`
	CreatedAt              time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
}
