package validation

import (
	"strings"
	"testing"
	"time"

	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func validUser() models.User {
	return models.User{
		Email:       "test@example.com",
		DisplayName: "Test User",
		AuthMethods: []models.AuthMethod{models.AuthMethodPassword},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func TestValidate_ValidUser(t *testing.T) {
	u := validUser()
	if err := Validate(&u); err != nil {
		t.Errorf("expected valid user to pass: %v", err)
	}
}

func TestValidate_UserMissingEmail(t *testing.T) {
	u := validUser()
	u.Email = ""
	err := Validate(&u)
	if err == nil {
		t.Fatal("expected validation error for missing email")
	}
	if !strings.Contains(err.Error(), "Email") {
		t.Errorf("expected error to mention Email, got: %v", err)
	}
}

func TestValidate_UserInvalidEmail(t *testing.T) {
	u := validUser()
	u.Email = "not-an-email"
	if err := Validate(&u); err == nil {
		t.Fatal("expected validation error for invalid email")
	}
}

func TestValidate_UserMissingDisplayName(t *testing.T) {
	u := validUser()
	u.DisplayName = ""
	if err := Validate(&u); err == nil {
		t.Fatal("expected validation error for missing display name")
	}
}

func TestValidate_UserEmptyAuthMethods(t *testing.T) {
	u := validUser()
	u.AuthMethods = nil
	if err := Validate(&u); err == nil {
		t.Fatal("expected validation error for empty auth methods")
	}
}

func TestValidate_UserInvalidAuthMethod(t *testing.T) {
	u := validUser()
	u.AuthMethods = []models.AuthMethod{"carrier_pigeon"}
	if err := Validate(&u); err == nil {
		t.Fatal("expected validation error for invalid auth method")
	}
}

func TestValidate_UserValidThemePreference(t *testing.T) {
	for _, theme := range []string{"light", "dark", "system", ""} {
		u := validUser()
		u.ThemePreference = theme
		if err := Validate(&u); err != nil {
			t.Errorf("theme %q should be valid: %v", theme, err)
		}
	}
}

func TestValidate_UserInvalidThemePreference(t *testing.T) {
	u := validUser()
	u.ThemePreference = "neon"
	if err := Validate(&u); err == nil {
		t.Fatal("expected validation error for invalid theme preference")
	}
}

func TestValidate_ValidTenant(t *testing.T) {
	tenant := models.Tenant{
		Name:      "Acme Corp",
		Slug:      "acme-corp",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := Validate(&tenant); err != nil {
		t.Errorf("expected valid tenant to pass: %v", err)
	}
}

func TestValidate_TenantMissingName(t *testing.T) {
	tenant := models.Tenant{Slug: "slug", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := Validate(&tenant); err == nil {
		t.Fatal("expected validation error for missing tenant name")
	}
}

func TestValidate_TenantInvalidBillingStatus(t *testing.T) {
	tenant := models.Tenant{
		Name: "Test", Slug: "test", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		BillingStatus: "bogus",
	}
	if err := Validate(&tenant); err == nil {
		t.Fatal("expected validation error for invalid billing status")
	}
}

func TestValidate_ValidMembership(t *testing.T) {
	m := models.TenantMembership{
		UserID:    primitive.NewObjectID(),
		TenantID:  primitive.NewObjectID(),
		Role:      models.RoleAdmin,
		JoinedAt:  time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := Validate(&m); err != nil {
		t.Errorf("expected valid membership to pass: %v", err)
	}
}

func TestValidate_MembershipInvalidRole(t *testing.T) {
	m := models.TenantMembership{
		UserID: primitive.NewObjectID(), TenantID: primitive.NewObjectID(),
		Role: "superadmin", JoinedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := Validate(&m); err == nil {
		t.Fatal("expected validation error for invalid role")
	}
}

func validStaffProfile() models.StaffProfile {
	now := time.Now()
	return models.StaffProfile{
		ID: primitive.NewObjectID(), TenantID: primitive.NewObjectID(), UserID: primitive.NewObjectID(),
		BusinessRole: models.BusinessRoleViewer, AllLocations: false,
		LocationIDs: []primitive.ObjectID{}, PermissionOverrides: []models.PermissionOverride{},
		Status: models.StaffProfileActive, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
}

func TestValidate_StaffProfileStrictValuesAndScope(t *testing.T) {
	if err := Validate(validStaffProfile()); err != nil {
		t.Fatalf("valid staff profile failed validation: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*models.StaffProfile)
	}{
		{"unknown role", func(profile *models.StaffProfile) { profile.BusinessRole = "unknown" }},
		{"unknown permission", func(profile *models.StaffProfile) {
			profile.PermissionOverrides = []models.PermissionOverride{{Permission: "unknown", Allowed: true}}
		}},
		{"unknown status", func(profile *models.StaffProfile) { profile.Status = "disabled" }},
		{"nil locations", func(profile *models.StaffProfile) { profile.LocationIDs = nil }},
		{"nil overrides", func(profile *models.StaffProfile) { profile.PermissionOverrides = nil }},
		{"ambiguous scope", func(profile *models.StaffProfile) {
			profile.AllLocations = true
			profile.LocationIDs = []primitive.ObjectID{primitive.NewObjectID()}
		}},
		{"duplicate locations", func(profile *models.StaffProfile) {
			id := primitive.NewObjectID()
			profile.LocationIDs = []primitive.ObjectID{id, id}
		}},
		{"duplicate overrides", func(profile *models.StaffProfile) {
			profile.PermissionOverrides = []models.PermissionOverride{
				{Permission: models.PermissionStorageAreasRead, Allowed: true},
				{Permission: models.PermissionStorageAreasRead, Allowed: false},
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := validStaffProfile()
			test.mutate(&profile)
			if err := Validate(profile); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidate_TenantBrandingTokens(t *testing.T) {
	now := time.Now()
	valid := models.TenantBranding{
		TenantID: primitive.NewObjectID(), PrimaryColor: "#1a2b3c", AccentColor: "#abcdef",
		Font: models.BrandingFontHumanist, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := Validate(valid); err != nil {
		t.Fatalf("valid tenant branding failed validation: %v", err)
	}
	for name, mutate := range map[string]func(*models.TenantBranding){
		"uppercase color": func(branding *models.TenantBranding) { branding.PrimaryColor = "#AABBCC" },
		"short color":     func(branding *models.TenantBranding) { branding.AccentColor = "#fff" },
		"css payload":     func(branding *models.TenantBranding) { branding.PrimaryColor = "red; color: white" },
		"unknown font":    func(branding *models.TenantBranding) { branding.Font = "custom-font" },
	} {
		t.Run(name, func(t *testing.T) {
			branding := valid
			mutate(&branding)
			if err := Validate(branding); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	valid.PrimaryColor = ""
	valid.AccentColor = ""
	valid.Font = ""
	if err := Validate(valid); err != nil {
		t.Fatalf("inheritance defaults should be valid: %v", err)
	}
}

func TestValidate_LocationBrandingTokens(t *testing.T) {
	now := time.Now()
	valid := models.LocationBranding{
		TenantID: primitive.NewObjectID(), LocationID: primitive.NewObjectID(), DisplayName: "Flagship",
		PrimaryColor: "#aabbcc", AccentColor: "", Font: models.BrandingFontSerif,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := Validate(&valid); err != nil {
		t.Fatalf("valid location branding rejected: %v", err)
	}
	for name, mutate := range map[string]func(*models.LocationBranding){
		"oversized name": func(branding *models.LocationBranding) { branding.DisplayName = strings.Repeat("x", 201) },
		"unsafe color":   func(branding *models.LocationBranding) { branding.PrimaryColor = "red;display:none" },
		"unknown font":   func(branding *models.LocationBranding) { branding.Font = "remote" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := Validate(&candidate); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidate_ValidAPIKey(t *testing.T) {
	k := models.APIKey{
		Name: "test-key", KeyHash: "hash", KeyPreview: "prev",
		Authority: models.APIKeyAuthorityAdmin,
		CreatedBy: primitive.NewObjectID(), CreatedAt: time.Now(),
	}
	if err := Validate(&k); err != nil {
		t.Errorf("expected valid API key to pass: %v", err)
	}
}

func TestValidate_APIKeyInvalidAuthority(t *testing.T) {
	k := models.APIKey{
		Name: "test-key", KeyHash: "hash", KeyPreview: "prev",
		Authority: "superuser",
		CreatedBy: primitive.NewObjectID(), CreatedAt: time.Now(),
	}
	if err := Validate(&k); err == nil {
		t.Fatal("expected validation error for invalid authority")
	}
}

func TestValidate_ValidPlan(t *testing.T) {
	p := models.Plan{
		Name: "Pro", PricingModel: models.PricingModelFlat,
		CreditResetPolicy: models.CreditResetPolicyReset,
		CreatedAt:         time.Now(), UpdatedAt: time.Now(),
	}
	if err := Validate(&p); err != nil {
		t.Errorf("expected valid plan to pass: %v", err)
	}
}

func TestValidate_PlanInvalidPricingModel(t *testing.T) {
	p := models.Plan{
		Name: "Bad", PricingModel: "usage_based",
		CreditResetPolicy: models.CreditResetPolicyReset,
		CreatedAt:         time.Now(), UpdatedAt: time.Now(),
	}
	if err := Validate(&p); err == nil {
		t.Fatal("expected validation error for invalid pricing model")
	}
}

func TestValidate_PlanNegativePrice(t *testing.T) {
	p := models.Plan{
		Name: "Bad", PricingModel: models.PricingModelFlat,
		CreditResetPolicy: models.CreditResetPolicyReset,
		MonthlyPriceCents: -100,
		CreatedAt:         time.Now(), UpdatedAt: time.Now(),
	}
	if err := Validate(&p); err == nil {
		t.Fatal("expected validation error for negative price")
	}
}

func TestValidate_ValidWebhook(t *testing.T) {
	w := models.Webhook{
		Name: "test", URL: "https://example.com/hook",
		Secret: "whsec_test", SecretPreview: "test1234",
		Events:    []models.WebhookEventType{models.WebhookEventPaymentReceived},
		CreatedBy: primitive.NewObjectID(),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := Validate(&w); err != nil {
		t.Errorf("expected valid webhook to pass: %v", err)
	}
}

func TestValidate_WebhookInvalidEvent(t *testing.T) {
	w := models.Webhook{
		Name: "test", URL: "https://example.com/hook",
		Secret: "whsec_test", SecretPreview: "test1234",
		Events:    []models.WebhookEventType{"bogus.event"},
		CreatedBy: primitive.NewObjectID(),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := Validate(&w); err == nil {
		t.Fatal("expected validation error for invalid webhook event")
	}
}

func TestValidate_ValidInvitation(t *testing.T) {
	inv := models.Invitation{
		TenantID: primitive.NewObjectID(), Email: "user@test.com",
		Role: models.RoleUser, Token: "tok123",
		Status: models.InvitationPending, InvitedBy: primitive.NewObjectID(),
		ExpiresAt: time.Now().Add(24 * time.Hour), CreatedAt: time.Now(),
	}
	if err := Validate(&inv); err != nil {
		t.Errorf("expected valid invitation to pass: %v", err)
	}
}

func TestValidate_InvitationInvalidStatus(t *testing.T) {
	inv := models.Invitation{
		TenantID: primitive.NewObjectID(), Email: "user@test.com",
		Role: models.RoleUser, Token: "tok123",
		Status: "expired", InvitedBy: primitive.NewObjectID(),
		ExpiresAt: time.Now(), CreatedAt: time.Now(),
	}
	if err := Validate(&inv); err == nil {
		t.Fatal("expected validation error for invalid invitation status")
	}
}

func TestValidate_ValidConfigVar(t *testing.T) {
	cv := models.ConfigVar{
		Name: "app.title", Type: models.ConfigTypeString,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := Validate(&cv); err != nil {
		t.Errorf("expected valid config var to pass: %v", err)
	}
}

func TestValidate_ConfigVarInvalidType(t *testing.T) {
	cv := models.ConfigVar{
		Name: "test", Type: "yaml",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := Validate(&cv); err == nil {
		t.Fatal("expected validation error for invalid config var type")
	}
}

func TestValidate_CreditBundleZeroCredits(t *testing.T) {
	cb := models.CreditBundle{
		Name: "Small", Credits: 0, PriceCents: 100,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := Validate(&cb); err == nil {
		t.Fatal("expected validation error for zero credits")
	}
}

func TestValidate_ValidFinancialTransaction(t *testing.T) {
	ft := models.FinancialTransaction{
		TenantID: primitive.NewObjectID(), UserID: primitive.NewObjectID(),
		Type: models.TransactionSubscription, Currency: "usd",
		InvoiceNumber: "INV-000001", CreatedAt: time.Now(),
	}
	if err := Validate(&ft); err != nil {
		t.Errorf("expected valid transaction to pass: %v", err)
	}
}

func TestValidate_TransactionInvalidType(t *testing.T) {
	ft := models.FinancialTransaction{
		TenantID: primitive.NewObjectID(), UserID: primitive.NewObjectID(),
		Type: "chargeback", Currency: "usd",
		InvoiceNumber: "INV-000001", CreatedAt: time.Now(),
	}
	if err := Validate(&ft); err == nil {
		t.Fatal("expected validation error for invalid transaction type")
	}
}

func TestValidate_ErrorFormatting(t *testing.T) {
	u := models.User{} // all required fields missing
	err := Validate(&u)
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "validation failed: ") {
		t.Errorf("expected 'validation failed:' prefix, got: %s", msg)
	}
}

func validLocation() models.Location {
	now := time.Now()
	return models.Location{
		TenantID: primitive.NewObjectID(), Code: "berlin-mitte", Name: "Berlin Mitte",
		Timezone: "Europe/Berlin", IsActive: true, Version: 1, LimitSlot: 1, CreatedAt: now, UpdatedAt: now,
	}
}

func TestValidate_ValidLocation(t *testing.T) {
	location := validLocation()
	if err := Validate(&location); err != nil {
		t.Fatalf("expected valid location to pass: %v", err)
	}
}

func TestValidate_LocationCodeMustBeLowerCase(t *testing.T) {
	location := validLocation()
	location.Code = "Berlin-Mitte"
	if err := Validate(&location); err == nil {
		t.Fatal("expected mixed-case location code to fail")
	}
}

func TestValidate_LocationCodeRejectsInvalidShape(t *testing.T) {
	for _, code := range []string{"berlin mitte", "berlin_mitte", "-berlin", "berlin-"} {
		location := validLocation()
		location.Code = code
		if err := Validate(&location); err == nil {
			t.Errorf("expected location code %q to fail", code)
		}
	}
}

func TestValidate_LocationRequiresIANATimezone(t *testing.T) {
	for _, timezone := range []string{"Europe/Not_A_City", "Local"} {
		location := validLocation()
		location.Timezone = timezone
		if err := Validate(&location); err == nil {
			t.Errorf("expected timezone %q to fail", timezone)
		}
	}
}

func validRestaurantSettings() models.RestaurantSettings {
	now := time.Now()
	return models.RestaurantSettings{TenantID: primitive.NewObjectID(), Currency: "EUR", Language: "de-DE", DefaultTimezone: "Europe/Berlin", Version: 1, CreatedAt: now, UpdatedAt: now}
}

func TestValidate_RestaurantSettings(t *testing.T) {
	settings := validRestaurantSettings()
	if err := Validate(&settings); err != nil {
		t.Fatalf("expected valid restaurant settings: %v", err)
	}
	for _, mutate := range []func(*models.RestaurantSettings){
		func(s *models.RestaurantSettings) { s.Currency = "EURO" },
		func(s *models.RestaurantSettings) { s.Currency = "eur" },
		func(s *models.RestaurantSettings) { s.Language = "de-de" },
		func(s *models.RestaurantSettings) { s.Language = "german" },
		func(s *models.RestaurantSettings) { s.DefaultTimezone = "Europe/Nowhere" },
	} {
		invalid := validRestaurantSettings()
		mutate(&invalid)
		if err := Validate(&invalid); err == nil {
			t.Errorf("expected invalid restaurant settings to fail: %#v", invalid)
		}
	}
}

func TestValidate_StorageArea(t *testing.T) {
	now := time.Now()
	valid := models.StorageArea{TenantID: primitive.NewObjectID(), LocationID: primitive.NewObjectID(), Name: "Walk-in", Type: models.StorageAreaRefrigerated, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := Validate(&valid); err != nil {
		t.Fatalf("expected valid storage area: %v", err)
	}
	for _, area := range []models.StorageArea{
		{TenantID: valid.TenantID, LocationID: valid.LocationID, Name: "", Type: valid.Type, Version: 1, CreatedAt: now, UpdatedAt: now},
		{TenantID: valid.TenantID, LocationID: valid.LocationID, Name: "Walk-in", Type: "warehouse", Version: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if err := Validate(&area); err == nil {
			t.Errorf("expected invalid storage area to fail: %#v", area)
		}
	}
}

func TestValidate_Category(t *testing.T) {
	now := time.Now()
	valid := models.Category{TenantID: primitive.NewObjectID(), Code: "hot-food", Name: "Hot Food", IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := Validate(&valid); err != nil {
		t.Fatalf("expected valid category: %v", err)
	}
	for _, code := range []string{"Hot-Food", "hot food", "hot_food", "-hot", "hot-"} {
		candidate := valid
		candidate.Code = code
		if err := Validate(&candidate); err == nil {
			t.Errorf("expected category code %q to fail", code)
		}
	}
}

func TestValidate_Item(t *testing.T) {
	now := time.Now()
	valid := models.Item{TenantID: primitive.NewObjectID(), SKU: "burger-001", Name: "Burger", CategoryID: primitive.NewObjectID(), BaseUnitID: primitive.NewObjectID(), Allergens: []string{"milk", "eggs"}, Stockable: false, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := Validate(&valid); err != nil {
		t.Fatalf("expected valid item: %v", err)
	}
	for name, mutate := range map[string]func(*models.Item){
		"uppercase sku":      func(item *models.Item) { item.SKU = "Burger-001" },
		"invalid allergen":   func(item *models.Item) { item.Allergens = []string{"wheat"} },
		"duplicate allergen": func(item *models.Item) { item.Allergens = []string{"milk", "milk"} },
		"oversized name":     func(item *models.Item) { item.Name = strings.Repeat("x", 161) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := Validate(&candidate); err == nil {
				t.Fatal("expected invalid item to fail")
			}
		})
	}
}

func TestValidate_ItemConversion(t *testing.T) {
	now := time.Now()
	valid := models.ItemConversion{
		TenantID: primitive.NewObjectID(), ItemID: primitive.NewObjectID(), FromUnitID: primitive.NewObjectID(),
		Numerator: 3, Denominator: 4, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := Validate(&valid); err != nil {
		t.Fatalf("expected valid item conversion: %v", err)
	}
	for _, mutate := range []func(*models.ItemConversion){
		func(conversion *models.ItemConversion) { conversion.Numerator = 0 },
		func(conversion *models.ItemConversion) { conversion.Denominator = 1_000_000_001 },
	} {
		candidate := valid
		mutate(&candidate)
		if err := Validate(&candidate); err == nil {
			t.Fatal("expected invalid item conversion to fail")
		}
	}
}

func validStockPosting() models.StockPosting {
	now := time.Now()
	return models.StockPosting{
		ID: primitive.NewObjectID(), TenantID: primitive.NewObjectID(), LocationID: primitive.NewObjectID(),
		StorageAreaID: primitive.NewObjectID(), UserID: primitive.NewObjectID(), Type: models.StockPostingCount,
		IdempotencyKey: "stock-count-post-key", RequestHash: strings.Repeat("a", 64), EffectiveAt: now, RecordedAt: now,
	}
}

func validStockCount() models.StockCount {
	now := time.Now()
	return models.StockCount{
		ID: primitive.NewObjectID(), TenantID: primitive.NewObjectID(), LocationID: primitive.NewObjectID(),
		StorageAreaID: primitive.NewObjectID(), CreatedBy: primitive.NewObjectID(), Status: models.StockCountCancelled,
		Version: 1, IdempotencyKey: "stock-count-lifecycle-key", RequestHash: strings.Repeat("b", 64),
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestValidate_StockCountLifecycleValues(t *testing.T) {
	count := validStockCount()
	if err := Validate(&count); err != nil {
		t.Fatalf("cancelled stock count without timestamp should pass: %v", err)
	}
	for _, status := range []models.StockCountStatus{models.StockCountDraft, models.StockCountFrozen, models.StockCountReviewed, models.StockCountPosted} {
		count.Status = status
		if err := Validate(&count); err != nil {
			t.Errorf("existing stock count status %q should remain valid: %v", status, err)
		}
	}
	count.Status = models.StockCountCancelled
	now := time.Now()
	count.CancelledAt = &now
	if err := Validate(&count); err != nil {
		t.Fatalf("cancelled stock count with timestamp should pass: %v", err)
	}

	count.Status = "abandoned"
	if err := Validate(&count); err == nil {
		t.Fatal("unknown stock count status should fail")
	}
}

func TestValidate_StockPostingCountType(t *testing.T) {
	posting := validStockPosting()
	if err := Validate(&posting); err != nil {
		t.Fatalf("stock_count posting type should pass: %v", err)
	}
	posting.Type = "count_adjustment"
	if err := Validate(&posting); err == nil {
		t.Fatal("unknown stock posting type should fail")
	}
}
