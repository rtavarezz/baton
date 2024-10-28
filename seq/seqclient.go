package seq

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"sync"
	"time"

	"github.com/AnomalyFi/hypersdk/chain"
	"github.com/AnomalyFi/hypersdk/crypto/ed25519"
	"github.com/AnomalyFi/hypersdk/pubsub"
	hrpc "github.com/AnomalyFi/hypersdk/rpc"
	srpc "github.com/AnomalyFi/nodekit-seq/rpc"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/sirupsen/logrus"
)

type SeqClientConfig struct {
	PrivateKey ed25519.PrivateKey // signer hex

	URL       string
	ChainID   ids.ID
	NetworkID uint32

	BlockWaitTime time.Duration

	Log *logrus.Entry
}

type BaseSeqClient interface {
	GetChainID() ids.ID
	GetNetworkID() uint32

	SeqHead() *chain.StatefulBlock
	NamespaceExists() bool
	SetNamespace(namespace []byte)
	NextProposer(ctx context.Context) *hrpc.NextProposerReply
	CurrentValidators(ctx context.Context) []*hrpc.Validator
	SetOnNewBlockHandler(handler func(*chain.StatefulBlock, *hrpc.NextProposerReply))
	GetBuilder() []byte
	MarkEpochExit() uint64
	IsEpochExited() uint64
}

type SeqClient struct {
	srpc  *srpc.JSONRPCClient
	hrpc  *hrpc.JSONRPCClient
	wsCli *hrpc.WebSocketClient

	signer ed25519.PrivateKey

	blockHead    *chain.StatefulBlock
	proposerInfo *hrpc.NextProposerReply
	blockHeadL   sync.Mutex

	// ETH Chain related
	Namespace []byte // ChainID bytes

	// SEQ related
	parser            chain.Parser
	ChainID           ids.ID
	NetworkID         uint32
	onNewBlockHandler func(*chain.StatefulBlock, *hrpc.NextProposerReply)

	logger        *logrus.Entry
	stop          chan struct{}
	BuilderPubKey []byte
	epochNum      uint64
	epochMap      *ShardedMap[uint64, int]
	epochL        sync.Mutex
}

func NewSeqClient(config *SeqClientConfig) (*SeqClient, error) {
	config.Log.WithFields(logrus.Fields{
		"url":           config.URL,
		"chainID":       config.ChainID,
		"sk":            config.PrivateKey,
		"blockWaitTime": config.BlockWaitTime,
	}).Info("initializing SEQ")
	hcli := hrpc.NewJSONRPCClient(config.URL)
	scli := srpc.NewJSONRPCClient(config.URL, config.NetworkID, config.ChainID)
	parser, err := scli.Parser(context.TODO())
	if err != nil {
		return nil, err
	}

	wsCli, err := hrpc.NewWebSocketClient(config.URL, hrpc.DefaultHandshakeTimeout, pubsub.MaxPendingMessages, pubsub.MaxReadMessageSize)
	if err != nil {
		return nil, err
	}
	if err := wsCli.RegisterBlocks(); err != nil {
		return nil, err
	}

	stopSig := make(chan struct{})
	logger := config.Log
	hashFn := func(key uint64) uint64 { return key }
	epochMap := NewShardedMap[uint64, int](1024, 16, hashFn)

	client := SeqClient{
		srpc:   scli,
		hrpc:   hcli,
		wsCli:  wsCli,
		signer: config.PrivateKey,

		Namespace: nil,

		parser:    parser,
		ChainID:   config.ChainID,
		NetworkID: uint32(config.NetworkID),

		logger:        logger,
		stop:          stopSig,
		BuilderPubKey: nil,
		epochMap:      epochMap,
	}

	// keep track of head of SEQ, this is used for calculating `Slot` in SubmitBlock to Baton
	go func() {
		ctx := context.Background()
		for {
			select {
			case <-stopSig:
				logger.Info("stopping as receiving stop signal")
				return
			default:
				bctx, cancel := context.WithTimeout(ctx, config.BlockWaitTime)
				blk, _, _, _, err := wsCli.ListenBlock(bctx, parser)
				if err != nil {
					logger.Error("unable to listen block", "err", err)
					cancel()
					continue
				}

				// release the lock after duty map is updated
				client.blockHeadL.Lock()
				client.blockHead = blk
				if blk.Hght%6 == 0 {
					client.epochL.Lock()
					client.epochNum += 1
					client.BuilderPubKey = client.GetBuilder()
					client.epochL.Unlock()
				}

				// query next proposer on receiving a new block, this save us time while we do compuating during round trip
				start := time.Now()
				nextProposer, err := client.nextProposer(bctx, blk.Hght+1)
				if err != nil {
					client.proposerInfo = nil // set next proposer to nil to notify the ToB block built on top of this is invalid
					logger.WithError(err).Error("unable to fetch next proposer")
					cancel()
					client.blockHeadL.Unlock()
					continue
				} else {
					client.proposerInfo = nextProposer
				}
				logger.WithField("elapsed", time.Since(start).Milliseconds()).Debug("next proposer guage")

				logger.Info("setting proposer", "proposer", nextProposer.NodeID.String(), "pubkey", hexutil.Encode(nextProposer.PublicKey))

				client.onNewBlockHandler(blk, nextProposer)

				cancel()
				client.blockHeadL.Unlock()
			}
		}
	}()

	return &client, nil
}

func (s *SeqClient) SeqHead() *chain.StatefulBlock {
	s.blockHeadL.Lock()
	defer s.blockHeadL.Unlock()

	return s.blockHead
}

func (s *SeqClient) NamespaceExists() bool {
	return len(s.Namespace) != 0
}

func (s *SeqClient) SetNamespace(namespace []byte) {
	s.Namespace = namespace
}

func (s *SeqClient) NextProposer(ctx context.Context) *hrpc.NextProposerReply {
	s.blockHeadL.Lock()
	defer s.blockHeadL.Unlock()

	return s.proposerInfo
}

func (s *SeqClient) nextProposer(ctx context.Context, height uint64) (*hrpc.NextProposerReply, error) {
	nextProposer, err := s.hrpc.NextProposer(ctx, height)
	if err != nil {
		return nil, err
	}
	return nextProposer, nil
}

func (s *SeqClient) CurrentValidators(ctx context.Context) []*hrpc.Validator {
	s.blockHeadL.Lock()
	defer s.blockHeadL.Unlock()

	if s.proposerInfo != nil {
		return s.proposerInfo.Validators
	}

	return nil
}

func (s *SeqClient) GetNetworkID() uint32 {
	return s.NetworkID
}

func (s *SeqClient) GetChainID() ids.ID {
	return s.ChainID
}

func (s *SeqClient) SetOnNewBlockHandler(handler func(*chain.StatefulBlock, *hrpc.NextProposerReply)) {
	s.onNewBlockHandler = handler
}

func (s *SeqClient) GetBuilder() []byte {
	s.epochL.Lock()
	defer s.epochL.Unlock()

	return s.BuilderPubKey
}

func (s *SeqClient) MarkEpochExit(epoch uint64) {
	s.epochL.Lock()
	defer s.epochL.Unlock()

	count, ok := s.epochMap.Get(epoch)
	if ok {
		s.epochMap.Put(epoch, count+1)
	} else {
		s.epochMap.Put(epoch, 1)
	}
}

func (s *SeqClient) IsEpochExited(epoch uint64) int {
	s.epochL.Lock()
	defer s.epochL.Unlock()

	count, ok := s.epochMap.Get(epoch)
	if ok {
		return count
	}

	return 0
}

func (s *SeqClient) CurrentEpoch() uint64 {
	s.epochL.Lock()
	defer s.epochL.Unlock()

	return s.epochNum
}

func (s *SeqClient) CheckCache(epoch uint64) bool {
	_, ok := s.epochMap.Get(epoch)
	return ok
}

// Updated version for checking ahead of time, assuming checkEpoch is the epoch to check for
func (s *SeqClient) IsExited(checkEpoch uint64) (bool, error) {
	if checkEpoch < 2 {
		return false, fmt.Errorf("Cannot check epoch. Allowed exits only at n >= 2(epoch - n.")
	}
	ctx := context.Background()

	s.epochL.Lock()
	defer s.epochL.Unlock()
	currEpoch := s.CurrentEpoch()

	for i := uint64(2); i <= checkEpoch; i++ {
		checkEpoch = currEpoch - i
		if s.CheckCache(checkEpoch) {
			return true, nil
		}

		info, err := s.srpc.GetEpochExits(ctx, currEpoch)
		if err != nil {
			log.Error("unable to get epoch exits", "err", err)
			return false, err
		}

		for _, exit := range info.Exits {
			if bytes.Equal(exit.Namespace, s.Namespace) && currEpoch == exit.Epoch {
				return true, nil
			}
		}
	}

	return false, nil
}
