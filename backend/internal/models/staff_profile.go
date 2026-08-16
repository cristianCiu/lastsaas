package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BusinessRole string

const (
	BusinessRoleCompanyOwner      BusinessRole = "company_owner"
	BusinessRoleOperationsManager BusinessRole = "operations_manager"
	BusinessRoleHeadChef          BusinessRole = "head_chef"
	BusinessRolePurchasing        BusinessRole = "purchasing"
	BusinessRoleStockService      BusinessRole = "stock_service"
	BusinessRoleController        BusinessRole = "controller"
	BusinessRoleViewer            BusinessRole = "viewer"
)

type BusinessPermission string

const (
	PermissionStorageAreasRead     BusinessPermission = "storage_areas.read"
	PermissionStorageAreasManage   BusinessPermission = "storage_areas.manage"
	PermissionCatalogRead          BusinessPermission = "catalog.read"
	PermissionCatalogManage        BusinessPermission = "catalog.manage"
	PermissionInventoryRead        BusinessPermission = "inventory.read"
	PermissionInventoryPost        BusinessPermission = "inventory.post"
	PermissionInventoryManage      BusinessPermission = "inventory.manage"
	PermissionInventoryLotOverride BusinessPermission = "inventory.lot_override"
	PermissionPurchasingRead       BusinessPermission = "purchasing.read"
	PermissionPurchasingManage     BusinessPermission = "purchasing.manage"
	PermissionPurchasingApprove    BusinessPermission = "purchasing.approve"
	PermissionPurchasingReceive    BusinessPermission = "purchasing.receive"
	PermissionSalesRead            BusinessPermission = "sales.read"
	PermissionSalesManage          BusinessPermission = "sales.manage"
	PermissionForecastRead         BusinessPermission = "forecast.read"
	PermissionForecastManage       BusinessPermission = "forecast.manage"
	PermissionForecastRun          BusinessPermission = "forecast.run"
)

type StaffProfileStatus string

const (
	StaffProfileActive   StaffProfileStatus = "active"
	StaffProfileInactive StaffProfileStatus = "inactive"
)

type PermissionOverride struct {
	Permission BusinessPermission `json:"permission" bson:"permission" validate:"required,business_permission"`
	Allowed    bool               `json:"allowed" bson:"allowed"`
}

type StaffProfile struct {
	ID                  primitive.ObjectID   `json:"id" bson:"_id,omitempty"`
	TenantID            primitive.ObjectID   `json:"-" bson:"tenantId" validate:"required"`
	UserID              primitive.ObjectID   `json:"userId" bson:"userId" validate:"required"`
	BusinessRole        BusinessRole         `json:"businessRole" bson:"businessRole" validate:"required,business_role"`
	AllLocations        bool                 `json:"allLocations" bson:"allLocations"`
	LocationIDs         []primitive.ObjectID `json:"locationIds" bson:"locationIds" validate:"required,dive,required"`
	PermissionOverrides []PermissionOverride `json:"permissionOverrides" bson:"permissionOverrides" validate:"required,dive"`
	Status              StaffProfileStatus   `json:"status" bson:"status" validate:"required,staff_profile_status"`
	Version             int64                `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt           time.Time            `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt           time.Time            `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

func ValidBusinessRole(role BusinessRole) bool {
	switch role {
	case BusinessRoleCompanyOwner, BusinessRoleOperationsManager, BusinessRoleHeadChef,
		BusinessRolePurchasing, BusinessRoleStockService, BusinessRoleController, BusinessRoleViewer:
		return true
	}
	return false
}

func ValidBusinessPermission(permission BusinessPermission) bool {
	return permission == PermissionStorageAreasRead || permission == PermissionStorageAreasManage ||
		permission == PermissionCatalogRead || permission == PermissionCatalogManage || permission == PermissionSalesRead || permission == PermissionSalesManage ||
		permission == PermissionPurchasingRead || permission == PermissionPurchasingManage || permission == PermissionPurchasingApprove || permission == PermissionPurchasingReceive ||
		permission == PermissionInventoryRead || permission == PermissionInventoryPost || permission == PermissionInventoryManage || permission == PermissionInventoryLotOverride ||
		permission == PermissionForecastRead || permission == PermissionForecastManage || permission == PermissionForecastRun
}

func ValidStaffProfileStatus(status StaffProfileStatus) bool {
	return status == StaffProfileActive || status == StaffProfileInactive
}
