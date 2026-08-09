package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TenantBrandingAssetKind string

const (
	TenantBrandingAssetPrimary TenantBrandingAssetKind = "primary"
	TenantBrandingAssetCompact TenantBrandingAssetKind = "compact"
)

func ValidTenantBrandingAssetKind(value TenantBrandingAssetKind) bool {
	return value == TenantBrandingAssetPrimary || value == TenantBrandingAssetCompact
}

type TenantBrandingAsset struct {
	ID          primitive.ObjectID      `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID    primitive.ObjectID      `json:"-" bson:"tenantId" validate:"required"`
	Kind        TenantBrandingAssetKind `json:"kind" bson:"kind" validate:"required,tenant_branding_asset_kind"`
	StorageKey  string                  `json:"-" bson:"storageKey" validate:"required,len=32,hexadecimal"`
	ContentType string                  `json:"contentType" bson:"contentType" validate:"required,oneof=image/png image/jpeg"`
	Data        []byte                  `json:"-" bson:"data" validate:"required,max=921600"`
	Size        int64                   `json:"size" bson:"size" validate:"required,gt=0,lte=921600"`
	Width       int                     `json:"width" bson:"width" validate:"required,gte=16,lte=2048"`
	Height      int                     `json:"height" bson:"height" validate:"required,gte=16,lte=2048"`
	Version     int64                   `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt   time.Time               `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt   time.Time               `json:"updatedAt" bson:"updatedAt" validate:"required"`
}
