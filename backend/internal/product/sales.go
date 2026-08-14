package product

// This file owns the v4 sales lane.  Sales are deliberately kept in the
// product module: ingestion, recipe snapshots, and inventory writes must use
// the same tenant and authorization boundaries as the catalog.

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"lastsaas/internal/apierror"
	"lastsaas/internal/db"
	masterimports "lastsaas/internal/imports"
	"lastsaas/internal/inventory"
	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrSaleNotFound       = errors.New("sale not found")
	ErrSaleMappingMissing = errors.New("external product mapping is missing")
	ErrSaleMappingInvalid = errors.New("external product mapping is invalid")
	ErrRecipeCycle        = errors.New("recipe cycle detected")
	ErrRecipeUnavailable  = errors.New("released recipe version is unavailable")
)

const salesImportSourceDefault = "csv"

type salesImportRequest struct {
	Content        string            `json:"content"`
	Source         string            `json:"source"`
	Mapping        map[string]string `json:"mapping,omitempty"`
	IdempotencyKey string            `json:"idempotencyKey"`
}

type salesImportReport struct {
	Run            *models.SalesImportRun  `json:"run,omitempty"`
	Errors         []models.ImportRowError `json:"errors"`
	DryRun         bool                    `json:"dryRun"`
	TotalRows      int32                   `json:"totalRows"`
	CreatedSales   int32                   `json:"createdSales"`
	CreatedLines   int32                   `json:"createdLines"`
	UnresolvedRows int32                   `json:"unresolvedRows"`
}

type plannedSale struct {
	Sale       models.Sale
	Lines      []models.SaleLine
	Unresolved []models.UnresolvedSaleLine
	Consumed   []inventory.SaleConsumptionLine
}

type salesEngine struct{ db *db.MongoDB }

func requireSalesManager() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile, ok := GetStaffProfileFromContext(r.Context())
			if !ok || (profile.BusinessRole != models.BusinessRoleCompanyOwner && profile.BusinessRole != models.BusinessRoleOperationsManager) {
				apierror.Forbidden(w, r, "Operations manager permission required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *productHandler) getSalesImportTemplate(w http.ResponseWriter, r *http.Request) {
	content, err := masterimports.Template(models.ImportTargetSales)
	if err != nil {
		apierror.Internal(w, r, "Sales import template is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="sales.csv"`)
	_, _ = w.Write([]byte(content))
}

// plan parses all rows without writing. It is also used inside the apply
// transaction, so mapping and recipe resolution are from the transaction
// snapshot that is ultimately committed.
func (e *salesEngine) plan(ctx context.Context, tenantID primitive.ObjectID, req salesImportRequest) ([]plannedSale, []models.ImportRowError, error) {
	content, err := masterimports.ParseCSV(req.Content)
	if err != nil {
		return nil, []models.ImportRowError{{Row: 1, Field: "content", Code: "invalid_csv", Message: err.Error()}}, nil
	}
	fields := []string{"source", "externalSaleId", "externalLineId", "externalProductId", "quantity", "occurredAt", "locationId", "storageAreaId", "status"}
	if content, err = masterimports.ApplyMapping(content, req.Mapping, fields); err != nil {
		return nil, []models.ImportRowError{{Row: 1, Field: "mapping", Code: "invalid_mapping", Message: err.Error()}}, nil
	}
	plans := make([]plannedSale, 0)
	byIdentity := make(map[string]int)
	seenLines := make(map[string]struct{})
	errs := make([]models.ImportRowError, 0)
	for _, row := range content.Rows {
		v := row.Values
		source := strings.ToLower(strings.TrimSpace(v["source"]))
		if source == "" {
			source = strings.ToLower(strings.TrimSpace(req.Source))
		}
		if source == "" {
			source = salesImportSourceDefault
		}
		occurred, parseErr := time.Parse(time.RFC3339, v["occurredAt"])
		if parseErr != nil {
			errs = append(errs, salesRowError(row.Number, "occurredAt", "invalid_timestamp", "occurredAt must be RFC3339"))
			continue
		}
		quantity, parseErr := inventory.ParseQuantity(v["quantity"])
		if parseErr != nil || quantity <= 0 {
			errs = append(errs, salesRowError(row.Number, "quantity", "invalid_quantity", "quantity must be positive with at most six decimal places"))
			continue
		}
		locationID, idErr := primitive.ObjectIDFromHex(v["locationId"])
		if idErr != nil {
			errs = append(errs, salesRowError(row.Number, "locationId", "invalid_id", "locationId is invalid"))
			continue
		}
		areaID, idErr := primitive.ObjectIDFromHex(v["storageAreaId"])
		if idErr != nil {
			errs = append(errs, salesRowError(row.Number, "storageAreaId", "invalid_id", "storageAreaId is invalid"))
			continue
		}
		status := models.SaleStatus(strings.ToLower(strings.TrimSpace(v["status"])))
		if status == "" {
			status = models.SaleStatusCompleted
		}
		if !models.ValidSaleStatus(status) {
			errs = append(errs, salesRowError(row.Number, "status", "invalid_status", "status must be completed or cancelled"))
			continue
		}
		externalSale, externalLine, externalProduct := strings.TrimSpace(v["externalSaleId"]), strings.TrimSpace(v["externalLineId"]), strings.TrimSpace(v["externalProductId"])
		if externalSale == "" || externalLine == "" || externalProduct == "" {
			errs = append(errs, salesRowError(row.Number, "identity", "missing_identity", "sale, line, and product identities are required"))
			continue
		}
		lineIdentity := source + "\x00" + externalSale + "\x00" + externalLine
		if _, exists := seenLines[lineIdentity]; exists {
			errs = append(errs, salesRowError(row.Number, "externalLineId", "duplicate_row", "duplicate sale line identity"))
			continue
		}
		seenLines[lineIdentity] = struct{}{}
		saleIdentity := source + "\x00" + externalSale
		index, exists := byIdentity[saleIdentity]
		if !exists {
			now := time.Now().UTC()
			plans = append(plans, plannedSale{Sale: models.Sale{ID: primitive.NewObjectID(), TenantID: tenantID, Source: source, ExternalSaleID: externalSale, OccurredAt: occurred.UTC(), LocationID: locationID, StorageAreaID: areaID, Status: status, Version: 1, CreatedAt: now, UpdatedAt: now}})
			index = len(plans) - 1
			byIdentity[saleIdentity] = index
		} else if plans[index].Sale.Status != status || plans[index].Sale.LocationID != locationID || plans[index].Sale.StorageAreaID != areaID || !plans[index].Sale.OccurredAt.Equal(occurred.UTC()) {
			errs = append(errs, salesRowError(row.Number, "externalSaleId", "inconsistent_sale", "sale header fields must be consistent across lines"))
			continue
		}
		line := models.SaleLine{ID: primitive.NewObjectID(), TenantID: tenantID, SaleID: plans[index].Sale.ID, Source: source, ExternalSaleID: externalSale, ExternalLineID: externalLine, ExternalProductID: externalProduct, QuantityMicros: quantity, CreatedAt: plans[index].Sale.CreatedAt, UpdatedAt: plans[index].Sale.CreatedAt}
		mapping, mappingErr := e.resolveMapping(ctx, tenantID, source, externalProduct, occurred.UTC())
		if mappingErr == nil {
			version, versionErr := e.loadRecipeVersion(ctx, tenantID, mapping.RecipeVersionID, occurred.UTC())
			if versionErr == nil {
				line.MappingID, line.RecipeVersionID = &mapping.ID, &version.ID
				line.MappingVersion, line.RecipeVersionNumber = mapping.Version, version.Number
				items, expandErr := e.expandRecipe(ctx, tenantID, version.ID, quantity, occurred.UTC(), map[primitive.ObjectID]bool{})
				if expandErr == nil {
					for itemID, itemQuantity := range items {
						plans[index].Consumed = append(plans[index].Consumed, inventory.SaleConsumptionLine{ItemID: itemID, QuantityMicros: itemQuantity})
					}
				} else {
					mappingErr = expandErr
				}
			} else {
				mappingErr = versionErr
			}
		}
		if mappingErr != nil {
			line.MappingID, line.RecipeVersionID = nil, nil
			if !errors.Is(mappingErr, ErrSaleMappingMissing) {
				errs = append(errs, salesRowError(row.Number, "externalProductId", "invalid_mapping", salesReason(mappingErr)))
			} else {
				plans[index].Unresolved = append(plans[index].Unresolved, models.UnresolvedSaleLine{ID: primitive.NewObjectID(), TenantID: tenantID, Source: source, ExternalSaleID: externalSale, ExternalLineID: externalLine, ExternalProductID: externalProduct, OccurredAt: occurred.UTC(), LocationID: locationID, StorageAreaID: areaID, QuantityMicros: quantity, Reason: salesReason(mappingErr), CreatedAt: plans[index].Sale.CreatedAt})
			}
		}
		plans[index].Lines = append(plans[index].Lines, line)
	}
	return plans, boundedSalesErrors(errs), nil
}

func (e *salesEngine) resolveMapping(ctx context.Context, tenantID primitive.ObjectID, source, product string, at time.Time) (models.ExternalProductMapping, error) {
	filter := bson.M{"tenantId": tenantID, "adapter": source, "externalProductId": product, "isActive": true, "effectiveFrom": bson.M{"$lte": at}, "$or": bson.A{bson.M{"effectiveTo": bson.M{"$exists": false}}, bson.M{"effectiveTo": nil}, bson.M{"effectiveTo": bson.M{"$gt": at}}}}
	cur, err := e.db.ExternalProductMappings().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "effectiveFrom", Value: -1}}))
	if err != nil {
		return models.ExternalProductMapping{}, err
	}
	defer cur.Close(ctx)
	var mappings []models.ExternalProductMapping
	if err := cur.All(ctx, &mappings); err != nil {
		return models.ExternalProductMapping{}, err
	}
	if len(mappings) == 0 {
		return models.ExternalProductMapping{}, ErrSaleMappingMissing
	}
	if len(mappings) > 1 {
		return models.ExternalProductMapping{}, ErrSaleMappingInvalid
	}
	return mappings[0], nil
}

func (e *salesEngine) loadRecipeVersion(ctx context.Context, tenantID, id primitive.ObjectID, at time.Time) (models.RecipeVersion, error) {
	var version models.RecipeVersion
	filter := bson.M{"_id": id, "tenantId": tenantID, "status": models.RecipeVersionReleased, "effectiveFrom": bson.M{"$lte": at}, "$or": bson.A{bson.M{"effectiveTo": bson.M{"$exists": false}}, bson.M{"effectiveTo": nil}, bson.M{"effectiveTo": bson.M{"$gt": at}}}}
	if err := e.db.RecipeVersions().FindOne(ctx, filter).Decode(&version); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return version, ErrRecipeUnavailable
		}
		return version, err
	}
	return version, nil
}

// expandRecipe uses integer arithmetic throughout. A recipe graph is walked
// with a recursion stack, rather than a global visited set, because a shared
// subrecipe is valid while a back-edge is not.
func (e *salesEngine) expandRecipe(ctx context.Context, tenantID, versionID primitive.ObjectID, outputMicros int64, at time.Time, stack map[primitive.ObjectID]bool) (map[primitive.ObjectID]int64, error) {
	var version models.RecipeVersion
	if err := e.db.RecipeVersions().FindOne(ctx, bson.M{"_id": versionID, "tenantId": tenantID, "status": models.RecipeVersionReleased}).Decode(&version); err != nil {
		return nil, ErrRecipeUnavailable
	}
	if stack[version.RecipeID] {
		return nil, ErrRecipeCycle
	}
	stack[version.RecipeID] = true
	defer delete(stack, version.RecipeID)
	cur, err := e.db.RecipeComponents().Find(ctx, bson.M{"tenantId": tenantID, "recipeVersionId": versionID}, options.Find().SetSort(bson.D{{Key: "sortOrder", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var components []models.RecipeComponent
	if err := cur.All(ctx, &components); err != nil {
		return nil, err
	}
	result := make(map[primitive.ObjectID]int64)
	for _, component := range components {
		componentQuantity := component.QuantityMicros
		if component.ComponentType == models.RecipeComponentItem && component.ItemID != nil {
			converted, err := e.convertComponentToBase(ctx, tenantID, *component.ItemID, component.UnitID, component.QuantityMicros)
			if err != nil {
				return nil, err
			}
			componentQuantity = converted
		}
		quantity, err := fixedRecipeQuantity(outputMicros, componentQuantity, version.PortionCount, version.YieldFactorMicros, version.LossFactorMicros)
		if err != nil {
			return nil, err
		}
		if component.ComponentType == models.RecipeComponentItem && component.ItemID != nil {
			result[*component.ItemID], err = addInt64(result[*component.ItemID], quantity)
			if err != nil {
				return nil, err
			}
			continue
		}
		if component.SubrecipeID == nil {
			return nil, ErrRecipeUnavailable
		}
		subVersion, err := e.findRecipeVersionForRecipe(ctx, tenantID, *component.SubrecipeID, at)
		if err != nil {
			return nil, err
		}
		children, err := e.expandRecipe(ctx, tenantID, subVersion.ID, quantity, at, stack)
		if err != nil {
			return nil, err
		}
		for itemID, childQuantity := range children {
			result[itemID], err = addInt64(result[itemID], childQuantity)
			if err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (e *salesEngine) findRecipeVersionForRecipe(ctx context.Context, tenantID, recipeID primitive.ObjectID, at time.Time) (models.RecipeVersion, error) {
	filter := bson.M{"tenantId": tenantID, "recipeId": recipeID, "status": models.RecipeVersionReleased, "effectiveFrom": bson.M{"$lte": at}, "$or": bson.A{bson.M{"effectiveTo": bson.M{"$exists": false}}, bson.M{"effectiveTo": nil}, bson.M{"effectiveTo": bson.M{"$gt": at}}}}
	var version models.RecipeVersion
	if err := e.db.RecipeVersions().FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "effectiveFrom", Value: -1}})).Decode(&version); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return version, ErrRecipeUnavailable
		}
		return version, err
	}
	return version, nil
}

func (e *salesEngine) convertComponentToBase(ctx context.Context, tenantID, itemID, unitID primitive.ObjectID, quantity int64) (int64, error) {
	var item models.Item
	if err := e.db.Items().FindOne(ctx, bson.M{"_id": itemID, "tenantId": tenantID, "isActive": true, "stockable": true}).Decode(&item); err != nil {
		return 0, ErrSaleMappingInvalid
	}
	if item.BaseUnitID == unitID {
		return quantity, nil
	}
	var conversion models.ItemConversion
	if err := e.db.ItemConversions().FindOne(ctx, bson.M{"tenantId": tenantID, "itemId": itemID, "fromUnitId": unitID, "isActive": true}).Decode(&conversion); err != nil {
		return 0, ErrSaleMappingInvalid
	}
	return mulDiv(quantity, conversion.Numerator, conversion.Denominator)
}

func fixedRecipeQuantity(output, component, portions, yield, loss int64) (int64, error) {
	if output <= 0 || component <= 0 || portions <= 0 || yield <= 0 {
		return 0, ErrSaleMappingInvalid
	}
	// output and component are micros. First divide by the fixed-point scale
	// and recipe portions, then apply yield/loss as rational factors.
	value, err := mulDiv(output, component, inventory.QuantityScale*portions)
	if err != nil {
		return 0, err
	}
	return mulDiv(value, inventory.QuantityScale+loss, yield)
}

func mulDiv(a, b, divisor int64) (int64, error) {
	if a < 0 || b < 0 || divisor <= 0 {
		return 0, inventory.ErrQuantityOverflow
	}
	n := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	n.Quo(n, big.NewInt(divisor))
	if !n.IsInt64() || n.Sign() <= 0 {
		return 0, inventory.ErrQuantityOverflow
	}
	return n.Int64(), nil
}

func addInt64(a, b int64) (int64, error) {
	if b > 0 && a > int64(^uint64(0)>>1)-b {
		return 0, inventory.ErrQuantityOverflow
	}
	return a + b, nil
}

func salesRowError(row int, field, code, message string) models.ImportRowError {
	return models.ImportRowError{Row: row, Field: field, Code: code, Message: message}
}
func boundedSalesErrors(errs []models.ImportRowError) []models.ImportRowError {
	if len(errs) > 100 {
		return errs[:100]
	}
	return errs
}
func salesReason(err error) string {
	switch {
	case errors.Is(err, ErrSaleMappingMissing):
		return "unmapped product"
	case errors.Is(err, ErrRecipeCycle):
		return "recipe cycle detected"
	case errors.Is(err, ErrRecipeUnavailable):
		return "released recipe version unavailable"
	default:
		return "invalid mapping or recipe"
	}
}

func (h *productHandler) decodeSalesImport(w http.ResponseWriter, r *http.Request) (salesImportRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req salesImportRequest
	if !decodeStrict(w, r, &req) {
		return req, false
	}
	req.Source = strings.ToLower(strings.TrimSpace(req.Source))
	if req.Source == "" {
		req.Source = salesImportSourceDefault
	}
	if len(req.IdempotencyKey) < 8 || len(req.IdempotencyKey) > 128 {
		apierror.Validation(w, r, "idempotencyKey must be between 8 and 128 characters")
		return req, false
	}
	return req, true
}

func (h *productHandler) dryRunSalesImport(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	req, ok := h.decodeSalesImport(w, r)
	if !ok {
		return
	}
	engine := &salesEngine{db: h.db}
	plans, errs, err := engine.plan(r.Context(), tenant.ID, req)
	if err != nil {
		apierror.Internal(w, r, "Sales import dry run could not be completed")
		return
	}
	report := salesReportFromPlans(plans, errs, true)
	writeJSON(w, http.StatusOK, report)
}

func (h *productHandler) applySalesImport(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	req, ok := h.decodeSalesImport(w, r)
	if !ok {
		return
	}
	report, err := h.applySalesImportRequest(r.Context(), tenant.ID, user.ID, req)
	if err != nil {
		apierror.Internal(w, r, "Sales import could not be completed")
		return
	}
	if report.Run != nil && h.logger != nil {
		h.logger.LogTenantActivity(r.Context(), models.LogMedium, "Sales import completed", user.ID, tenant.ID, "sales.import.completed", map[string]interface{}{"source": req.Source, "rows": report.TotalRows, "unresolved": report.UnresolvedRows})
	}
	writeJSON(w, http.StatusOK, report)
}

func salesReportFromPlans(plans []plannedSale, errs []models.ImportRowError, dry bool) salesImportReport {
	report := salesImportReport{DryRun: dry, Errors: boundedSalesErrors(errs), CreatedSales: int32(len(plans))}
	for _, plan := range plans {
		report.CreatedLines += int32(len(plan.Lines))
		report.UnresolvedRows += int32(len(plan.Unresolved))
	}
	return report
}

func (h *productHandler) applySalesImportRequest(ctx context.Context, tenantID, userID primitive.ObjectID, req salesImportRequest) (*salesImportReport, error) {
	engine := &salesEngine{db: h.db}
	if existing, err := h.findSalesImportRun(ctx, tenantID, req.Source, req.IdempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		report := salesReportFromRun(existing)
		return &report, nil
	}
	// Parse once before opening a transaction to provide deterministic dry-run
	// style row errors. The transaction repeats planning on its own snapshot.
	plans, errs, err := engine.plan(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		report := salesReportFromPlans(plans, errs, false)
		return &report, nil
	}
	content, err := masterimports.ParseCSV(req.Content)
	if err != nil {
		return nil, err
	}
	session, err := h.db.Client.StartSession()
	if err != nil {
		return nil, inventory.ErrTransactionRequired
	}
	defer session.EndSession(ctx)
	var run models.SalesImportRun
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		plans, errs, planErr := engine.plan(sc, tenantID, req)
		if planErr != nil {
			return nil, planErr
		}
		if len(errs) > 0 {
			return nil, salesImportValidationError{errors: errs}
		}
		now := time.Now().UTC()
		run = models.SalesImportRun{ID: primitive.NewObjectID(), TenantID: tenantID, UserID: userID, Source: req.Source, IdempotencyKey: req.IdempotencyKey, Status: models.SalesImportPending, TotalRows: int32(len(content.Rows)), Errors: []models.ImportRowError{}, CreatedAt: now, UpdatedAt: now}
		for _, plan := range plans {
			run.CreatedLines += int32(len(plan.Lines))
			run.UnresolvedRows += int32(len(plan.Unresolved))
			run.CreatedSales++
		}
		if err := validation.Validate(&run); err != nil {
			return nil, err
		}
		if _, err := h.db.SalesImportRuns().InsertOne(sc, run); err != nil {
			return nil, err
		}
		service := inventory.NewService(h.db)
		for _, plan := range plans {
			var existing models.Sale
			existingErr := h.db.Sales().FindOne(sc, bson.M{"tenantId": tenantID, "source": plan.Sale.Source, "externalSaleId": plan.Sale.ExternalSaleID}).Decode(&existing)
			if existingErr == nil {
				if existing.Status == models.SaleStatusCompleted && plan.Sale.Status == models.SaleStatusCancelled {
					if existing.ConsumptionPostingID != nil && existing.ReversalPostingID == nil {
						reversal, reverseErr := service.ReverseInTransaction(sc, inventory.ReverseCommand{TenantID: tenantID, UserID: userID, LocationID: existing.LocationID, PostingID: *existing.ConsumptionPostingID, IdempotencyKey: "sale-reversal-" + existing.ID.Hex()})
						if reverseErr != nil {
							return nil, reverseErr
						}
						existing.ReversalPostingID = &reversal.Posting.ID
					}
					existing.Status, existing.CancelledAt, existing.UpdatedAt = models.SaleStatusCancelled, ptrTime(time.Now().UTC()), time.Now().UTC()
					set := bson.M{"status": existing.Status, "cancelledAt": existing.CancelledAt, "updatedAt": existing.UpdatedAt}
					if existing.ReversalPostingID != nil {
						set["reversalPostingId"] = existing.ReversalPostingID
					}
					if _, err := h.db.Sales().UpdateOne(sc, bson.M{"_id": existing.ID, "tenantId": tenantID, "status": models.SaleStatusCompleted}, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}}); err != nil {
						return nil, err
					}
				}
				continue
			}
			if !errors.Is(existingErr, mongo.ErrNoDocuments) {
				return nil, existingErr
			}
			if err := validation.Validate(&plan.Sale); err != nil {
				return nil, err
			}
			if _, err := h.db.Sales().InsertOne(sc, plan.Sale); err != nil {
				return nil, err
			}
			for _, line := range plan.Lines {
				if err := validation.Validate(&line); err != nil {
					return nil, err
				}
				if _, err := h.db.SalesLines().InsertOne(sc, line); err != nil {
					return nil, err
				}
			}
			for _, unresolved := range plan.Unresolved {
				unresolved.ImportRunID = run.ID
				if err := validation.Validate(&unresolved); err != nil {
					return nil, err
				}
				if _, err := h.db.UnresolvedSaleLines().InsertOne(sc, unresolved); err != nil {
					return nil, err
				}
			}
			if plan.Sale.Status == models.SaleStatusCompleted && len(plan.Consumed) > 0 {
				consumed, aggregateErr := aggregateConsumption(plan.Consumed)
				if aggregateErr != nil {
					return nil, aggregateErr
				}
				result, err := service.PostSaleConsumptionInTransaction(sc, inventory.SaleConsumptionCommand{TenantID: tenantID, UserID: userID, LocationID: plan.Sale.LocationID, StorageAreaID: plan.Sale.StorageAreaID, Lines: consumed, IdempotencyKey: "sale-consumption-" + plan.Sale.ID.Hex(), EffectiveAt: plan.Sale.OccurredAt, Reason: "Theoretical consumption for sale " + plan.Sale.ExternalSaleID})
				if err != nil {
					return nil, err
				}
				if _, err := h.db.Sales().UpdateOne(sc, bson.M{"_id": plan.Sale.ID, "tenantId": tenantID}, bson.M{"$set": bson.M{"consumptionPostingId": result.Posting.ID, "updatedAt": time.Now().UTC()}, "$inc": bson.M{"version": int64(1)}}); err != nil {
					return nil, err
				}
			}
		}
		run.Status, run.UpdatedAt = models.SalesImportCompleted, time.Now().UTC()
		_, err := h.db.SalesImportRuns().ReplaceOne(sc, bson.M{"_id": run.ID, "tenantId": tenantID}, run)
		return nil, err
	})
	if err != nil {
		var validationErr salesImportValidationError
		if errors.As(err, &validationErr) {
			report := salesReportFromPlans(plans, validationErr.errors, false)
			return &report, nil
		}
		if mongo.IsDuplicateKeyError(err) {
			if winner, findErr := h.findSalesImportRun(ctx, tenantID, req.Source, req.IdempotencyKey); findErr == nil && winner != nil {
				report := salesReportFromRun(winner)
				return &report, nil
			}
		}
		return nil, err
	}
	report := salesReportFromRun(&run)
	return &report, nil
}

type salesImportValidationError struct{ errors []models.ImportRowError }

func (e salesImportValidationError) Error() string { return "sales import validation failed" }

func aggregateConsumption(lines []inventory.SaleConsumptionLine) ([]inventory.SaleConsumptionLine, error) {
	byItem := make(map[primitive.ObjectID]int64)
	for _, line := range lines {
		quantity, err := addInt64(byItem[line.ItemID], line.QuantityMicros)
		if err != nil {
			return nil, err
		}
		byItem[line.ItemID] = quantity
	}
	result := make([]inventory.SaleConsumptionLine, 0, len(byItem))
	for itemID, quantity := range byItem {
		result = append(result, inventory.SaleConsumptionLine{ItemID: itemID, QuantityMicros: quantity})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ItemID.Hex() < result[j].ItemID.Hex() })
	return result, nil
}
func ptrTime(value time.Time) *time.Time { return &value }

func (h *productHandler) findSalesImportRun(ctx context.Context, tenantID primitive.ObjectID, source, key string) (*models.SalesImportRun, error) {
	var run models.SalesImportRun
	err := h.db.SalesImportRuns().FindOne(ctx, bson.M{"tenantId": tenantID, "source": source, "idempotencyKey": key, "status": models.SalesImportCompleted}).Decode(&run)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}
func salesReportFromRun(run *models.SalesImportRun) salesImportReport {
	return salesImportReport{Run: run, TotalRows: run.TotalRows, CreatedSales: run.CreatedSales, CreatedLines: run.CreatedLines, UnresolvedRows: run.UnresolvedRows, Errors: run.Errors}
}

func (h *productHandler) listUnmappedSales(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	filter := bson.M{"tenantId": tenant.ID}
	if source := strings.TrimSpace(r.URL.Query().Get("source")); source != "" {
		filter["source"] = strings.ToLower(source)
	}
	limit := int64(100)
	cur, err := h.db.UnresolvedSaleLines().Find(r.Context(), filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		apierror.Internal(w, r, "Failed to list unmapped sales")
		return
	}
	defer cur.Close(r.Context())
	lines := []models.UnresolvedSaleLine{}
	if err := cur.All(r.Context(), &lines); err != nil {
		apierror.Internal(w, r, "Failed to list unmapped sales")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

func (h *productHandler) listSalesImportRuns(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	filter := bson.M{"tenantId": tenant.ID}
	if source := strings.TrimSpace(r.URL.Query().Get("source")); source != "" {
		filter["source"] = strings.ToLower(source)
	}
	cur, err := h.db.SalesImportRuns().Find(r.Context(), filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(100))
	if err != nil {
		apierror.Internal(w, r, "Failed to list sales imports")
		return
	}
	defer cur.Close(r.Context())
	runs := []models.SalesImportRun{}
	if err := cur.All(r.Context(), &runs); err != nil {
		apierror.Internal(w, r, "Failed to list sales imports")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (h *productHandler) getSalesImportRun(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["runId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid sales import run ID")
		return
	}
	var run models.SalesImportRun
	if err := h.db.SalesImportRuns().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&run); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Sales import run not found")
		} else {
			apierror.Internal(w, r, "Failed to get sales import run")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}

func (h *productHandler) cancelSale(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	saleID, err := primitive.ObjectIDFromHex(mux.Vars(r)["saleId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid sale ID")
		return
	}
	var sale models.Sale
	if err := h.db.Sales().FindOne(r.Context(), bson.M{"_id": saleID, "tenantId": tenant.ID}).Decode(&sale); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Sale not found")
		} else {
			apierror.Internal(w, r, "Failed to load sale")
		}
		return
	}
	if sale.Status == models.SaleStatusCancelled {
		writeJSON(w, http.StatusOK, map[string]any{"sale": sale})
		return
	}
	session, err := h.db.Client.StartSession()
	if err != nil {
		apierror.Internal(w, r, "Sale cancellation requires a transaction")
		return
	}
	defer session.EndSession(r.Context())
	_, err = session.WithTransaction(r.Context(), func(sc mongo.SessionContext) (interface{}, error) {
		var reversalID *primitive.ObjectID
		if sale.ConsumptionPostingID != nil {
			result, reverseErr := inventory.NewService(h.db).ReverseInTransaction(sc, inventory.ReverseCommand{TenantID: tenant.ID, UserID: user.ID, LocationID: sale.LocationID, PostingID: *sale.ConsumptionPostingID, IdempotencyKey: "sale-reversal-" + sale.ID.Hex()})
			if reverseErr != nil {
				return nil, reverseErr
			}
			reversalID = &result.Posting.ID
		}
		now := time.Now().UTC()
		set := bson.M{"status": models.SaleStatusCancelled, "cancelledAt": now, "updatedAt": now}
		if reversalID != nil {
			set["reversalPostingId"] = reversalID
		}
		_, updateErr := h.db.Sales().UpdateOne(sc, bson.M{"_id": sale.ID, "tenantId": tenant.ID, "status": models.SaleStatusCompleted}, bson.M{"$set": set, "$inc": bson.M{"version": int64(1)}})
		if updateErr != nil {
			return nil, updateErr
		}
		sale.Status, sale.CancelledAt, sale.ReversalPostingID = models.SaleStatusCancelled, &now, reversalID
		return nil, nil
	})
	if err != nil {
		if errors.Is(err, inventory.ErrPostingAlreadyReversed) {
			apierror.Conflict(w, r, "Sale is already cancelled")
		} else {
			apierror.Internal(w, r, "Failed to cancel sale")
		}
		return
	}
	if h.logger != nil {
		h.logger.LogTenantActivity(r.Context(), models.LogMedium, "Sale cancelled", user.ID, tenant.ID, "sale.cancelled", map[string]interface{}{"saleId": sale.ID.Hex()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sale": sale})
}
