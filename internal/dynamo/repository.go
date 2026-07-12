package dynamo

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type Repository struct {
	Client    *dynamodb.Client
	TableName string
}

func NewRepository(client *dynamodb.Client, tableName string) *Repository {
	return &Repository{client, tableName}
}

func (r *Repository) SaveTransaction(ctx context.Context, transaction Transaction) error {
	item, err := attributevalue.MarshalMap(transaction)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to marshal transaction: %v", err))
		return err
	}

	_, err = r.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &r.TableName,
		Item:      item,
	})
	if err != nil {
		slog.Error(fmt.Sprintf("failed to save transaction: %v", err))
		return err
	}
	return nil
}

func (r *Repository) GetAllTransactions(ctx context.Context) ([]Transaction, error) {
	scan, err := r.Client.Scan(ctx, &dynamodb.ScanInput{
		TableName: &r.TableName,
	})
	if err != nil {
		slog.Error(fmt.Sprintf("failed to scan table: %v", err))
		return nil, err
	}

	var transactions []Transaction
	err = attributevalue.UnmarshalListOfMaps(scan.Items, &transactions)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to unmarshal transactions: %v", err))
		return nil, err
	}

	return transactions, nil
}
