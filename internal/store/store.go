package store

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

const maxDeliveryFeeKobo = 100_000_000

type Settings struct {
	DeliveryFeeKobo      int64     `json:"delivery_fee_kobo"`
	OnlineEnabled        bool      `json:"online_enabled"`
	BankTransferEnabled  bool      `json:"bank_transfer_enabled"`
	PayOnPickupEnabled   bool      `json:"pay_on_pickup_enabled"`
	PayOnDeliveryEnabled bool      `json:"pay_on_delivery_enabled"`
	BankName             string    `json:"bank_name"`
	BankAccountNumber    string    `json:"bank_account_number"`
	BankAccountName      string    `json:"bank_account_name"`
	WhatsAppNumber       string    `json:"whatsapp_number"`
	WhatsAppMessage      string    `json:"whatsapp_message"`
	NotificationEmail    string    `json:"notification_email"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type UpdateInput struct {
	DeliveryFeeKobo      *int64  `json:"delivery_fee_kobo"`
	OnlineEnabled        *bool   `json:"online_enabled"`
	BankTransferEnabled  *bool   `json:"bank_transfer_enabled"`
	PayOnPickupEnabled   *bool   `json:"pay_on_pickup_enabled"`
	PayOnDeliveryEnabled *bool   `json:"pay_on_delivery_enabled"`
	BankName             *string `json:"bank_name"`
	BankAccountNumber    *string `json:"bank_account_number"`
	BankAccountName      *string `json:"bank_account_name"`
	WhatsAppNumber       *string `json:"whatsapp_number"`
	WhatsAppMessage      *string `json:"whatsapp_message"`
	NotificationEmail    *string `json:"notification_email"`
}

func (s Settings) BankTransferReady() bool {
	return s.BankName != "" && s.BankAccountNumber != "" && s.BankAccountName != ""
}

const columns = `delivery_fee_kobo, online_enabled, bank_transfer_enabled, pay_on_pickup_enabled,
	pay_on_delivery_enabled, bank_name, bank_account_number, bank_account_name,
	whatsapp_number, whatsapp_message, notification_email, updated_at`

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (Repository) Get(ctx context.Context, db database.DBTX) (Settings, error) {
	return scan(db.QueryRow(ctx, `SELECT `+columns+` FROM store_settings`))
}

func scan(row interface{ Scan(...any) error }) (Settings, error) {
	var s Settings
	err := row.Scan(&s.DeliveryFeeKobo, &s.OnlineEnabled, &s.BankTransferEnabled, &s.PayOnPickupEnabled,
		&s.PayOnDeliveryEnabled, &s.BankName, &s.BankAccountNumber, &s.BankAccountName,
		&s.WhatsAppNumber, &s.WhatsAppMessage, &s.NotificationEmail, &s.UpdatedAt)
	return s, err
}

func (Repository) Update(ctx context.Context, db database.DBTX, in UpdateInput, now time.Time) (Settings, error) {
	return scan(db.QueryRow(ctx,
		`UPDATE store_settings SET
		   delivery_fee_kobo = COALESCE($1, delivery_fee_kobo),
		   online_enabled = COALESCE($2, online_enabled),
		   bank_transfer_enabled = COALESCE($3, bank_transfer_enabled),
		   pay_on_pickup_enabled = COALESCE($4, pay_on_pickup_enabled),
		   pay_on_delivery_enabled = COALESCE($5, pay_on_delivery_enabled),
		   bank_name = COALESCE($6, bank_name),
		   bank_account_number = COALESCE($7, bank_account_number),
		   bank_account_name = COALESCE($8, bank_account_name),
		   whatsapp_number = COALESCE($9, whatsapp_number),
		   whatsapp_message = COALESCE($10, whatsapp_message),
		   notification_email = COALESCE($11, notification_email),
		   updated_at = $12
		 RETURNING `+columns,
		in.DeliveryFeeKobo, in.OnlineEnabled, in.BankTransferEnabled, in.PayOnPickupEnabled,
		in.PayOnDeliveryEnabled, in.BankName, in.BankAccountNumber, in.BankAccountName,
		in.WhatsAppNumber, in.WhatsAppMessage, in.NotificationEmail, now))
}

type Handler struct {
	pool *pgxpool.Pool
	repo *Repository
	log  *slog.Logger
	now  func() time.Time
}

func NewHandler(pool *pgxpool.Pool, log *slog.Logger) *Handler {
	return &Handler{pool: pool, repo: NewRepository(), log: log, now: time.Now}
}

func (h *Handler) Mount(mux *http.ServeMux, access auth.Access) {
	mux.Handle("GET /api/v1/admin/settings", access.Staff(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /api/v1/admin/settings", access.Admin(http.HandlerFunc(h.update)))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	s, err := h.repo.Get(r.Context(), h.pool)
	h.respond(w, r, map[string]any{"settings": s}, err)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in UpdateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	if err := in.normalize(); err != nil {
		apierror.Write(w, err)
		return
	}
	s, err := h.repo.Update(r.Context(), h.pool, in, h.now())
	h.respond(w, r, map[string]any{"settings": s}, err)
}

func validation(msg string) *apierror.Error {
	return apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", msg)
}

// normalize trims and checks every field that was sent.
func (in *UpdateInput) normalize() *apierror.Error {
	if in.DeliveryFeeKobo == nil && in.OnlineEnabled == nil && in.BankTransferEnabled == nil &&
		in.PayOnPickupEnabled == nil && in.PayOnDeliveryEnabled == nil && in.BankName == nil &&
		in.BankAccountNumber == nil && in.BankAccountName == nil && in.WhatsAppNumber == nil &&
		in.WhatsAppMessage == nil && in.NotificationEmail == nil {
		return validation("No fields to update")
	}
	if in.DeliveryFeeKobo != nil && (*in.DeliveryFeeKobo < 0 || *in.DeliveryFeeKobo > maxDeliveryFeeKobo) {
		return validation("delivery_fee_kobo must be between 0 and 100000000")
	}

	for name, field := range map[string]*string{
		"bank_name": in.BankName, "bank_account_name": in.BankAccountName,
	} {
		if field == nil {
			continue
		}
		*field = strings.TrimSpace(*field)
		if utf8.RuneCountInString(*field) > 100 {
			return validation(name + " must be at most 100 characters")
		}
	}

	if in.BankAccountNumber != nil {
		digits, ok := digitsOnly(*in.BankAccountNumber, "")
		if !ok || (digits != "" && (len(digits) < 6 || len(digits) > 20)) {
			return validation("bank_account_number must be 6 to 20 digits")
		}
		in.BankAccountNumber = &digits
	}
	if in.WhatsAppNumber != nil {
		digits, ok := digitsOnly(*in.WhatsAppNumber, "+")
		if !ok || (digits != "" && (len(digits) < 8 || len(digits) > 15)) {
			return validation("whatsapp_number must be an international number with 8 to 15 digits")
		}
		in.WhatsAppNumber = &digits
	}
	if in.WhatsAppMessage != nil {
		*in.WhatsAppMessage = strings.TrimSpace(*in.WhatsAppMessage)
		if utf8.RuneCountInString(*in.WhatsAppMessage) > 300 {
			return validation("whatsapp_message must be at most 300 characters")
		}
	}
	if in.NotificationEmail != nil {
		addr := strings.TrimSpace(*in.NotificationEmail)
		if addr != "" {
			parsed, err := mail.ParseAddress(addr)
			if err != nil || parsed.Address != addr || len(addr) > 254 {
				return validation("notification_email must be a valid email address")
			}
		}
		in.NotificationEmail = &addr
	}
	return nil
}

// digitsOnly keeps the digits of a number, ignoring spaces, dashes and
// brackets, and a leading character from allowedPrefix. Anything else is invalid.
func digitsOnly(raw, allowedPrefix string) (string, bool) {
	var b strings.Builder
	for i, c := range strings.TrimSpace(raw) {
		switch {
		case c >= '0' && c <= '9':
			b.WriteRune(c)
		case i == 0 && strings.ContainsRune(allowedPrefix, c):
		case c == ' ' || c == '-' || c == '(' || c == ')':
		default:
			return "", false
		}
	}
	return b.String(), true
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, body any, err error) {
	if err == nil {
		apierror.JSON(w, http.StatusOK, body)
		return
	}
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		apierror.Write(w, apiErr)
		return
	}
	h.log.Error("request failed", "request_id", middleware.RequestID(r.Context()), "error", err)
	apierror.Write(w, apierror.ErrInternal)
}
