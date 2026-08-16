package forecast

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"lastsaas/internal/models"
	"lastsaas/internal/syslog"
	"lastsaas/internal/validation"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Worker struct {
	Jobs  *JobStore
	Store *MongoStore
	Log   *syslog.Logger
	Owner string
}

func (w *Worker) RunOnce(ctx context.Context) error {
	lease, err := w.Jobs.Claim(ctx, w.Owner, time.Now().UTC())
	if errors.Is(err, ErrJobNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if w.Log != nil {
		w.Log.Medium(ctx, "forecast worker claimed job "+lease.Job.ID.Hex())
	}
	workCtx, cancel := context.WithCancel(ctx)
	heartbeatErr := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		heartbeatEvery := w.Jobs.Config.LeaseDuration / 3
		if heartbeatEvery < time.Millisecond {
			heartbeatEvery = time.Millisecond
		}
		ticker := time.NewTicker(heartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				return
			case now := <-ticker.C:
				if err := w.Jobs.Heartbeat(workCtx, lease.Job.ID, w.Owner, lease.Token, now.UTC()); err != nil {
					heartbeatErr <- err
					return
				}
			}
		}
	}()
	err = w.execute(workCtx, lease)
	cancel()
	<-done
	select {
	case hbErr := <-heartbeatErr:
		if err == nil {
			err = hbErr
		}
	default:
	}
	if err != nil {
		if lease.Job.RunID != nil && w.Store != nil && w.Store.DB != nil {
			_, _ = w.Store.DB.ForecastRuns().UpdateOne(ctx, bson.M{"_id": *lease.Job.RunID, "tenantId": lease.Job.TenantID, "locationId": lease.Job.LocationID, "status": models.ForecastRunRunning}, bson.M{"$set": bson.M{"status": models.ForecastRunFailed, "error": truncateError(err), "updatedAt": time.Now().UTC()}})
		}
		_, failErr := w.Jobs.Fail(ctx, lease.Job.ID, w.Owner, lease.Token, err, time.Now().UTC())
		if w.Log != nil {
			w.Log.High(ctx, "forecast worker failed job "+lease.Job.ID.Hex()+": "+err.Error())
		}
		if failErr != nil {
			return fmt.Errorf("forecast job failed: %v; lease failure: %w", err, failErr)
		}
		return err
	}
	if w.Log != nil {
		w.Log.Medium(ctx, "forecast worker completed job "+lease.Job.ID.Hex())
	}
	return nil
}

func (w *Worker) execute(ctx context.Context, lease Lease) error {
	job := lease.Job
	if job.RunID == nil {
		return errors.New("forecast job has no deterministic run id")
	}
	dataset, err := loadJobDataset(ctx, w.Store, job)
	if err != nil {
		return err
	}
	snapshot, err := w.Store.loadSealed(ctx, Scope{TenantID: job.TenantID, LocationID: job.LocationID}, dataset)
	if err != nil {
		return err
	}
	var policy models.ForecastPolicy
	if err := w.Store.DB.ForecastPolicies().FindOne(ctx, bson.M{"_id": job.PolicyID, "tenantId": job.TenantID, "locationId": job.LocationID}).Decode(&policy); err != nil {
		return fmt.Errorf("load forecast policy: %w", err)
	}
	now := time.Now().UTC()
	run := models.ForecastRun{ID: *job.RunID, TenantID: job.TenantID, LocationID: job.LocationID, DatasetID: job.DatasetID, PolicyID: job.PolicyID, Status: models.ForecastRunRunning, Algorithm: AlgorithmSeasonalWeekday, AlgorithmVersion: AlgorithmVersion, CreatedAt: now, UpdatedAt: now}
	if err := w.ensureRun(ctx, run); err != nil {
		return err
	}
	seriesByItem := make(map[primitive.ObjectID][]Day)
	for _, row := range snapshot.Rows {
		if row.Kind != models.ForecastInputDemand || row.ItemID == nil {
			continue
		}
		seriesByItem[*row.ItemID] = append(seriesByItem[*row.ItemID], Day{Date: row.EffectiveAt, DemandMicros: row.QuantityMicros})
	}
	itemIDs := make([]primitive.ObjectID, 0, len(seriesByItem))
	for id := range seriesByItem {
		itemIDs = append(itemIDs, id)
	}
	sort.Slice(itemIDs, func(i, j int) bool { return itemIDs[i].Hex() < itemIDs[j].Hex() })
	allMetrics := map[string]int64{}
	for _, itemID := range itemIDs {
		series := seriesByItem[itemID]
		if policy.LookbackDays > 0 {
			series = trimLookback(series, policy.LookbackDays)
		}
		result, e := Forecast(series, Config{HorizonDays: int(policy.HorizonDays), MovingAverageDays: 28, MinimumTrainDays: 14, MaximumBacktests: 30, MaturityDays: 28})
		if e != nil {
			return e
		}
		if e := w.persistPoints(ctx, run, itemID, result.SelectedModel, result.Points); e != nil {
			return e
		}
		allMetrics["items"]++
		allMetrics["quality-observed-days"] += int64(result.Quality.ObservedDays)
		allMetrics["quality-missing-days"] += int64(result.Quality.MissingDays)
		for key, value := range result.Metadata.Metrics {
			allMetrics["item-"+metricName(key)] += value
		}
		if result.SelectedModel == ModelMovingAverage {
			allMetrics["selected-moving-average"]++
		} else {
			allMetrics["selected-seasonal-weekday"]++
		}
	}
	if err := w.persistMetrics(ctx, run, allMetrics); err != nil {
		return err
	}
	completed := time.Now().UTC()
	parameters := map[string]string{"core": AlgorithmVersion, "horizon-days": fmt.Sprint(policy.HorizonDays), "lookback-days": fmt.Sprint(policy.LookbackDays), "selection": "rolling-origin"}
	_, err = w.Store.DB.ForecastRuns().UpdateOne(ctx, bson.M{"_id": run.ID, "tenantId": run.TenantID, "locationId": run.LocationID, "status": models.ForecastRunRunning}, bson.M{"$set": bson.M{"status": models.ForecastRunSucceeded, "completedAt": completed, "updatedAt": completed, "algorithm": AlgorithmSeasonalWeekday, "algorithmVersion": AlgorithmVersion, "parameters": parameters, "metrics": allMetrics}})
	if err != nil {
		return err
	}
	// The recommendation lane reads the now-persisted successful run and its
	// points. It remains independently idempotent and immutable if a job is
	// retried after a transient recommendation write failure.
	if err := w.persistReorderOutputs(ctx, run, policy, snapshot); err != nil {
		return err
	}
	return w.Jobs.Complete(ctx, job.ID, w.Owner, lease.Token, run.ID, completed)
}

func (w *Worker) ensureRun(ctx context.Context, run models.ForecastRun) error {
	var existing models.ForecastRun
	err := w.Store.DB.ForecastRuns().FindOne(ctx, bson.M{"_id": run.ID, "tenantId": run.TenantID, "locationId": run.LocationID}).Decode(&existing)
	if err == nil {
		if existing.Status == models.ForecastRunSucceeded {
			return nil
		}
		return nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	_, err = w.Store.DB.ForecastRuns().InsertOne(ctx, run)
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}

func (w *Worker) persistPoints(ctx context.Context, run models.ForecastRun, itemID primitive.ObjectID, model Model, points []Point) error {
	for _, p := range points {
		point := models.ForecastPoint{ID: primitive.NewObjectID(), TenantID: run.TenantID, LocationID: run.LocationID, RunID: run.ID, ItemID: itemID, ModelID: string(model), TargetDate: p.Date, ForecastMicros: p.P50Micros, LowerMicros: p.P10Micros, UpperMicros: p.P90Micros, CreatedAt: run.CreatedAt}
		filter := bson.M{"tenantId": run.TenantID, "locationId": run.LocationID, "runId": run.ID, "itemId": itemID, "targetDate": p.Date}
		_, err := w.Store.DB.ForecastPoints().UpdateOne(ctx, filter, bson.M{"$setOnInsert": point}, options.Update().SetUpsert(true))
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) persistMetrics(ctx context.Context, run models.ForecastRun, metrics map[string]int64) error {
	for name, value := range metrics {
		metric := models.ForecastMetric{ID: primitive.NewObjectID(), TenantID: run.TenantID, LocationID: run.LocationID, RunID: run.ID, Name: name, Value: value, CreatedAt: run.CreatedAt}
		_, err := w.Store.DB.ForecastMetrics().UpdateOne(ctx, bson.M{"tenantId": run.TenantID, "locationId": run.LocationID, "runId": run.ID, "name": name}, bson.M{"$setOnInsert": metric}, options.Update().SetUpsert(true))
		if err != nil {
			return err
		}
	}
	return nil
}

type usableStock struct {
	QuantityMicros     int64
	ExpiryExcluded     bool
	QuarantineExcluded bool
}

type inboundStock struct {
	QuantityMicros int64
	SourceIDs      []string
}

type persistedPointSet struct {
	P90     []int64
	ModelID string
}

// persistReorderOutputs consumes only persisted forecast points and sealed
// input rows. Inventory and purchasing collections are read snapshots; this
// function has no write path to either subsystem.
func (w *Worker) persistReorderOutputs(ctx context.Context, run models.ForecastRun, policy models.ForecastPolicy, snapshot SealedSnapshot) error {
	cur, err := w.Store.DB.ForecastPoints().Find(ctx, bson.M{"tenantId": run.TenantID, "locationId": run.LocationID, "runId": run.ID}, options.Find().SetSort(bson.D{{Key: "itemId", Value: 1}, {Key: "targetDate", Value: 1}}))
	if err != nil {
		return fmt.Errorf("load persisted forecast points for reorder recommendations: %w", err)
	}
	defer cur.Close(ctx)
	var points []models.ForecastPoint
	if err := cur.All(ctx, &points); err != nil {
		return err
	}
	pointsByItem := make(map[primitive.ObjectID]persistedPointSet)
	for _, point := range points {
		set := pointsByItem[point.ItemID]
		set.P90 = append(set.P90, point.UpperMicros)
		if set.ModelID == "" && point.ModelID != "" {
			set.ModelID = point.ModelID
		}
		pointsByItem[point.ItemID] = set
	}
	if len(pointsByItem) == 0 {
		return nil
	}

	cutoff := time.Now().UTC()
	if snapshot.Dataset.CutoffAt != nil {
		cutoff = snapshot.Dataset.CutoffAt.UTC()
	}
	stock, err := w.readUsableStock(ctx, run, policy, cutoff)
	if err != nil {
		return err
	}
	inbound := confirmedInboundFromSnapshot(snapshot.Rows)
	seriesByItem := make(map[primitive.ObjectID][]Day)
	for _, row := range snapshot.Rows {
		if row.Kind == models.ForecastInputDemand && row.ItemID != nil {
			seriesByItem[*row.ItemID] = append(seriesByItem[*row.ItemID], Day{Date: row.EffectiveAt, DemandMicros: row.QuantityMicros})
		}
	}
	itemIDs := make([]primitive.ObjectID, 0, len(pointsByItem))
	for itemID := range pointsByItem {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Slice(itemIDs, func(i, j int) bool { return itemIDs[i].Hex() < itemIDs[j].Hex() })
	for _, itemID := range itemIDs {
		pointSet := pointsByItem[itemID]
		p90 := pointSet.P90
		calc, err := CalculateRecommendation(RecommendationInput{P90Micros: p90, HorizonDays: int64(policy.HorizonDays), SafetyDays: int64(policy.SafetyStockDays), UsableStockMicros: stock[itemID].QuantityMicros, ConfirmedInboundMicros: inbound[itemID].QuantityMicros})
		if err != nil {
			return fmt.Errorf("calculate reorder recommendation for %s: %w", itemID.Hex(), err)
		}
		quality, err := AssessQuality(seriesByItem[itemID], Config{HorizonDays: int(policy.HorizonDays), MaturityDays: DefaultConfig().MaturityDays})
		if err != nil {
			return err
		}
		flags := make([]string, len(quality.Flags))
		for i, flag := range quality.Flags {
			flags[i] = string(flag)
		}
		reasons := append([]string(nil), calc.ReasonCodes...)
		reasons = append(reasons, ReasonCodesForInventory(stock[itemID].ExpiryExcluded, stock[itemID].QuarantineExcluded, inbound[itemID].SourceIDs)...)
		maturity := "immature"
		if quality.Has(FlagMature) {
			maturity = "mature"
		}
		modelID := pointSet.ModelID
		if modelID == "" {
			modelID = run.Algorithm
		}
		created := run.CreatedAt
		status := models.ReorderRecommendationNoNeed
		if calc.RoundedQuantityMicros > 0 {
			status = models.ReorderRecommendationReady
		}
		common := struct {
			P90, Safety, Stock, Inbound, Requested, Rounded int64
		}{calc.P90DemandMicros, calc.SafetyDemandMicros, stock[itemID].QuantityMicros, inbound[itemID].QuantityMicros, calc.RequestedQuantityMicros, calc.RoundedQuantityMicros}
		coverage := models.ReorderCoverage{ID: primitive.NewObjectID(), TenantID: run.TenantID, LocationID: run.LocationID, RunID: run.ID, DatasetID: run.DatasetID, PolicyID: run.PolicyID, ItemID: itemID, ModelID: modelID, AlgorithmVersion: run.AlgorithmVersion, HorizonDays: policy.HorizonDays, P90DemandMicros: common.P90, SafetyDemandMicros: common.Safety, UsableStockMicros: common.Stock, ConfirmedInboundMicros: common.Inbound, ProjectedAvailableMicros: calc.ProjectedAvailableMicros, RequestedQuantityMicros: common.Requested, RoundedQuantityMicros: common.Rounded, CoverageDays: calc.CoverageDays, QualityFlags: flags, ObservedDays: int32(quality.ObservedDays), ExpectedDays: int32(quality.ExpectedDays), CoveragePermille: quality.CoveragePermille, Maturity: maturity, Formula: calc.Formula, ReasonCodes: reasons, InboundSourceIDs: append([]string(nil), inbound[itemID].SourceIDs...), CreatedAt: created}
		recommendation := models.ReorderRecommendation{ID: primitive.NewObjectID(), TenantID: run.TenantID, LocationID: run.LocationID, RunID: run.ID, DatasetID: run.DatasetID, PolicyID: run.PolicyID, ItemID: itemID, ModelID: modelID, AlgorithmVersion: run.AlgorithmVersion, HorizonDays: policy.HorizonDays, P90DemandMicros: common.P90, SafetyDemandMicros: common.Safety, UsableStockMicros: common.Stock, ConfirmedInboundMicros: common.Inbound, RequestedQuantityMicros: common.Requested, RoundedQuantityMicros: common.Rounded, QuantityMicros: common.Rounded, PackSizeMicros: 0, MOQ: 0, PackMOQDeltaMicros: calc.PackMOQDeltaMicros, Status: status, QualityFlags: flags, ObservedDays: int32(quality.ObservedDays), ExpectedDays: int32(quality.ExpectedDays), CoveragePermille: quality.CoveragePermille, Maturity: maturity, Formula: calc.Formula, ReasonCodes: reasons, InboundSourceIDs: append([]string(nil), inbound[itemID].SourceIDs...), CreatedAt: created}
		if err := validation.Validate(&coverage); err != nil {
			return fmt.Errorf("validate reorder coverage: %w", err)
		}
		if err := validation.Validate(&recommendation); err != nil {
			return fmt.Errorf("validate reorder recommendation: %w", err)
		}
		if _, err := w.Store.DB.ForecastCoverages().InsertOne(ctx, coverage); err != nil && !mongo.IsDuplicateKeyError(err) {
			return err
		}
		if _, err := w.Store.DB.ReorderRecommendations().InsertOne(ctx, recommendation); err != nil && !mongo.IsDuplicateKeyError(err) {
			return err
		}
	}
	return nil
}

func (w *Worker) readUsableStock(ctx context.Context, run models.ForecastRun, policy models.ForecastPolicy, cutoff time.Time) (map[primitive.ObjectID]usableStock, error) {
	cur, err := w.Store.DB.StockBalances().Find(ctx, bson.M{"tenantId": run.TenantID, "locationId": run.LocationID, "quantityMicros": bson.M{"$gt": int64(0)}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var balances []models.StockBalance
	if err := cur.All(ctx, &balances); err != nil {
		return nil, err
	}
	lotIDs := make([]primitive.ObjectID, 0)
	for _, balance := range balances {
		if balance.LotID != nil {
			lotIDs = append(lotIDs, *balance.LotID)
		}
	}
	lots := make(map[primitive.ObjectID]models.StockLot)
	if len(lotIDs) > 0 {
		lcur, err := w.Store.DB.StockLots().Find(ctx, bson.M{"tenantId": run.TenantID, "_id": bson.M{"$in": lotIDs}})
		if err != nil {
			return nil, err
		}
		defer lcur.Close(ctx)
		var values []models.StockLot
		if err := lcur.All(ctx, &values); err != nil {
			return nil, err
		}
		for _, lot := range values {
			lots[lot.ID] = lot
		}
	}
	out := make(map[primitive.ObjectID]usableStock)
	horizonEnd := cutoff.UTC().AddDate(0, 0, int(policy.HorizonDays))
	for _, balance := range balances {
		if balance.LotID == nil {
			out[balance.ItemID] = addUsable(out[balance.ItemID], balance.QuantityMicros)
			continue
		}
		lot, ok := lots[*balance.LotID]
		if !ok {
			value := out[balance.ItemID]
			value.QuarantineExcluded = true
			out[balance.ItemID] = value
			continue
		}
		usable, reason := IsUsableAvailableLot(string(lot.Status), lot.ExpiresAt, horizonEnd)
		if !usable && reason == "expiry_excluded_over_horizon" {
			value := out[balance.ItemID]
			value.ExpiryExcluded = true
			out[balance.ItemID] = value
			continue
		}
		if !usable {
			value := out[balance.ItemID]
			value.QuarantineExcluded = true
			out[balance.ItemID] = value
			continue
		}
		out[balance.ItemID] = addUsable(out[balance.ItemID], balance.QuantityMicros)
	}
	return out, nil
}

func addUsable(value usableStock, amount int64) usableStock {
	if amount > 0 && value.QuantityMicros <= int64(^uint64(0)>>1)-amount {
		value.QuantityMicros += amount
	}
	return value
}

func confirmedInboundFromSnapshot(rows []models.ForecastInputRow) map[primitive.ObjectID]inboundStock {
	out := make(map[primitive.ObjectID]inboundStock)
	seen := make(map[string]struct{})
	for _, row := range rows {
		if row.Kind != models.ForecastInputConfirmedInbound || row.ItemID == nil || row.QuantityMicros <= 0 {
			continue
		}
		sourceIDs := row.SourceIDs
		if len(sourceIDs) == 0 {
			sourceIDs = []string{row.ID.Hex()}
		}
		duplicate := false
		for _, sourceID := range sourceIDs {
			if _, exists := seen[sourceID]; exists {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		for _, sourceID := range sourceIDs {
			seen[sourceID] = struct{}{}
		}
		value := out[*row.ItemID]
		if value.QuantityMicros <= int64(^uint64(0)>>1)-row.QuantityMicros {
			value.QuantityMicros += row.QuantityMicros
		}
		value.SourceIDs = append(value.SourceIDs, sourceIDs...)
		out[*row.ItemID] = value
	}
	for itemID := range out {
		sort.Strings(out[itemID].SourceIDs)
	}
	return out
}

func trimLookback(series []Day, lookback int32) []Day {
	if len(series) == 0 || lookback <= 0 {
		return series
	}
	latest := series[0].Date
	for _, d := range series[1:] {
		if d.Date.After(latest) {
			latest = d.Date
		}
	}
	cut := latest.AddDate(0, 0, -int(lookback)+1)
	out := make([]Day, 0, len(series))
	for _, d := range series {
		if !d.Date.Before(cut) {
			out = append(out, d)
		}
	}
	return out
}

func loadJobDataset(ctx context.Context, store *MongoStore, job models.ForecastJob) (models.ForecastDataset, error) {
	var dataset models.ForecastDataset
	err := store.DB.ForecastDatasets().FindOne(ctx, bson.M{"_id": job.DatasetID, "tenantId": job.TenantID, "locationId": job.LocationID, "status": models.ForecastDatasetSealed}).Decode(&dataset)
	return dataset, err
}

func metricName(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		if name[i] == '_' {
			out = append(out, '-')
		} else {
			out = append(out, name[i])
		}
	}
	return string(out)
}
