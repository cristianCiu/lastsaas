package forecast

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"
)

const MaterializationVersion = "v6-sealed-journal-1"

type JournalKind string

const (
	JournalSaleConsumption JournalKind = "sale_consumption"
	JournalReversal        JournalKind = "reversal"
)

// JournalRecord is an immutable fact. In particular, it has no sale status;
// cancellation is represented only by a later reversal record.
type JournalRecord struct {
	ID             string
	PostingID      string
	ReversalOf     string
	TenantID       string
	LocationID     string
	ItemID         string
	Kind           JournalKind
	QuantityMicros int64
	EffectiveAt    time.Time
	RecordedAt     time.Time
}

type GuestPlanRecord struct {
	ID            string
	TenantID      string
	LocationID    string
	PlanDate      time.Time
	ServicePeriod string
	GuestCount    int64
	RecordedAt    time.Time
	IsActual      bool
}

// ConfirmedInboundRecord is a cutoff-bound supplier confirmation snapshot.
// The line ID is its identity, so a line can enter a dataset only once.
type ConfirmedInboundRecord struct {
	ID             string
	TenantID       string
	LocationID     string
	ItemID         string
	QuantityMicros int64
	EffectiveAt    time.Time
	RecordedAt     time.Time
}

type MaterializeRequest struct {
	CutoffAt         time.Time
	EffectiveFrom    time.Time
	EffectiveTo      time.Time
	Journal          []JournalRecord
	GuestPlans       []GuestPlanRecord
	ConfirmedInbound []ConfirmedInboundRecord
}

type MaterializedRow struct {
	ID             string
	Kind           string
	ItemID         string
	ServicePeriod  string
	QuantityMicros int64
	GuestCount     int64
	EffectiveAt    time.Time
	RecordedAt     time.Time // known-at time, never replaced by effective time
	SourceIDs      []string
}

type Manifest struct {
	Version     string
	CutoffAt    time.Time
	RowCount    int
	ContentHash string
}

type SealedDataset struct {
	CutoffAt time.Time
	Rows     []MaterializedRow
	Manifest Manifest
}

var (
	errCutoff  = errors.New("materialization cutoff is required")
	errJournal = errors.New("invalid forecast journal record")
)

// Materialize normalizes immutable consumption and reversal facts at exactly
// cutoff. A reversal changes the original consumption date, not the date on
// which the correction was posted. Manual plans are included as separate
// guest-plan rows and never treated as actual sales.
func Materialize(req MaterializeRequest) (SealedDataset, error) {
	if req.CutoffAt.IsZero() {
		return SealedDataset{}, errCutoff
	}
	cutoff := req.CutoffAt.UTC()
	selected := make([]JournalRecord, 0, len(req.Journal))
	byID := make(map[string]JournalRecord)
	byPosting := make(map[string]JournalRecord)
	byPostingItem := make(map[string]JournalRecord)
	for _, r := range req.Journal {
		if r.ID == "" || r.EffectiveAt.IsZero() || r.RecordedAt.IsZero() {
			return SealedDataset{}, errJournal
		}
		if r.RecordedAt.After(cutoff) {
			continue
		}
		if r.Kind == JournalSaleConsumption {
			if r.ItemID == "" || r.QuantityMicros <= 0 {
				return SealedDataset{}, errJournal
			}
			if _, exists := byID[r.ID]; exists {
				return SealedDataset{}, fmt.Errorf("%w: duplicate journal id %s", errJournal, r.ID)
			}
			byID[r.ID] = r
			if r.PostingID != "" {
				byPosting[r.PostingID] = r
				byPostingItem[r.PostingID+"\x00"+r.ItemID] = r
			}
			selected = append(selected, r)
		} else if r.Kind == JournalReversal {
			if r.ReversalOf == "" {
				return SealedDataset{}, errJournal
			}
			selected = append(selected, r)
		} else {
			return SealedDataset{}, fmt.Errorf("%w: unknown kind %q", errJournal, r.Kind)
		}
	}

	type aggregate struct {
		sum              *big.Int
		effective, known time.Time
		sources          []string
	}
	aggs := make(map[string]*aggregate)
	addDelta := func(r JournalRecord, amount int64, effective time.Time) error {
		if !effectiveInRange(effective, req.EffectiveFrom, req.EffectiveTo) {
			return nil
		}
		key := r.ItemID + "\x00" + day(effective).Format("2006-01-02")
		a := aggs[key]
		if a == nil {
			a = &aggregate{sum: new(big.Int), effective: day(effective)}
			aggs[key] = a
		}
		a.sum.Add(a.sum, big.NewInt(amount))
		if r.RecordedAt.After(a.known) {
			a.known = r.RecordedAt
		}
		a.sources = append(a.sources, r.ID)
		return nil
	}
	for _, r := range selected {
		switch r.Kind {
		case JournalSaleConsumption:
			if err := addDelta(r, r.QuantityMicros, r.EffectiveAt); err != nil {
				return SealedDataset{}, err
			}
		case JournalReversal:
			original, ok := byID[r.ReversalOf]
			if !ok && r.ItemID != "" {
				original, ok = byPostingItem[r.ReversalOf+"\x00"+r.ItemID]
			}
			if !ok {
				original, ok = byPosting[r.ReversalOf]
			}
			if !ok {
				return SealedDataset{}, fmt.Errorf("%w: reversal %s has no known original", errJournal, r.ID)
			}
			amount := r.QuantityMicros
			if amount < 0 {
				amount = -amount
				if amount < 0 { // MinInt64 cannot be represented as a magnitude.
					return SealedDataset{}, errJournal
				}
			}
			if amount == 0 {
				amount = original.QuantityMicros
			}
			if amount > original.QuantityMicros {
				amount = original.QuantityMicros
			}
			// The effective date deliberately comes from the original fact.
			correction := r
			correction.ItemID = original.ItemID
			if err := addDelta(correction, -amount, original.EffectiveAt); err != nil {
				return SealedDataset{}, err
			}
		}
	}

	rows := make([]MaterializedRow, 0, len(aggs)+len(req.GuestPlans))
	for key, a := range aggs {
		quantity := int64(0)
		if a.sum.Sign() > 0 {
			if !a.sum.IsInt64() {
				quantity = int64(^uint64(0) >> 1)
			} else {
				quantity = a.sum.Int64()
			}
		}
		sort.Strings(a.sources)
		rows = append(rows, MaterializedRow{ID: "demand:" + key, Kind: "demand", ItemID: key[:len(key)-len("\x00")-10], QuantityMicros: quantity, EffectiveAt: a.effective, RecordedAt: a.known, SourceIDs: uniqueStrings(a.sources)})
	}
	for _, p := range req.GuestPlans {
		if p.ID == "" || p.PlanDate.IsZero() || p.RecordedAt.IsZero() || p.GuestCount < 0 {
			return SealedDataset{}, errors.New("invalid guest plan record")
		}
		if p.RecordedAt.After(cutoff) || p.IsActual || !effectiveInRange(p.PlanDate, req.EffectiveFrom, req.EffectiveTo) {
			continue
		}
		rows = append(rows, MaterializedRow{ID: "guest-plan:" + p.ID, Kind: "guest_plan", ServicePeriod: p.ServicePeriod, GuestCount: p.GuestCount, EffectiveAt: day(p.PlanDate), RecordedAt: p.RecordedAt, SourceIDs: []string{p.ID}})
	}
	seenInbound := make(map[string]struct{}, len(req.ConfirmedInbound))
	for _, inbound := range req.ConfirmedInbound {
		if inbound.ID == "" || inbound.ItemID == "" || inbound.QuantityMicros <= 0 || inbound.EffectiveAt.IsZero() || inbound.RecordedAt.IsZero() {
			return SealedDataset{}, errors.New("invalid confirmed inbound record")
		}
		if _, exists := seenInbound[inbound.ID]; exists {
			return SealedDataset{}, errors.New("duplicate confirmed inbound record")
		}
		seenInbound[inbound.ID] = struct{}{}
		if inbound.RecordedAt.After(cutoff) || !effectiveInRange(inbound.EffectiveAt, req.EffectiveFrom, req.EffectiveTo) {
			continue
		}
		rows = append(rows, MaterializedRow{ID: "confirmed-inbound:" + inbound.ID, Kind: "confirmed_inbound", ItemID: inbound.ItemID, QuantityMicros: inbound.QuantityMicros, EffectiveAt: day(inbound.EffectiveAt), RecordedAt: inbound.RecordedAt, SourceIDs: []string{inbound.ID}})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].EffectiveAt != rows[j].EffectiveAt {
			return rows[i].EffectiveAt.Before(rows[j].EffectiveAt)
		}
		if rows[i].Kind != rows[j].Kind {
			return rows[i].Kind < rows[j].Kind
		}
		if rows[i].ItemID != rows[j].ItemID {
			return rows[i].ItemID < rows[j].ItemID
		}
		return rows[i].ID < rows[j].ID
	})
	manifest := Manifest{Version: MaterializationVersion, CutoffAt: cutoff, RowCount: len(rows)}
	manifest.ContentHash = HashRows(manifest, rows)
	return SealedDataset{CutoffAt: cutoff, Rows: rows, Manifest: manifest}, nil
}

func effectiveInRange(t, from, to time.Time) bool {
	if !from.IsZero() && day(t).Before(day(from)) {
		return false
	}
	if !to.IsZero() && !day(t).Before(day(to)) {
		return false
	}
	return true
}

func uniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}

// HashRows hashes a canonical JSON stream of typed records. Inputs are sorted
// before materialization and SourceIDs are sorted, so input order cannot alter
// the manifest.
func HashRows(manifest Manifest, rows []MaterializedRow) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	_ = enc.Encode(struct {
		Version  string
		CutoffAt string
		RowCount int
	}{manifest.Version, manifest.CutoffAt.UTC().Format(time.RFC3339Nano), len(rows)})
	canonical := cloneRows(rows)
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].EffectiveAt != canonical[j].EffectiveAt {
			return canonical[i].EffectiveAt.Before(canonical[j].EffectiveAt)
		}
		if canonical[i].Kind != canonical[j].Kind {
			return canonical[i].Kind < canonical[j].Kind
		}
		if canonical[i].ItemID != canonical[j].ItemID {
			return canonical[i].ItemID < canonical[j].ItemID
		}
		return canonical[i].ID < canonical[j].ID
	})
	for _, r := range canonical {
		ids := append([]string(nil), r.SourceIDs...)
		sort.Strings(ids)
		_ = enc.Encode(struct {
			ID, Kind, ItemID, ServicePeriod string
			QuantityMicros, GuestCount      int64
			EffectiveAt, RecordedAt         string
			SourceIDs                       []string
		}{r.ID, r.Kind, r.ItemID, r.ServicePeriod, r.QuantityMicros, r.GuestCount, r.EffectiveAt.UTC().Format(time.RFC3339Nano), r.RecordedAt.UTC().Format(time.RFC3339Nano), ids})
	}
	return hex.EncodeToString(h.Sum(nil))
}
