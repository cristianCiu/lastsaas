package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Item struct {
	ID            primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	TenantID      primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	SKU           string             `json:"sku" bson:"sku" validate:"required,max=64,location_code"`
	Name          string             `json:"name" bson:"name" validate:"required,not_blank,max=160"`
	Description   string             `json:"description,omitempty" bson:"description,omitempty" validate:"omitempty,max=2000"`
	Brand         string             `json:"brand,omitempty" bson:"brand,omitempty" validate:"omitempty,max=100"`
	CategoryID    primitive.ObjectID `json:"categoryId" bson:"categoryId" validate:"required"`
	BaseUnitID    primitive.ObjectID `json:"baseUnitId" bson:"baseUnitId" validate:"required"`
	Allergens     []string           `json:"allergens,omitempty" bson:"allergens,omitempty" validate:"omitempty,dive,eu_allergen"`
	ShelfLifeDays *int32             `json:"shelfLifeDays,omitempty" bson:"shelfLifeDays,omitempty" validate:"omitempty,gte=0,lte=36500"`
	Stockable     bool               `json:"stockable" bson:"stockable"`
	LotTracking   LotTrackingMode    `json:"lotTracking,omitempty" bson:"lotTracking,omitempty" validate:"lot_tracking_mode"`
	IsActive      bool               `json:"isActive" bson:"isActive"`
	Version       int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt     time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt     time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

// ItemConversion defines how many base units are represented by one source unit.
// The stored numerator and denominator are always reduced to lowest terms.
type ItemConversion struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	TenantID    primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	ItemID      primitive.ObjectID `json:"itemId" bson:"itemId" validate:"required"`
	FromUnitID  primitive.ObjectID `json:"fromUnitId" bson:"fromUnitId" validate:"required"`
	Numerator   int64              `json:"numerator" bson:"numerator" validate:"gt=0,lte=1000000000"`
	Denominator int64              `json:"denominator" bson:"denominator" validate:"gt=0,lte=1000000000"`
	IsActive    bool               `json:"isActive" bson:"isActive"`
	Version     int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt   time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt   time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

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

// Category is a tenant-wide, flat catalog category.
type Category struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	TenantID  primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	Code      string             `json:"code" bson:"code" validate:"required,max=32,location_code"`
	Name      string             `json:"name" bson:"name" validate:"required,min=1,max=100"`
	IsActive  bool               `json:"isActive" bson:"isActive"`
	Version   int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type Supplier struct {
	ID                  primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	TenantID            primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	Code                string             `json:"code" bson:"code" validate:"required,max=64,location_code"`
	Name                string             `json:"name" bson:"name" validate:"required,not_blank,max=160"`
	ContactName         string             `json:"contactName,omitempty" bson:"contactName,omitempty" validate:"omitempty,max=100"`
	Email               string             `json:"email,omitempty" bson:"email,omitempty" validate:"omitempty,supplier_email,max=254"`
	Phone               string             `json:"phone,omitempty" bson:"phone,omitempty" validate:"omitempty,max=32"`
	OrderingDays        []int32            `json:"orderingDays" bson:"orderingDays" validate:"dive,gte=1,lte=7"`
	DefaultLeadTimeDays int32              `json:"defaultLeadTimeDays" bson:"defaultLeadTimeDays" validate:"gte=0,lte=3650"`
	IsActive            bool               `json:"isActive" bson:"isActive"`
	Version             int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt           time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt           time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type SupplierItem struct {
	ID             primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	TenantID       primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	SupplierID     primitive.ObjectID `json:"supplierId" bson:"supplierId" validate:"required"`
	ItemID         primitive.ObjectID `json:"itemId" bson:"itemId" validate:"required"`
	SupplierSKU    string             `json:"supplierSKU,omitempty" bson:"supplierSKU,omitempty" validate:"omitempty,max=100"`
	PackSizeMicros int64              `json:"packSizeMicros,string" bson:"packSizeMicros" validate:"gt=0"`
	MOQ            int32              `json:"moq" bson:"moq" validate:"gte=1,lte=1000000"`
	UnitPriceMinor int64              `json:"unitPriceMinor,string" bson:"unitPriceMinor" validate:"gte=0"`
	Currency       string             `json:"currency" bson:"currency" validate:"required,currency_code"`
	LeadTimeDays   *int32             `json:"leadTimeDays,omitempty" bson:"leadTimeDays,omitempty" validate:"omitempty,gte=0,lte=3650"`
	IsActive       bool               `json:"isActive" bson:"isActive"`
	Version        int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt      time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt      time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}
