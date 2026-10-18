package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// NewClient constructs the client with the dependencies and limits required for distributed
// presence and ownership.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - addr: Operator-controlled network address.
//   - password: Plaintext password accepted only for validation or hashing.
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded client.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewClient(ctx context.Context, addr string, password string, db int) (*goredis.Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:                  addr,
		Password:              password,
		DB:                    db,
		DialTimeout:           3 * time.Second,
		ReadTimeout:           3 * time.Second,
		WriteTimeout:          3 * time.Second,
		PoolTimeout:           3 * time.Second,
		MaxRetries:            2,
		ContextTimeoutEnabled: true,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	return client, nil
}
