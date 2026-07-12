package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/duducury/investimentos-backend/internal/dynamo"
)

type TransactionsHandler struct {
	repository *dynamo.Repository
}

func NewTransactionsHandler(repository *dynamo.Repository) *TransactionsHandler {
	return &TransactionsHandler{repository: repository}
}

func (h *TransactionsHandler) AddTransactions(w http.ResponseWriter, r *http.Request) {
	slog.Info("add transactions requested")

	var transactions []dynamo.Transaction
	if err := json.NewDecoder(r.Body).Decode(&transactions); err != nil {
		slog.Error("failed to decode request body", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	slog.Info("transactions decoded", "count", len(transactions))

	for i := range transactions {
		transactions[i].FillID()
	}

	for _, transaction := range transactions {
		if err := h.repository.SaveTransaction(r.Context(), transaction); err != nil {
			slog.Error("failed to save transaction", "ticker", transaction.Ticker, "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save transaction"})
			return
		}
		slog.Info("transaction saved", "ticker", transaction.Ticker, "id", transaction.ID)
	}

	slog.Info("all transactions saved", "count", len(transactions))
	writeJSON(w, http.StatusOK, transactions)
}
