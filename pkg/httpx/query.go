package httpx

import (
	"net/http"
	"strconv"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

func Pagination(r *http.Request) (limit, offset int, err *apierror.Error) {
	limit, offset = defaultLimit, 0

	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil || n < 1 || n > maxLimit {
			return 0, 0, apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "limit must be between 1 and 100")
		}
		limit = n
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil || n < 0 {
			return 0, 0, apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "offset must be zero or greater")
		}
		offset = n
	}
	return limit, offset, nil
}

func ValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}
