package handler

import (
	"log/slog"
	"net/http"

	"github.com/duducury/investimentos-backend/internal/parser"
)

func (h *TransactionsHandler) UploadNota(w http.ResponseWriter, r *http.Request) {
	slog.Info("nota upload requested")

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		slog.Warn("failed to parse multipart form", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		slog.Warn("missing file field in form")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'file' field"})
		return
	}
	defer file.Close()
	slog.Info("parsing nota PDF", "filename", header.Filename)

	transactions, err := parser.ParseNota(file)
	if err != nil {
		slog.Error("failed to parse nota", "filename", header.Filename, "err", err)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	slog.Info("nota parsed", "filename", header.Filename, "transactions", len(transactions))

	for i := range transactions {
		transactions[i].FillID()
	}

	for _, tx := range transactions {
		if err := h.repository.SaveTransaction(r.Context(), tx); err != nil {
			slog.Error("failed to save transaction", "ticker", tx.Ticker, "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save transaction"})
			return
		}
		slog.Info("transaction saved", "ticker", tx.Ticker, "id", tx.ID, "nota", tx.NotaNumber)
	}

	slog.Info("nota upload complete", "filename", header.Filename, "count", len(transactions))
	writeJSON(w, http.StatusOK, transactions)
}
