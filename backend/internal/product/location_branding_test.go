package product

import (
	"testing"

	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestResolveLocationBrandingFallbackOrder(t *testing.T) {
	location := models.Location{ID: primitive.NewObjectID(), Name: "Base Location"}
	tenant := &models.TenantBranding{PrimaryColor: "#111111", AccentColor: "#222222", Font: models.BrandingFontHumanist, Version: 4}
	override := &models.LocationBranding{DisplayName: "Flagship", PrimaryColor: "#333333", Version: 2}

	resolved := resolveLocationBranding(location, tenant, override)
	if resolved.DisplayName != "Flagship" || resolved.PrimaryColor != "#333333" || resolved.AccentColor != "#222222" || resolved.Font != models.BrandingFontHumanist {
		t.Fatalf("unexpected resolution: %#v", resolved)
	}
	if resolved.Sources["displayName"] != "location_branding" || resolved.Sources["primaryColor"] != "location_branding" || resolved.Sources["accentColor"] != "tenant" || resolved.Sources["font"] != "tenant" {
		t.Fatalf("unexpected sources: %#v", resolved.Sources)
	}

	resolved = resolveLocationBranding(location, &models.TenantBranding{}, &models.LocationBranding{})
	if resolved.DisplayName != location.Name || resolved.PrimaryColor != "" || resolved.Sources["primaryColor"] != "platform" {
		t.Fatalf("platform fallback failed: %#v", resolved)
	}
}
