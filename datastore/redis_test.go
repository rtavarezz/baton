package datastore

import (
	"context"
	"errors"
	"github.com/AnomalyFi/baton/common"
	//"github.com/AnomalyFi/baton/services/api"
	"github.com/alicebob/miniredis/v2"
	builderApiV1 "github.com/attestantio/go-builder-client/api/v1"
	"github.com/attestantio/go-eth2-client/spec/bellatrix"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"math/big"
	"sync"
	"testing"
	"time"
)

func setupTestRedis(t *testing.T) *RedisCache {
	t.Helper()
	var err error

	redisTestServer, err := miniredis.Run()
	require.NoError(t, err)
	redisService, err := NewRedisCache("", redisTestServer.Addr(), "")
	// redisService, err := NewRedisCache("", "localhost:6379", "")
	require.NoError(t, err)

	return redisService
}

func TestRedisValidatorRegistration(t *testing.T) {
	cache := setupTestRedis(t)

	t.Run("Can save and get validator registration from cache", func(t *testing.T) {
		key := common.ValidPayloadRegisterValidator.Message.Pubkey
		value := common.ValidPayloadRegisterValidator
		pkHex := common.NewPubkeyHex(key.String())
		err := cache.SetValidatorRegistrationTimestamp(pkHex, uint64(value.Message.Timestamp.Unix()))
		require.NoError(t, err)
		result, err := cache.GetValidatorRegistrationTimestamp(common.NewPubkeyHex(key.String()))
		require.NoError(t, err)
		require.Equal(t, result, uint64(value.Message.Timestamp.Unix()))
	})

	t.Run("Returns nil if validator registration is not in cache", func(t *testing.T) {
		key := phase0.BLSPubKey{}
		result, err := cache.GetValidatorRegistrationTimestamp(common.NewPubkeyHex(key.String()))
		require.NoError(t, err)
		require.Equal(t, uint64(0), result)
	})

	t.Run("test SetValidatorRegistrationTimestampIfNewer", func(t *testing.T) {
		key := common.ValidPayloadRegisterValidator.Message.Pubkey
		value := common.ValidPayloadRegisterValidator

		pkHex := common.NewPubkeyHex(key.String())
		timestamp := uint64(value.Message.Timestamp.Unix())

		err := cache.SetValidatorRegistrationTimestampIfNewer(pkHex, timestamp)
		require.NoError(t, err)

		result, err := cache.GetValidatorRegistrationTimestamp(common.NewPubkeyHex(key.String()))
		require.NoError(t, err)
		require.Equal(t, result, timestamp)

		// Try to set an older timestamp (should not work)
		timestamp2 := timestamp - 10
		err = cache.SetValidatorRegistrationTimestampIfNewer(pkHex, timestamp2)
		require.NoError(t, err)
		result, err = cache.GetValidatorRegistrationTimestamp(common.NewPubkeyHex(key.String()))
		require.NoError(t, err)
		require.Equal(t, result, timestamp)

		// Try to set an older timestamp (should not work)
		timestamp3 := timestamp + 10
		err = cache.SetValidatorRegistrationTimestampIfNewer(pkHex, timestamp3)
		require.NoError(t, err)
		result, err = cache.GetValidatorRegistrationTimestamp(common.NewPubkeyHex(key.String()))
		require.NoError(t, err)
		require.Equal(t, result, timestamp3)
	})
}

func TestRedisProposerDuties(t *testing.T) {
	cache := setupTestRedis(t)
	duties := []common.BuilderGetValidatorsResponseEntry{
		{
			Slot: 1,
			Entry: &builderApiV1.SignedValidatorRegistration{
				Signature: phase0.BLSSignature{},
				Message: &builderApiV1.ValidatorRegistration{
					FeeRecipient: bellatrix.ExecutionAddress{0x02},
					GasLimit:     5000,
					Timestamp:    time.Unix(0xffffffff, 0),
					Pubkey:       phase0.BLSPubKey{},
				},
			},
		},
	}
	err := cache.SetProposerDuties(duties)
	require.NoError(t, err)

	duties2, err := cache.GetProposerDuties()
	require.NoError(t, err)

	require.Len(t, duties2, 1)
	require.Equal(t, duties[0].Entry.Message.FeeRecipient, duties2[0].Entry.Message.FeeRecipient)
}

func TestRedisURIs(t *testing.T) {
	t.Helper()
	var err error

	redisTestServer, err := miniredis.Run()
	require.NoError(t, err)

	// test connection with and without protocol
	_, err = NewRedisCache("", redisTestServer.Addr(), "")
	require.NoError(t, err)
	_, err = NewRedisCache("", "redis://"+redisTestServer.Addr(), "")
	require.NoError(t, err)

	// test connection w/ credentials
	username := "user"
	password := "pass"
	redisTestServer.RequireUserAuth(username, password)
	fullURL := "redis://" + username + ":" + password + "@" + redisTestServer.Addr()
	_, err = NewRedisCache("", fullURL, "")
	require.NoError(t, err)

	// ensure malformed URL throws error
	malformURL := "http://" + username + ":" + password + "@" + redisTestServer.Addr()
	_, err = NewRedisCache("", malformURL, "")
	require.Error(t, err)
	malformURL = "redis://" + username + ":" + "wrongpass" + "@" + redisTestServer.Addr()
	_, err = NewRedisCache("", malformURL, "")
	require.Error(t, err)
}

func TestCheckAndSetLastSlotAndHashDelivered(t *testing.T) {
	cache := setupTestRedis(t)
	newSlot := uint64(123)
	newHash := "0x0000000000000000000000000000000000000000000000000000000000000000"

	// should return redis.Nil if wasn't set
	slot, err := cache.GetLastSlotDelivered(context.Background(), cache.NewPipeline())
	require.ErrorIs(t, err, redis.Nil)
	require.Equal(t, uint64(0), slot)

	// should be able to set once
	err = cache.CheckAndSetLastSlotAndHashDelivered(newSlot, newHash)
	require.NoError(t, err)

	// should get slot
	slot, err = cache.GetLastSlotDelivered(context.Background(), cache.NewPipeline())
	require.NoError(t, err)
	require.Equal(t, newSlot, slot)

	// should get hash
	hash, err := cache.GetLastHashDelivered()
	require.NoError(t, err)
	require.Equal(t, newHash, hash)

	// should fail on a different payload (mismatch block hash)
	differentHash := "0x0000000000000000000000000000000000000000000000000000000000000001"
	err = cache.CheckAndSetLastSlotAndHashDelivered(newSlot, differentHash)
	require.ErrorIs(t, err, ErrAnotherPayloadAlreadyDeliveredForSlot)

	// should not return error for same hash
	err = cache.CheckAndSetLastSlotAndHashDelivered(newSlot, newHash)
	require.NoError(t, err)

	// should also fail on earlier slots
	err = cache.CheckAndSetLastSlotAndHashDelivered(newSlot-1, newHash)
	require.ErrorIs(t, err, ErrPastSlotAlreadyDelivered)
}

// Test_CheckAndSetLastSlotAndHashDeliveredForTesting ensures the optimistic locking works
// i.e. running CheckAndSetLastSlotAndHashDelivered leading to err == redis.TxFailedErr
func Test_CheckAndSetLastSlotAndHashDeliveredForTesting(t *testing.T) {
	cache := setupTestRedis(t)
	newSlot := uint64(123)
	hash := "0x0000000000000000000000000000000000000000000000000000000000000000"
	n := 3

	errC := make(chan error, n)
	waitC := make(chan bool, n)
	syncWG := sync.WaitGroup{}

	// Kick off goroutines, that will all try to set the same slot
	for i := 0; i < n; i++ {
		syncWG.Add(1)
		go func() {
			errC <- _CheckAndSetLastSlotAndHashDeliveredForTesting(cache, waitC, &syncWG, newSlot, hash)
		}()
	}

	syncWG.Wait()

	// Continue first goroutine (should succeed)
	waitC <- true
	err := <-errC
	require.NoError(t, err)

	// Continue all other goroutines (all should return the race error redis.TxFailedErr)
	for i := 1; i < n; i++ {
		waitC <- true
		err := <-errC
		require.ErrorIs(t, err, redis.TxFailedErr)
	}

	// Any later call with a different hash should return ErrPayloadAlreadyDeliveredForSlot
	differentHash := "0x0000000000000000000000000000000000000000000000000000000000000001"
	err = _CheckAndSetLastSlotAndHashDeliveredForTesting(cache, waitC, &syncWG, newSlot, differentHash)
	waitC <- true
	require.ErrorIs(t, err, ErrAnotherPayloadAlreadyDeliveredForSlot)
}

func _CheckAndSetLastSlotAndHashDeliveredForTesting(r *RedisCache, waitC chan bool, wg *sync.WaitGroup, slot uint64, hash string) (err error) {
	// copied from redis.go, with added channel and waitgroup to test the race condition in a controlled way
	txf := func(tx *redis.Tx) error {
		lastSlotDelivered, err := tx.Get(context.Background(), r.keyLastSlotDelivered).Uint64()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}

		if slot < lastSlotDelivered {
			return ErrPastSlotAlreadyDelivered
		}

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

		wg.Done()
		<-waitC

		_, err = tx.TxPipelined(context.Background(), func(pipe redis.Pipeliner) error {
			pipe.Set(context.Background(), r.keyLastSlotDelivered, slot, 0)
			pipe.Set(context.Background(), r.keyLastHashDelivered, hash, 0)
			return nil
		})

		return err
	}

	return r.client.Watch(context.Background(), txf, r.keyLastSlotDelivered)
}

func TestPipelineNilCheck(t *testing.T) {
	cache := setupTestRedis(t)
	f, err := cache.GetFloorBidValue(context.Background(), cache.NewPipeline(), 0, "1", "2")
	require.NoError(t, err)
	require.Equal(t, big.NewInt(0), f)
}

func TestSetTobTxValue(t *testing.T) {
	cache := setupTestRedis(t)

	slot := uint64(123)
	parentHash := "0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747"

	// Set a value
	value := big.NewInt(123)
	_, err := cache.client.TxPipelined(context.Background(), func(tx redis.Pipeliner) error {
		v, err := cache.GetTobTxValue(context.Background(), tx, slot, parentHash)
		require.NoError(t, err)
		require.Equal(t, v, big.NewInt(0))

		err = cache.SetTobTxValue(context.Background(), tx, value, slot, parentHash)
		require.NoError(t, err)

		// Get the value back
		v, err = cache.GetTobTxValue(context.Background(), tx, slot, parentHash)
		require.NoError(t, err)
		require.Equal(t, v, big.NewInt(123))

		return nil
	})

	require.NoError(t, err)
}
