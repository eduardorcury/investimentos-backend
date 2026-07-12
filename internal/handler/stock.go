package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/duducury/investimentos-backend/internal/integrations"
	"github.com/duducury/investimentos-backend/internal/model"
)

var tickerRe = regexp.MustCompile(`^(?:[A-Z]{4}|[A-Z][0-9][A-Z]{2})[0-9]{1,2}B?$`)

func StockHistory(w http.ResponseWriter, r *http.Request) {
	ticker := r.PathValue("ticker")
	slog.Info("stock history requested", "ticker", ticker)

	if !tickerRe.MatchString(ticker) {
		slog.Warn("invalid ticker format", "ticker", ticker)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ticker format"})
		return
	}

	prices, err := integrations.FetchYearHistory(ticker)
	switch {
	case errors.Is(err, integrations.ErrNotFound):
		slog.Warn("ticker not found", "ticker", ticker)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ticker not found"})
	case errors.Is(err, integrations.ErrRateLimited):
		slog.Warn("upstream rate limited", "ticker", ticker)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "upstream rate limited, retry later"})
	case err != nil:
		slog.Error("upstream fetch failed", "ticker", ticker, "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream fetch failed"})
	default:
		slog.Info("stock history fetched", "ticker", ticker, "points", len(prices))
		writeJSON(w, http.StatusOK, model.StockHistory{
			Ticker: ticker,
			Year:   time.Now().Year(),
			Prices: prices,
		})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
