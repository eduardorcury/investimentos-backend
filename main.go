package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/duducury/investimentos-backend/internal/auth"
	"github.com/duducury/investimentos-backend/internal/dynamo"
	"github.com/duducury/investimentos-backend/internal/handler"
	"github.com/duducury/investimentos-backend/internal/middleware"
)

func main() {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	client := dynamodb.NewFromConfig(cfg)

	repository := dynamo.NewRepository(client, "investimentos")
	transactionHandler := handler.NewTransactionsHandler(repository)

	authService, err := auth.NewService(context.TODO(), ssm.NewFromConfig(cfg))
	if err != nil {
		log.Fatalf("unable to init auth service, %v", err)
	}
	authHandler := handler.NewAuthHandler(authService)

	// Public route: exchanges a password for a JWT.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", authHandler.Login)

	// Protected routes: require a valid bearer token via the auth middleware.
	protected := http.NewServeMux()
	protected.HandleFunc("GET /stocks/{ticker}/history", handler.StockHistory)
	protected.HandleFunc("GET /portfolio", transactionHandler.GetPortfolio)
	protected.HandleFunc("POST /transactions", transactionHandler.AddTransactions)
	protected.HandleFunc("POST /transactions/upload", transactionHandler.UploadNota)
	mux.Handle("/", authService.Middleware(protected))

	// CORS wraps everything: the frontend (served from CloudFront) calls this
	// API cross-origin, so preflight OPTIONS must be answered before routing.
	allowedOrigin := os.Getenv("CORS_ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      middleware.CORS(allowedOrigin, mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("server listening on :8080")
	log.Fatal(srv.ListenAndServe())
}
