package dynamo

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// FillID sets ID to a deterministic 16-char hex hash of the transaction fields.
// Uploading the same nota twice produces the same ID, preventing duplicates.
func (t *Transaction) FillID() {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d|%.4f",
		t.Ticker, t.Date.Format("2006-01-02"), t.Type, t.Quantity, t.Value)))
	t.ID = fmt.Sprintf("%x", h)[:16]
}

type Transaction struct {
	ID         string    `dynamodbav:"id"          json:"id"`
	Ticker     string    `dynamodbav:"ticker"      json:"ticker"`
	Date       time.Time `dynamodbav:"date"        json:"date"`
	Quantity   int       `dynamodbav:"quantity"    json:"quantity"`
	Value      float64   `dynamodbav:"value"       json:"value"`
	Type       string    `dynamodbav:"type"        json:"type"`
	NotaNumber string    `dynamodbav:"notaNumber"  json:"notaNumber"`
}
