package datastore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flashbots/go-boost-utils/bls"
	"math/big"
	"strconv"
	"strings"
	"time"

	eth "github.com/ethereum/go-ethereum/common"
	"github.com/flashbots/go-utils/cli"
	"github.com/flashbots/mev-boost-relay/common"
	"github.com/go-redis/redis/v9"
)

var (
	redisPrefix = "boost-relay"

	expiryBidCache = 45 * time.Second

	RedisConfigFieldPubkey         = "pubkey"
	RedisStatsFieldLatestSlot      = "latest-slot"
	RedisStatsFieldValidatorsTotal = "validators-total"

	ErrFailedUpdatingTopBidNoBids            = errors.New("failed to update top bid because no bids were found")
	ErrAnotherPayloadAlreadyDeliveredForSlot = errors.New("another payload block hash for slot was already delivered")
	ErrPastSlotAlreadyDelivered              = errors.New("payload for past slot was already delivered")

	// Docs about redis settings: https://redis.io/docs/reference/clients/
	redisConnectionPoolSize = cli.GetEnvInt("REDIS_CONNECTION_POOL_SIZE", 0) // 0 means use default (10 per CPU)
	redisMinIdleConnections = cli.GetEnvInt("REDIS_MIN_IDLE_CONNECTIONS", 0) // 0 means use default
	redisReadTimeoutSec     = cli.GetEnvInt("REDIS_READ_TIMEOUT_SEC", 0)     // 0 means use default (3 sec)
	redisPoolTimeoutSec     = cli.GetEnvInt("REDIS_POOL_TIMEOUT_SEC", 0)     // 0 means use default (ReadTimeout + 1 sec)
	redisWriteTimeoutSec    = cli.GetEnvInt("REDIS_WRITE_TIMEOUT_SEC", 0)    // 0 means use default (3 seconds)
)

func PubkeyHexToLowerStr(pk common.PubkeyHex) string {
	return strings.ToLower(string(pk))
}

func connectRedis(redisURI string) (*redis.Client, error) {
	// Handle both URIs and full URLs, assume unencrypted connections
	if !strings.HasPrefix(redisURI, "redis://") && !strings.HasPrefix(redisURI, "rediss://") {
		redisURI = "redis://" + redisURI
	}

	redisOpts, err := redis.ParseURL(redisURI)
	if err != nil {
		return nil, err
	}

	if redisConnectionPoolSize > 0 {
		redisOpts.PoolSize = redisConnectionPoolSize
	}
	if redisMinIdleConnections > 0 {
		redisOpts.MinIdleConns = redisMinIdleConnections
	}
	if redisReadTimeoutSec > 0 {
		redisOpts.ReadTimeout = time.Duration(redisReadTimeoutSec) * time.Second
	}
	if redisPoolTimeoutSec > 0 {
		redisOpts.PoolTimeout = time.Duration(redisPoolTimeoutSec) * time.Second
	}
	if redisWriteTimeoutSec > 0 {
		redisOpts.WriteTimeout = time.Duration(redisWriteTimeoutSec) * time.Second
	}

	redisClient := redis.NewClient(redisOpts)
	if _, err := redisClient.Ping(context.Background()).Result(); err != nil {
		// unable to connect to redis
		return nil, err
	}
	return redisClient, nil
}

type RedisCache struct {
	client         *redis.Client
	readonlyClient *redis.Client

	// TOB tx prefixes
	prefixTopTobTxValue string
	prefixTobTobTx      string

	// prefixes (keys generated with a function)
	prefixGetHeaderResponse           string
	prefixExecAnchorPayload           string
	prefixExecPayloadCapella          string
	prefixBidTrace                    string
	prefixBlockBuilderLatestBids      string // latest bid for a given slot
	prefixBlockBuilderLatestBidsValue string // value of latest bid for a given slot
	prefixBlockBuilderLatestBidsTime  string // when the request was received, to avoid older requests overwriting newer ones after a slot validation
	prefixTopBidValue                 string
	prefixFloorBid                    string
	prefixFloorBidValue               string

	prefixHighestRob      string
	prefixHighestRobValue string

	// keys
	keyValidatorRegistrationTimestamp string

	keyRelayConfig        string
	keyStats              string
	keyProposerDuties     string
	keyBlockBuilderStatus string
	keyLastSlotDelivered  string
	keyLastHashDelivered  string
}

func NewRedisCache(prefix, redisURI, readonlyURI string) (*RedisCache, error) {
	client, err := connectRedis(redisURI)
	if err != nil {
		return nil, err
	}

	roClient := client
	if readonlyURI != "" {
		roClient, err = connectRedis(readonlyURI)
		if err != nil {
			return nil, err
		}
	}

	return &RedisCache{
		client:         client,
		readonlyClient: roClient,

		prefixTobTobTx:      fmt.Sprintf("%s/%s:cache-tobtobtx", redisPrefix, prefix),
		prefixTopTobTxValue: fmt.Sprintf("%s/%s:cache-toptobtx-value", redisPrefix, prefix),

		prefixGetHeaderResponse:  fmt.Sprintf("%s/%s:cache-gethead-response", redisPrefix, prefix),
		prefixExecAnchorPayload:  fmt.Sprintf("%s/%s:cache-execpayload-anchor", redisPrefix, prefix),
		prefixExecPayloadCapella: fmt.Sprintf("%s/%s:cache-execpayload-capella", redisPrefix, prefix),
		prefixBidTrace:           fmt.Sprintf("%s/%s:cache-bid-trace", redisPrefix, prefix),

		prefixBlockBuilderLatestBids:      fmt.Sprintf("%s/%s:block-builder-latest-bid", redisPrefix, prefix),       // hashmap for slot+parentHash+proposerPubkey with builderPubkey as field
		prefixBlockBuilderLatestBidsValue: fmt.Sprintf("%s/%s:block-builder-latest-bid-value", redisPrefix, prefix), // hashmap for slot+parentHash+proposerPubkey with builderPubkey as field
		prefixBlockBuilderLatestBidsTime:  fmt.Sprintf("%s/%s:block-builder-latest-bid-time", redisPrefix, prefix),  // hashmap for slot+parentHash+proposerPubkey with builderPubkey as field
		prefixTopBidValue:                 fmt.Sprintf("%s/%s:top-bid-value", redisPrefix, prefix),                  // prefix:slot_parentHash_proposerPubkey
		prefixFloorBid:                    fmt.Sprintf("%s/%s:bid-floor", redisPrefix, prefix),                      // prefix:slot_parentHash_proposerPubkey
		prefixFloorBidValue:               fmt.Sprintf("%s/%s:bid-floor-value", redisPrefix, prefix),                // prefix:slot_parentHash_proposerPubkey

		keyValidatorRegistrationTimestamp: fmt.Sprintf("%s/%s:validator-registration-timestamp", redisPrefix, prefix),
		keyRelayConfig:                    fmt.Sprintf("%s/%s:relay-config", redisPrefix, prefix),

		prefixHighestRob:      fmt.Sprintf("%s/%s:highest-rob", redisPrefix, prefix),
		prefixHighestRobValue: fmt.Sprintf("%s/%s:highest-rob-value", redisPrefix, prefix),

		keyStats:              fmt.Sprintf("%s/%s:stats", redisPrefix, prefix),
		keyProposerDuties:     fmt.Sprintf("%s/%s:proposer-duties", redisPrefix, prefix),
		keyBlockBuilderStatus: fmt.Sprintf("%s/%s:block-builder-status", redisPrefix, prefix),
		keyLastSlotDelivered:  fmt.Sprintf("%s/%s:last-slot-delivered", redisPrefix, prefix),
		keyLastHashDelivered:  fmt.Sprintf("%s/%s:last-hash-delivered", redisPrefix, prefix),
	}, nil
}

func (r *RedisCache) keyCacheGetTobTxs(slot uint64, parentHash string) string {
	return fmt.Sprintf("%s:%d-%s", r.prefixTobTobTx, slot, parentHash)
}

func (r *RedisCache) keyCacheGetTobTxsValue(slot uint64, parentHash string) string {
	return fmt.Sprintf("%s:%d-%s", r.prefixTopTobTxValue, slot, parentHash)
}

func (r *RedisCache) keyCacheGetHighestRob(slot uint64, parentHash string) string {
	return fmt.Sprintf("%s:%d-%s", r.prefixHighestRob, slot, parentHash)
}

func (r *RedisCache) keyCacheGetHighestRobValue(slot uint64, parentHash string) string {
	return fmt.Sprintf("%s:%d-%s", r.prefixHighestRobValue, slot, parentHash)
}

func (r *RedisCache) keyCacheGetToBHeaderResponse(slot uint64, parentHash, proposerPubkey string) string {
	return fmt.Sprintf("tob,%s:%d_%s_%s", r.prefixGetHeaderResponse, slot, parentHash, proposerPubkey)
}

func (r *RedisCache) keyCacheGetRoBHeaderResponse(slot uint64, parentHash, proposerPubkey string, chainID string) string {
	return fmt.Sprintf("rob,%s:%d_%s_%s_%s", r.prefixGetHeaderResponse, slot, parentHash, proposerPubkey, chainID)
}

func (r *RedisCache) keyExecToBAnchorPayload(slot uint64, proposerPubkey, blockHash string) string {
	return fmt.Sprintf("tob,%s:%d_%s_%s", r.prefixExecPayloadCapella, slot, proposerPubkey, blockHash)
}

func (r *RedisCache) keyExecRoBAnchorPayload(slot uint64, proposerPubkey, blockHash string, chainID string) string {
	return fmt.Sprintf("rob,%s:%d_%s_%s_%s", r.prefixExecPayloadCapella, slot, proposerPubkey, blockHash, chainID)
}

func (r *RedisCache) keyExecPayloadCapella(slot uint64, proposerPubkey, blockHash string) string {
	return fmt.Sprintf("%s:%d_%s_%s", r.prefixExecPayloadCapella, slot, proposerPubkey, blockHash)
}

func (r *RedisCache) keyCacheToBBidTrace(slot uint64, proposerPubkey, blockHash string) string {
	return fmt.Sprintf("tob,%s:%d_%s_%s", r.prefixBidTrace, slot, proposerPubkey, blockHash)
}

func (r *RedisCache) keyCacheRoBBidTrace(slot uint64, proposerPubkey, blockHash string, chainID string) string {
	return fmt.Sprintf("rob,%s:%d_%s_%s_%s", r.prefixBidTrace, slot, proposerPubkey, blockHash, chainID)
}

// keyLatestToBBidByBuilder returns the key for the getHeader response the latest tob bid by a specific builder
func (r *RedisCache) keyLatestToBBidByBuilder(slot uint64, parentHash, proposerPubkey, builderPubkey string) string {
	return fmt.Sprintf("tob,%s:%d_%s_%s/%s", r.prefixBlockBuilderLatestBids, slot, parentHash, proposerPubkey, builderPubkey)
}

func (r *RedisCache) keyLatestRoBBidByBuilder(slot uint64, parentHash, proposerPubkey, builderPubkey string, chainID string) string {
	return fmt.Sprintf("rob,%s:%d_%s_%s/%s_%s", r.prefixBlockBuilderLatestBids, slot, parentHash, proposerPubkey, builderPubkey, chainID)
}

// keyLatestBidByBuilder returns the key for the getHeader response the latest bid by a specific builder
func (r *RedisCache) keyLatestBidByBuilder(slot uint64, parentHash, proposerPubkey, builderPubkey string) string {
	return fmt.Sprintf("%s:%d_%s_%s/%s", r.prefixBlockBuilderLatestBids, slot, parentHash, proposerPubkey, builderPubkey)
}

// keyBlockBuilderLatestBidValue returns the hashmap key for the value of the latest bid by a specific builder
func (r *RedisCache) keyBlockBuilderLatestBidsValue(slot uint64, parentHash, proposerPubkey string) string {
	return fmt.Sprintf("%s:%d_%s_%s", r.prefixBlockBuilderLatestBidsValue, slot, parentHash, proposerPubkey)
}

// keyBlockBuilderLatestToBBidValue returns the hashmap key for the value of the latest bid by a specific builder
func (r *RedisCache) keyBlockBuilderLatestToBBidsValue(slot uint64, parentHash, proposerPubkey string) string {
	return fmt.Sprintf("tob,%s:%d_%s_%s", r.prefixBlockBuilderLatestBidsValue, slot, parentHash, proposerPubkey)
}

// keyBlockBuilderLatestRoBBidValue returns the hashmap key for the value of the latest bid by a specific builder
func (r *RedisCache) keyBlockBuilderLatestRoBBidsValue(slot uint64, parentHash, proposerPubkey string, chainID string) string {
	return fmt.Sprintf("rob,%s:%d_%s_%s_%s", r.prefixBlockBuilderLatestBidsValue, slot, parentHash, proposerPubkey, chainID)
}

// keyBlockBuilderLatestBidValue returns the hashmap key for the time of the latest bid by a specific builder
func (r *RedisCache) keyBlockBuilderLatestBidsTime(slot uint64, parentHash, proposerPubkey string) string {
	return fmt.Sprintf("%s:%d_%s_%s", r.prefixBlockBuilderLatestBidsTime, slot, parentHash, proposerPubkey)
}

// keyBlockBuilderLatestToBBidsTime returns the hashmap key for the time of the latest bid by a specific builder
func (r *RedisCache) keyBlockBuilderLatestToBBidsTime(slot uint64, parentHash, proposerPubkey string) string {
	return fmt.Sprintf("tob,%s:%d_%s_%s", r.prefixBlockBuilderLatestBidsTime, slot, parentHash, proposerPubkey)
}

// keyBlockBuilderLatestRoBBidsTime returns the hashmap key for the time of the latest bid by a specific builder
func (r *RedisCache) keyBlockBuilderLatestRoBBidsTime(slot uint64, parentHash, proposerPubkey string, chainID string) string {
	return fmt.Sprintf("rob,%s:%d_%s_%s_%s", r.prefixBlockBuilderLatestBidsTime, slot, parentHash, proposerPubkey, chainID)
}

// keyTopBidValue returns the hashmap key for the time of the latest bid by a specific builder
func (r *RedisCache) keyTopToBBidValue(slot uint64, parentHash, proposerPubkey string) string {
	return fmt.Sprintf("tob,%s:%d_%s_%s", r.prefixTopBidValue, slot, parentHash, proposerPubkey)
}

// keyTopBidValue returns the hashmap key for the time of the latest bid by a specific builder
func (r *RedisCache) keyTopRoBBidValue(slot uint64, parentHash, proposerPubkey string, chainID string) string {
	return fmt.Sprintf("rob,%s:%d_%s_%s_%s", r.prefixTopBidValue, slot, parentHash, proposerPubkey, chainID)
}

// keyFloorBid returns the key for the highest non-cancellable bid of a given slot+parentHash+proposerPubkey
func (r *RedisCache) keyFloorBid(slot uint64, parentHash, proposerPubkey string) string {
	return fmt.Sprintf("%s:%d_%s_%s", r.prefixFloorBid, slot, parentHash, proposerPubkey)
}

// keyFloorBid returns the key for the highest non-cancellable bid of a given slot+parentHash+proposerPubkey
func (r *RedisCache) keyFloorToBBid(slot uint64, parentHash, proposerPubkey string) string {
	return fmt.Sprintf("tob,%s:%d_%s_%s", r.prefixFloorBid, slot, parentHash, proposerPubkey)
}

func (r *RedisCache) keyFloorRoBBid(slot uint64, parentHash, proposerPubkey string, chainID string) string {
	return fmt.Sprintf("%s:%d_%s_%s_%s", r.prefixFloorBid, slot, parentHash, proposerPubkey, chainID)
}

// keyFloorBidValue returns the key for the highest non-cancellable value of a given slot+parentHash+proposerPubkey
func (r *RedisCache) keyFloorBidValue(slot uint64, parentHash, proposerPubkey string) string {
	return fmt.Sprintf("tob, %s:%d_%s_%s", r.prefixFloorBidValue, slot, parentHash, proposerPubkey)
}

// keyFloorBidValue returns the key for the highest non-cancellable value of a given slot+parentHash+proposerPubkey
func (r *RedisCache) keyFloorToBBidValue(slot uint64, parentHash, proposerPubkey string) string {
	return fmt.Sprintf("tob,%s:%d_%s_%s", r.prefixFloorBidValue, slot, parentHash, proposerPubkey)
}

// keyFloorBidValue returns the key for the highest non-cancellable value of a given slot+parentHash+proposerPubkey
func (r *RedisCache) keyFloorRoBBidValue(slot uint64, parentHash, proposerPubkey string, chainID string) string {
	return fmt.Sprintf("rob,%s:%d_%s_%s_%s", r.prefixFloorBidValue, slot, parentHash, proposerPubkey, chainID)
}

func (r *RedisCache) GetObj(key string, obj any) (err error) {
	value, err := r.client.Get(context.Background(), key).Result()
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(value), &obj)
}

func (r *RedisCache) SetObj(key string, value any, expiration time.Duration) (err error) {
	marshalledValue, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return r.client.Set(context.Background(), key, marshalledValue, expiration).Err()
}

// SetObjPipelined saves an object in the given Redis key on a Redis pipeline (JSON encoded)
func (r *RedisCache) SetObjPipelined(ctx context.Context, pipeline redis.Pipeliner, key string, value any, expiration time.Duration) (err error) {
	marshalledValue, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return pipeline.Set(ctx, key, marshalledValue, expiration).Err()
}

func (r *RedisCache) HSetObj(key, field string, value any, expiration time.Duration) (err error) {
	marshalledValue, err := json.Marshal(value)
	if err != nil {
		return err
	}

	err = r.client.HSet(context.Background(), key, field, marshalledValue).Err()
	if err != nil {
		return err
	}

	return r.client.Expire(context.Background(), key, expiration).Err()
}

func (r *RedisCache) GetValidatorRegistrationTimestamp(proposerPubkey common.PubkeyHex) (uint64, error) {
	timestamp, err := r.client.HGet(context.Background(), r.keyValidatorRegistrationTimestamp, strings.ToLower(proposerPubkey.String())).Uint64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return timestamp, err
}

func (r *RedisCache) SetValidatorRegistrationTimestampIfNewer(proposerPubkey common.PubkeyHex, timestamp uint64) error {
	knownTimestamp, err := r.GetValidatorRegistrationTimestamp(proposerPubkey)
	if err != nil {
		return err
	}
	if knownTimestamp >= timestamp {
		return nil
	}
	return r.SetValidatorRegistrationTimestamp(proposerPubkey, timestamp)
}

func (r *RedisCache) SetValidatorRegistrationTimestamp(proposerPubkey common.PubkeyHex, timestamp uint64) error {
	return r.client.HSet(context.Background(), r.keyValidatorRegistrationTimestamp, proposerPubkey.String(), timestamp).Err()
}

func (r *RedisCache) CheckAndSetLastSlotAndHashDelivered(slot uint64, hash string) (err error) {
	// More details about Redis optimistic locking:
	// - https://redis.uptrace.dev/guide/go-redis-pipelines.html#transactions
	// - https://github.com/redis/go-redis/blob/6ecbcf6c90919350c42181ce34c1cbdfbd5d1463/race_test.go#L183
	txf := func(tx *redis.Tx) error {
		lastSlotDelivered, err := tx.Get(context.Background(), r.keyLastSlotDelivered).Uint64()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}

		// slot in the past, reject request
		if slot < lastSlotDelivered {
			return ErrPastSlotAlreadyDelivered
		}

		// current slot, reject request if hash is different
		if slot == lastSlotDelivered {
			lastHashDelivered, err := tx.Get(context.Background(), r.keyLastHashDelivered).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
			if hash != lastHashDelivered {
				return ErrAnotherPayloadAlreadyDeliveredForSlot
			}
			return nil
		}

		_, err = tx.TxPipelined(context.Background(), func(pipe redis.Pipeliner) error {
			pipe.Set(context.Background(), r.keyLastSlotDelivered, slot, 0)
			pipe.Set(context.Background(), r.keyLastHashDelivered, hash, 0)
			return nil
		})

		return err
	}

	return r.client.Watch(context.Background(), txf, r.keyLastSlotDelivered, r.keyLastHashDelivered)
}

func (r *RedisCache) GetLastSlotDelivered(ctx context.Context, pipeliner redis.Pipeliner) (slot uint64, err error) {
	c := pipeliner.Get(ctx, r.keyLastSlotDelivered)
	_, err = pipeliner.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return c.Uint64()
}

func (r *RedisCache) GetLastHashDelivered() (hash string, err error) {
	return r.client.Get(context.Background(), r.keyLastHashDelivered).Result()
}

func (r *RedisCache) SetStats(field string, value any) (err error) {
	return r.client.HSet(context.Background(), r.keyStats, field, value).Err()
}

func (r *RedisCache) GetStats(field string) (value string, err error) {
	return r.client.HGet(context.Background(), r.keyStats, field).Result()
}

// GetStatsUint64 returns (valueUint64, nil), or (0, redis.Nil) if the field does not exist
func (r *RedisCache) GetStatsUint64(field string) (value uint64, err error) {
	valStr, err := r.client.HGet(context.Background(), r.keyStats, field).Result()
	if err != nil {
		return 0, err
	}

	value, err = strconv.ParseUint(valStr, 10, 64)
	return value, err
}

func (r *RedisCache) SetProposerDuties(proposerDuties []common.BuilderGetValidatorsResponseEntry) (err error) {
	return r.SetObj(r.keyProposerDuties, proposerDuties, 0)
}

func (r *RedisCache) GetProposerDuties() (proposerDuties []common.BuilderGetValidatorsResponseEntry, err error) {
	proposerDuties = make([]common.BuilderGetValidatorsResponseEntry, 0)
	err = r.GetObj(r.keyProposerDuties, &proposerDuties)
	if errors.Is(err, redis.Nil) {
		return proposerDuties, nil
	}
	return proposerDuties, err
}

func (r *RedisCache) SetRelayConfig(field, value string) (err error) {
	return r.client.HSet(context.Background(), r.keyRelayConfig, field, value).Err()
}

func (r *RedisCache) GetRelayConfig(field string) (string, error) {
	res, err := r.client.HGet(context.Background(), r.keyRelayConfig, field).Result()
	if errors.Is(err, redis.Nil) {
		return res, nil
	}
	return res, err
}

func (r *RedisCache) GetBestToBBid(slot uint64, parentHash, proposerPubkey string) (*common.AnchorHeader, error) {
	key := r.keyCacheGetToBHeaderResponse(slot, parentHash, proposerPubkey)
	resp := new(common.AnchorHeader)
	err := r.GetObj(key, resp)
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return resp, err
}

func (r *RedisCache) GetBestRoBBid(slot uint64, parentHash, proposerPubkey string, chainID string) (*common.AnchorHeader, error) {
	key := r.keyCacheGetRoBHeaderResponse(slot, parentHash, proposerPubkey, chainID)
	resp := new(common.AnchorHeader)
	err := r.GetObj(key, resp)
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return resp, err
}

// TODO: We need to figure how to save multiple ToB/RoB domains to the database.
// Currently it is just Get/Set functions for each

/*
func (r *RedisCache) GetHighestRob(slot uint64, parentHash string) (*common.BuilderSubmitBlockRequest, error) {
	key := r.keyCacheGetHighestRob(slot, parentHash)
	resp := new(common.BuilderSubmitBlockRequest)
	err := r.GetObj(key, resp)
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return resp, err
}

func (r *RedisCache) SetHighestRob(slot uint64, parentHash string, highestRob *common.BuilderSubmitBlockRequest) (err error) {
	key := r.keyCacheGetHighestRob(slot, parentHash)
	return r.SetObj(key, highestRob, 0)
}

func (r *RedisCache) SetHighestRobValue(ctx context.Context, tx redis.Pipeliner, value *big.Int, slot uint64, parentHash string) (err error) {
	key := r.keyCacheGetHighestRobValue(slot, parentHash)
	err = tx.Set(ctx, key, value.String(), expiryBidCache).Err()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx)
	return err
}

func (r *RedisCache) GetHighestRobValue(ctx context.Context, tx redis.Pipeliner, slot uint64, parentHash string) (robBidValue *big.Int, err error) {
	keyRobValue := r.keyCacheGetHighestRobValue(slot, parentHash)
	c := tx.Get(ctx, keyRobValue)
	_, err = tx.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return big.NewInt(0), nil
	} else if err != nil {
		return nil, err
	}

	robBidValueStr, err := c.Result()
	if err != nil {
		return nil, err
	}
	robBidValue = new(big.Int)
	robBidValue.SetString(robBidValueStr, 10)
	return robBidValue, nil
}
*/

func (r *RedisCache) SetTobTx(ctx context.Context, tx redis.Pipeliner, slot uint64, parentHash string, txs [][]byte) error {
	key := r.keyCacheGetTobTxs(slot, parentHash)
	finalTxStrings := []string{}
	for _, tx := range txs {
		finalTxStrings = append(finalTxStrings, hex.EncodeToString(tx))
	}
	finalTxString := strings.Join(finalTxStrings, ",")

	err := tx.Set(ctx, key, finalTxString, expiryBidCache).Err()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx)
	return err
}

func (r *RedisCache) GetTobTx(ctx context.Context, tx redis.Pipeliner, slot uint64, parentHash string) ([][]byte, error) {
	key := r.keyCacheGetTobTxs(slot, parentHash)
	resp := make([][]byte, 0)
	c := tx.Get(ctx, key)
	_, err := tx.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return resp, nil
	} else if err != nil {
		return resp, err
	}
	tobTxs, err := c.Result()
	if err != nil {
		return resp, err
	}
	tobTxStrings := strings.Split(tobTxs, ",")
	for _, tobTxString := range tobTxStrings {
		tobTx, err := hex.DecodeString(tobTxString)
		if err != nil {
			return resp, err
		}
		resp = append(resp, tobTx)
	}

	return resp, err
}

func (r *RedisCache) SetTobTxValue(ctx context.Context, tx redis.Pipeliner, value *big.Int, slot uint64, parentHash string) (err error) {
	key := r.keyCacheGetTobTxsValue(slot, parentHash)
	err = tx.Set(ctx, key, value.String(), expiryBidCache).Err()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx)
	return err
}

func (r *RedisCache) GetTobTxValue(ctx context.Context, tx redis.Pipeliner, slot uint64, parentHash string) (tobTxValue *big.Int, err error) {
	keyTobTxValue := r.keyCacheGetTobTxsValue(slot, parentHash)
	c := tx.Get(ctx, keyTobTxValue)
	_, err = tx.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return big.NewInt(0), nil
	} else if err != nil {
		return nil, err
	}

	tobTxValueStr, err := c.Result()
	if err != nil {
		return nil, err
	}
	tobTxValue = new(big.Int)
	tobTxValue.SetString(tobTxValueStr, 10)
	return tobTxValue, nil
}

// Here is how they save information
func (r *RedisCache) SaveExecutionToBAnchorPayload(
	ctx context.Context,
	pipeliner redis.Pipeliner,
	slot uint64,
	proposerPubkey,
	blockHash string,
	payload *common.AnchorPayload,
) (err error) {
	key := r.keyExecToBAnchorPayload(slot, proposerPubkey, blockHash)
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return pipeliner.Set(ctx, key, b, expiryBidCache).Err()
}

func (r *RedisCache) SaveExecutionRoBAnchorPayload(
	ctx context.Context,
	pipeline redis.Pipeliner,
	slot uint64,
	proposerPubkey,
	blockHash string,
	payload *common.AnchorPayload,
	chainID string,
) (err error) {
	key := r.keyExecRoBAnchorPayload(slot, proposerPubkey, blockHash, chainID)
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return pipeline.Set(ctx, key, b, expiryBidCache).Err()
}

func (r *RedisCache) GetExecutionToBAnchorPayload(
	slot uint64,
	proposerPubkey,
	blockHash string,
) (*common.AnchorPayload, error) {
	resp := new(common.AnchorPayload)

	key := r.keyExecToBAnchorPayload(slot, proposerPubkey, blockHash)
	val, err := r.client.Get(context.Background(), key).Result()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal([]byte(val), resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *RedisCache) GetExecutionRoBAnchorPayload(
	slot uint64,
	proposerPubkey,
	blockHash string,
	chainID string,
) (*common.AnchorPayload, error) {
	resp := new(common.AnchorPayload)

	key := r.keyExecRoBAnchorPayload(slot, proposerPubkey, blockHash, chainID)
	val, err := r.client.Get(context.Background(), key).Result()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal([]byte(val), resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *RedisCache) SaveToBBidTrace(
	ctx context.Context,
	pipeliner redis.Pipeliner,
	trace *common.BidTraceV3,
) (err error) {
	key := r.keyCacheToBBidTrace(trace.Slot, trace.ProposerPubkey, trace.BlockHash)
	return r.SetObjPipelined(ctx, pipeliner, key, trace, expiryBidCache)
}

func (r *RedisCache) SaveRoBBidTrace(
	ctx context.Context,
	pipeline redis.Pipeliner,
	trace *common.BidTraceV3,
	chainID string,
) (err error) {
	key := r.keyCacheRoBBidTrace(trace.Slot, trace.ProposerPubkey, trace.BlockHash, chainID)
	return r.SetObjPipelined(ctx, pipeline, key, trace, expiryBidCache)
}

// GetBidTrace returns (trace, nil), or (nil, redis.Nil) if the trace does not exist
func (r *RedisCache) GetToBBidTrace(
	slot uint64,
	proposerPubkey,
	blockHash string,
) (*common.BidTraceV3, error) {
	key := r.keyCacheToBBidTrace(slot, proposerPubkey, blockHash)
	resp := new(common.BidTraceV3)
	err := r.GetObj(key, resp)
	return resp, err
}

func (r *RedisCache) GetRoBBidTrace(
	slot uint64,
	proposerPubkey,
	blockHash string,
	chainID string,
) (*common.BidTraceV3, error) {
	key := r.keyCacheRoBBidTrace(slot, proposerPubkey, blockHash, chainID)
	resp := new(common.BidTraceV3)
	err := r.GetObj(key, resp)
	return resp, err
}

func (r *RedisCache) GetBuilderLatestPayloadReceivedAt(ctx context.Context, pipeliner redis.Pipeliner, slot uint64, builderPubkey, parentHash, proposerPubkey string) (int64, error) {
	keyLatestBidsTime := r.keyBlockBuilderLatestBidsTime(slot, parentHash, proposerPubkey)
	c := pipeliner.HGet(context.Background(), keyLatestBidsTime, builderPubkey)
	_, err := pipeliner.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return c.Int64()
}

func (r *RedisCache) SaveToBBuilderBid(
	ctx context.Context,
	pipeliner redis.Pipeliner,
	slot uint64,
	parentHash,
	proposerPubkey,
	builderPubkey string,
	receivedAt time.Time,
	headerResp *eth.Hash,
) (err error) {
	// save the actual bid
	keyLatestBid := r.keyLatestToBBidByBuilder(slot, parentHash, proposerPubkey, builderPubkey)
	err = r.SetObjPipelined(ctx, pipeliner, keyLatestBid, headerResp.Bytes(), expiryBidCache)
	if err != nil {
		return err
	}

	// set the time of the request
	keyLatestBidsTime := r.keyBlockBuilderLatestToBBidsTime(slot, parentHash, proposerPubkey)
	err = pipeliner.HSet(ctx, keyLatestBidsTime, builderPubkey, receivedAt.UnixMilli()).Err()
	if err != nil {
		return err
	}
	err = pipeliner.Expire(ctx, keyLatestBidsTime, expiryBidCache).Err()
	if err != nil {
		return err
	}

	// set the value last, because that's iterated over when updating the best bid, and the payload has to be available
	keyLatestBidsValue := r.keyBlockBuilderLatestToBBidsValue(slot, parentHash, proposerPubkey)
	err = pipeliner.HSet(ctx, keyLatestBidsValue, builderPubkey, headerResp.Bytes()).Err()
	if err != nil {
		return err
	}
	return pipeliner.Expire(ctx, keyLatestBidsValue, expiryBidCache).Err()
}

func (r *RedisCache) SaveRoBBuilderBid(
	ctx context.Context,
	pipeline redis.Pipeliner,
	slot uint64,
	parentHash,
	proposerPubkey,
	builderPubkey string,
	receivedAt time.Time,
	headerResp *eth.Hash,
	chainID string,
) (err error) {
	// save the actual bid
	keyLatestBid := r.keyLatestRoBBidByBuilder(slot, parentHash, proposerPubkey, builderPubkey, chainID)
	err = r.SetObjPipelined(ctx, pipeline, keyLatestBid, headerResp.Bytes(), expiryBidCache)
	if err != nil {
		return err
	}

	// set the time of the request
	keyLatestBidsTime := r.keyBlockBuilderLatestRoBBidsTime(slot, parentHash, proposerPubkey, chainID)
	err = pipeline.HSet(ctx, keyLatestBidsTime, builderPubkey, receivedAt.UnixMilli()).Err()
	if err != nil {
		return err
	}
	err = pipeline.Expire(ctx, keyLatestBidsTime, expiryBidCache).Err()
	if err != nil {
		return err
	}

	// set the value last, because that's iterated over when updating the best bid, and the payload has to be available
	keyLatestBidsValue := r.keyBlockBuilderLatestRoBBidsValue(slot, parentHash, proposerPubkey, chainID)
	// note
	err = pipeline.HSet(ctx, keyLatestBidsValue, builderPubkey, headerResp.Bytes()).Err()
	if err != nil {
		return err
	}
	return pipeline.Expire(ctx, keyLatestBidsValue, expiryBidCache).Err()
}

// how to save bid
type SaveBidAndUpdateTopBidResponse struct {
	WasBidSaved      bool // Whether this bid was saved
	WasTopBidUpdated bool // Whether the top bid was updated
	IsNewTopBid      bool // Whether the submitted bid became the new top bid

	TopBidValue     *big.Int
	PrevTopBidValue *big.Int

	TimePrep         time.Duration
	TimeSavePayload  time.Duration
	TimeSaveBid      time.Duration
	TimeSaveTrace    time.Duration
	TimeUpdateTopBid time.Duration
	TimeUpdateFloor  time.Duration
}

func (r *RedisCache) SaveToBBidAndUpdateTopBid(
	ctx context.Context,
	pipeliner redis.Pipeliner,
	payload *common.SubmitNewBlockRequest,
	value *big.Int,
	getPayload *common.AnchorPayload,
	getHeader *common.AnchorHeader,
	reqReceivedAt time.Time,
	isCancellationEnabled bool,
	floorValue *big.Int,
	trace *common.BidTraceV3,
) (state SaveBidAndUpdateTopBidResponse, err error) {
	var prevTime, nextTime time.Time
	prevTime = time.Now()

	// Load latest bids for a given slot+parent+proposer
	builderBids, err := NewToBBuilderBidsFromRedis(
		ctx,
		r,
		pipeliner,
		payload.Slot(),
		payload.ParentHash().String(),
		payload.ProposerPubKeyAsStr())
	if err != nil {
		return state, err
	}

	// Load floor value (if not passed in already)
	if floorValue == nil {
		floorValue, err = r.GetFloorToBBidValue(
			ctx,
			pipeliner,
			payload.Slot(),
			payload.ParentHash().String(),
			payload.ProposerPubKeyAsStr())
		if err != nil {
			return state, err
		}
	}

	// Get the reference top bid value
	_, state.TopBidValue = builderBids.getTopBid()
	if floorValue.Cmp(state.TopBidValue) == 1 {
		state.TopBidValue = floorValue
	}
	state.PrevTopBidValue = state.TopBidValue

	// Abort now if non-cancellation bid is lower than floor value
	isBidAboveFloor := value.Cmp(floorValue) == 1
	if !isCancellationEnabled && !isBidAboveFloor {
		return state, nil
	}

	// Record time needed
	nextTime = time.Now().UTC()
	state.TimePrep = nextTime.Sub(prevTime)
	prevTime = nextTime

	// note
	// Time to save things in Redis
	//
	// 1. Save the execution payload
	err = r.SaveExecutionToBAnchorPayload(
		ctx,
		pipeliner,
		payload.Slot(),
		payload.ProposerPubKeyAsStr(),
		payload.BlockHash().String(), getPayload)
	if err != nil {
		return state, err
	}

	// Record time needed to save payload
	nextTime = time.Now().UTC()
	state.TimeSavePayload = nextTime.Sub(prevTime)
	prevTime = nextTime

	// 2. Save latest bid for this builder
	err = r.SaveToBBuilderBid(
		ctx,
		pipeliner,
		payload.Slot(),
		payload.ParentHash().String(),
		payload.ProposerPubKeyAsStr(),
		payload.BuilderPubkeyAsStr(),
		reqReceivedAt,
		getHeader.Header)
	if err != nil {
		return state, err
	}
	builderBids.bidValues[payload.BuilderPubkeyAsStr()] = value

	// Record time needed to save bid
	nextTime = time.Now().UTC()
	state.TimeSaveBid = nextTime.Sub(prevTime)
	prevTime = nextTime

	// 3. Save the bid trace
	err = r.SaveToBBidTrace(ctx, pipeliner, trace)
	if err != nil {
		return state, err
	}

	// Record time needed to save trace
	nextTime = time.Now().UTC()
	state.TimeSaveTrace = nextTime.Sub(prevTime)
	prevTime = nextTime

	// If top bid value hasn't change, abort now
	_, state.TopBidValue = builderBids.getTopBid()
	if state.TopBidValue.Cmp(state.PrevTopBidValue) == 0 {
		return state, nil
	}

	state, err = r._updateToBTopBid(ctx, pipeliner, state, builderBids, payload.Slot(), payload.ParentHash().String(), payload.ProposerPubKeyAsStr(), floorValue)
	if err != nil {
		return state, err
	}
	state.IsNewTopBid = value.Cmp(state.TopBidValue) == 0
	// An Exec happens in _updateToBTopBid.
	state.WasBidSaved = true

	// Record time needed to update top bid
	nextTime = time.Now().UTC()
	state.TimeUpdateTopBid = nextTime.Sub(prevTime)
	prevTime = nextTime

	if isCancellationEnabled || !isBidAboveFloor {
		return state, nil
	}

	// Non-cancellable bid above floor should set new floor
	keyBidSource := r.keyLatestToBBidByBuilder(
		payload.Slot(),
		payload.ParentHash().String(),
		payload.ProposerPubKeyAsStr(),
		payload.BuilderPubkeyAsStr())
	keyFloorBid := r.keyFloorToBBid(payload.Slot(), payload.ParentHash().String(), payload.ProposerPubKeyAsStr())
	c := pipeliner.Copy(ctx, keyBidSource, keyFloorBid, 0, true)
	_, err = pipeliner.Exec(ctx)
	if err != nil {
		return state, err
	}

	wasCopied, copyErr := c.Result()
	if copyErr != nil {
		return state, copyErr
	} else if wasCopied == 0 {
		return state, fmt.Errorf("could not copy floor bid from %s to %s", keyBidSource, keyFloorBid) //nolint:goerr113
	}
	err = pipeliner.Expire(ctx, keyFloorBid, expiryBidCache).Err()
	if err != nil {
		return state, err
	}

	keyFloorBidValue := r.keyFloorToBBidValue(payload.Slot(), payload.ParentHash().String(), payload.ProposerPubKeyAsStr())
	err = pipeliner.Set(ctx, keyFloorBidValue, value.String(), expiryBidCache).Err()
	if err != nil {
		return state, err
	}

	// Execute setting the floor bid
	_, err = pipeliner.Exec(ctx)

	// Record time needed to update floor
	nextTime = time.Now().UTC()
	state.TimeUpdateFloor = nextTime.Sub(prevTime)

	return state, err
}

func (r *RedisCache) SaveRoBBidAndUpdateTopBid(
	ctx context.Context,
	pipeline redis.Pipeliner,
	payload *common.SubmitNewBlockRequest,
	value *big.Int,
	getPayload *common.AnchorPayload,
	getHeader *common.AnchorHeader,
	chainID string,
	reqReceivedAt time.Time,
	isCancellationEnabled bool,
	floorValue *big.Int,
	trace *common.BidTraceV3,
) (state SaveBidAndUpdateTopBidResponse, err error) {
	var prevTime, nextTime time.Time
	prevTime = time.Now()

	// Load latest bids for a given slot+parent+proposer
	builderBids, err := NewRoBBuilderBidsFromRedis(ctx, r, pipeline, payload.Slot(), payload.ParentHash().String(), payload.ProposerPubKeyAsStr(), chainID)
	if err != nil {
		return state, err
	}

	// Load floor value (if not passed in already)
	if floorValue == nil {
		floorValue, err = r.GetFloorRoBBidValue(ctx, pipeline, payload.Slot(), payload.ParentHash().String(), payload.ProposerPubKeyAsStr(), chainID)
		if err != nil {
			return state, err
		}
	}

	// Get the reference top bid value
	_, state.TopBidValue = builderBids.getTopBid()
	if floorValue.Cmp(state.TopBidValue) == 1 {
		state.TopBidValue = floorValue
	}
	state.PrevTopBidValue = state.TopBidValue

	// Abort now if non-cancellation bid is lower than floor value
	isBidAboveFloor := value.Cmp(floorValue) == 1
	if !isCancellationEnabled && !isBidAboveFloor {
		return state, nil
	}

	// Record time needed
	nextTime = time.Now().UTC()
	state.TimePrep = nextTime.Sub(prevTime)
	prevTime = nextTime

	// note
	// Time to save things in Redis
	//
	// 1. Save the execution payload
	err = r.SaveExecutionRoBAnchorPayload(ctx, pipeline, payload.Slot(), payload.ProposerPubKeyAsStr(), payload.BlockHash().String(), getPayload, chainID)
	if err != nil {
		return state, err
	}

	// Record time needed to save payload
	nextTime = time.Now().UTC()
	state.TimeSavePayload = nextTime.Sub(prevTime)
	prevTime = nextTime

	// 2. Save latest bid for this builder
	err = r.SaveRoBBuilderBid(ctx, pipeline, payload.Slot(), payload.ParentHash().String(), payload.ProposerPubKeyAsStr(), payload.BuilderPubkeyAsStr(), reqReceivedAt, getHeader.Header, chainID)
	if err != nil {
		return state, err
	}
	builderBids.bidValues[payload.BuilderPubkeyAsStr()] = value

	// Record time needed to save bid
	nextTime = time.Now().UTC()
	state.TimeSaveBid = nextTime.Sub(prevTime)
	prevTime = nextTime

	//3. Save the bid trace
	err = r.SaveRoBBidTrace(ctx, pipeline, trace, chainID)
	if err != nil {
		return state, err
	}

	// Record time needed to save trace
	nextTime = time.Now().UTC()
	state.TimeSaveTrace = nextTime.Sub(prevTime)
	prevTime = nextTime

	// If top bid value hasn't change, abort now
	_, state.TopBidValue = builderBids.getTopBid()
	if state.TopBidValue.Cmp(state.PrevTopBidValue) == 0 {
		return state, nil
	}

	state, err = r._updateRoBTopBid(ctx, pipeline, state, builderBids, payload.Slot(), payload.ParentHash().String(), payload.ProposerPubKeyAsStr(), floorValue, chainID)
	if err != nil {
		return state, err
	}
	state.IsNewTopBid = value.Cmp(state.TopBidValue) == 0
	// An Exec happens in _updateToBTopBid.
	state.WasBidSaved = true

	// Record time needed to update top bid
	nextTime = time.Now().UTC()
	state.TimeUpdateTopBid = nextTime.Sub(prevTime)
	prevTime = nextTime

	if isCancellationEnabled || !isBidAboveFloor {
		return state, nil
	}

	// Non-cancellable bid above floor should set new floor
	keyBidSource := r.keyLatestRoBBidByBuilder(payload.Slot(), payload.ParentHash().String(), payload.ProposerPubKeyAsStr(), payload.BuilderPubkeyAsStr(), chainID)
	keyFloorBid := r.keyFloorRoBBid(payload.Slot(), payload.ParentHash().String(), payload.ProposerPubKeyAsStr(), chainID)
	c := pipeline.Copy(ctx, keyBidSource, keyFloorBid, 0, true)
	_, err = pipeline.Exec(ctx)
	if err != nil {
		return state, err
	}

	wasCopied, copyErr := c.Result()
	if copyErr != nil {
		return state, copyErr
	} else if wasCopied == 0 {
		return state, fmt.Errorf("could not copy floor bid from %s to %s", keyBidSource, keyFloorBid) //nolint:goerr113
	}
	err = pipeline.Expire(ctx, keyFloorBid, expiryBidCache).Err()
	if err != nil {
		return state, err
	}

	keyFloorBidValue := r.keyFloorRoBBidValue(payload.Slot(), payload.ParentHash().String(), payload.ProposerPubKeyAsStr(), chainID)
	err = pipeline.Set(ctx, keyFloorBidValue, value.String(), expiryBidCache).Err()
	if err != nil {
		return state, err
	}

	// Execute setting the floor bid
	_, err = pipeline.Exec(ctx)

	// Record time needed to update floor
	nextTime = time.Now().UTC()
	state.TimeUpdateFloor = nextTime.Sub(prevTime)

	return state, err
}

/*
func (r *RedisCache) SaveBidAndUpdateTopBid(ctx context.Context, pipeliner redis.Pipeliner, trace *common.BidTraceV2, payload *common.BuilderSubmitBlockRequest, getPayloadResponse *common.GetPayloadResponse, getHeaderResponse *common.GetHeaderResponse, reqReceivedAt time.Time, isCancellationEnabled bool, floorValue *big.Int) (state SaveBidAndUpdateTopBidResponse, err error) {
	var prevTime, nextTime time.Time
	prevTime = time.Now()

	// Load latest bids for a given slot+parent+proposer
	builderBids, err := NewBuilderBidsFromRedis(ctx, r, pipeliner, payload.Slot(), payload.ParentHash(), payload.ProposerPubkey())
	if err != nil {
		return state, err
	}

	// Load floor value (if not passed in already)
	if floorValue == nil {
		floorValue, err = r.GetFloorBidValue(ctx, pipeliner, payload.Slot(), payload.ParentHash(), payload.ProposerPubkey())
		if err != nil {
			return state, err
		}
	}

	// Get the reference top bid value
	_, state.TopBidValue = builderBids.getTopBid()
	if floorValue.Cmp(state.TopBidValue) == 1 {
		state.TopBidValue = floorValue
	}
	state.PrevTopBidValue = state.TopBidValue

	// Abort now if non-cancellation bid is lower than floor value
	isBidAboveFloor := payload.Value().Cmp(floorValue) == 1
	if !isCancellationEnabled && !isBidAboveFloor {
		return state, nil
	}

	// Record time needed
	nextTime = time.Now().UTC()
	state.TimePrep = nextTime.Sub(prevTime)
	prevTime = nextTime

	// note
	// Time to save things in Redis
	//
	// 1. Save the execution payload
	err = r.SaveExecutionPayloadCapella(ctx, pipeliner, payload.Slot(), payload.ProposerPubkey(), payload.BlockHash(), getPayloadResponse.Capella.Capella)
	if err != nil {
		return state, err
	}

	// Record time needed to save payload
	nextTime = time.Now().UTC()
	state.TimeSavePayload = nextTime.Sub(prevTime)
	prevTime = nextTime

	// 2. Save latest bid for this builder
	err = r.SaveBuilderBid(ctx, pipeliner, payload.Slot(), payload.ParentHash(), payload.ProposerPubkey(), payload.BuilderPubkey().String(), reqReceivedAt, getHeaderResponse)
	if err != nil {
		return state, err
	}
	builderBids.bidValues[payload.BuilderPubkey().String()] = payload.Value()

	// Record time needed to save bid
	nextTime = time.Now().UTC()
	state.TimeSaveBid = nextTime.Sub(prevTime)
	prevTime = nextTime

	// 3. Save the bid trace
	err = r.SaveBidTrace(ctx, pipeliner, trace)
	if err != nil {
		return state, err
	}

	// Record time needed to save trace
	nextTime = time.Now().UTC()
	state.TimeSaveTrace = nextTime.Sub(prevTime)
	prevTime = nextTime

	// If top bid value hasn't change, abort now
	_, state.TopBidValue = builderBids.getTopBid()
	if state.TopBidValue.Cmp(state.PrevTopBidValue) == 0 {
		return state, nil
	}

	state, err = r._updateToBTopBid(ctx, pipeliner, state, builderBids, payload.Slot(), payload.ParentHash(), payload.ProposerPubkey(), floorValue)
	if err != nil {
		return state, err
	}
	state.IsNewTopBid = payload.Value().Cmp(state.TopBidValue) == 0
	// An Exec happens in _updateToBTopBid.
	state.WasBidSaved = true

	// Record time needed to update top bid
	nextTime = time.Now().UTC()
	state.TimeUpdateTopBid = nextTime.Sub(prevTime)
	prevTime = nextTime

	if isCancellationEnabled || !isBidAboveFloor {
		return state, nil
	}

	// Non-cancellable bid above floor should set new floor
	keyBidSource := r.keyLatestBidByBuilder(payload.Slot(), payload.ParentHash(), payload.ProposerPubkey(), payload.BuilderPubkey().String())
	keyFloorBid := r.keyFloorBid(payload.Slot(), payload.ParentHash(), payload.ProposerPubkey())
	c := pipeliner.Copy(ctx, keyBidSource, keyFloorBid, 0, true)
	_, err = pipeliner.Exec(ctx)
	if err != nil {
		return state, err
	}

	wasCopied, copyErr := c.Result()
	if copyErr != nil {
		return state, copyErr
	} else if wasCopied == 0 {
		return state, fmt.Errorf("could not copy floor bid from %s to %s", keyBidSource, keyFloorBid) //nolint:goerr113
	}
	err = pipeliner.Expire(ctx, keyFloorBid, expiryBidCache).Err()
	if err != nil {
		return state, err
	}

	keyFloorBidValue := r.keyFloorBidValue(payload.Slot(), payload.ParentHash(), payload.ProposerPubkey())
	err = pipeliner.Set(ctx, keyFloorBidValue, payload.Value().String(), expiryBidCache).Err()
	if err != nil {
		return state, err
	}

	// Execute setting the floor bid
	_, err = pipeliner.Exec(ctx)

	// Record time needed to update floor
	nextTime = time.Now().UTC()
	state.TimeUpdateFloor = nextTime.Sub(prevTime)

	return state, err
}
*/

// func (r *RedisCache) _updateTopBid(ctx context.Context, pipeliner redis.Pipeliner, state SaveBidAndUpdateTopBidResponse, builderBids *BuilderBids, slot uint64, parentHash, proposerPubkey string, floorValue *big.Int) (resp SaveBidAndUpdateTopBidResponse, err error) {
// 	if builderBids == nil {
// 		builderBids, err = NewBuilderBidsFromRedis(ctx, r, pipeliner, slot, parentHash, proposerPubkey)
// 		if err != nil {
// 			return state, err
// 		}
// 	}

// 	if len(builderBids.bidValues) == 0 {
// 		return state, nil
// 	}

// 	// Load floor value (if not passed in already)
// 	if floorValue == nil {
// 		floorValue, err = r.GetFloorBidValue(ctx, pipeliner, slot, parentHash, proposerPubkey)
// 		if err != nil {
// 			return state, err
// 		}
// 	}

// 	topBidBuilder := ""
// 	topBidBuilder, state.TopBidValue = builderBids.getTopBid()
// 	keyBidSource := r.keyLatestBidByBuilder(slot, parentHash, proposerPubkey, topBidBuilder)

// 	// If floor value is higher than this bid, use floor bid instead
// 	if floorValue.Cmp(state.TopBidValue) == 1 {
// 		state.TopBidValue = floorValue
// 		keyBidSource = r.keyFloorBid(slot, parentHash, proposerPubkey)
// 	}

// 	// Copy winning bid to top bid cache
// 	keyTopBid := r.keyCacheGetHeaderResponse(slot, parentHash, proposerPubkey)
// 	c := pipeliner.Copy(context.Background(), keyBidSource, keyTopBid, 0, true)
// 	_, err = pipeliner.Exec(ctx)
// 	if err != nil {
// 		return state, err
// 	}
// 	wasCopied, err := c.Result()
// 	if err != nil {
// 		return state, err
// 	} else if wasCopied == 0 {
// 		return state, fmt.Errorf("could not copy top bid from %s to %s", keyBidSource, keyTopBid) //nolint:goerr113
// 	}
// 	err = pipeliner.Expire(context.Background(), keyTopBid, expiryBidCache).Err()
// 	if err != nil {
// 		return state, err
// 	}

// 	state.WasTopBidUpdated = state.PrevTopBidValue == nil || state.PrevTopBidValue.Cmp(state.TopBidValue) != 0

// 	// 6. Finally, update the global top bid value
// 	keyTopBidValue := r.keyTopBidValue(slot, parentHash, proposerPubkey)
// 	err = pipeliner.Set(context.Background(), keyTopBidValue, state.TopBidValue.String(), expiryBidCache).Err()
// 	if err != nil {
// 		return state, err
// 	}

// 	_, err = pipeliner.Exec(ctx)
// 	return state, err
// }

func (r *RedisCache) _updateToBTopBid(
	ctx context.Context,
	pipeliner redis.Pipeliner,
	state SaveBidAndUpdateTopBidResponse,
	builderBids *BuilderBids,
	slot uint64,
	parentHash,
	proposerPubkey string,
	floorValue *big.Int,
) (resp SaveBidAndUpdateTopBidResponse, err error) {
	if builderBids == nil {
		builderBids, err = NewToBBuilderBidsFromRedis(ctx, r, pipeliner, slot, parentHash, proposerPubkey)
		if err != nil {
			return state, err
		}
	}

	if len(builderBids.bidValues) == 0 {
		return state, nil
	}

	// Load floor value (if not passed in already)
	if floorValue == nil {
		floorValue, err = r.GetFloorBidValue(ctx, pipeliner, slot, parentHash, proposerPubkey)
		if err != nil {
			return state, err
		}
	}

	topBidBuilder := ""
	topBidBuilder, state.TopBidValue = builderBids.getTopBid()
	keyBidToBSource := r.keyLatestToBBidByBuilder(slot, parentHash, proposerPubkey, topBidBuilder)

	// If floor value is higher than this bid, use floor bid instead
	if floorValue.Cmp(state.TopBidValue) == 1 {
		state.TopBidValue = floorValue
		keyBidToBSource = r.keyFloorBid(slot, parentHash, proposerPubkey)
	}

	// Copy winning bid to top bid cache
	keyTopBid := r.keyCacheGetToBHeaderResponse(slot, parentHash, proposerPubkey)
	c := pipeliner.Copy(context.Background(), keyBidToBSource, keyTopBid, 0, true)
	_, err = pipeliner.Exec(ctx)
	if err != nil {
		return state, err
	}
	wasCopied, err := c.Result()
	if err != nil {
		return state, err
	} else if wasCopied == 0 {
		return state, fmt.Errorf("could not copy top bid from %s to %s", keyBidToBSource, keyTopBid) //nolint:goerr113
	}
	err = pipeliner.Expire(context.Background(), keyTopBid, expiryBidCache).Err()
	if err != nil {
		return state, err
	}

	state.WasTopBidUpdated = state.PrevTopBidValue == nil || state.PrevTopBidValue.Cmp(state.TopBidValue) != 0

	// 6. Finally, update the global top bid value
	keyTopBidValue := r.keyTopToBBidValue(slot, parentHash, proposerPubkey)
	err = pipeliner.Set(context.Background(), keyTopBidValue, state.TopBidValue.String(), expiryBidCache).Err()
	if err != nil {
		return state, err
	}

	_, err = pipeliner.Exec(ctx)
	return state, err
}

func (r *RedisCache) _updateRoBTopBid(
	ctx context.Context,
	pipeline redis.Pipeliner,
	state SaveBidAndUpdateTopBidResponse,
	builderBids *BuilderBids,
	slot uint64,
	parentHash,
	proposerPubkey string,
	floorValue *big.Int,
	chainID string,
) (resp SaveBidAndUpdateTopBidResponse, err error) {
	if builderBids == nil {
		builderBids, err = NewRoBBuilderBidsFromRedis(ctx, r, pipeline, slot, parentHash, proposerPubkey, chainID)
		if err != nil {
			return state, err
		}
	}

	if len(builderBids.bidValues) == 0 {
		return state, nil
	}

	// Load floor value (if not passed in already)
	if floorValue == nil {
		floorValue, err = r.GetFloorRoBBidValue(ctx, pipeline, slot, parentHash, proposerPubkey, chainID)
		if err != nil {
			return state, err
		}
	}

	topBidBuilder := ""
	topBidBuilder, state.TopBidValue = builderBids.getTopBid()
	keyBidRoBSource := r.keyLatestRoBBidByBuilder(slot, parentHash, proposerPubkey, topBidBuilder, chainID)

	// If floor value is higher than this bid, use floor bid instead
	if floorValue.Cmp(state.TopBidValue) == 1 {
		state.TopBidValue = floorValue
		keyBidRoBSource = r.keyFloorRoBBid(slot, parentHash, proposerPubkey, chainID)
	}

	// Copy winning bid to top bid cache
	keyTopBid := r.keyCacheGetRoBHeaderResponse(slot, parentHash, proposerPubkey, chainID)
	c := pipeline.Copy(context.Background(), keyBidRoBSource, keyTopBid, 0, true)
	_, err = pipeline.Exec(ctx)
	if err != nil {
		return state, err
	}

	wasCopied, err := c.Result()
	if err != nil {
		return state, err
	} else if wasCopied == 0 {
		return state, fmt.Errorf("could not copy top bid from %s to %s", keyBidRoBSource, keyTopBid) //nolint:goerr113
	}
	err = pipeline.Expire(context.Background(), keyTopBid, expiryBidCache).Err()
	if err != nil {
		return state, err
	}

	state.WasTopBidUpdated = state.PrevTopBidValue == nil || state.PrevTopBidValue.Cmp(state.TopBidValue) != 0

	// 6. Finally, update the global top bid value
	keyTopBidValue := r.keyTopRoBBidValue(slot, parentHash, proposerPubkey, chainID)
	err = pipeline.Set(context.Background(), keyTopBidValue, state.TopBidValue.String(), expiryBidCache).Err()
	if err != nil {
		return state, err
	}

	_, err = pipeline.Exec(ctx)
	return state, err
}

// DEPRECATED
/*
// GetTopBidValue gets the top bid value for a given slot+parent+proposer combination
func (r *RedisCache) GetTopBidValue(ctx context.Context, pipeliner redis.Pipeliner, slot uint64, parentHash, proposerPubkey string) (topBidValue *big.Int, err error) {
	keyTopBidValue := r.keyTopBidValue(slot, parentHash, proposerPubkey)
	c := pipeliner.Get(ctx, keyTopBidValue)
	_, err = pipeliner.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return big.NewInt(0), nil
	} else if err != nil {
		return nil, err
	}

	topBidValueStr, err := c.Result()
	if err != nil {
		return nil, err
	}
	topBidValue = new(big.Int)
	topBidValue.SetString(topBidValueStr, 10)
	return topBidValue, nil
}
*/

func (r *RedisCache) HasTopToBBidValue(
	ctx context.Context,
	slot uint64,
	parentHash eth.Hash,
	proposerPubkey bls.PublicKey,
) (bool, error) {
	parentString := parentHash.String()
	proposerString := common.PublicKeyToByteString(&proposerPubkey)
	keyTopBidValue := r.keyTopToBBidValue(slot, parentString, proposerString)
	fmt.Println("keyTopBidValue:", keyTopBidValue)
	//TODO: keyTopBidValue returns correct info, yet exists returns 0 meaning redis isn't finding the key
	exists, err := r.client.Exists(ctx, keyTopBidValue).Result()
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func (r *RedisCache) HasTopRoBBidValue(
	ctx context.Context,
	slot uint64,
	parentHash eth.Hash,
	proposerPubkey bls.PublicKey,
	chainID string,
) (bool bool, err error) {
	parentString := parentHash.String()
	proposerString := common.PublicKeyToByteString(&proposerPubkey)
	keyTopBidValue := r.keyTopRoBBidValue(slot, parentString, proposerString, chainID)
	fmt.Println("keyTopBidValue:", keyTopBidValue)
	//TODO: keyTopBidValue returns correct info, yet exists returns 0 meaning redis isn't finding the key
	exists, err := r.client.Exists(ctx, keyTopBidValue).Result()
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func (r *RedisCache) GetTopToBBidValue(
	ctx context.Context,
	pipeliner redis.Pipeliner,
	slot uint64,
	parentHash eth.Hash,
	proposerPubkey bls.PublicKey,
) (topBidValue *big.Int, err error) {
	parentString := parentHash.String()
	proposerString := common.PublicKeyToByteString(&proposerPubkey)
	keyTopBidValue := r.keyTopToBBidValue(slot, parentString, proposerString)
	c := pipeliner.Get(ctx, keyTopBidValue)
	_, err = pipeliner.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return big.NewInt(0), nil
	} else if err != nil {
		return nil, err
	}

	topBidValueStr, err := c.Result()
	if err != nil {
		return nil, err
	}
	topBidValue = new(big.Int)
	topBidValue.SetString(topBidValueStr, 10)
	return topBidValue, nil
}

func (r *RedisCache) GetTopRoBBidValue(
	ctx context.Context,
	pipeliner redis.Pipeliner,
	slot uint64,
	parentHash eth.Hash,
	proposerPubkey bls.PublicKey,
	chainID string,
) (topBidValue *big.Int, err error) {
	parentString := parentHash.String()
	proposerString := common.PublicKeyToByteString(&proposerPubkey)
	keyTopBidValue := r.keyTopRoBBidValue(slot, parentString, proposerString, chainID)
	//fmt.Println("keyTopBidValue:", keyTopBidValue)
	// TODO: keyTopBidValue returns correct info, yet exists returns 0 meaning redis isn't finding the key
	//exists, err := pipeliner.Exists(ctx, keyTopBidValue).Result()
	//if exists == 0 {
	//	// dealing with tob already
	//	fmt.Println("keyTopBidValue not found in Redis")
	//	return big.NewInt(0), nil
	//}
	// otherwise key is rob
	c := pipeliner.Get(ctx, keyTopBidValue)
	//TODO: FAILS HERE since key is not found in redis
	_, err = pipeliner.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return big.NewInt(0), nil
	} else if err != nil {
		return nil, err
	}

	topBidValueStr, err := c.Result()
	if err != nil {
		return nil, err
	}
	topBidValue = new(big.Int)
	topBidValue.SetString(topBidValueStr, 10)
	return topBidValue, nil
}

// GetBuilderLatestValue gets the latest bid value for a given slot+parent+proposer combination for a specific builder pubkey.
func (r *RedisCache) GetBuilderLatestValue(slot uint64, parentHash, proposerPubkey, builderPubkey string) (topBidValue *big.Int, err error) {
	keyLatestValue := r.keyBlockBuilderLatestBidsValue(slot, parentHash, proposerPubkey)
	topBidValueStr, err := r.client.HGet(context.Background(), keyLatestValue, builderPubkey).Result()
	if errors.Is(err, redis.Nil) {
		return big.NewInt(0), nil
	} else if err != nil {
		return nil, err
	}
	topBidValue = new(big.Int)
	topBidValue.SetString(topBidValueStr, 10)
	return topBidValue, nil
}

// DelBuilderBid removes a builders most recent bid
func (r *RedisCache) DelBuilderBid(ctx context.Context, pipeliner redis.Pipeliner, slot uint64, parentHash, proposerPubkey, builderPubkey string) (err error) {
	// delete the value
	keyLatestValue := r.keyBlockBuilderLatestBidsValue(slot, parentHash, proposerPubkey)
	err = r.client.HDel(ctx, keyLatestValue, builderPubkey).Err()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}

	// delete the time
	keyLatestBidsTime := r.keyBlockBuilderLatestBidsTime(slot, parentHash, proposerPubkey)
	err = r.client.HDel(ctx, keyLatestBidsTime, builderPubkey).Err()
	if err != nil {
		return err
	}

	// update bids now to compute current top bid
	state := SaveBidAndUpdateTopBidResponse{} //nolint:exhaustruct
	_, err = r._updateToBTopBid(ctx, pipeliner, state, nil, slot, parentHash, proposerPubkey, nil)
	return err
}

// GetFloorBidValue returns the value of the highest non-cancellable bid
func (r *RedisCache) GetFloorToBBidValue(ctx context.Context, pipeliner redis.Pipeliner, slot uint64, parentHash, proposerPubkey string) (floorValue *big.Int, err error) {
	keyFloorBidValue := r.keyFloorToBBidValue(slot, parentHash, proposerPubkey)
	c := pipeliner.Get(ctx, keyFloorBidValue)

	_, err = pipeliner.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return big.NewInt(0), nil
	} else if err != nil {
		return nil, err
	}

	topBidValueStr, err := c.Result()
	if err != nil {
		return nil, err
	}
	floorValue = new(big.Int)
	floorValue.SetString(topBidValueStr, 10)
	return floorValue, nil
}

// GetFloorBidValue returns the value of the highest non-cancellable bid
func (r *RedisCache) GetFloorRoBBidValue(ctx context.Context, pipeliner redis.Pipeliner, slot uint64, parentHash, proposerPubkey string, chainID string) (floorValue *big.Int, err error) {
	keyFloorBidValue := r.keyFloorRoBBidValue(slot, parentHash, proposerPubkey, chainID)
	c := pipeliner.Get(ctx, keyFloorBidValue)

	_, err = pipeliner.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return big.NewInt(0), nil
	} else if err != nil {
		return nil, err
	}

	topBidValueStr, err := c.Result()
	if err != nil {
		return nil, err
	}
	floorValue = new(big.Int)
	floorValue.SetString(topBidValueStr, 10)
	return floorValue, nil
}

// GetFloorBidValue returns the value of the highest non-cancellable bid
func (r *RedisCache) GetFloorBidValue(ctx context.Context, pipeliner redis.Pipeliner, slot uint64, parentHash, proposerPubkey string) (floorValue *big.Int, err error) {
	keyFloorBidValue := r.keyFloorBidValue(slot, parentHash, proposerPubkey)
	c := pipeliner.Get(ctx, keyFloorBidValue)

	_, err = pipeliner.Exec(ctx)
	if errors.Is(err, redis.Nil) {
		return big.NewInt(0), nil
	} else if err != nil {
		return nil, err
	}

	topBidValueStr, err := c.Result()
	if err != nil {
		return nil, err
	}
	floorValue = new(big.Int)
	floorValue.SetString(topBidValueStr, 10)
	return floorValue, nil
}

// SetFloorBidValue is used only for testing.
func (r *RedisCache) SetFloorBidValue(slot uint64, parentHash, proposerPubkey, value string) error {
	keyFloorBidValue := r.keyFloorBidValue(slot, parentHash, proposerPubkey)
	err := r.client.Set(context.Background(), keyFloorBidValue, value, 0).Err()
	return err
}

func (r *RedisCache) NewPipeline() redis.Pipeliner { //nolint:ireturn,nolintlint
	return r.client.Pipeline()
}

func (r *RedisCache) NewTxPipeline() redis.Pipeliner { //nolint:ireturn
	return r.client.TxPipeline()
}

// For use in testing only
func (r *RedisCache) SetToBTopBid() {
	keyTopBid := r.keyCacheGetRoBHeaderResponse(slot, parentHash, proposerPubkey, chainID)
	/*
	   if len(builderBids.bidValues) == 0 {
	     return state, nil
	   }

	   // Load floor value (if not passed in already)
	   if floorValue == nil {
	     floorValue, err = r.GetFloorRoBBidValue(ctx, pipeline, slot, parentHash, proposerPubkey, chainID)
	     if err != nil {
	       return state, err
	     }
	   }

	   topBidBuilder := ""
	   topBidBuilder, state.TopBidValue = builderBids.getTopBid()
	   keyBidRoBSource := r.keyLatestRoBBidByBuilder(slot, parentHash, proposerPubkey, topBidBuilder, chainID)

	   // If floor value is higher than this bid, use floor bid instead
	   if floorValue.Cmp(state.TopBidValue) == 1 {
	     state.TopBidValue = floorValue
	     keyBidRoBSource = r.keyFloorRoBBid(slot, parentHash, proposerPubkey, chainID)
	   }

	   // Copy winning bid to top bid cache
	   keyTopBid := r.keyCacheGetRoBHeaderResponse(slot, parentHash, proposerPubkey, chainID)
	   c := pipeline.Copy(context.Background(), keyBidRoBSource, keyTopBid, 0, true)
	*/
}
