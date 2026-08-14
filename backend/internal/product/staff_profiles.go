package product

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"lastsaas/internal/apierror"
	"lastsaas/internal/db"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrStaffProfileNotFound        = errors.New("staff profile not found")
	ErrStaffProfileVersionConflict = errors.New("staff profile version conflict")
	ErrOwnershipStateChanged       = errors.New("ownership membership state changed")
)

type staffProfileContextKey struct{}

var allBusinessPermissions = []models.BusinessPermission{
	models.PermissionStorageAreasRead,
	models.PermissionStorageAreasManage,
	models.PermissionCatalogRead,
	models.PermissionCatalogManage,
	models.PermissionInventoryRead,
	models.PermissionInventoryPost,
	models.PermissionInventoryManage,
	models.PermissionInventoryLotOverride,
}

func DefaultBusinessRole(role models.MemberRole) models.BusinessRole {
	switch role {
	case models.RoleOwner:
		return models.BusinessRoleCompanyOwner
	case models.RoleAdmin:
		return models.BusinessRoleOperationsManager
	default:
		return models.BusinessRoleViewer
	}
}

func NewDefaultStaffProfile(tenantID, userID primitive.ObjectID, role models.MemberRole, now time.Time) models.StaffProfile {
	allLocations := role == models.RoleOwner || role == models.RoleAdmin
	return models.StaffProfile{
		ID: primitive.NewObjectID(), TenantID: tenantID, UserID: userID,
		BusinessRole: DefaultBusinessRole(role), AllLocations: allLocations,
		LocationIDs: make([]primitive.ObjectID, 0), PermissionOverrides: make([]models.PermissionOverride, 0),
		Status: models.StaffProfileActive, Version: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}
}

// InsertDefaultStaffProfile inserts the membership-derived profile. Callers
// creating a membership should treat an error as failure of the whole operation.
func InsertDefaultStaffProfile(ctx context.Context, database *db.MongoDB, tenantID, userID primitive.ObjectID, role models.MemberRole) error {
	var tenant models.Tenant
	if err := database.Tenants().FindOne(ctx, bson.M{"_id": tenantID}).Decode(&tenant); err != nil {
		return fmt.Errorf("load tenant for default staff profile: %w", err)
	}
	if tenant.IsRoot {
		return nil
	}
	profile := NewDefaultStaffProfile(tenantID, userID, role, time.Now())
	if err := ValidateStaffProfile(&profile); err != nil {
		return err
	}
	if _, err := database.StaffProfiles().InsertOne(ctx, profile); err != nil {
		return fmt.Errorf("insert default staff profile: %w", err)
	}
	return nil
}

// ReconcileStaffProfiles validates existing profiles and inserts only profiles
// missing for non-root tenant memberships. Existing profiles are never changed.
func ReconcileStaffProfiles(ctx context.Context, database *db.MongoDB) error {
	profiles, err := loadAndValidateExistingStaffProfiles(ctx, database)
	if err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		existing[profile.TenantID.Hex()+":"+profile.UserID.Hex()] = struct{}{}
	}

	cursor, err := database.TenantMemberships().Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("list memberships for staff profile reconciliation: %w", err)
	}
	defer cursor.Close(ctx)
	var memberships []models.TenantMembership
	if err := cursor.All(ctx, &memberships); err != nil {
		return fmt.Errorf("decode memberships for staff profile reconciliation: %w", err)
	}
	for _, membership := range memberships {
		count, err := database.Tenants().CountDocuments(ctx, bson.M{"_id": membership.TenantID, "isRoot": bson.M{"$ne": true}})
		if err != nil {
			return fmt.Errorf("check reconciliation tenant: %w", err)
		}
		if count == 0 {
			continue
		}
		if _, ok := existing[membership.TenantID.Hex()+":"+membership.UserID.Hex()]; ok {
			continue
		}
		profile := NewDefaultStaffProfile(membership.TenantID, membership.UserID, membership.Role, time.Now())
		_, err = database.StaffProfiles().UpdateOne(ctx,
			bson.M{"tenantId": membership.TenantID, "userId": membership.UserID},
			bson.M{"$setOnInsert": profile}, options.Update().SetUpsert(true))
		if err != nil {
			return fmt.Errorf("reconcile staff profile for tenant %s user %s: %w", membership.TenantID.Hex(), membership.UserID.Hex(), err)
		}
	}
	return nil
}

func loadAndValidateExistingStaffProfiles(ctx context.Context, database *db.MongoDB) ([]models.StaffProfile, error) {
	cursor, err := database.StaffProfiles().Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("list existing staff profiles for reconciliation: %w", err)
	}
	defer cursor.Close(ctx)
	allowedFields := map[string]struct{}{
		"_id": {}, "tenantId": {}, "userId": {}, "businessRole": {}, "allLocations": {}, "locationIds": {},
		"permissionOverrides": {}, "status": {}, "version": {}, "createdAt": {}, "updatedAt": {},
	}
	profiles := make([]models.StaffProfile, 0)
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode existing staff profile document: %w", err)
		}
		if len(raw) != len(allowedFields) {
			return nil, fmt.Errorf("invalid existing staff profile fields")
		}
		for field := range raw {
			if _, ok := allowedFields[field]; !ok {
				return nil, fmt.Errorf("invalid existing staff profile field %q", field)
			}
		}
		encoded, err := bson.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("encode existing staff profile: %w", err)
		}
		var profile models.StaffProfile
		if err := bson.Unmarshal(encoded, &profile); err != nil {
			return nil, fmt.Errorf("decode existing staff profile: %w", err)
		}
		profiles = append(profiles, profile)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing staff profiles for reconciliation: %w", err)
	}
	for i := range profiles {
		profile := &profiles[i]
		if err := ValidateStaffProfile(profile); err != nil {
			return nil, fmt.Errorf("invalid existing staff profile %s: %w", profile.ID.Hex(), err)
		}
		var tenant models.Tenant
		if err := database.Tenants().FindOne(ctx, bson.M{"_id": profile.TenantID, "isRoot": bson.M{"$ne": true}}).Decode(&tenant); err != nil {
			return nil, fmt.Errorf("invalid existing staff profile %s tenant: %w", profile.ID.Hex(), err)
		}
		if err := database.Users().FindOne(ctx, bson.M{"_id": profile.UserID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
			return nil, fmt.Errorf("invalid existing staff profile %s user: %w", profile.ID.Hex(), err)
		}
		if err := database.TenantMemberships().FindOne(ctx, bson.M{"tenantId": profile.TenantID, "userId": profile.UserID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
			return nil, fmt.Errorf("invalid existing staff profile %s membership: %w", profile.ID.Hex(), err)
		}
		if len(profile.LocationIDs) > 0 {
			count, err := database.Locations().CountDocuments(ctx, bson.M{"tenantId": profile.TenantID, "_id": bson.M{"$in": profile.LocationIDs}})
			if err != nil || count != int64(len(profile.LocationIDs)) {
				return nil, fmt.Errorf("invalid existing staff profile %s locations", profile.ID.Hex())
			}
		}
	}
	return profiles, nil
}

func ValidateStaffProfile(profile *models.StaffProfile) error {
	if err := validation.Validate(profile); err != nil {
		return err
	}
	if profile.AllLocations && len(profile.LocationIDs) != 0 {
		return fmt.Errorf("validation failed: locationIds must be empty when allLocations is true")
	}
	locations := make(map[primitive.ObjectID]struct{}, len(profile.LocationIDs))
	for _, id := range profile.LocationIDs {
		if id.IsZero() {
			return fmt.Errorf("validation failed: locationIds contains an invalid ID")
		}
		if _, exists := locations[id]; exists {
			return fmt.Errorf("validation failed: duplicate location ID")
		}
		locations[id] = struct{}{}
	}
	overrides := make(map[models.BusinessPermission]struct{}, len(profile.PermissionOverrides))
	for _, override := range profile.PermissionOverrides {
		if _, exists := overrides[override.Permission]; exists {
			return fmt.Errorf("validation failed: duplicate permission override")
		}
		overrides[override.Permission] = struct{}{}
	}
	return nil
}

func EffectivePermissions(profile *models.StaffProfile) []models.BusinessPermission {
	if profile.Status != models.StaffProfileActive {
		return []models.BusinessPermission{}
	}
	fullDefaults := profile.BusinessRole == models.BusinessRoleCompanyOwner || profile.BusinessRole == models.BusinessRoleOperationsManager
	allowed := map[models.BusinessPermission]bool{}
	for _, permission := range allBusinessPermissions {
		allowed[permission] = fullDefaults
	}
	if profile.BusinessRole == models.BusinessRoleHeadChef || profile.BusinessRole == models.BusinessRoleStockService {
		allowed[models.PermissionInventoryRead] = true
		allowed[models.PermissionInventoryPost] = true
	}
	if profile.BusinessRole == models.BusinessRolePurchasing || profile.BusinessRole == models.BusinessRoleController {
		allowed[models.PermissionInventoryRead] = true
	}
	for _, override := range profile.PermissionOverrides {
		allowed[override.Permission] = override.Allowed
	}
	result := make([]models.BusinessPermission, 0, len(allBusinessPermissions))
	for _, permission := range allBusinessPermissions {
		if allowed[permission] {
			result = append(result, permission)
		}
	}
	return result
}

func HasBusinessPermission(profile *models.StaffProfile, permission models.BusinessPermission) bool {
	if permission == models.PermissionInventoryRead || permission == models.PermissionInventoryPost || permission == models.PermissionInventoryManage || permission == models.PermissionInventoryLotOverride {
		return HasInventoryPermission(profile, permission)
	}
	for _, effective := range EffectivePermissions(profile) {
		if effective == permission {
			return true
		}
	}
	return false
}

// EffectiveInventoryPermissions returns the inventory subset of the effective
// profile permissions. Inventory permissions are location-scoped by
// RequireAuthorizedLocation.
func EffectiveInventoryPermissions(profile *models.StaffProfile) []models.BusinessPermission {
	result := make([]models.BusinessPermission, 0, 3)
	for _, permission := range allBusinessPermissions {
		if (permission == models.PermissionInventoryRead || permission == models.PermissionInventoryPost || permission == models.PermissionInventoryManage || permission == models.PermissionInventoryLotOverride) && HasBusinessPermissionFromEffective(profile, permission) {
			result = append(result, permission)
		}
	}
	return result
}

func HasBusinessPermissionFromEffective(profile *models.StaffProfile, permission models.BusinessPermission) bool {
	for _, effective := range EffectivePermissions(profile) {
		if effective == permission {
			return true
		}
	}
	return false
}

func HasInventoryPermission(profile *models.StaffProfile, permission models.BusinessPermission) bool {
	for _, effective := range EffectiveInventoryPermissions(profile) {
		if effective == permission {
			return true
		}
	}
	return false
}

func GetStaffProfileFromContext(ctx context.Context) (*models.StaffProfile, bool) {
	profile, ok := ctx.Value(staffProfileContextKey{}).(*models.StaffProfile)
	return profile, ok
}

func RequireStaffProfile(database *db.MongoDB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
			user, userOK := middleware.GetUserFromContext(r.Context())
			if !tenantOK || !userOK {
				apierror.Forbidden(w, r, "Active staff profile required")
				return
			}
			var profile models.StaffProfile
			err := database.StaffProfiles().FindOne(r.Context(), bson.M{"tenantId": tenant.ID, "userId": user.ID}).Decode(&profile)
			if errors.Is(err, mongo.ErrNoDocuments) {
				apierror.Write(w, http.StatusForbidden, apierror.Code("STAFF_PROFILE_REQUIRED"), "Active staff profile required", r)
				return
			}
			if err != nil {
				apierror.Internal(w, r, "Failed to load staff profile")
				return
			}
			if profile.Status != models.StaffProfileActive {
				apierror.Write(w, http.StatusForbidden, apierror.Code("STAFF_PROFILE_INACTIVE"), "Staff profile is inactive", r)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), staffProfileContextKey{}, &profile)))
		})
	}
}

func RequireAuthorizedLocation(database *db.MongoDB, pathVariable string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant, _ := middleware.GetTenantFromContext(r.Context())
			profile, ok := GetStaffProfileFromContext(r.Context())
			locationID, err := primitive.ObjectIDFromHex(mux.Vars(r)[pathVariable])
			if err != nil {
				apierror.BadRequest(w, r, "Invalid location ID")
				return
			}
			if tenant == nil || !ok {
				apierror.Forbidden(w, r, "Active staff profile required")
				return
			}
			if err := database.Locations().FindOne(r.Context(), bson.M{"_id": locationID, "tenantId": tenant.ID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
				if errors.Is(err, mongo.ErrNoDocuments) {
					apierror.NotFound(w, r, "Location not found")
				} else {
					apierror.Internal(w, r, "Failed to authorize location")
				}
				return
			}
			if !profile.AllLocations && !profileHasLocation(profile, locationID) {
				apierror.NotFound(w, r, "Location not found")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireBusinessPermission(permission models.BusinessPermission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile, ok := GetStaffProfileFromContext(r.Context())
			if !ok || !HasBusinessPermission(profile, permission) {
				apierror.Write(w, http.StatusForbidden, apierror.Code("BUSINESS_PERMISSION_DENIED"), "Business permission denied", r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requireCoreRole(role models.MemberRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			membership, ok := middleware.GetMembershipFromContext(r.Context())
			if !ok || !models.RoleHasPermission(membership.Role, role) {
				apierror.Forbidden(w, r, "Insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func profileHasLocation(profile *models.StaffProfile, locationID primitive.ObjectID) bool {
	for _, assigned := range profile.LocationIDs {
		if assigned == locationID {
			return true
		}
	}
	return false
}

type staffProfileResponse struct {
	models.StaffProfile
	EffectivePermissions []models.BusinessPermission `json:"effectivePermissions"`
}

func newStaffProfileResponse(profile models.StaffProfile) staffProfileResponse {
	return staffProfileResponse{StaffProfile: profile, EffectivePermissions: EffectivePermissions(&profile)}
}

func (h *productHandler) getStaffProfile(w http.ResponseWriter, r *http.Request) {
	tenant, tenantOK := middleware.GetTenantFromContext(r.Context())
	user, userOK := middleware.GetUserFromContext(r.Context())
	if !tenantOK || !userOK {
		apierror.NotFound(w, r, "Staff profile not found")
		return
	}
	var profile models.StaffProfile
	err := h.db.StaffProfiles().FindOne(r.Context(), bson.M{"tenantId": tenant.ID, "userId": user.ID}).Decode(&profile)
	if errors.Is(err, mongo.ErrNoDocuments) {
		apierror.NotFound(w, r, "Staff profile not found")
		return
	}
	if err != nil {
		apierror.Internal(w, r, "Failed to load staff profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"staffProfile": newStaffProfileResponse(profile)})
}

func (h *productHandler) listStaffProfiles(w http.ResponseWriter, r *http.Request) {
	tenant, _ := middleware.GetTenantFromContext(r.Context())
	cursor, err := h.db.StaffProfiles().Find(r.Context(), bson.M{"tenantId": tenant.ID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		apierror.Internal(w, r, "Failed to list staff profiles")
		return
	}
	defer cursor.Close(r.Context())
	profiles := make([]models.StaffProfile, 0)
	if err := cursor.All(r.Context(), &profiles); err != nil {
		apierror.Internal(w, r, "Failed to list staff profiles")
		return
	}
	responses := make([]staffProfileResponse, 0, len(profiles))
	for _, profile := range profiles {
		responses = append(responses, newStaffProfileResponse(profile))
	}
	writeJSON(w, http.StatusOK, map[string]any{"staffProfiles": responses})
}

type replaceStaffProfileRequest struct {
	Version             *int64                       `json:"version"`
	BusinessRole        *models.BusinessRole         `json:"businessRole"`
	AllLocations        *bool                        `json:"allLocations"`
	LocationIDs         *[]primitive.ObjectID        `json:"locationIds"`
	PermissionOverrides *[]models.PermissionOverride `json:"permissionOverrides"`
	Status              *models.StaffProfileStatus   `json:"status"`
}

func (h *productHandler) replaceStaffProfile(w http.ResponseWriter, r *http.Request) {
	tenant, _ := middleware.GetTenantFromContext(r.Context())
	actingMembership, _ := middleware.GetMembershipFromContext(r.Context())
	targetUserID, err := primitive.ObjectIDFromHex(mux.Vars(r)["userId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid user ID")
		return
	}
	var request replaceStaffProfileRequest
	if !decodeStrict(w, r, &request) {
		return
	}
	if request.Version == nil || request.BusinessRole == nil || request.AllLocations == nil || request.LocationIDs == nil || request.PermissionOverrides == nil || request.Status == nil {
		apierror.Validation(w, r, "Complete staff profile body is required")
		return
	}

	var targetMembership models.TenantMembership
	if err := h.db.TenantMemberships().FindOne(r.Context(), bson.M{"tenantId": tenant.ID, "userId": targetUserID}).Decode(&targetMembership); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Staff member not found")
		} else {
			apierror.Internal(w, r, "Failed to load staff member")
		}
		return
	}
	if targetMembership.Role == models.RoleOwner {
		apierror.Forbidden(w, r, "Owner profile cannot be edited")
		return
	}
	if actingMembership.Role == models.RoleAdmin && targetMembership.Role != models.RoleUser {
		apierror.Forbidden(w, r, "Admins can only manage core users")
		return
	}
	if err := h.db.Users().FindOne(r.Context(), bson.M{"_id": targetUserID}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Staff member not found")
		} else {
			apierror.Internal(w, r, "Failed to load staff member")
		}
		return
	}

	profile := models.StaffProfile{
		TenantID: tenant.ID, UserID: targetUserID, BusinessRole: *request.BusinessRole,
		AllLocations: *request.AllLocations, LocationIDs: *request.LocationIDs,
		PermissionOverrides: *request.PermissionOverrides, Status: *request.Status,
		Version: *request.Version, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := ValidateStaffProfile(&profile); err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	if len(profile.LocationIDs) > 0 {
		count, err := h.db.Locations().CountDocuments(r.Context(), bson.M{"tenantId": tenant.ID, "_id": bson.M{"$in": profile.LocationIDs}})
		if err != nil {
			apierror.Internal(w, r, "Failed to validate locations")
			return
		}
		if count != int64(len(profile.LocationIDs)) {
			apierror.Validation(w, r, "One or more locations do not belong to this tenant")
			return
		}
	}

	var updated models.StaffProfile
	err = h.db.StaffProfiles().FindOneAndUpdate(r.Context(),
		bson.M{"tenantId": tenant.ID, "userId": targetUserID, "version": *request.Version},
		bson.M{"$set": bson.M{
			"businessRole": profile.BusinessRole, "allLocations": profile.AllLocations,
			"locationIds": profile.LocationIDs, "permissionOverrides": profile.PermissionOverrides,
			"status": profile.Status, "updatedAt": time.Now().UTC(),
		}, "$inc": bson.M{"version": int64(1)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updated)
	if errors.Is(err, mongo.ErrNoDocuments) {
		exists, checkErr := h.db.StaffProfiles().CountDocuments(r.Context(), bson.M{"tenantId": tenant.ID, "userId": targetUserID})
		if checkErr != nil {
			apierror.Internal(w, r, "Failed to update staff profile")
		} else if exists == 0 {
			apierror.NotFound(w, r, "Staff profile not found")
		} else {
			apierror.VersionConflict(w, r, "Staff profile was modified by another request")
		}
		return
	}
	if err != nil {
		apierror.Internal(w, r, "Failed to update staff profile")
		return
	}
	if actingUser, ok := middleware.GetUserFromContext(r.Context()); ok && h.logger != nil {
		h.logger.LogTenantActivity(r.Context(), models.LogMedium, "Staff profile updated", actingUser.ID, tenant.ID, "staff_profile.updated", map[string]interface{}{
			"targetUserId": targetUserID.Hex(), "version": updated.Version,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"staffProfile": newStaffProfileResponse(updated)})
}

// TransferTenantOwnership demotes the expected owner before promoting the
// expected target membership. Call it inside a MongoDB transaction.
func TransferTenantOwnership(ctx context.Context, database *db.MongoDB, tenantID, oldOwnerID, newOwnerID primitive.ObjectID) error {
	if oldOwnerID == newOwnerID {
		return nil
	}
	var oldOwner, target models.TenantMembership
	if err := database.TenantMemberships().FindOne(ctx, bson.M{"tenantId": tenantID, "userId": oldOwnerID, "role": models.RoleOwner}).Decode(&oldOwner); err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("load expected owner: %w", err)
		}
		return fmt.Errorf("load expected owner: %w", ErrOwnershipStateChanged)
	}
	if err := database.TenantMemberships().FindOne(ctx, bson.M{"tenantId": tenantID, "userId": newOwnerID, "role": bson.M{"$ne": models.RoleOwner}}).Decode(&target); err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("load expected ownership target: %w", err)
		}
		return fmt.Errorf("load expected ownership target: %w", ErrOwnershipStateChanged)
	}
	now := time.Now().UTC()
	result, err := database.TenantMemberships().UpdateOne(ctx,
		bson.M{"_id": oldOwner.ID, "tenantId": tenantID, "userId": oldOwnerID, "role": models.RoleOwner},
		bson.M{"$set": bson.M{"role": models.RoleAdmin, "updatedAt": now}})
	if err != nil {
		return fmt.Errorf("demote expected owner: %w", err)
	}
	if result.MatchedCount != 1 {
		return fmt.Errorf("demote expected owner: %w", ErrOwnershipStateChanged)
	}
	result, err = database.TenantMemberships().UpdateOne(ctx,
		bson.M{"_id": target.ID, "tenantId": tenantID, "userId": newOwnerID, "role": target.Role},
		bson.M{"$set": bson.M{"role": models.RoleOwner, "updatedAt": now}})
	if err != nil {
		return fmt.Errorf("promote expected ownership target: %w", err)
	}
	if result.MatchedCount != 1 {
		return fmt.Errorf("promote expected ownership target: %w", ErrOwnershipStateChanged)
	}
	return SetOwnershipProfiles(ctx, database, tenantID, oldOwnerID, newOwnerID)
}

// SetOwnershipProfiles enforces the owner and former-owner profile defaults.
func SetOwnershipProfiles(ctx context.Context, database *db.MongoDB, tenantID, oldOwnerID, newOwnerID primitive.ObjectID) error {
	now := time.Now().UTC()
	newOwner := NewDefaultStaffProfile(tenantID, newOwnerID, models.RoleOwner, now)
	result, err := database.StaffProfiles().UpdateOne(ctx,
		bson.M{"tenantId": tenantID, "userId": newOwnerID},
		bson.M{"$set": bson.M{
			"businessRole": models.BusinessRoleCompanyOwner, "allLocations": true,
			"locationIds": bson.A{}, "permissionOverrides": bson.A{}, "status": models.StaffProfileActive,
			"updatedAt": now,
		}, "$setOnInsert": bson.M{
			"_id": newOwner.ID, "tenantId": tenantID, "userId": newOwnerID,
			"version": int64(1), "createdAt": now,
		}}, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("ensure new owner staff profile: %w", err)
	}
	if result.MatchedCount > 0 {
		_, err = database.StaffProfiles().UpdateOne(ctx, bson.M{"tenantId": tenantID, "userId": newOwnerID}, bson.M{"$inc": bson.M{"version": int64(1)}})
		if err != nil {
			return fmt.Errorf("version new owner staff profile: %w", err)
		}
	}
	if oldOwnerID == newOwnerID {
		return nil
	}
	formerOwner := NewDefaultStaffProfile(tenantID, oldOwnerID, models.RoleAdmin, now)
	result, err = database.StaffProfiles().UpdateOne(ctx,
		bson.M{"tenantId": tenantID, "userId": oldOwnerID},
		bson.M{"$set": bson.M{"businessRole": models.BusinessRoleOperationsManager, "allLocations": true, "locationIds": bson.A{}, "permissionOverrides": bson.A{}, "status": models.StaffProfileActive, "updatedAt": now}, "$setOnInsert": bson.M{
			"_id": formerOwner.ID, "tenantId": tenantID, "userId": oldOwnerID, "version": int64(1), "createdAt": now,
		}}, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("update former owner staff profile: %w", err)
	}
	if result.MatchedCount > 0 {
		if _, err := database.StaffProfiles().UpdateOne(ctx, bson.M{"tenantId": tenantID, "userId": oldOwnerID}, bson.M{"$inc": bson.M{"version": int64(1)}}); err != nil {
			return fmt.Errorf("version former owner staff profile: %w", err)
		}
	}
	return nil
}
