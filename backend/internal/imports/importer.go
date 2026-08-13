package imports

import (
	"context"
	"encoding/csv"
	"errors"
	"strconv"
	"strings"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/models"
	"lastsaas/internal/validation"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Request struct {
	Target         models.ImportTarget `json:"target"`
	Content        string              `json:"content"`
	Mapping        map[string]string   `json:"mapping,omitempty"`
	IdempotencyKey string              `json:"idempotencyKey"`
}

type Report struct {
	Run         *models.ImportRun       `json:"run,omitempty"`
	Errors      []models.ImportRowError `json:"errors"`
	DryRun      bool                    `json:"dryRun"`
	TotalRows   int32                   `json:"totalRows"`
	CreatedRows int32                   `json:"createdRows"`
	UpdatedRows int32                   `json:"updatedRows"`
}

type Engine struct {
	DB *db.MongoDB

	// testAfterWriteHook is intentionally unexported and nil in production. It
	// exists only for package-level transaction failure tests.
	testAfterWriteHook func(writeNumber int) error
}

var errNoTransaction = errors.New("MongoDB transactions are required for imports")

type transactionValidationError struct{ errors []models.ImportRowError }

func (e *transactionValidationError) Error() string { return "import validation failed" }

type internalDBError struct{ err error }

func (e *internalDBError) Error() string { return e.err.Error() }
func (e *internalDBError) Unwrap() error { return e.err }

var templateFields = map[models.ImportTarget][]string{
	models.ImportTargetUnits:         {"code", "name", "symbol", "dimension", "precision", "isActive"},
	models.ImportTargetCategories:    {"code", "name", "isActive"},
	models.ImportTargetItems:         {"sku", "name", "description", "brand", "categoryCode", "baseUnitCode", "allergens", "shelfLifeDays", "stockable", "isActive"},
	models.ImportTargetSuppliers:     {"code", "name", "contactName", "email", "phone", "orderingDays", "defaultLeadTimeDays", "isActive"},
	models.ImportTargetSupplierItems: {"supplierCode", "itemSKU", "supplierSKU", "packSizeMicros", "moq", "unitPriceMinor", "currency", "leadTimeDays", "isActive"},
}

func Template(target models.ImportTarget) (string, error) {
	fields, ok := templateFields[target]
	if !ok {
		return "", errors.New("invalid import target")
	}
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(fields); err != nil {
		return "", err
	}
	w.Flush()
	return b.String(), w.Error()
}

func (e *Engine) DryRun(ctx context.Context, tenantID, userID primitive.ObjectID, req Request) (Report, error) {
	if meta := validateRequestMetaDry(req); len(meta) > 0 {
		return Report{DryRun: true, Errors: meta}, nil
	}
	doc, errs := parseRequest(req, false)
	if len(errs) > 0 {
		return Report{DryRun: true, Errors: errs}, nil
	}
	if mapped, err := ApplyMapping(doc, req.Mapping, templateFields[req.Target]); err != nil {
		return Report{DryRun: true, Errors: []models.ImportRowError{{Row: 1, Field: "mapping", Code: "invalid_mapping", Message: safeMessage(err.Error())}}}, nil
	} else {
		doc = mapped
	}
	plans, errs, internalErr := e.plan(ctx, tenantID, req.Target, doc)
	if internalErr != nil {
		return Report{}, internalErr
	}
	return Report{DryRun: true, TotalRows: int32(len(doc.Rows)), CreatedRows: countCreated(plans), UpdatedRows: countUpdated(plans), Errors: errs}, nil
}

func (e *Engine) Apply(ctx context.Context, tenantID, userID primitive.ObjectID, req Request) (*Report, error) {
	if errs := validateRequestMeta(req); len(errs) > 0 {
		return &Report{Errors: errs}, nil
	}
	// Check the reservation before parsing. Replays therefore return the same
	// completed run even if the retried body is malformed or has changed.
	if existing, err := e.findCompletedRun(ctx, tenantID, req); err != nil {
		return nil, err
	} else if existing != nil {
		return reportFromRun(existing), nil
	}
	doc, errs := parseRequest(req, true)
	if len(errs) > 0 {
		return &Report{Errors: errs, TotalRows: int32(len(doc.Rows))}, nil
	}
	if mapped, err := ApplyMapping(doc, req.Mapping, templateFields[req.Target]); err != nil {
		return &Report{Errors: []models.ImportRowError{{Row: 1, Field: "mapping", Code: "invalid_mapping", Message: safeMessage(err.Error())}}, TotalRows: int32(len(doc.Rows))}, nil
	} else {
		doc = mapped
	}
	var run models.ImportRun
	session, err := e.DB.Client.StartSession()
	if err != nil {
		return nil, errNoTransaction
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		// Planning, reference resolution, immutable checks, and expected-version
		// filters all execute on the transaction snapshot.
		plans, planErrs, internalErr := e.plan(sc, tenantID, req.Target, doc)
		if internalErr != nil {
			return nil, internalErr
		}
		if len(planErrs) > 0 {
			return nil, &transactionValidationError{errors: planErrs}
		}
		now := time.Now().UTC()
		run = models.ImportRun{ID: primitive.NewObjectID(), TenantID: tenantID, UserID: userID, Target: req.Target, IdempotencyKey: req.IdempotencyKey, Status: models.ImportRunPending, TotalRows: int32(len(doc.Rows)), CreatedRows: countCreated(plans), UpdatedRows: countUpdated(plans), Errors: []models.ImportRowError{}, CreatedAt: now, UpdatedAt: now}
		if err := validation.Validate(&run); err != nil {
			return nil, err
		}
		if _, err := e.DB.ImportRuns().InsertOne(sc, run); err != nil {
			return nil, err
		}
		for writeNumber, plan := range plans {
			if err := e.applyPlan(sc, plan); err != nil {
				return nil, err
			}
			if e.testAfterWriteHook != nil {
				if err := e.testAfterWriteHook(writeNumber + 1); err != nil {
					return nil, err
				}
			}
		}
		run.Status = models.ImportRunCompleted
		run.UpdatedAt = time.Now().UTC()
		_, err := e.DB.ImportRuns().ReplaceOne(sc, bson.M{"_id": run.ID, "tenantId": tenantID}, run)
		return nil, err
	})
	if err != nil {
		var validationErr *transactionValidationError
		if errors.As(err, &validationErr) {
			return &Report{TotalRows: int32(len(doc.Rows)), Errors: boundedErrors(validationErr.errors)}, nil
		}
		if mongo.IsDuplicateKeyError(err) {
			winner, fetchErr := e.findCompletedRun(ctx, tenantID, req)
			if fetchErr != nil {
				return nil, fetchErr
			}
			if winner != nil {
				return reportFromRun(winner), nil
			}
		}
		return nil, err
	}
	return reportFromRun(&run), nil
}

func validateRequestMeta(req Request) []models.ImportRowError {
	errs := make([]models.ImportRowError, 0, 1)
	if !models.ValidImportTarget(req.Target) {
		errs = append(errs, rowError(1, "target", "invalid_target", "invalid import target"))
	}
	if len(req.IdempotencyKey) < 8 || len(req.IdempotencyKey) > 128 {
		errs = append(errs, rowError(1, "idempotencyKey", "invalid_idempotency_key", "idempotency key length is invalid"))
	}
	return errs
}
func validateRequestMetaDry(req Request) []models.ImportRowError {
	errs := make([]models.ImportRowError, 0, 1)
	if !models.ValidImportTarget(req.Target) {
		errs = append(errs, rowError(1, "target", "invalid_target", "invalid import target"))
	}
	return errs
}

func (e *Engine) findCompletedRun(ctx context.Context, tenantID primitive.ObjectID, req Request) (*models.ImportRun, error) {
	var run models.ImportRun
	err := e.DB.ImportRuns().FindOne(ctx, bson.M{"tenantId": tenantID, "target": req.Target, "idempotencyKey": req.IdempotencyKey, "status": models.ImportRunCompleted}).Decode(&run)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (e *Engine) findCompletedRunWithRetry(ctx context.Context, tenantID primitive.ObjectID, req Request) (*models.ImportRun, error) {
	for attempt := 0; attempt < 10; attempt++ {
		run, err := e.findCompletedRun(ctx, tenantID, req)
		if err != nil || run != nil {
			return run, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return nil, nil
}

// findCompletedRun deliberately returns only completed runs. A pending run is
// rolled back with its transaction, so it cannot strand an idempotency key.

func reportFromRun(run *models.ImportRun) *Report {
	errs := run.Errors
	if errs == nil {
		errs = []models.ImportRowError{}
	}
	return &Report{Run: run, TotalRows: run.TotalRows, CreatedRows: run.CreatedRows, UpdatedRows: run.UpdatedRows, Errors: errs}
}

func parseRequest(req Request, requireIdempotencyKey bool) (CSVDocument, []models.ImportRowError) {
	if !models.ValidImportTarget(req.Target) {
		return CSVDocument{}, []models.ImportRowError{{Row: 1, Field: "target", Code: "invalid_target", Message: "invalid import target"}}
	}
	if requireIdempotencyKey && (len(req.IdempotencyKey) < 8 || len(req.IdempotencyKey) > 128) {
		return CSVDocument{}, []models.ImportRowError{{Row: 1, Field: "idempotencyKey", Code: "invalid_idempotency_key", Message: "idempotency key length is invalid"}}
	}
	doc, err := ParseCSV(req.Content)
	if err != nil {
		return CSVDocument{}, []models.ImportRowError{{Row: 1, Field: "content", Code: "invalid_csv", Message: err.Error()}}
	}
	return doc, nil
}

type rowPlan struct {
	target   models.ImportTarget
	filter   bson.M
	document interface{}
	created  bool
	version  int64
}

func (e *Engine) plan(ctx context.Context, tenantID primitive.ObjectID, target models.ImportTarget, doc CSVDocument) ([]rowPlan, []models.ImportRowError, error) {
	fields := templateFields[target]
	headers := make(map[string]string, len(fields))
	for _, field := range fields {
		headers[field] = field
	}
	// Mapping is applied by the request caller before planning through this stable field.
	_ = headers
	plans := make([]rowPlan, 0, len(doc.Rows))
	errs := make([]models.ImportRowError, 0)
	identities := map[string]struct{}{}
	for _, row := range doc.Rows {
		plan, identity, err := e.planRow(ctx, tenantID, target, row)
		if err != nil {
			var dbErr *internalDBError
			if errors.As(err, &dbErr) {
				return nil, nil, dbErr
			}
			errs = append(errs, rowError(row.Number, "", "invalid_row", err.Error()))
			continue
		}
		if _, exists := identities[identity]; exists {
			errs = append(errs, rowError(row.Number, "", "duplicate_row", "duplicate identity in CSV"))
			continue
		}
		identities[identity] = struct{}{}
		plans = append(plans, plan)
	}
	return plans, boundedErrors(errs), nil
}

func (e *Engine) planRow(ctx context.Context, tenantID primitive.ObjectID, target models.ImportTarget, row CSVRow) (rowPlan, string, error) {
	v := row.Values
	now := time.Now().UTC()
	active, err := parseBool(v["isActive"], true)
	if err != nil {
		return rowPlan{}, "", err
	}
	switch target {
	case models.ImportTargetUnits:
		precision, err := parseInt32(v["precision"], 0, 6)
		if err != nil {
			return rowPlan{}, "", err
		}
		u := models.Unit{TenantID: tenantID, Code: strings.ToLower(v["code"]), Name: v["name"], Symbol: v["symbol"], Dimension: models.UnitDimension(v["dimension"]), Precision: precision, IsActive: active, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := validation.Validate(&u); err != nil {
			return rowPlan{}, "", err
		}
		existing := models.Unit{}
		found, err := findByCode(ctx, e.DB.Units(), tenantID, "code", u.Code, &existing)
		if err != nil {
			return rowPlan{}, "", err
		}
		if found {
			if existing.Dimension != u.Dimension {
				return rowPlan{}, "", errors.New("unit dimension is immutable")
			}
			u.ID, u.Version, u.CreatedAt = existing.ID, existing.Version+1, existing.CreatedAt
		}
		filter := bson.M{"tenantId": tenantID, "code": u.Code}
		if found {
			filter["_id"], filter["version"] = existing.ID, existing.Version
		}
		return rowPlan{target: target, filter: filter, document: u, created: !found, version: existing.Version}, u.Code, nil
	case models.ImportTargetCategories:
		c := models.Category{TenantID: tenantID, Code: strings.ToLower(v["code"]), Name: v["name"], IsActive: active, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := validation.Validate(&c); err != nil {
			return rowPlan{}, "", err
		}
		existing := models.Category{}
		found, err := findByCode(ctx, e.DB.Categories(), tenantID, "code", c.Code, &existing)
		if err != nil {
			return rowPlan{}, "", err
		}
		if found {
			c.ID, c.Version, c.CreatedAt = existing.ID, existing.Version+1, existing.CreatedAt
		}
		filter := bson.M{"tenantId": tenantID, "code": c.Code}
		if found {
			filter["_id"], filter["version"] = existing.ID, existing.Version
		}
		return rowPlan{target: target, filter: filter, document: c, created: !found, version: existing.Version}, c.Code, nil
	case models.ImportTargetItems:
		category, unit, err := e.lookupRefs(ctx, tenantID, v["categoryCode"], v["baseUnitCode"])
		if err != nil {
			return rowPlan{}, "", err
		}
		shelf, err := parseOptionalInt32(v["shelfLifeDays"], 0, 36500)
		if err != nil {
			return rowPlan{}, "", err
		}
		allergens := splitList(v["allergens"])
		stockable, err := parseBool(v["stockable"], false)
		if err != nil {
			return rowPlan{}, "", err
		}
		i := models.Item{TenantID: tenantID, SKU: strings.ToLower(v["sku"]), Name: v["name"], Description: v["description"], Brand: v["brand"], CategoryID: category.ID, BaseUnitID: unit.ID, Allergens: allergens, ShelfLifeDays: shelf, Stockable: stockable, IsActive: active, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := validation.Validate(&i); err != nil {
			return rowPlan{}, "", err
		}
		existing := models.Item{}
		found, err := findByCode(ctx, e.DB.Items(), tenantID, "sku", i.SKU, &existing)
		if err != nil {
			return rowPlan{}, "", err
		}
		if found {
			if existing.BaseUnitID != i.BaseUnitID {
				count, countErr := e.DB.ItemConversions().CountDocuments(ctx, bson.M{"tenantId": tenantID, "itemId": existing.ID})
				if countErr != nil {
					return rowPlan{}, "", &internalDBError{countErr}
				}
				if count > 0 {
					return rowPlan{}, "", errors.New("item base unit cannot change while conversions exist")
				}
			}
			i.ID, i.Version, i.CreatedAt = existing.ID, existing.Version+1, existing.CreatedAt
		}
		filter := bson.M{"tenantId": tenantID, "sku": i.SKU}
		if found {
			filter["_id"], filter["version"] = existing.ID, existing.Version
		}
		return rowPlan{target: target, filter: filter, document: i, created: !found, version: existing.Version}, i.SKU, nil
	case models.ImportTargetSuppliers:
		lead, err := parseInt32(v["defaultLeadTimeDays"], 0, 3650)
		if err != nil {
			return rowPlan{}, "", err
		}
		days, err := parseDays(v["orderingDays"])
		if err != nil {
			return rowPlan{}, "", err
		}
		s := models.Supplier{TenantID: tenantID, Code: strings.ToLower(v["code"]), Name: v["name"], ContactName: v["contactName"], Email: v["email"], Phone: v["phone"], OrderingDays: days, DefaultLeadTimeDays: lead, IsActive: active, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := validation.Validate(&s); err != nil {
			return rowPlan{}, "", err
		}
		existing := models.Supplier{}
		found, err := findByCode(ctx, e.DB.Suppliers(), tenantID, "code", s.Code, &existing)
		if err != nil {
			return rowPlan{}, "", err
		}
		if found {
			s.ID, s.Version, s.CreatedAt = existing.ID, existing.Version+1, existing.CreatedAt
		}
		filter := bson.M{"tenantId": tenantID, "code": s.Code}
		if found {
			filter["_id"], filter["version"] = existing.ID, existing.Version
		}
		return rowPlan{target: target, filter: filter, document: s, created: !found, version: existing.Version}, s.Code, nil
	case models.ImportTargetSupplierItems:
		supplier, item, err := e.lookupSupplierItemRefs(ctx, tenantID, v["supplierCode"], v["itemSKU"])
		if err != nil {
			return rowPlan{}, "", err
		}
		pack, err := parsePositiveInt64(v["packSizeMicros"])
		if err != nil {
			return rowPlan{}, "", err
		}
		price, err := parseNonNegativeInt64(v["unitPriceMinor"])
		if err != nil {
			return rowPlan{}, "", err
		}
		moq, err := parseInt32(v["moq"], 1, 1_000_000)
		if err != nil {
			return rowPlan{}, "", err
		}
		lead, err := parseOptionalInt32(v["leadTimeDays"], 0, 3650)
		if err != nil {
			return rowPlan{}, "", err
		}
		si := models.SupplierItem{TenantID: tenantID, SupplierID: supplier.ID, ItemID: item.ID, SupplierSKU: v["supplierSKU"], PackSizeMicros: pack, MOQ: moq, UnitPriceMinor: price, Currency: v["currency"], LeadTimeDays: lead, IsActive: active, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := validation.Validate(&si); err != nil {
			return rowPlan{}, "", err
		}
		existing := models.SupplierItem{}
		found, err := findSupplierItem(ctx, e.DB.SupplierItems(), tenantID, supplier.ID, item.ID, &existing)
		if err != nil {
			return rowPlan{}, "", err
		}
		if found {
			si.ID, si.Version, si.CreatedAt = existing.ID, existing.Version+1, existing.CreatedAt
		}
		filter := bson.M{"tenantId": tenantID, "supplierId": supplier.ID, "itemId": item.ID}
		if found {
			filter["_id"], filter["version"] = existing.ID, existing.Version
		}
		return rowPlan{target: target, filter: filter, document: si, created: !found, version: existing.Version}, supplier.ID.Hex() + ":" + item.ID.Hex(), nil
	}
	return rowPlan{}, "", errors.New("invalid import target")
}

func (e *Engine) lookupRefs(ctx context.Context, tenantID primitive.ObjectID, categoryCode, unitCode string) (models.Category, models.Unit, error) {
	var c models.Category
	var u models.Unit
	if ok, err := findByCode(ctx, e.DB.Categories(), tenantID, "code", strings.ToLower(categoryCode), &c); err != nil {
		return c, u, &internalDBError{err}
	} else if !ok || !c.IsActive {
		return c, u, errors.New("category reference is missing or inactive")
	}
	if ok, err := findByCode(ctx, e.DB.Units(), tenantID, "code", strings.ToLower(unitCode), &u); err != nil {
		return c, u, &internalDBError{err}
	} else if !ok || !u.IsActive {
		return c, u, errors.New("base unit reference is missing or inactive")
	}
	return c, u, nil
}
func (e *Engine) lookupSupplierItemRefs(ctx context.Context, tenantID primitive.ObjectID, supplierCode, sku string) (models.Supplier, models.Item, error) {
	var s models.Supplier
	var i models.Item
	if ok, err := findByCode(ctx, e.DB.Suppliers(), tenantID, "code", strings.ToLower(supplierCode), &s); err != nil {
		return s, i, &internalDBError{err}
	} else if !ok || !s.IsActive {
		return s, i, errors.New("supplier reference is missing or inactive")
	}
	if ok, err := findByCode(ctx, e.DB.Items(), tenantID, "sku", strings.ToLower(sku), &i); err != nil {
		return s, i, &internalDBError{err}
	} else if !ok || !i.IsActive {
		return s, i, errors.New("item reference is missing or inactive")
	}
	return s, i, nil
}

func (e *Engine) applyPlan(ctx mongo.SessionContext, p rowPlan) error {
	var result *mongo.UpdateResult
	var err error
	switch p.target {
	case models.ImportTargetUnits:
		result, err = e.DB.Units().ReplaceOne(ctx, p.filter, p.document, options.Replace().SetUpsert(true))
	case models.ImportTargetCategories:
		result, err = e.DB.Categories().ReplaceOne(ctx, p.filter, p.document, options.Replace().SetUpsert(true))
	case models.ImportTargetItems:
		result, err = e.DB.Items().ReplaceOne(ctx, p.filter, p.document, options.Replace().SetUpsert(true))
	case models.ImportTargetSuppliers:
		result, err = e.DB.Suppliers().ReplaceOne(ctx, p.filter, p.document, options.Replace().SetUpsert(true))
	case models.ImportTargetSupplierItems:
		result, err = e.DB.SupplierItems().ReplaceOne(ctx, p.filter, p.document, options.Replace().SetUpsert(true))
	}
	if err == nil && !p.created && result.MatchedCount != 1 {
		return errors.New("catalog record changed during import")
	}
	return err
}

func findByCode(ctx context.Context, collection *mongo.Collection, tenantID primitive.ObjectID, field, value string, target interface{}) (bool, error) {
	err := collection.FindOne(ctx, bson.M{"tenantId": tenantID, field: value}).Decode(target)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, &internalDBError{err}
	}
	return true, nil
}
func findSupplierItem(ctx context.Context, collection *mongo.Collection, tenantID, supplierID, itemID primitive.ObjectID, target interface{}) (bool, error) {
	err := collection.FindOne(ctx, bson.M{"tenantId": tenantID, "supplierId": supplierID, "itemId": itemID}).Decode(target)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, &internalDBError{err}
	}
	return true, nil
}
func countCreated(plans []rowPlan) int32 {
	var n int32
	for _, p := range plans {
		if p.created {
			n++
		}
	}
	return n
}
func countUpdated(plans []rowPlan) int32 { return int32(len(plans)) - countCreated(plans) }
func rowError(row int, field, code, message string) models.ImportRowError {
	return models.ImportRowError{Row: row, Field: field, Code: code, Message: safeMessage(message)}
}
func boundedErrors(errs []models.ImportRowError) []models.ImportRowError {
	if len(errs) > 100 {
		return errs[:100]
	}
	return errs
}
func safeMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if r < 32 && r != '\t' {
			return ' '
		}
		return r
	}, message)
	if len(message) > 240 {
		return message[:240]
	}
	return message
}
func parseBool(value string, defaultValue bool) (bool, error) {
	if value == "" {
		return defaultValue, nil
	}
	switch strings.ToLower(value) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	}
	return false, errors.New("invalid boolean")
}
func parseInt32(value string, min, max int32) (int32, error) {
	n, err := strconv.ParseInt(value, 10, 32)
	if err != nil || int32(n) < min || int32(n) > max {
		return 0, errors.New("invalid integer")
	}
	return int32(n), nil
}
func parseOptionalInt32(value string, min, max int32) (*int32, error) {
	if value == "" {
		return nil, nil
	}
	n, err := parseInt32(value, min, max)
	return &n, err
}
func parsePositiveInt64(value string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("invalid positive int64")
	}
	return n, nil
}
func parseNonNegativeInt64(value string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New("invalid non-negative int64")
	}
	return n, nil
}
func splitList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '|' || r == ';' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.TrimSpace(part))
	}
	return result
}
func parseDays(value string) ([]int32, error) {
	if value == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '|' || r == ',' || r == ';' })
	result := make([]int32, 0, len(parts))
	seen := map[int32]bool{}
	for _, part := range parts {
		n, err := parseInt32(strings.TrimSpace(part), 1, 7)
		if err != nil || seen[n] {
			return nil, errors.New("invalid ordering days")
		}
		seen[n] = true
		result = append(result, n)
	}
	return result, nil
}
