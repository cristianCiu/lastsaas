package forecast

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"time"
)

// RecommendationInput is deliberately free of database models. The worker
// supplies only sealed forecast points and read-only inventory snapshots.
type RecommendationInput struct {
	P90Micros              []int64
	HorizonDays            int64
	SafetyDays             int64
	UsableStockMicros      int64
	ConfirmedInboundMicros int64
	PackSizeMicros         int64
	MOQ                    int64
}

type RecommendationCalculation struct {
	P90DemandMicros          int64
	SafetyDemandMicros       int64
	UsableStockMicros        int64
	ConfirmedInboundMicros   int64
	ProjectedAvailableMicros int64
	RequestedQuantityMicros  int64
	RoundedQuantityMicros    int64
	PackMOQDeltaMicros       int64
	OrderedPacks             int64
	CoverageDays             int64
	Formula                  string
	ReasonCodes              []string
}

var errRecommendationOverflow = errors.New("reorder recommendation quantity overflows int64")

// RoundRecommendationQuantity performs exact pack/MOQ arithmetic. A zero
// pack size means that no supplier terms were selected, so the base quantity
// is retained rather than selecting a supplier implicitly.
func RoundRecommendationQuantity(requested, packSize, moq int64) (rounded, delta, packs int64, err error) {
	if requested < 0 || packSize < 0 || moq < 0 {
		return 0, 0, 0, errors.New("reorder quantities cannot be negative")
	}
	if requested == 0 {
		return 0, 0, 0, nil
	}
	if packSize == 0 {
		return requested, 0, 0, nil
	}
	packs = ceilDiv(requested, packSize)
	if moq > packs {
		packs = moq
	}
	product, ok := multiplyInt64(packs, packSize)
	if !ok {
		return 0, 0, 0, errRecommendationOverflow
	}
	return product, product - requested, packs, nil
}

// CalculateRecommendation uses the upper (P90) forecast band throughout.
// Safety demand is the conservative ceiling of average P90 demand multiplied
// by policy safety days. All quantities are fixed-point integer quantities.
func CalculateRecommendation(in RecommendationInput) (RecommendationCalculation, error) {
	if in.HorizonDays < 1 || in.SafetyDays < 0 || in.UsableStockMicros < 0 || in.ConfirmedInboundMicros < 0 {
		return RecommendationCalculation{}, errors.New("invalid reorder recommendation inputs")
	}
	if len(in.P90Micros) == 0 {
		return RecommendationCalculation{}, errors.New("reorder recommendation requires forecast points")
	}
	for _, n := range in.P90Micros {
		if n < 0 {
			return RecommendationCalculation{}, errors.New("P90 demand cannot be negative")
		}
	}
	p90, ok := sumInt64(in.P90Micros)
	if !ok {
		return RecommendationCalculation{}, errRecommendationOverflow
	}
	safety, ok := ceilMulDiv(p90, in.SafetyDays, in.HorizonDays)
	if !ok {
		return RecommendationCalculation{}, errRecommendationOverflow
	}
	available, ok := addInt64(in.UsableStockMicros, in.ConfirmedInboundMicros)
	if !ok {
		return RecommendationCalculation{}, errRecommendationOverflow
	}
	target, ok := addInt64(p90, safety)
	if !ok {
		return RecommendationCalculation{}, errRecommendationOverflow
	}
	requested := int64(0)
	if target > available {
		requested = target - available
	}
	rounded, delta, packs, err := RoundRecommendationQuantity(requested, in.PackSizeMicros, in.MOQ)
	if err != nil {
		return RecommendationCalculation{}, err
	}
	coverageDays := int64(0)
	if p90 > 0 {
		coverageDays, _ = floorMulDiv(available, in.HorizonDays, p90)
	}
	reasons := []string{"p90_horizon_demand", "safety_days", "usable_available_stock", "confirmed_inbound_once"}
	if in.PackSizeMicros > 0 {
		reasons = append(reasons, "pack_moq_rounding")
	} else {
		reasons = append(reasons, "supplier_terms_not_selected")
	}
	if requested == 0 {
		reasons = append(reasons, "no_reorder_needed")
	} else {
		reasons = append(reasons, "below_target_coverage")
	}
	return RecommendationCalculation{
		P90DemandMicros: p90, SafetyDemandMicros: safety,
		UsableStockMicros: in.UsableStockMicros, ConfirmedInboundMicros: in.ConfirmedInboundMicros,
		ProjectedAvailableMicros: available, RequestedQuantityMicros: requested,
		RoundedQuantityMicros: rounded, PackMOQDeltaMicros: delta, OrderedPacks: packs,
		CoverageDays: coverageDays,
		Formula:      fmt.Sprintf("requested=max(0,(p90=%d + ceil(p90*safetyDays=%d*%d/%d)) - usableStock=%d - confirmedInbound=%d); rounded=packMOQ(%d)", p90, p90, in.SafetyDays, in.HorizonDays, in.UsableStockMicros, in.ConfirmedInboundMicros, rounded),
		ReasonCodes:  reasons,
	}, nil
}

// CalculateReorder is kept as a short alias for callers that do not need the
// longer persistence-oriented name.
func CalculateReorder(in RecommendationInput) (RecommendationCalculation, error) {
	return CalculateRecommendation(in)
}

func ceilDiv(a, b int64) int64 {
	if a == 0 {
		return 0
	}
	return (a-1)/b + 1
}

func ceilMulDiv(a, b, c int64) (int64, bool) {
	if b == 0 || a == 0 {
		return 0, true
	}
	x := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	x.Add(x, new(big.Int).Sub(big.NewInt(c), big.NewInt(1)))
	x.Quo(x, big.NewInt(c))
	if !x.IsInt64() {
		return 0, false
	}
	return x.Int64(), true
}

func floorMulDiv(a, b, c int64) (int64, bool) {
	x := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	x.Quo(x, big.NewInt(c))
	if !x.IsInt64() {
		return 0, false
	}
	return x.Int64(), true
}

func sumInt64(values []int64) (int64, bool) {
	x := new(big.Int)
	for _, value := range values {
		x.Add(x, big.NewInt(value))
	}
	if !x.IsInt64() {
		return 0, false
	}
	return x.Int64(), true
}

func addInt64(a, b int64) (int64, bool) {
	x := new(big.Int).Add(big.NewInt(a), big.NewInt(b))
	if !x.IsInt64() {
		return 0, false
	}
	return x.Int64(), true
}

func multiplyInt64(a, b int64) (int64, bool) {
	x := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	if !x.IsInt64() {
		return 0, false
	}
	return x.Int64(), true
}

// ReasonCodesForInventory appends the exclusion reasons in stable order for
// persistence and human-readable audit output.
func ReasonCodesForInventory(expired, quarantined bool, inboundSources []string) []string {
	reasons := []string{}
	if expired {
		reasons = append(reasons, "expiry_excluded_over_horizon")
	}
	if quarantined {
		reasons = append(reasons, "quarantine_excluded")
	}
	if len(inboundSources) > 0 {
		sort.Strings(inboundSources)
	}
	return reasons
}

// IsUsableAvailableLot applies the conservative horizon rule shared by the
// worker and its tests. A lot expiring on the horizon boundary is excluded.
func IsUsableAvailableLot(status string, expiresAt *time.Time, horizonEnd time.Time) (bool, string) {
	if status != "available" {
		return false, "quarantine_excluded"
	}
	if expiresAt != nil && !expiresAt.After(horizonEnd) {
		return false, "expiry_excluded_over_horizon"
	}
	return true, ""
}

func formatInt(n int64) string { return strconv.FormatInt(n, 10) }
