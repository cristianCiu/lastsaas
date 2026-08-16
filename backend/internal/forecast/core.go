// Package forecast contains the deterministic, side-effect free v6 forecast
// engine.  It deliberately works on strings and standard-library types so the
// engine cannot accidentally read mutable application state or a live model.
package forecast

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"
)

const (
	AlgorithmSeasonalWeekday = "seasonal-weekday-naive"
	AlgorithmMovingAverage   = "trailing-moving-average"
	AlgorithmVersion         = "v6"
)

type Model string

const (
	ModelSeasonalWeekday Model = AlgorithmSeasonalWeekday
	ModelMovingAverage   Model = AlgorithmMovingAverage
)

// Day is a sealed, calendar-day observation. Missing and closed are distinct:
// neither contributes demand, but both are retained for quality reporting.
type Day struct {
	Date         time.Time
	DemandMicros int64
	IsClosed     bool
	IsMissing    bool
}

type Config struct {
	HorizonDays       int
	MovingAverageDays int
	MinimumTrainDays  int
	MaximumBacktests  int
	MaturityDays      int
}

func DefaultConfig() Config {
	return Config{HorizonDays: 14, MovingAverageDays: 28, MinimumTrainDays: 14, MaximumBacktests: 30, MaturityDays: 28}
}

type Point struct {
	Date           time.Time
	ForecastMicros int64
	P10Micros      int64
	P50Micros      int64
	P90Micros      int64
}

type ModelMetric struct {
	Model        Model
	Origins      int
	Observations int
	MAEMicros    int64
}

type BacktestResult struct {
	Selected  Model
	Metrics   []ModelMetric
	Residuals map[Model][]int64
}

type Metadata struct {
	Model            Model
	AlgorithmVersion string
	Parameters       map[string]string
	Metrics          map[string]int64
}

type Result struct {
	Points        []Point
	SelectedModel Model
	Metadata      Metadata
	Quality       Quality
}

type QualityFlag string

const (
	FlagColdStart   QualityFlag = "cold_start"
	FlagSparse      QualityFlag = "sparse_history"
	FlagZeroDemand  QualityFlag = "zero_demand"
	FlagMissingDays QualityFlag = "missing_days"
	FlagClosureDays QualityFlag = "closure_days"
	FlagOutliers    QualityFlag = "outliers_clipped"
	FlagMature      QualityFlag = "mature"
)

type Quality struct {
	Flags            []QualityFlag
	ObservedDays     int
	ExpectedDays     int
	MissingDays      int
	ClosureDays      int
	CoveragePermille int64
}

// SeasonalWeekdayNaive and TrailingMovingAverage are the two intentionally
// small v6 baselines. They return zero for invalid input rather than making a
// partial, non-deterministic forecast; ForecastModel is the error-reporting
// entry point.
func SeasonalWeekdayNaive(series []Day, target time.Time) int64 {
	v, err := ForecastModel(series, ModelSeasonalWeekday, target, DefaultConfig())
	if err != nil {
		return 0
	}
	return v
}

func TrailingMovingAverage(series []Day, target time.Time, windowDays int) int64 {
	cfg := DefaultConfig()
	if windowDays > 0 {
		cfg.MovingAverageDays = windowDays
	}
	v, err := ForecastModel(series, ModelMovingAverage, target, cfg)
	if err != nil {
		return 0
	}
	return v
}

func EmpiricalResidualBands(base int64, residuals []int64) (p10, p50, p90 int64) {
	return band(base, residuals)
}

func AssessQuality(series []Day, cfg Config) (Quality, error) {
	cfg = withDefaults(cfg)
	days, err := normalizeDays(series)
	if err != nil {
		return Quality{}, err
	}
	return qualityOf(days, cfg), nil
}

// RollingOriginBacktest evaluates each candidate only against observations
// strictly after its origin. It is exposed separately for audit and tests;
// Forecast uses the same implementation for selection.
func RollingOriginBacktest(series []Day, cfg Config) (BacktestResult, error) {
	cfg = withDefaults(cfg)
	days, err := normalizeDays(series)
	if err != nil {
		return BacktestResult{}, err
	}
	selected, metrics, _ := selectModel(days, cfg)
	residuals := make(map[Model][]int64, 2)
	for _, model := range []Model{ModelSeasonalWeekday, ModelMovingAverage} {
		_, _, _, r := backtest(days, model, cfg)
		residuals[model] = r
	}
	return BacktestResult{Selected: selected, Metrics: metrics, Residuals: residuals}, nil
}

func (q Quality) Has(flag QualityFlag) bool {
	for _, f := range q.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

var errBadConfig = errors.New("invalid forecast configuration")

// Forecast selects one of the two v6 baselines using rolling-origin tests and
// returns empirical residual bands. All arithmetic producing quantities is
// integer arithmetic; no float64 is used for a micro-unit result.
func Forecast(series []Day, cfg Config) (Result, error) {
	cfg = withDefaults(cfg)
	if cfg.HorizonDays < 1 || cfg.MovingAverageDays < 1 || cfg.MinimumTrainDays < 1 || cfg.MaximumBacktests < 1 || cfg.MaturityDays < 1 {
		return Result{}, errBadConfig
	}
	days, err := normalizeDays(series)
	if err != nil {
		return Result{}, err
	}
	quality := qualityOf(days, cfg)
	if len(days) == 0 {
		return Result{SelectedModel: ModelMovingAverage, Quality: quality, Metadata: metadata(ModelMovingAverage, cfg, nil, nil)}, nil
	}
	selected, metrics, residuals := selectModel(days, cfg)
	latest := days[len(days)-1].Date
	points := make([]Point, 0, cfg.HorizonDays)
	for i := 1; i <= cfg.HorizonDays; i++ {
		date := latest.AddDate(0, 0, i)
		base := predict(selected, days, date, cfg)
		p10, p50, p90 := band(base, residuals)
		points = append(points, Point{Date: date, ForecastMicros: p50, P10Micros: p10, P50Micros: p50, P90Micros: p90})
	}
	return Result{Points: points, SelectedModel: selected, Metadata: metadata(selected, cfg, metrics, residuals), Quality: quality}, nil
}

// ForecastModel is useful when a caller wants a baseline without selection.
func ForecastModel(series []Day, model Model, target time.Time, cfg Config) (int64, error) {
	cfg = withDefaults(cfg)
	days, err := normalizeDays(series)
	if err != nil {
		return 0, err
	}
	if model != ModelSeasonalWeekday && model != ModelMovingAverage {
		return 0, fmt.Errorf("unknown forecast model %q", model)
	}
	return predict(model, days, day(target), cfg), nil
}

func withDefaults(c Config) Config {
	d := DefaultConfig()
	if c.HorizonDays == 0 {
		c.HorizonDays = d.HorizonDays
	}
	if c.MovingAverageDays == 0 {
		c.MovingAverageDays = d.MovingAverageDays
	}
	if c.MinimumTrainDays == 0 {
		c.MinimumTrainDays = d.MinimumTrainDays
	}
	if c.MaximumBacktests == 0 {
		c.MaximumBacktests = d.MaximumBacktests
	}
	if c.MaturityDays == 0 {
		c.MaturityDays = d.MaturityDays
	}
	return c
}

func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func normalizeDays(in []Day) ([]Day, error) {
	byDate := make(map[int64]Day, len(in))
	for _, v := range in {
		if v.Date.IsZero() {
			return nil, errors.New("forecast day has no date")
		}
		if v.DemandMicros < 0 {
			return nil, errors.New("forecast demand cannot be negative")
		}
		d := day(v.Date)
		key := d.Unix() / 86400
		// Duplicate rows are combined exactly, while an explicit missing/closed
		// marker wins over an accidental ordinary row.
		old, ok := byDate[key]
		if ok {
			old.DemandMicros, ok = add(old.DemandMicros, v.DemandMicros)
			if !ok {
				return nil, errors.New("forecast demand overflows int64")
			}
			old.IsClosed = old.IsClosed || v.IsClosed
			old.IsMissing = old.IsMissing || v.IsMissing
			byDate[key] = old
		} else {
			v.Date, v.DemandMicros = d, v.DemandMicros
			byDate[key] = v
		}
	}
	out := make([]Day, 0, len(byDate))
	for _, v := range byDate {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

func add(a, b int64) (int64, bool) {
	if b > 0 && a > int64(^uint64(0)>>1)-b {
		return 0, false
	}
	return a + b, true
}

func valid(d Day) bool { return !d.IsClosed && !d.IsMissing }

func predict(model Model, train []Day, target time.Time, cfg Config) int64 {
	target = day(target)
	if model == ModelSeasonalWeekday {
		values := make([]int64, 0)
		for _, d := range train {
			if valid(d) && d.Date.Weekday() == target.Weekday() && d.Date.Before(target) {
				values = append(values, d.DemandMicros)
			}
		}
		if len(values) > 0 {
			return robustAverage(values)
		}
	}
	values := make([]int64, 0)
	cut := target.AddDate(0, 0, -cfg.MovingAverageDays)
	for _, d := range train {
		if valid(d) && d.Date.Before(target) && !d.Date.Before(cut) {
			values = append(values, d.DemandMicros)
		}
	}
	if len(values) == 0 {
		for _, d := range train {
			if valid(d) && d.Date.Before(target) {
				values = append(values, d.DemandMicros)
			}
		}
	}
	return robustAverage(values)
}

func robustAverage(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	v := append([]int64(nil), values...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	if len(v) >= 5 {
		q1, q3 := nearestQuantile(v, 1, 4), nearestQuantile(v, 3, 4)
		iqr := q3 - q1
		if iqr >= 0 {
			spread := triple(iqr)
			lo := int64(0)
			if spread <= q1 {
				lo = q1 - spread
			}
			hi := int64(^uint64(0) >> 1)
			if spread <= int64(^uint64(0)>>1)-q3 {
				hi = q3 + spread
			}
			for i := range v {
				if v[i] < lo {
					v[i] = lo
				}
				if v[i] > hi {
					v[i] = hi
				}
			}
		}
	}
	return average(v)
}

func triple(v int64) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	if v > maxInt64/3 {
		return maxInt64
	}
	return v * 3
}

func average(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	sum := new(big.Int)
	for _, n := range v {
		sum.Add(sum, big.NewInt(n))
	}
	sum.Quo(sum, big.NewInt(int64(len(v))))
	if !sum.IsInt64() {
		return int64(^uint64(0) >> 1)
	}
	return sum.Int64()
}

func nearestQuantile(sorted []int64, numerator, denominator int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	// ceil(p*n), with one-based rank, is stable for small samples and avoids
	// fractional micro-units.
	rank := (numerator*len(sorted) + denominator - 1) / denominator
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func band(base int64, residuals []int64) (int64, int64, int64) {
	if len(residuals) == 0 {
		return base, base, base
	}
	v := append([]int64(nil), residuals...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	return addSigned(base, nearestSigned(v, 1, 10)), addSigned(base, nearestSigned(v, 1, 2)), addSigned(base, nearestSigned(v, 9, 10))
}

func nearestSigned(sorted []int64, n, d int) int64 {
	rank := (n*len(sorted) + d - 1) / d
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func addSigned(a, b int64) int64 {
	x := new(big.Int).Add(big.NewInt(a), big.NewInt(b))
	if x.Sign() < 0 {
		return 0
	}
	if !x.IsInt64() {
		return int64(^uint64(0) >> 1)
	}
	return x.Int64()
}

func selectModel(days []Day, cfg Config) (Model, []ModelMetric, []int64) {
	models := []Model{ModelSeasonalWeekday, ModelMovingAverage}
	metrics := make([]ModelMetric, 0, 2)
	best := models[0]
	bestMAE := int64(^uint64(0) >> 1)
	var bestResiduals []int64
	for _, model := range models {
		origins, observations, mae, residuals := backtest(days, model, cfg)
		metrics = append(metrics, ModelMetric{Model: model, Origins: origins, Observations: observations, MAEMicros: mae})
		if observations > 0 && (mae < bestMAE || (mae == bestMAE && model == ModelSeasonalWeekday)) {
			best, bestMAE, bestResiduals = model, mae, residuals
		}
	}
	if bestMAE == int64(^uint64(0)>>1) {
		bestResiduals = nil
	}
	return best, metrics, bestResiduals
}

func backtest(days []Day, model Model, cfg Config) (int, int, int64, []int64) {
	if len(days) <= cfg.MinimumTrainDays {
		return 0, 0, 0, nil
	}
	start := cfg.MinimumTrainDays - 1
	end := len(days) - 2
	if total := end - start + 1; total > cfg.MaximumBacktests {
		start = end - cfg.MaximumBacktests + 1
	}
	var errors []int64
	origins, observations := 0, 0
	var total big.Int
	for oi := start; oi <= end; oi++ {
		origin := days[oi].Date
		train := make([]Day, 0, oi+1)
		for _, d := range days {
			if !d.Date.After(origin) {
				train = append(train, d)
			}
		}
		usedOrigin := false
		for ti := oi + 1; ti < len(days) && ti <= oi+cfg.HorizonDays; ti++ {
			actual := days[ti]
			if !valid(actual) {
				continue
			}
			pred := predict(model, train, actual.Date, cfg)
			err := new(big.Int).Sub(big.NewInt(actual.DemandMicros), big.NewInt(pred))
			abs := new(big.Int).Abs(err)
			if abs.IsInt64() {
				errors = append(errors, err.Int64())
				total.Add(&total, abs)
			} else {
				total.SetInt64(int64(^uint64(0) >> 1))
			}
			observations++
			usedOrigin = true
		}
		if usedOrigin {
			origins++
		}
	}
	if observations == 0 {
		return origins, 0, 0, errors
	}
	total.Quo(&total, big.NewInt(int64(observations)))
	mae := int64(^uint64(0) >> 1)
	if total.IsInt64() {
		mae = total.Int64()
	}
	return origins, observations, mae, errors
}

func qualityOf(days []Day, cfg Config) Quality {
	q := Quality{}
	if len(days) == 0 {
		q.Flags = []QualityFlag{FlagColdStart}
		return q
	}
	q.ExpectedDays = int(days[len(days)-1].Date.Sub(days[0].Date).Hours()/24) + 1
	present := make(map[int64]bool, len(days))
	for _, d := range days {
		present[d.Date.Unix()/86400] = true
		if d.IsClosed {
			q.ClosureDays++
		}
		if d.IsMissing {
			q.MissingDays++
		}
		if valid(d) {
			q.ObservedDays++
		}
	}
	q.MissingDays += q.ExpectedDays - len(present)
	if q.ExpectedDays > 0 {
		q.CoveragePermille = int64(q.ObservedDays * 1000 / q.ExpectedDays)
	}
	if q.ObservedDays < cfg.MaturityDays {
		q.Flags = append(q.Flags, FlagColdStart)
	}
	if q.ObservedDays < 7 {
		q.Flags = append(q.Flags, FlagSparse)
	}
	if q.ObservedDays > 0 {
		allZero := true
		vals := make([]int64, 0, q.ObservedDays)
		for _, d := range days {
			if valid(d) {
				vals = append(vals, d.DemandMicros)
				if d.DemandMicros != 0 {
					allZero = false
				}
			}
		}
		if allZero {
			q.Flags = append(q.Flags, FlagZeroDemand)
		}
		if len(vals) >= 5 {
			sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
			q1, q3 := nearestQuantile(vals, 1, 4), nearestQuantile(vals, 3, 4)
			limit := q3 + triple(q3-q1)
			if limit < q3 {
				limit = int64(^uint64(0) >> 1)
			}
			for _, n := range vals {
				if n > limit {
					q.Flags = append(q.Flags, FlagOutliers)
					break
				}
			}
		}
	}
	if q.MissingDays > 0 {
		q.Flags = append(q.Flags, FlagMissingDays)
	}
	if q.ClosureDays > 0 {
		q.Flags = append(q.Flags, FlagClosureDays)
	}
	if q.ObservedDays >= cfg.MaturityDays && q.CoveragePermille >= 800 {
		q.Flags = append(q.Flags, FlagMature)
	}
	return q
}

func metadata(model Model, cfg Config, metrics []ModelMetric, residuals []int64) Metadata {
	p := map[string]string{"horizon_days": fmt.Sprint(cfg.HorizonDays), "moving_average_days": fmt.Sprint(cfg.MovingAverageDays), "minimum_train_days": fmt.Sprint(cfg.MinimumTrainDays), "backtest_limit": fmt.Sprint(cfg.MaximumBacktests), "residual_quantiles": "nearest-rank-p10-p50-p90"}
	m := map[string]int64{"residual_count": int64(len(residuals))}
	for _, x := range metrics {
		prefix := string(x.Model)
		m[prefix+"_origins"] = int64(x.Origins)
		m[prefix+"_observations"] = int64(x.Observations)
		m[prefix+"_mae_micros"] = x.MAEMicros
	}
	return Metadata{Model: model, AlgorithmVersion: AlgorithmVersion, Parameters: p, Metrics: m}
}
