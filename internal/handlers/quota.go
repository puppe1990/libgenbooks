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

// nextQuotaReset is when the daily allowance renews: Z-Library resets it at
// 00:00 UTC (their pages say the limit "will reset" then); the API exposes no
// timestamp, so the handler derives it.
func nextQuotaReset(now time.Time) time.Time {
	utc := now.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
}
