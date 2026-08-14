package product

import (
	"errors"
	"net/http"
	"strings"

	"lastsaas/internal/apierror"
	masterimports "lastsaas/internal/imports"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (h *productHandler) importRequest(w http.ResponseWriter, r *http.Request) (*models.Tenant, *models.User, bool) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		apierror.BadRequest(w, r, "Tenant context required")
		return nil, nil, false
	}
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		apierror.Unauthorized(w, r, "User context required")
		return nil, nil, false
	}
	return tenant, user, true
}

func (h *productHandler) getImportTemplate(w http.ResponseWriter, r *http.Request) {
	target := models.ImportTarget(mux.Vars(r)["target"])
	content, err := masterimports.Template(target)
	if err != nil {
		apierror.NotFound(w, r, "Import template not found")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeTemplateName(string(target))+`.csv"`)
	_, _ = w.Write([]byte(content))
}

func safeTemplateName(value string) string {
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, value)
	return value
}

func (h *productHandler) decodeImport(w http.ResponseWriter, r *http.Request) (masterimports.Request, bool) {
	// The global body cap remains 1 MiB; ParseCSV independently enforces the
	// truthful 128 KiB CSV content cap.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request masterimports.Request
	if !decodeStrict(w, r, &request) {
		return request, false
	}
	return request, true
}

func (h *productHandler) dryRunImport(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	request, ok := h.decodeImport(w, r)
	if !ok {
		return
	}
	if request.Target == models.ImportTargetSales {
		source := strings.ToLower(strings.TrimSpace(request.Source))
		if source == "" {
			source = salesImportSourceDefault
		}
		plans, errs, err := (&salesEngine{db: h.db}).plan(r.Context(), tenant.ID, salesImportRequest{Content: request.Content, Source: source, Mapping: request.Mapping, IdempotencyKey: request.IdempotencyKey})
		if err != nil {
			apierror.Internal(w, r, "Sales import dry run could not be completed")
			return
		}
		salesReport := salesReportFromPlans(plans, errs, true)
		writeJSON(w, http.StatusOK, salesReport)
		return
	}
	report, err := (&masterimports.Engine{DB: h.db}).DryRun(r.Context(), tenant.ID, user.ID, request)
	if err != nil {
		apierror.Internal(w, r, "Import dry run could not be completed")
		return
	}
	importResponseReport(&report)
	writeJSON(w, http.StatusOK, report)
}

func importResponseReport(report *masterimports.Report) {
	if report.Errors == nil {
		report.Errors = []models.ImportRowError{}
	}
}

func (h *productHandler) applyImport(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	request, ok := h.decodeImport(w, r)
	if !ok {
		return
	}
	if request.Target == models.ImportTargetSales {
		profile, profileOK := GetStaffProfileFromContext(r.Context())
		if !profileOK || (profile.BusinessRole != models.BusinessRoleCompanyOwner && profile.BusinessRole != models.BusinessRoleOperationsManager) {
			apierror.Forbidden(w, r, "Operations manager permission required")
			return
		}
		source := strings.ToLower(strings.TrimSpace(request.Source))
		if source == "" {
			source = salesImportSourceDefault
		}
		report, err := h.applySalesImportRequest(r.Context(), tenant.ID, user.ID, salesImportRequest{Content: request.Content, Source: source, Mapping: request.Mapping, IdempotencyKey: request.IdempotencyKey})
		if err != nil {
			apierror.Internal(w, r, "Sales import could not be completed")
			return
		}
		writeJSON(w, http.StatusOK, report)
		return
	}
	report, err := (&masterimports.Engine{DB: h.db}).Apply(r.Context(), tenant.ID, user.ID, request)
	if err != nil {
		apierror.Internal(w, r, "Import could not be completed")
		return
	}
	if report.Run != nil {
		h.auditImport(r, report.Run)
	}
	importResponseReport(report)
	writeJSON(w, http.StatusOK, report)
}

func (h *productHandler) listImportRuns(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	cursor, err := h.db.ImportRuns().Find(r.Context(), bson.M{"tenantId": tenant.ID}, optionsImportRunList())
	if err != nil {
		apierror.Internal(w, r, "Failed to list imports")
		return
	}
	defer cursor.Close(r.Context())
	runs := []models.ImportRun{}
	if err := cursor.All(r.Context(), &runs); err != nil {
		apierror.Internal(w, r, "Failed to list imports")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func optionsImportRunList() *options.FindOptions {
	return options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
}

func (h *productHandler) getImportRun(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := h.importRequest(w, r)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["runId"])
	if err != nil {
		apierror.BadRequest(w, r, "Invalid import run ID")
		return
	}
	var run models.ImportRun
	if err := h.db.ImportRuns().FindOne(r.Context(), bson.M{"_id": id, "tenantId": tenant.ID}).Decode(&run); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			apierror.NotFound(w, r, "Import run not found")
		} else {
			apierror.Internal(w, r, "Failed to get import run")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}

func (h *productHandler) auditImport(r *http.Request, run *models.ImportRun) {
	if h.logger == nil {
		return
	}
	if user, ok := middleware.GetUserFromContext(r.Context()); ok {
		if tenant, ok := middleware.GetTenantFromContext(r.Context()); ok {
			h.logger.LogTenantActivity(r.Context(), models.LogLow, "Catalog import completed", user.ID, tenant.ID, "catalog.import.completed", map[string]interface{}{"target": run.Target, "status": run.Status, "rows": run.TotalRows})
		}
	}
}
