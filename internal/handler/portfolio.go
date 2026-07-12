package handler

import (
	"log/slog"
	"math"
	"net/http"
)

type position struct {
	Ticker    string  `json:"ticker"`
	Quantity  int     `json:"quantity"`
	AvgPrice  float64 `json:"avgPrice"`
	TotalCost float64 `json:"totalCost"`
	AssetType string  `json:"assetType"`
}

func (h *TransactionsHandler) GetPortfolio(w http.ResponseWriter, r *http.Request) {
	slog.Info("portfolio requested")

	transactions, err := h.repository.GetAllTransactions(r.Context())
	if err != nil {
		slog.Error("failed to fetch transactions", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch transactions"})
		return
	}

	// Group by ticker: accumulate cost and quantity separately for buys and sells
	type bucket struct {
		qty       int
		cost      float64
		assetType string
	}
	byTicker := map[string]*bucket{}

	for _, tx := range transactions {
		b, ok := byTicker[tx.Ticker]
		if !ok {
			b = &bucket{}
			byTicker[tx.Ticker] = b
		}
		if tx.AssetType != "" {
			b.assetType = tx.AssetType
		}
		if tx.Type == "C" {
			b.qty += tx.Quantity
			b.cost += float64(tx.Quantity) * tx.Value
		} else {
			b.qty -= tx.Quantity
			b.cost -= float64(tx.Quantity) * tx.Value
		}
	}

	positions := make([]position, 0, len(byTicker))
	for ticker, b := range byTicker {
		if b.qty <= 0 {
			continue
		}
		avgPrice := b.cost / float64(b.qty)
		positions = append(positions, position{
			Ticker:    ticker,
			Quantity:  b.qty,
			AvgPrice:  math.Round(avgPrice*10000) / 10000,
			TotalCost: math.Round(b.cost*100) / 100,
			AssetType: b.assetType,
		})
	}

	slog.Info("portfolio calculated", "tickers", len(positions))
	writeJSON(w, http.StatusOK, positions)
}
