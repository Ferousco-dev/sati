package addresses

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

type Service struct {
	pool *pgxpool.Pool
	repo *Repository
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, repo: NewRepository(), now: time.Now}
}

func (s *Service) List(ctx context.Context, userID string) ([]Address, error) {
	return s.repo.List(ctx, s.pool, userID)
}

func (s *Service) Create(ctx context.Context, userID string, in CreateInput) (Address, error) {
	in.RecipientName = strings.TrimSpace(in.RecipientName)
	in.AddressLine = strings.TrimSpace(in.AddressLine)
	in.Area = strings.TrimSpace(in.Area)
	in.City = strings.TrimSpace(in.City)
	in.Instructions = strings.TrimSpace(in.Instructions)

	phone, err := validateAll(in.RecipientName, in.Phone, in.AddressLine, in.Area, in.City, in.Instructions)
	if err != nil {
		return Address{}, err
	}
	in.Phone = phone

	if in.ZoneID != "" {
		if err := s.checkZone(ctx, in.ZoneID); err != nil {
			return Address{}, err
		}
	}

	n, err := s.repo.Count(ctx, s.pool, userID)
	if err != nil {
		return Address{}, err
	}
	if n >= MaxPerUser {
		return Address{}, ErrLimit
	}
	return s.repo.Create(ctx, s.pool, userID, in)
}

func (s *Service) Update(ctx context.Context, userID, id string, in UpdateInput) (Address, error) {
	if !httpx.ValidUUID(id) {
		return Address{}, ErrNotFound
	}
	if in.RecipientName == nil && in.Phone == nil && in.AddressLine == nil && in.Area == nil && in.City == nil && in.Instructions == nil && in.ZoneID == nil {
		return Address{}, validation("No fields to update")
	}

	for _, field := range []*string{in.RecipientName, in.AddressLine, in.Area, in.City, in.Instructions} {
		if field != nil {
			*field = strings.TrimSpace(*field)
		}
	}
	if in.Phone != nil {
		phone, ok := cleanPhone(*in.Phone)
		if !ok {
			return Address{}, validation("Phone number is not valid")
		}
		in.Phone = &phone
	}
	checks := []struct {
		name  string
		value *string
		min   int
		max   int
	}{
		{"recipient_name", in.RecipientName, 1, 100},
		{"address_line", in.AddressLine, 1, 200},
		{"area", in.Area, 1, 100},
		{"city", in.City, 1, 100},
		{"instructions", in.Instructions, 0, 500},
	}
	for _, c := range checks {
		if c.value == nil {
			continue
		}
		if err := checkText(c.name, *c.value, c.min, c.max); err != nil {
			return Address{}, err
		}
	}
	if in.ZoneID != nil && *in.ZoneID != "" {
		if err := s.checkZone(ctx, *in.ZoneID); err != nil {
			return Address{}, err
		}
	}
	return s.repo.Update(ctx, s.pool, userID, id, in, s.now())
}

func (s *Service) Delete(ctx context.Context, userID, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, s.pool, userID, id)
}

func validateAll(name, phone, line, area, city, instructions string) (string, error) {
	cleaned, ok := cleanPhone(phone)
	if !ok {
		return "", validation("Phone number is not valid")
	}
	for _, c := range []struct {
		name  string
		value string
		min   int
		max   int
	}{
		{"recipient_name", name, 1, 100},
		{"address_line", line, 1, 200},
		{"area", area, 1, 100},
		{"city", city, 1, 100},
		{"instructions", instructions, 0, 500},
	} {
		if err := checkText(c.name, c.value, c.min, c.max); err != nil {
			return "", err
		}
	}
	return cleaned, nil
}

func (s *Service) checkZone(ctx context.Context, zoneID string) error {
	active, err := s.repo.ZoneActive(ctx, s.pool, zoneID)
	if err != nil {
		return err
	}
	if !active {
		return ErrZone
	}
	return nil
}
