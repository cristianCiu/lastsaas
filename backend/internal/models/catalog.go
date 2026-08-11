package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type UnitDimension string

const (
	UnitDimensionMass   UnitDimension = "mass"
	UnitDimensionVolume UnitDimension = "volume"
	UnitDimensionCount  UnitDimension = "count"
)

func ValidUnitDimension(value UnitDimension) bool {
	return value == UnitDimensionMass || value == UnitDimensionVolume || value == UnitDimensionCount
}

type Unit struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	TenantID  primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	Code      string             `json:"code" bson:"code" validate:"required,max=32,location_code"`
	Name      string             `json:"name" bson:"name" validate:"required,min=1,max=100"`
	Symbol    string             `json:"symbol" bson:"symbol" validate:"required,min=1,max=16"`
	Dimension UnitDimension      `json:"dimension" bson:"dimension" validate:"required,unit_dimension"`
	Precision int32              `json:"precision" bson:"precision" validate:"gte=0,lte=6"`
	IsActive  bool               `json:"isActive" bson:"isActive"`
	Version   int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}
