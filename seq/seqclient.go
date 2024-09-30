package seq

import (
	"context"
	"sync"
	"time"

	"github.com/AnomalyFi/hypersdk/chain"
	"github.com/AnomalyFi/hypersdk/crypto/ed25519"
	"github.com/AnomalyFi/hypersdk/pubsub"
	hrpc "github.com/AnomalyFi/hypersdk/rpc"
	srpc "github.com/AnomalyFi/nodekit-seq/rpc"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ethereum/go-ethereum/log"
)

type SeqClientConfig struct {
	PrivateKey ed25519.PrivateKey // signer hex

	URL       string
	ChainID   ids.ID
	NetworkID uint32
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
}

type SeqClient struct {
	srpc  *srpc.JSONRPCClient
	hrpc  *hrpc.JSONRPCClient
	wsCli *hrpc.WebSocketClient

	signer ed25519.PrivateKey

	blockHead  *chain.StatefulBlock
	blockHeadL sync.Mutex

	proposerInfo  *hrpc.NextProposerReply
	proposerInfoL sync.Mutex

	// ETH Chain related
	Namespace []byte // ChainID bytes

	// SEQ related
	parser            chain.Parser
	ChainID           ids.ID
	NetworkID         uint32
	onNewBlockHandler func(*chain.StatefulBlock, *hrpc.NextProposerReply)

	stop chan struct{}
}

func NewSeqClient(config *SeqClientConfig) (*SeqClient, error) {
	log.Info("initializing SEQ", "url", config.URL, "chainID", config.ChainID, "sk", config.PrivateKey)
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

	client := SeqClient{
		srpc:   scli,
		hrpc:   hcli,
		wsCli:  wsCli,
		signer: config.PrivateKey,

		Namespace: nil,

		parser:    parser,
		ChainID:   config.ChainID,
		NetworkID: uint32(config.NetworkID),

		stop: stopSig,
	}

	// keep track of head of SEQ, this is used for calculating `Slot` in SubmitBlock to Baton
	go func() {
		ctx := context.Background()
		for {
			select {
			case <-stopSig:
				log.Info("stopping as receiving stop signal")
				return
			default:
				bctx, cancel := context.WithTimeout(ctx, 1*time.Second)
				blk, _, _, _, err := wsCli.ListenBlock(bctx, parser)
				cancel()
				if err != nil {
					log.Error("unable to listen block", "err", err)
					continue
				}

				client.blockHeadL.Lock()
				client.blockHead = blk
				client.blockHeadL.Unlock()

				// query next proposer on receiving a new block, this save us time while we do compuating during round trip
				nextProposer, err := client.nextProposer(bctx)

				client.proposerInfoL.Lock()
				if err != nil {
					client.proposerInfo = nil // set next proposer to nil to notify the ToB block built on top of this is invalid
					log.Error("unable to fetch next proposer", "err", err)
				} else {
					client.proposerInfo = nextProposer
				}
				client.proposerInfoL.Unlock()
				log.Info("setting proposer", "proposer", nextProposer.NodeID.String())

				go client.onNewBlockHandler(blk, nextProposer)
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
	s.proposerInfoL.Lock()
	defer s.proposerInfoL.Unlock()

	return s.proposerInfo
}

func (s *SeqClient) nextProposer(ctx context.Context) (*hrpc.NextProposerReply, error) {
	return s.hrpc.NextProposer(ctx)
}

func (s *SeqClient) CurrentValidators(ctx context.Context) []*hrpc.Validator {
	s.proposerInfoL.Lock()
	defer s.proposerInfoL.Unlock()

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
