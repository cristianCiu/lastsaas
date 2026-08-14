package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Recipe is the stable identity of a recipe. Its contents are changed by
// publishing a new RecipeVersion, never by changing a released version.
type Recipe struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID    primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	Code        string             `json:"code" bson:"code" validate:"required,max=64,location_code"`
	Name        string             `json:"name" bson:"name" validate:"required,not_blank,max=160"`
	Description string             `json:"description,omitempty" bson:"description,omitempty" validate:"omitempty,max=2000"`
	IsActive    bool               `json:"isActive" bson:"isActive"`
	Version     int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt   time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt   time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type RecipeVersionStatus string

const (
	RecipeVersionDraft    RecipeVersionStatus = "draft"
	RecipeVersionReleased RecipeVersionStatus = "released"
	RecipeVersionRetired  RecipeVersionStatus = "retired"
)

func ValidRecipeVersionStatus(status RecipeVersionStatus) bool {
	return status == RecipeVersionDraft || status == RecipeVersionReleased || status == RecipeVersionRetired
}

// RecipeVersion is an immutable snapshot of a recipe's production rules.
// EffectiveTo is exclusive; nil means open ended.
type RecipeVersion struct {
	ID                primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID          primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	RecipeID          primitive.ObjectID  `json:"recipeId" bson:"recipeId" validate:"required"`
	Number            int64               `json:"number" bson:"number" validate:"gte=1"`
	OutputUnitID      primitive.ObjectID  `json:"outputUnitId" bson:"outputUnitId" validate:"required"`
	PortionCount      int64               `json:"portionCount" bson:"portionCount" validate:"gt=0"`
	YieldFactorMicros int64               `json:"yieldFactorMicros,string" bson:"yieldFactorMicros" validate:"gt=0,lte=1000000"`
	LossFactorMicros  int64               `json:"lossFactorMicros,string" bson:"lossFactorMicros" validate:"gte=0,lte=1000000"`
	EffectiveFrom     time.Time           `json:"effectiveFrom" bson:"effectiveFrom" validate:"required"`
	EffectiveTo       *time.Time          `json:"effectiveTo,omitempty" bson:"effectiveTo,omitempty"`
	Status            RecipeVersionStatus `json:"status" bson:"status" validate:"required,recipe_version_status"`
	Version           int64               `json:"version" bson:"version" validate:"gte=1"`
	ReleasedAt        *time.Time          `json:"releasedAt,omitempty" bson:"releasedAt,omitempty"`
	CreatedAt         time.Time           `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt         time.Time           `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type RecipeComponentType string

const (
	RecipeComponentItem      RecipeComponentType = "item"
	RecipeComponentSubrecipe RecipeComponentType = "subrecipe"
)

func ValidRecipeComponentType(componentType RecipeComponentType) bool {
	return componentType == RecipeComponentItem || componentType == RecipeComponentSubrecipe
}

// RecipeComponent stores quantities as fixed-point micros. Exactly one of
// ItemID and SubrecipeID is set, according to ComponentType.
type RecipeComponent struct {
	ID              primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID        primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	RecipeVersionID primitive.ObjectID  `json:"recipeVersionId" bson:"recipeVersionId" validate:"required"`
	ComponentType   RecipeComponentType `json:"componentType" bson:"componentType" validate:"required,recipe_component_type"`
	ItemID          *primitive.ObjectID `json:"itemId,omitempty" bson:"itemId,omitempty"`
	SubrecipeID     *primitive.ObjectID `json:"subrecipeId,omitempty" bson:"subrecipeId,omitempty"`
	QuantityMicros  int64               `json:"quantityMicros,string" bson:"quantityMicros" validate:"gt=0"`
	UnitID          primitive.ObjectID  `json:"unitId" bson:"unitId" validate:"required"`
	SortOrder       int32               `json:"sortOrder" bson:"sortOrder" validate:"gte=0,lte=1000000"`
	CreatedAt       time.Time           `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt       time.Time           `json:"updatedAt" bson:"updatedAt" validate:"required"`
}
