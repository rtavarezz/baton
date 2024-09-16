package datastore

import (
	"context"
	"errors"
	"math/big"

	"github.com/go-redis/redis/v9"
)

// BuilderBids supports redis.SaveBidAndUpdateTopBid
type BuilderBids struct {
	bidValues map[string]*big.Int
}

func NewBuilderToBBidsFromRedis(ctx context.Context, r *RedisCache, pipeliner redis.Pipeliner, slot uint64, parentHash, proposerPubkey string) (*BuilderBids, error) {
	keyBidValues := r.keyBlockBuilderLatestToBBidsValue(slot, parentHash, proposerPubkey)
	c := pipeliner.HGetAll(ctx, keyBidValues)
	_, err := pipeliner.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	bidValueMap, err := c.Result()
	if err != nil {
		return nil, err
	}
	return NewBuilderBids(bidValueMap), nil
}

func (b *BuilderBids) getTopToBBid() (string, *big.Int) {
	topBidBuilderPubkey := ""
	topBidValue := big.NewInt(0)
	for builderPubkey, bidValue := range b.bidValues {
		if bidValue.Cmp(topBidValue) > 0 {
			topBidValue = bidValue
			topBidBuilderPubkey = builderPubkey
		}
	}
	return topBidBuilderPubkey, topBidValue
}

func NewToBBuilderBidsFromRedis(ctx context.Context, r *RedisCache, pipeline redis.Pipeliner, slot uint64, parentHash, proposerPubkey string) (*BuilderBids, error) {
	keyBidValues := r.keyBlockBuilderLatestToBBidsValue(slot, parentHash, proposerPubkey)
	c := pipeline.HGetAll(ctx, keyBidValues)
	_, err := pipeline.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	bidValueMap, err := c.Result()
	if err != nil {
		return nil, err
	}
	return NewBuilderBids(bidValueMap), nil
}

func NewRoBBuilderBidsFromRedis(ctx context.Context, r *RedisCache, pipeline redis.Pipeliner, slot uint64, parentHash, proposerPubkey string, chainID string) (*BuilderBids, error) {
	keyBidValues := r.keyBlockBuilderLatestRoBBidsValue(slot, parentHash, proposerPubkey, chainID)
	c := pipeline.HGetAll(ctx, keyBidValues)
	_, err := pipeline.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	bidValueMap, err := c.Result()
	if err != nil {
		return nil, err
	}
	return NewBuilderBids(bidValueMap), nil
}

func NewBuilderBids(bidValueMap map[string]string) *BuilderBids {
	b := BuilderBids{
		bidValues: make(map[string]*big.Int),
	}
	for builderPubkey, bidValue := range bidValueMap {
		b.bidValues[builderPubkey] = new(big.Int)
		b.bidValues[builderPubkey].SetString(bidValue, 10)
	}
	return &b
}

func (b *BuilderBids) getTopBid() (string, *big.Int) {
	topBidBuilderPubkey := ""
	topBidValue := big.NewInt(0)
	for builderPubkey, bidValue := range b.bidValues {
		if bidValue.Cmp(topBidValue) > 0 {
			topBidValue = bidValue
			topBidBuilderPubkey = builderPubkey
		}
	}
	return topBidBuilderPubkey, topBidValue
}
