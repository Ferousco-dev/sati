package email

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
)

const Brand = "SATI Yoghurt"

const (
	KindVerifyEmail         = "verify_email"
	KindPasswordReset       = "password_reset"
	KindInvite              = "invite"
	KindOrderPlaced         = "order_placed"
	KindPaymentReceived     = "payment_received"
	KindOrderConfirmed      = "order_confirmed"
	KindOrderReadyPickup    = "order_ready_pickup"
	KindOrderOutForDelivery = "order_out_for_delivery"
	KindOrderDelivered      = "order_delivered"
	KindOrderCancelled      = "order_cancelled"
	KindOrderFailed         = "order_failed"
	KindStoreNewOrder       = "store_new_order"
)

type Item struct {
	Name          string `json:"name"`
	Quantity      int    `json:"quantity"`
	LineTotalKobo int64  `json:"line_total_kobo"`
}

type Bank struct {
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	WhatsAppURL   string `json:"whatsapp_url,omitempty"`
}

type Order struct {
	ID              string `json:"id"`
	Number          string `json:"number"`
	CustomerName    string `json:"customer_name"`
	CustomerEmail   string `json:"customer_email"`
	Fulfillment     string `json:"fulfillment"`
	PaymentMethod   string `json:"payment_method"`
	SubtotalKobo    int64  `json:"subtotal_kobo"`
	DeliveryFeeKobo int64  `json:"delivery_fee_kobo"`
	TotalKobo       int64  `json:"total_kobo"`
	Items           []Item `json:"items,omitempty"`
	Bank            *Bank  `json:"bank,omitempty"`
}

// Payload is everything needed to write one email, stored with the queued job.
type Payload struct {
	Name      string `json:"name,omitempty"`
	Role      string `json:"role,omitempty"`
	Link      string `json:"link,omitempty"`
	Order     *Order `json:"order,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Confirmed bool   `json:"confirmed,omitempty"`
}

// Naira formats kobo as naira with thousands separators, for example ₦7,500.00.
func Naira(kobo int64) string {
	naira := fmt.Sprintf("%d", kobo/100)
	var groups []string
	for len(naira) > 3 {
		groups = append([]string{naira[len(naira)-3:]}, groups...)
		naira = naira[:len(naira)-3]
	}
	groups = append([]string{naira}, groups...)
	return fmt.Sprintf("₦%s.%02d", strings.Join(groups, ","), kobo%100)
}

type view struct {
	Brand     string
	Subject   string
	Heading   string
	Name      string
	Lines     []string
	Link      string
	LinkLabel string
	Order     *Order
	Bank      *Bank
	Footer    string
}

func (v view) Money(kobo int64) string { return Naira(kobo) }

var htmlLayout = htmltemplate.Must(htmltemplate.New("html").Funcs(htmltemplate.FuncMap{
	"naira": Naira,
}).Parse(`<!doctype html>
<html><body style="margin:0;padding:24px;background:#f6f6f4;font-family:Arial,Helvetica,sans-serif;color:#1f2937">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border-radius:8px;padding:32px">
<tr><td>
<p style="margin:0 0 24px;font-size:14px;font-weight:bold;letter-spacing:1px;color:#6b7280">{{.Brand}}</p>
<h1 style="margin:0 0 16px;font-size:22px;color:#111827">{{.Heading}}</h1>
{{if .Name}}<p style="margin:0 0 12px;font-size:15px;line-height:22px">Hi {{.Name}},</p>{{end}}
{{range .Lines}}<p style="margin:0 0 12px;font-size:15px;line-height:22px">{{.}}</p>{{end}}
{{with .Bank}}<table role="presentation" cellpadding="6" cellspacing="0" style="margin:8px 0 16px;background:#f9fafb;border-radius:6px;font-size:15px">
<tr><td>Bank</td><td><strong>{{.BankName}}</strong></td></tr>
<tr><td>Account number</td><td><strong>{{.AccountNumber}}</strong></td></tr>
<tr><td>Account name</td><td><strong>{{.AccountName}}</strong></td></tr>
</table>{{end}}
{{with .Order}}{{if .Items}}<table role="presentation" width="100%" cellpadding="6" cellspacing="0" style="margin:8px 0 16px;font-size:14px;border-top:1px solid #e5e7eb">
{{range .Items}}<tr><td>{{.Quantity}} &times; {{.Name}}</td><td align="right">{{naira .LineTotalKobo}}</td></tr>{{end}}
<tr><td style="border-top:1px solid #e5e7eb">Subtotal</td><td align="right" style="border-top:1px solid #e5e7eb">{{naira .SubtotalKobo}}</td></tr>
{{if .DeliveryFeeKobo}}<tr><td>Delivery</td><td align="right">{{naira .DeliveryFeeKobo}}</td></tr>{{end}}
<tr><td><strong>Total</strong></td><td align="right"><strong>{{naira .TotalKobo}}</strong></td></tr>
</table>{{end}}{{end}}
{{if .Link}}<p style="margin:20px 0"><a href="{{.Link}}" style="display:inline-block;padding:12px 20px;background:#111827;color:#ffffff;text-decoration:none;border-radius:6px;font-size:15px">{{.LinkLabel}}</a></p>
<p style="margin:0 0 12px;font-size:12px;color:#6b7280;word-break:break-all">{{.Link}}</p>{{end}}
{{if .Footer}}<p style="margin:16px 0 0;font-size:12px;color:#6b7280">{{.Footer}}</p>{{end}}
</td></tr></table></td></tr></table></body></html>`))

var textLayout = texttemplate.Must(texttemplate.New("text").Funcs(texttemplate.FuncMap{
	"naira": Naira,
}).Parse(`{{.Brand}}

{{.Heading}}

{{if .Name}}Hi {{.Name}},

{{end}}{{range .Lines}}{{.}}

{{end}}{{with .Bank}}Bank: {{.BankName}}
Account number: {{.AccountNumber}}
Account name: {{.AccountName}}

{{end}}{{with .Order}}{{range .Items}}{{.Quantity}} x {{.Name}}  {{naira .LineTotalKobo}}
{{end}}{{if .Items}}Subtotal: {{naira .SubtotalKobo}}
{{if .DeliveryFeeKobo}}Delivery: {{naira .DeliveryFeeKobo}}
{{end}}Total: {{naira .TotalKobo}}

{{end}}{{end}}{{if .Link}}{{.LinkLabel}}: {{.Link}}

{{end}}{{if .Footer}}{{.Footer}}
{{end}}`))

// Render writes the subject, HTML and plain text for a queued email.
func Render(kind string, p Payload) (subject, html, text string, err error) {
	v, err := build(kind, p)
	if err != nil {
		return "", "", "", err
	}
	v.Brand, v.Name = Brand, p.Name

	var h, t bytes.Buffer
	if err := htmlLayout.Execute(&h, v); err != nil {
		return "", "", "", err
	}
	if err := textLayout.Execute(&t, v); err != nil {
		return "", "", "", err
	}
	return v.Subject, h.String(), strings.TrimSpace(t.String()) + "\n", nil
}

func orderNumber(p Payload) string {
	if p.Order == nil {
		return ""
	}
	return p.Order.Number
}

func build(kind string, p Payload) (view, error) {
	n := orderNumber(p)
	switch kind {
	case KindVerifyEmail:
		return view{Subject: "Verify your email", Heading: "Verify your email",
			Lines: []string{"Confirm your email address to keep your account secure. This link works once and expires in 15 minutes."},
			Link:  p.Link, LinkLabel: "Verify email", Footer: "If you did not create an account, you can ignore this email."}, nil
	case KindPasswordReset:
		return view{Subject: "Reset your password", Heading: "Reset your password",
			Lines: []string{"We received a request to reset your password. This link works once and expires in 1 hour."},
			Link:  p.Link, LinkLabel: "Choose a new password", Footer: "If you did not ask for this, you can ignore this email and your password stays the same."}, nil
	case KindInvite:
		return view{Subject: "You have been invited to " + Brand, Heading: "You have been invited",
			Lines: []string{fmt.Sprintf("You have been added to %s as %s. Set a password to get started. This link works once and expires in 72 hours.", Brand, strings.ToLower(p.Role))},
			Link:  p.Link, LinkLabel: "Set your password"}, nil
	case KindOrderPlaced:
		return orderPlaced(p), nil
	case KindPaymentReceived:
		lines := []string{fmt.Sprintf("We received your payment of %s for order %s.", Naira(p.Order.TotalKobo), n)}
		heading := "Payment received"
		if p.Confirmed {
			lines = append(lines, "Your order is confirmed and the store will start preparing it.")
			heading = "Payment received, order confirmed"
		}
		return view{Subject: fmt.Sprintf("Payment received for order %s", n), Heading: heading, Lines: lines,
			Link: p.Link, LinkLabel: "View order"}, nil
	case KindOrderConfirmed:
		return view{Subject: fmt.Sprintf("Order %s is confirmed", n), Heading: "Order confirmed",
			Lines: []string{fmt.Sprintf("The store has accepted order %s and will start preparing it.", n)},
			Link:  p.Link, LinkLabel: "View order"}, nil
	case KindOrderReadyPickup:
		return view{Subject: fmt.Sprintf("Order %s is ready for pickup", n), Heading: "Ready for pickup",
			Lines: []string{fmt.Sprintf("Order %s is ready. Come and collect it whenever you are ready.", n)},
			Link:  p.Link, LinkLabel: "View order"}, nil
	case KindOrderOutForDelivery:
		return view{Subject: fmt.Sprintf("Order %s is on its way", n), Heading: "Out for delivery",
			Lines: []string{fmt.Sprintf("Order %s is on its way to you.", n)},
			Link:  p.Link, LinkLabel: "View order"}, nil
	case KindOrderDelivered:
		word := "delivered"
		if p.Order.Fulfillment == "PICKUP" {
			word = "completed"
		}
		return view{Subject: fmt.Sprintf("Order %s %s", n, word), Heading: "Order " + word,
			Lines: []string{fmt.Sprintf("Order %s is %s. Thank you for choosing %s.", n, word, Brand)},
			Link:  p.Link, LinkLabel: "View order"}, nil
	case KindOrderCancelled, KindOrderFailed:
		heading, line := "Order cancelled", fmt.Sprintf("Order %s was cancelled.", n)
		if kind == KindOrderFailed {
			heading, line = "Order could not be completed", fmt.Sprintf("We could not complete order %s.", n)
		}
		lines := []string{line}
		if p.Reason != "" {
			lines = append(lines, "Reason: "+p.Reason)
		}
		return view{Subject: fmt.Sprintf("%s: %s", heading, n), Heading: heading, Lines: lines,
			Link: p.Link, LinkLabel: "View order", Footer: "If you have questions, please contact the store."}, nil
	case KindStoreNewOrder:
		o := p.Order
		return view{Subject: fmt.Sprintf("New order %s (%s)", n, Naira(o.TotalKobo)), Heading: "New order " + n,
			Lines: []string{
				fmt.Sprintf("%s placed an order for %s.", o.CustomerName, Naira(o.TotalKobo)),
				fmt.Sprintf("%s, paying by %s.", strings.ToLower(o.Fulfillment), strings.ToLower(strings.ReplaceAll(o.PaymentMethod, "_", " "))),
			},
			Order: o, Link: p.Link, LinkLabel: "Open order"}, nil
	}
	return view{}, fmt.Errorf("unknown email kind %q", kind)
}

func orderPlaced(p Payload) view {
	o := p.Order
	v := view{Subject: fmt.Sprintf("We received your order %s", o.Number), Heading: "Thanks for your order",
		Lines: []string{fmt.Sprintf("We received order %s.", o.Number)}, Order: o, Link: p.Link, LinkLabel: "View order"}

	switch o.PaymentMethod {
	case "BANK_TRANSFER":
		v.Lines = append(v.Lines, fmt.Sprintf("Please transfer %s to the account below and use %s as the reference. We confirm your order once we see the payment.", Naira(o.TotalKobo), o.Number))
		if o.Bank != nil {
			v.Bank = o.Bank
			if o.Bank.WhatsAppURL != "" {
				v.Lines = append(v.Lines, "You can send your proof of payment on WhatsApp: "+o.Bank.WhatsAppURL)
			}
		}
	case "ONLINE":
		v.Lines = append(v.Lines, "We confirm your order as soon as your payment goes through.")
	case "PAY_ON_PICKUP":
		v.Lines = append(v.Lines, fmt.Sprintf("You pay %s when you collect your order.", Naira(o.TotalKobo)))
	case "PAY_ON_DELIVERY":
		v.Lines = append(v.Lines, fmt.Sprintf("You pay %s to the rider when your order arrives.", Naira(o.TotalKobo)))
	}
	return v
}
