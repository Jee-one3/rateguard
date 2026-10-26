package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Store struct {
	db *dynamodb.Client
}

type Decision struct {
	Allowed   bool
	Remaining int32
	RetryMs   int64
}

func number(v int64) types.AttributeValue {
	return &types.AttributeValueMemberN{
		Value: strconv.FormatInt(v, 10),
	}
}

func getNumber(
	item map[string]types.AttributeValue,
	key string,
) (int64, error) {
	value, ok := item[key].(*types.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("missing numeric field: %s", key)
	}
	return strconv.ParseInt(value.Value, 10, 64)
}

func (s *Store) Check(
	ctx context.Context,
	userID, endpoint string,
) (Decision, error) {

	// Length prefix avoids ambiguous user/endpoint keys.
	key := fmt.Sprintf("%d:%s:%s", len(userID), userID, endpoint)

	for attempt := 0; attempt < 30; attempt++ {

		result, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String("RateLimits"),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: key},
			},
			ConsistentRead: aws.Bool(true),
		})
		if err != nil {
			return Decision{}, err
		}

		now := time.Now().UnixMilli()

		// First request: create a bucket containing
		// four tokens after consuming one.
		if len(result.Item) == 0 {
			_, err = s.db.PutItem(ctx, &dynamodb.PutItemInput{
				TableName: aws.String("RateLimits"),
				Item: map[string]types.AttributeValue{
					"PK":             &types.AttributeValueMemberS{Value: key},
					"tokens_milli":   number(capacity - tokenCost),
					"last_refill_ms": number(now),
					"version":        number(1),
				},
				ConditionExpression: aws.String(
					"attribute_not_exists(PK)",
				),
			})

			if err == nil {
				return Decision{Allowed: true, Remaining: 4}, nil
			}

			var conflict *types.ConditionalCheckFailedException
			if errors.As(err, &conflict) {
				continue
			}
			return Decision{}, err
		}

		tokens, err := getNumber(result.Item, "tokens_milli")
		if err != nil {
			return Decision{}, err
		}
		lastRefill, err := getNumber(result.Item, "last_refill_ms")
		if err != nil {
			return Decision{}, err
		}
		version, err := getNumber(result.Item, "version")
		if err != nil {
			return Decision{}, err
		}

		updated, allowed, retryMs := calculateTokens(
			tokens, lastRefill, now,
		)

		// Only update if nobody else has changed this version.
		_, err = s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName: aws.String("RateLimits"),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: key},
			},
			UpdateExpression: aws.String(
				"SET tokens_milli = :tokens, " +
					"last_refill_ms = :now, " +
					"#ver = :next",
			),
			ConditionExpression: aws.String(
				"attribute_exists(PK) AND #ver = :expected",
			),
			ExpressionAttributeNames: map[string]string{
				"#ver": "version",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":tokens":   number(updated),
				":now":      number(now),
				":next":     number(version + 1),
				":expected": number(version),
			},
		})

		if err == nil {
			return Decision{
				Allowed:   allowed,
				Remaining: int32(updated / tokenCost),
				RetryMs:   retryMs,
			}, nil
		}

		var conflict *types.ConditionalCheckFailedException
		if errors.As(err, &conflict) {
			continue
		}

		return Decision{}, err
	}

	return Decision{}, fmt.Errorf("too many concurrent update conflicts")
}
