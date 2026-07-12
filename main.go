package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/duducury/investimentos-backend/internal/dynamo"
	"github.com/duducury/investimentos-backend/internal/handler"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stocks/{ticker}/history", handler.StockHistory)

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	client := dynamodb.NewFromConfig(cfg)

	repository := dynamo.NewRepository(client, "investimentos")
	transactionHandler := handler.NewTransactionsHandler(repository)

	mux.HandleFunc("GET /portfolio", transactionHandler.GetPortfolio)
	mux.HandleFunc("POST /transactions", transactionHandler.AddTransactions)
	mux.HandleFunc("POST /transactions/upload", transactionHandler.UploadNota)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("server listening on :8080")
	log.Fatal(srv.ListenAndServe())
}
