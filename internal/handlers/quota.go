package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/puppe1990/libgenbooks/internal/zlibrary"
)

// QuotaHandler answers the sidebar's Z-Library quota poll.
type QuotaHandler struct {
	client *zlibrary.Client
}

// NewQuotaHandler builds the handler; a nil client means the feature is off.
func NewQuotaHandler(client *zlibrary.Client) *QuotaHandler {
	return &QuotaHandler{client: client}
}

func (h *QuotaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	response := map[string]any{"enabled": false}
	if h.client != nil {
		quota, err := h.client.Quota(r.Context())
		if err != nil {
			response = map[string]any{"enabled": true, "unavailable": true}
		} else {
			response = map[string]any{
				"enabled": true,
				"used":    quota.Used,
				"limit":   quota.Limit,
				"resetAt": nextQuotaReset(time.Now()).Format(time.RFC3339),
			}
		}
	}
	_ = json.NewEncoder(w).Encode(response)
}

// quotaResetHourUTC is when the daily allowance renews: Z-Library counts days
// on a UTC+3 boundary (00:00 Moscow = 21:00 UTC). Their own site tooltip
// ("Will reset in …") matches this; a reader in BRT sees the reset at 18:00.
const quotaResetHourUTC = 21

// nextQuotaReset is the next reset instant after now.
func nextQuotaReset(now time.Time) time.Time {
	utc := now.UTC()
	boundary := time.Date(utc.Year(), utc.Month(), utc.Day(), quotaResetHourUTC, 0, 0, 0, time.UTC)
	if !boundary.After(utc) {
		boundary = boundary.AddDate(0, 0, 1)
	}
	return boundary
}
