package forecast

import (
	"reflect"
	"testing"
	"time"
)

func testDay(n int) time.Time { return time.Date(2026, 1, 5+n, 0, 0, 0, 0, time.UTC) }

func TestForecastIsDeterministicAndBandsOrdered(t *testing.T) {
	series := make([]Day, 0, 42)
	for i := 0; i < 42; i++ {
		series = append(series, Day{Date: testDay(i), DemandMicros: int64((i%9 + 1) * 1000000)})
	}
	cfg := DefaultConfig()
	cfg.HorizonDays = 5
	a, err := Forecast(series, cfg)
	if err != nil {
		t.Fatal(err)
	}
	reversed := append([]Day(nil), series...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	b, err := Forecast(reversed, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("forecast changed with input order\n%+v\n%+v", a, b)
	}
	for _, p := range a.Points {
		if !(p.P10Micros <= p.P50Micros && p.P50Micros <= p.P90Micros) {
			t.Fatalf("unordered point: %+v", p)
		}
	}
}

func TestForecastNeverUsesFutureObservations(t *testing.T) {
	history := []Day{{Date: testDay(0), DemandMicros: 10}, {Date: testDay(1), DemandMicros: 20}, {Date: testDay(2), DemandMicros: 30}}
	cfg := Config{MovingAverageDays: 30}
	a, err := ForecastModel(history, ModelMovingAverage, testDay(3), cfg)
	if err != nil {
		t.Fatal(err)
	}
	withFuture := append(append([]Day(nil), history...), Day{Date: testDay(20), DemandMicros: 999999999})
	b, err := ForecastModel(withFuture, ModelMovingAverage, testDay(3), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("future value leaked into prediction: %d != %d", a, b)
	}
}

func TestSeasonalModelSelection(t *testing.T) {
	series := make([]Day, 0, 70)
	for i := 0; i < 70; i++ {
		v := int64(0)
		if testDay(i).Weekday() == time.Monday {
			v = 100
		}
		series = append(series, Day{Date: testDay(i), DemandMicros: v})
	}
	cfg := DefaultConfig()
	cfg.HorizonDays = 2
	r, err := Forecast(series, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.SelectedModel != ModelSeasonalWeekday {
		t.Fatalf("selected %q, want seasonal", r.SelectedModel)
	}
}

func TestColdStartZeroClosureAndMissingFlags(t *testing.T) {
	r, err := Forecast([]Day{{Date: testDay(0), IsClosed: true}, {Date: testDay(1), DemandMicros: 0}, {Date: testDay(2), DemandMicros: 0, IsMissing: true}}, Config{HorizonDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []QualityFlag{FlagColdStart, FlagSparse, FlagZeroDemand, FlagClosureDays, FlagMissingDays} {
		if !r.Quality.Has(flag) {
			t.Errorf("missing quality flag %q", flag)
		}
	}
}

func TestFixedPointAverageDoesNotRoundThroughFloat(t *testing.T) {
	v, err := ForecastModel([]Day{{Date: testDay(0), DemandMicros: 1}, {Date: testDay(1), DemandMicros: 2}}, ModelMovingAverage, testDay(2), Config{MovingAverageDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	if v != 1 {
		t.Fatalf("got %d, want exact integer quotient 1", v)
	}
}

func TestOutlierIsClippedByBaselineAndResidualQuantilesOrder(t *testing.T) {
	series := make([]Day, 0, 8)
	for i := 0; i < 8; i++ {
		series = append(series, Day{Date: testDay(i), DemandMicros: 100})
	}
	series[7].DemandMicros = 1000000000000
	pred := TrailingMovingAverage(series[:7], testDay(7), 30)
	if pred != 100 {
		t.Fatalf("outlier-safe baseline changed to %d", pred)
	}
	p10, p50, p90 := EmpiricalResidualBands(100, []int64{-20, 0, 10, 1000, 5})
	if p10 > p50 || p50 > p90 {
		t.Fatalf("quantiles out of order: %d,%d,%d", p10, p50, p90)
	}
}

func TestRollingOriginBacktestReportsBothCandidates(t *testing.T) {
	series := make([]Day, 0, 24)
	for i := 0; i < 24; i++ {
		series = append(series, Day{Date: testDay(i), DemandMicros: int64(i + 1)})
	}
	cfg := DefaultConfig()
	cfg.MinimumTrainDays = 5
	cfg.HorizonDays = 2
	r, err := RollingOriginBacktest(series, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Metrics) != 2 || len(r.Residuals) != 2 {
		t.Fatalf("incomplete backtest result: %+v", r)
	}
	for _, m := range r.Metrics {
		if m.Origins == 0 || m.Observations == 0 {
			t.Fatalf("empty metric: %+v", m)
		}
	}
}
