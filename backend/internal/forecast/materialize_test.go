package forecast

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestMaterializeCorrectsOriginalConsumptionDateAtCutoff(t *testing.T) {
	base := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	got, err := Materialize(MaterializeRequest{CutoffAt: base.AddDate(0, 0, 3), Journal: []JournalRecord{
		{ID: "sale-1", PostingID: "post-1", ItemID: "item-a", Kind: JournalSaleConsumption, QuantityMicros: 7000000, EffectiveAt: base, RecordedAt: base.Add(time.Hour)},
		{ID: "reverse-1", ReversalOf: "post-1", Kind: JournalReversal, QuantityMicros: -7000000, EffectiveAt: base.AddDate(0, 0, 2), RecordedAt: base.AddDate(0, 0, 2)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 1 || got.Rows[0].EffectiveAt != day(base) || got.Rows[0].QuantityMicros != 0 {
		t.Fatalf("reversal was not applied to original day: %+v", got.Rows)
	}
	if !got.Rows[0].RecordedAt.Equal(base.AddDate(0, 0, 2)) {
		t.Fatalf("known-at was not retained: %s", got.Rows[0].RecordedAt)
	}
}

func TestMaterializeIncludesManualPlansAndExcludesFutureKnownFacts(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	d, err := Materialize(MaterializeRequest{CutoffAt: now, GuestPlans: []GuestPlanRecord{
		{ID: "plan-1", PlanDate: now.AddDate(0, 0, 1), GuestCount: 44, RecordedAt: now.Add(-time.Hour)},
		{ID: "plan-future", PlanDate: now, GuestCount: 99, RecordedAt: now.Add(time.Second)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Rows) != 1 || d.Rows[0].Kind != "guest_plan" || d.Rows[0].GuestCount != 44 {
		t.Fatalf("plans not materialized as sealed inputs: %+v", d.Rows)
	}
}

func TestMaterializeIncludesConfirmedInboundOnceAtKnownCutoff(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	d, err := Materialize(MaterializeRequest{CutoffAt: now, EffectiveFrom: now, EffectiveTo: now.AddDate(0, 0, 8), ConfirmedInbound: []ConfirmedInboundRecord{
		{ID: "line-1", ItemID: "item-1", QuantityMicros: 9, EffectiveAt: now.AddDate(0, 0, 3), RecordedAt: now.Add(-time.Hour)},
		{ID: "line-1", ItemID: "item-1", QuantityMicros: 9, EffectiveAt: now.AddDate(0, 0, 3), RecordedAt: now.Add(-time.Hour)},
	}})
	if err == nil {
		t.Fatal("duplicate confirmed inbound line was accepted")
	}
	d, err = Materialize(MaterializeRequest{CutoffAt: now, EffectiveFrom: now, EffectiveTo: now.AddDate(0, 0, 8), ConfirmedInbound: []ConfirmedInboundRecord{
		{ID: "line-1", ItemID: "item-1", QuantityMicros: 9, EffectiveAt: now.AddDate(0, 0, 3), RecordedAt: now.Add(-time.Hour)},
		{ID: "line-future", ItemID: "item-1", QuantityMicros: 11, EffectiveAt: now.AddDate(0, 0, 3), RecordedAt: now.Add(time.Hour)},
	}})
	if err != nil || len(d.Rows) != 1 || d.Rows[0].Kind != "confirmed_inbound" || d.Rows[0].QuantityMicros != 9 {
		t.Fatalf("confirmed inbound cutoff snapshot incorrect: %v %+v", err, d.Rows)
	}
}

func TestMaterializeHashIsStableAndRepositoryIsImmutable(t *testing.T) {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	records := []JournalRecord{
		{ID: "b", ItemID: "i", Kind: JournalSaleConsumption, QuantityMicros: 2, EffectiveAt: now.AddDate(0, 0, 1), RecordedAt: now},
		{ID: "a", ItemID: "i", Kind: JournalSaleConsumption, QuantityMicros: 1, EffectiveAt: now, RecordedAt: now},
	}
	a, err := Materialize(MaterializeRequest{CutoffAt: now.Add(time.Hour), Journal: records})
	if err != nil {
		t.Fatal(err)
	}
	records[0], records[1] = records[1], records[0]
	b, err := Materialize(MaterializeRequest{CutoffAt: now.Add(time.Hour), Journal: records})
	if err != nil {
		t.Fatal(err)
	}
	if a.Manifest.ContentHash != b.Manifest.ContentHash {
		t.Fatal("manifest hash changed with source order")
	}
	r := NewMemoryRepository()
	if _, err := MaterializeAndStore(context.Background(), r, MaterializeRequest{CutoffAt: now.Add(time.Hour), Journal: records}); err != nil {
		t.Fatal(err)
	}
	stored, err := r.GetSealed(context.Background(), a.Manifest.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	stored.Rows[0].QuantityMicros = 999
	again, err := r.GetSealed(context.Background(), a.Manifest.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(stored.Rows, again.Rows) {
		t.Fatal("repository returned mutable row storage")
	}
}
