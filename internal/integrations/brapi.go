package integrations

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/duducury/investimentos-backend/internal/model"
)

var (
	ErrNotFound    = errors.New("ticker not found")
	ErrRateLimited = errors.New("upstream rate limited")
)

type brapiResponse struct {
	Results []brapiResult `json:"results"`
	Error   bool          `json:"error"`
	Message string        `json:"message"`
}

type brapiResult struct {
	Symbol              string       `json:"symbol"`
	HistoricalDataPrice []brapiPrice `json:"historicalDataPrice"`
}

type brapiPrice struct {
	Date  int64   `json:"date"`
	Close float64 `json:"close"`
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func FetchYearHistory(ticker string) ([]model.PricePoint, error) {
	url := fmt.Sprintf("https://brapi.dev/api/quote/%s?range=ytd&interval=1d", ticker)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", ticker, err)
	}
	defer resp.Body.Close()

	var body brapiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusUnauthorized:
		// brapi returns 401 for tickers outside the free tier or non-existent tickers
		return nil, fmt.Errorf("%w: %s", ErrNotFound, body.Message)
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, ticker)
	}

	if body.Error {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, body.Message)
	}
	if len(body.Results) == 0 || len(body.Results[0].HistoricalDataPrice) == 0 {
		return nil, ErrNotFound
	}

	brt := time.FixedZone("BRT", -3*60*60)
	raw := body.Results[0].HistoricalDataPrice
	prices := make([]model.PricePoint, 0, len(raw))
	for _, p := range raw {
		dt := time.Unix(p.Date, 0).In(brt)
		prices = append(prices, model.PricePoint{
			Date:  dt.Format("2006-01-02"),
			Close: math.Round(p.Close*100) / 100,
		})
	}
	return prices, nil
}
