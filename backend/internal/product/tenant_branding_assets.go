package product

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strconv"
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

const (
	maxTenantLogoSize      = int64(900 * 1024)
	maxTenantLogoRequest   = maxTenantLogoSize + 64*1024
	maxTenantLogoDimension = 2048
)

var (
	ErrTenantBrandingAssetNotFound        = errors.New("tenant branding asset not found")
	ErrTenantBrandingAssetVersionConflict = errors.New("tenant branding asset version conflict")
)

type TenantBrandingAssetRepository struct {
	db       *db.MongoDB
	tenantID primitive.ObjectID
}

func NewTenantBrandingAssetRepository(database *db.MongoDB, tenantID primitive.ObjectID) *TenantBrandingAssetRepository {
	return &TenantBrandingAssetRepository{db: database, tenantID: tenantID}
}

func (r *TenantBrandingAssetRepository) List(ctx context.Context) ([]models.TenantBrandingAsset, error) {
	cursor, err := r.db.TenantBrandingAssets().Find(ctx, bson.M{"tenantId": r.tenantID}, options.Find().SetProjection(bson.M{"data": 0}).SetSort(bson.D{{Key: "kind", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list tenant branding assets: %w", err)
	}
	defer cursor.Close(ctx)
	var assets []models.TenantBrandingAsset
	if err := cursor.All(ctx, &assets); err != nil {
		return nil, fmt.Errorf("decode tenant branding assets: %w", err)
	}
	if assets == nil {
		assets = []models.TenantBrandingAsset{}
	}
	return assets, nil
}

func (r *TenantBrandingAssetRepository) Get(ctx context.Context, kind models.TenantBrandingAssetKind) (*models.TenantBrandingAsset, error) {
	var asset models.TenantBrandingAsset
	err := r.db.TenantBrandingAssets().FindOne(ctx, bson.M{"tenantId": r.tenantID, "kind": kind}).Decode(&asset)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrTenantBrandingAssetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant branding asset: %w", err)
	}
	return &asset, nil
}

func (r *TenantBrandingAssetRepository) Put(ctx context.Context, kind models.TenantBrandingAssetKind, data []byte, contentType string, width, height int, expectedVersion int64) (*models.TenantBrandingAsset, error) {
	storageKey, err := randomStorageKey()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if expectedVersion == 0 {
		asset := models.TenantBrandingAsset{
			ID: primitive.NewObjectID(), TenantID: r.tenantID, Kind: kind, StorageKey: storageKey,
			ContentType: contentType, Data: data, Size: int64(len(data)), Width: width, Height: height,
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := validation.Validate(&asset); err != nil {
			return nil, err
		}
		if _, err := r.db.TenantBrandingAssets().InsertOne(ctx, asset); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return nil, ErrTenantBrandingAssetVersionConflict
			}
			return nil, fmt.Errorf("create tenant branding asset: %w", err)
		}
		asset.Data = nil
		return &asset, nil
	}
	candidate := models.TenantBrandingAsset{
		ID: primitive.NewObjectID(), TenantID: r.tenantID, Kind: kind, StorageKey: storageKey,
		ContentType: contentType, Data: data, Size: int64(len(data)), Width: width, Height: height,
		Version: expectedVersion, CreatedAt: now, UpdatedAt: now,
	}
	if err := validation.Validate(&candidate); err != nil {
		return nil, err
	}
	var asset models.TenantBrandingAsset
	err = r.db.TenantBrandingAssets().FindOneAndUpdate(ctx,
		bson.M{"tenantId": r.tenantID, "kind": kind, "version": expectedVersion},
		bson.M{"$set": bson.M{"storageKey": storageKey, "contentType": contentType, "data": data, "size": int64(len(data)), "width": width, "height": height, "updatedAt": now}, "$inc": bson.M{"version": int64(1)}},
		options.FindOneAndUpdate().SetProjection(bson.M{"data": 0}).SetReturnDocument(options.After),
	).Decode(&asset)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrTenantBrandingAssetVersionConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update tenant branding asset: %w", err)
	}
	return &asset, nil
}

func (r *TenantBrandingAssetRepository) Delete(ctx context.Context, kind models.TenantBrandingAssetKind, expectedVersion int64) error {
	result, err := r.db.TenantBrandingAssets().DeleteOne(ctx, bson.M{"tenantId": r.tenantID, "kind": kind, "version": expectedVersion})
	if err != nil {
		return fmt.Errorf("delete tenant branding asset: %w", err)
	}
	if result.DeletedCount == 1 {
		return nil
	}
	count, err := r.db.TenantBrandingAssets().CountDocuments(ctx, bson.M{"tenantId": r.tenantID, "kind": kind})
	if err != nil {
		return fmt.Errorf("check tenant branding asset: %w", err)
	}
	if count == 0 {
		return ErrTenantBrandingAssetNotFound
	}
	return ErrTenantBrandingAssetVersionConflict
}

func randomStorageKey() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate tenant branding asset key: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func validateTenantLogo(data []byte, declaredType string) (string, int, int, error) {
	if len(data) == 0 || int64(len(data)) > maxTenantLogoSize {
		return "", 0, 0, errors.New("logo must be between 1 byte and 900 KiB")
	}
	detected := http.DetectContentType(data)
	if detected != "image/png" && detected != "image/jpeg" {
		return "", 0, 0, errors.New("logo must be a PNG or JPEG image")
	}
	if declaredType != "" && declaredType != "application/octet-stream" && declaredType != detected {
		return "", 0, 0, errors.New("declared file type does not match image signature")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, errors.New("logo image is corrupt or unsupported")
	}
	wantFormat := "png"
	if detected == "image/jpeg" {
		wantFormat = "jpeg"
	}
	if format != wantFormat {
		return "", 0, 0, errors.New("decoded image format does not match image signature")
	}
	if config.Width < 16 || config.Height < 16 || config.Width > maxTenantLogoDimension || config.Height > maxTenantLogoDimension {
		return "", 0, 0, errors.New("logo dimensions must be between 16 and 2048 pixels")
	}
	return detected, config.Width, config.Height, nil
}

func tenantBrandingAssetKind(r *http.Request) (models.TenantBrandingAssetKind, bool) {
	kind := models.TenantBrandingAssetKind(mux.Vars(r)["kind"])
	return kind, models.ValidTenantBrandingAssetKind(kind)
}

func tenantForBrandingAsset(w http.ResponseWriter, r *http.Request) (*models.Tenant, bool) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return nil, false
	}
	if tenant.IsRoot {
		apierror.Forbidden(w, r, "Restaurant branding is not available for the platform tenant")
		return nil, false
	}
	return tenant, true
}

func (h *productHandler) listTenantBrandingAssets(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantForBrandingAsset(w, r)
	if !ok {
		return
	}
	assets, err := NewTenantBrandingAssetRepository(h.db, tenant.ID).List(r.Context())
	if err != nil {
		apierror.Internal(w, r, "Failed to list tenant branding assets")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": assets})
}

func (h *productHandler) serveTenantBrandingAsset(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantForBrandingAsset(w, r)
	if !ok {
		return
	}
	kind, valid := tenantBrandingAssetKind(r)
	if !valid {
		apierror.BadRequest(w, r, "Invalid branding asset kind")
		return
	}
	asset, err := NewTenantBrandingAssetRepository(h.db, tenant.ID).Get(r.Context(), kind)
	if errors.Is(err, ErrTenantBrandingAssetNotFound) {
		apierror.NotFound(w, r, "Branding asset not found")
		return
	}
	if err != nil {
		apierror.Internal(w, r, "Failed to get tenant branding asset")
		return
	}
	etag := `"` + asset.StorageKey + `"`
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(asset.Size, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(asset.Data)
}

func (h *productHandler) putTenantBrandingAsset(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantForBrandingAsset(w, r)
	if !ok {
		return
	}
	kind, valid := tenantBrandingAssetKind(r)
	if !valid {
		apierror.BadRequest(w, r, "Invalid branding asset kind")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxTenantLogoRequest)
	if err := r.ParseMultipartForm(maxTenantLogoRequest); err != nil {
		apierror.BadRequest(w, r, "Invalid upload or file too large")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		apierror.BadRequest(w, r, "Logo file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTenantLogoSize+1))
	if err != nil {
		apierror.BadRequest(w, r, "Failed to read logo file")
		return
	}
	version, err := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if err != nil || version < 0 {
		apierror.BadRequest(w, r, "Valid asset version is required")
		return
	}
	contentType, width, height, err := validateTenantLogo(data, header.Header.Get("Content-Type"))
	if err != nil {
		apierror.Validation(w, r, err.Error())
		return
	}
	asset, err := NewTenantBrandingAssetRepository(h.db, tenant.ID).Put(r.Context(), kind, data, contentType, width, height, version)
	switch {
	case err == nil:
		if user, found := middleware.GetUserFromContext(r.Context()); found && h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, "Tenant branding asset uploaded", user.ID, tenant.ID, "tenant_branding_asset.uploaded", map[string]interface{}{"kind": kind, "version": asset.Version, "size": asset.Size})
		}
		writeJSON(w, http.StatusOK, map[string]any{"asset": asset})
	case errors.Is(err, ErrTenantBrandingAssetVersionConflict):
		apierror.VersionConflict(w, r, "Branding asset was modified by another request")
	default:
		apierror.Internal(w, r, "Failed to save tenant branding asset")
	}
}

func (h *productHandler) deleteTenantBrandingAsset(w http.ResponseWriter, r *http.Request) {
	tenant, ok := tenantForBrandingAsset(w, r)
	if !ok {
		return
	}
	kind, valid := tenantBrandingAssetKind(r)
	if !valid {
		apierror.BadRequest(w, r, "Invalid branding asset kind")
		return
	}
	version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || version < 1 {
		apierror.BadRequest(w, r, "Valid asset version is required")
		return
	}
	err = NewTenantBrandingAssetRepository(h.db, tenant.ID).Delete(r.Context(), kind, version)
	switch {
	case err == nil:
		if user, found := middleware.GetUserFromContext(r.Context()); found && h.logger != nil {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, "Tenant branding asset deleted", user.ID, tenant.ID, "tenant_branding_asset.deleted", map[string]interface{}{"kind": kind, "version": version})
		}
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrTenantBrandingAssetNotFound):
		apierror.NotFound(w, r, "Branding asset not found")
	case errors.Is(err, ErrTenantBrandingAssetVersionConflict):
		apierror.VersionConflict(w, r, "Branding asset was modified by another request")
	default:
		apierror.Internal(w, r, "Failed to delete tenant branding asset")
	}
}
