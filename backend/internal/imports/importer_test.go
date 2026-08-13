package imports

import (
	"testing"

	"lastsaas/internal/models"
)

func TestTemplatesCoverAllImportTargets(t *testing.T) {
	for _, target := range []models.ImportTarget{models.ImportTargetUnits, models.ImportTargetCategories, models.ImportTargetItems, models.ImportTargetSuppliers, models.ImportTargetSupplierItems} {
		content, err := Template(target)
		if err != nil || len(content) == 0 {
			t.Errorf("Template(%q) error=%v", target, err)
		}
	}
}

func TestApplyRequestMetadataBeforeCSVParsing(t *testing.T) {
	errs := validateRequestMeta(Request{Target: "not-a-target", Content: "not csv", IdempotencyKey: "short"})
	if len(errs) != 2 || errs[0].Field != "target" || errs[1].Field != "idempotencyKey" {
		t.Fatalf("metadata errors=%v", errs)
	}
}
