package orders

import "testing"

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from, to Status
		f        Fulfillment
		want     bool
	}{
		{StatusPending, StatusConfirmed, Pickup, true},
		{StatusPending, StatusCancelled, Delivery, true},
		{StatusPending, StatusPreparing, Pickup, false},
		{StatusConfirmed, StatusPreparing, Pickup, true},
		{StatusPreparing, StatusReady, Delivery, true},
		{StatusReady, StatusCompleted, Pickup, true},
		{StatusReady, StatusCompleted, Delivery, false},
		{StatusReady, StatusOutForDelivery, Delivery, true},
		{StatusReady, StatusOutForDelivery, Pickup, false},
		{StatusOutForDelivery, StatusDelivered, Delivery, true},
		{StatusOutForDelivery, StatusFailed, Delivery, true},
		{StatusDelivered, StatusCancelled, Delivery, false},
		{StatusCompleted, StatusPending, Pickup, false},
		{StatusCancelled, StatusConfirmed, Pickup, false},
	}
	for _, tt := range tests {
		if got := CanTransition(tt.from, tt.to, tt.f); got != tt.want {
			t.Errorf("%s -> %s (%s): got %v, want %v", tt.from, tt.to, tt.f, got, tt.want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	if got := FormatNumber(10042); got != "SATI-10042" {
		t.Fatal(got)
	}
}

func TestValidateCreate(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000001"
	valid := func() CreateInput {
		return CreateInput{
			Items:         []LineInput{{ProductID: id, Quantity: 1}},
			Fulfillment:   FulfillmentInput{Type: Pickup},
			PaymentMethod: PayOnline,
		}
	}

	if _, err := validateCreate(ptr(valid())); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}

	merged := valid()
	merged.Items = []LineInput{{id, 2}, {id, 3}}
	lines, err := validateCreate(&merged)
	if err != nil || len(lines) != 1 || lines[0].Quantity != 5 {
		t.Fatalf("lines should merge: %v %v", lines, err)
	}

	bad := map[string]func(*CreateInput){
		"no items":              func(c *CreateInput) { c.Items = nil },
		"zero quantity":         func(c *CreateInput) { c.Items[0].Quantity = 0 },
		"huge quantity":         func(c *CreateInput) { c.Items[0].Quantity = 51 },
		"merged quantity":       func(c *CreateInput) { c.Items = []LineInput{{id, 30}, {id, 30}} },
		"bad product id":        func(c *CreateInput) { c.Items[0].ProductID = "abc" },
		"unknown fulfillment":   func(c *CreateInput) { c.Fulfillment.Type = "TELEPORT" },
		"delivery no address":   func(c *CreateInput) { c.Fulfillment.Type = Delivery },
		"pickup with address":   func(c *CreateInput) { c.Fulfillment.AddressID = id },
		"unknown payment":       func(c *CreateInput) { c.PaymentMethod = "BITCOIN" },
		"pay on pickup deliver": func(c *CreateInput) { c.Fulfillment = FulfillmentInput{Delivery, id}; c.PaymentMethod = PayOnPickup },
	}
	for name, mutate := range bad {
		in := valid()
		mutate(&in)
		if _, err := validateCreate(&in); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func ptr[T any](v T) *T { return &v }
