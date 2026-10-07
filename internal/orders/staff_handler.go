package orders

import (
	"github.com/Ferousco-dev/sati/backend/pkg/router"
	"net/http"
	"time"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

func (h *Handler) MountStaff(mux router.Mux, access auth.Access) {
	mux.Handle("GET /api/v1/admin/orders", access.Staff(http.HandlerFunc(h.staffList)))
	mux.Handle("GET /api/v1/admin/orders/{id}", access.Staff(http.HandlerFunc(h.staffGet)))
	mux.Handle("PATCH /api/v1/admin/orders/{id}/status", access.Staff(http.HandlerFunc(h.changeStatus)))
	mux.Handle("POST /api/v1/admin/orders/{id}/rider", access.Staff(http.HandlerFunc(h.assignRider)))
	mux.Handle("PATCH /api/v1/rider/deliveries/{id}/status", access.Rider(http.HandlerFunc(h.riderStatus)))
	mux.Handle("POST /api/v1/admin/orders/{id}/payment", access.Staff(http.HandlerFunc(h.recordPayment)))
}

func (h *Handler) staffList(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	q := r.URL.Query()
	f := StaffFilter{
		Status:        q.Get("status"),
		PaymentStatus: q.Get("payment_status"),
		PaymentMethod: q.Get("payment_method"),
		Fulfillment:   q.Get("fulfillment"),
		Query:         q.Get("q"),
		Limit:         limit,
		Offset:        offset,
	}
	for name, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if raw := q.Get(name); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				apierror.Write(w, validation(name+" must be an RFC 3339 timestamp"))
				return
			}
			*dst = &t
		}
	}

	page, err := h.svc.StaffList(r.Context(), f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, page)
}

func (h *Handler) staffGet(w http.ResponseWriter, r *http.Request) {
	o, err := h.svc.StaffGet(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"order": o})
}

func (h *Handler) changeStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status Status `json:"status"`
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	o, err := h.svc.ChangeStatus(r.Context(), userID(r), r.PathValue("id"), in.Status, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"order": o})
}

func (h *Handler) recordPayment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Note string `json:"note"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	o, err := h.svc.RecordPayment(r.Context(), userID(r), r.PathValue("id"), in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"order": o})
}

func (h *Handler) assignRider(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RiderID string `json:"rider_id"`
		Notes   string `json:"notes"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	o, err := h.svc.AssignRider(r.Context(), userID(r), r.PathValue("id"), in.RiderID, in.Notes)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"order": o})
}

func (h *Handler) riderStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	update, err := h.svc.RiderUpdateStatus(r.Context(), userID(r), r.PathValue("id"), in.Status, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, update)
}
