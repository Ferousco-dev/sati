package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
)

const maxBodyBytes = 1 << 20

func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) *apierror.Error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apierror.New(http.StatusBadRequest, "INVALID_JSON", "Request body is not valid JSON")
	}
	return nil
}
