package model

type PricePoint struct {
	Date  string  `json:"date"`
	Close float64 `json:"close"`
}

type StockHistory struct {
	Ticker string       `json:"ticker"`
	Year   int          `json:"year"`
	Prices []PricePoint `json:"prices"`
}
