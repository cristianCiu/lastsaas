package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type RestaurantSettings struct {
	ID              *primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty" validate:"omitempty"`
	TenantID        primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	Currency        string              `json:"currency" bson:"currency" validate:"required,currency_code"`
	Language        string              `json:"language" bson:"language" validate:"required,language_tag"`
	DefaultTimezone string              `json:"defaultTimezone" bson:"defaultTimezone" validate:"required,max=100,iana_timezone"`
	Version         int64               `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt       time.Time           `json:"createdAt,omitempty" bson:"createdAt" validate:"required"`
	UpdatedAt       time.Time           `json:"updatedAt,omitempty" bson:"updatedAt" validate:"required"`
}

type StorageAreaType string

const (
	StorageAreaRefrigerated StorageAreaType = "refrigerated"
	StorageAreaFrozen       StorageAreaType = "frozen"
	StorageAreaBar          StorageAreaType = "bar"
	StorageAreaDry          StorageAreaType = "dry"
	StorageAreaOther        StorageAreaType = "other"
)

func ValidStorageAreaType(value StorageAreaType) bool {
	switch value {
	case StorageAreaRefrigerated, StorageAreaFrozen, StorageAreaBar, StorageAreaDry, StorageAreaOther:
		return true
	default:
		return false
	}
}

type StorageArea struct {
	ID         primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	TenantID   primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	LocationID primitive.ObjectID `json:"locationId" bson:"locationId" validate:"required"`
	Name       string             `json:"name" bson:"name" validate:"required,min=1,max=200"`
	Type       StorageAreaType    `json:"type" bson:"type" validate:"required,storage_area_type"`
	IsActive   bool               `json:"isActive" bson:"isActive"`
	Version    int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt  time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt  time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}
