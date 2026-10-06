package payments

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

const maxWebhookBytes = 1 << 20

func identityID(r *http.Request) string {
	id, _ := auth.IdentityFrom(r.Context())
	return id.User.ID
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		apierror.Write(w, apiErr)
		return
	}
	h.log.Error("request failed", "request_id", middleware.RequestID(r.Context()), "error", err)
	apierror.Write(w, apierror.ErrInternal)
}

func (h *Handler) initialize(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OrderID string `json:"order_id"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	res, err := h.svc.InitializePayment(r.Context(), identityID(r), in.OrderID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"payment": res})
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reference string `json:"reference"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	view, err := h.svc.VerifyPayment(r.Context(), identityID(r), strings.TrimSpace(in.Reference))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, view)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	view, err := h.repo.ForCustomerReference(r.Context(), h.pool, identityID(r), r.PathValue("reference"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, view)
}

// webhook answers 200 once an event is safely handled. Any other answer makes
// Paystack send it again, which is safe because events are processed once.
func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		apierror.Write(w, apierror.New(http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "Request body is too large"))
		return
	}

	err = h.svc.HandleWebhook(r.Context(), r.Header.Get("X-Paystack-Signature"), body)
	switch {
	case errors.Is(err, ErrInvalidSignature):
		apierror.Write(w, apierror.New(http.StatusUnauthorized, "INVALID_SIGNATURE", "Signature check failed"))
	case err != nil:
		h.log.Error("webhook failed", "request_id", middleware.RequestID(r.Context()), "error", err)
		apierror.Write(w, apierror.ErrInternal)
	default:
		apierror.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func (h *Handler) refunded(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Note string `json:"note"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > 200 {
		apierror.Write(w, apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "note must be at most 200 characters"))
		return
	}

	err := pgx.BeginFunc(r.Context(), h.pool, func(tx pgx.Tx) error {
		return h.repo.MarkRefunded(r.Context(), tx, r.PathValue("id"), note, h.now())
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
