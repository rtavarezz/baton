package seq

import (
	"github.com/AnomalyFi/hypersdk/chain"
	"github.com/AnomalyFi/hypersdk/pubsub"
	hrpc "github.com/AnomalyFi/hypersdk/rpc"
	srpc "github.com/AnomalyFi/nodekit-seq/rpc"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ethereum/go-ethereum/log"
	"sync"
	"time"
)

import (
	"context"
	"github.com/AnomalyFi/hypersdk/crypto/ed25519"
)

type Config struct {
	PrivateKey ed25519.PrivateKey // signer hex

	URL       string
	ChainID   ids.ID
	NetworkID uint32
}

type SeqClient struct {
	srpc  *srpc.JSONRPCClient
	hrpc  *hrpc.JSONRPCClient
	wsCli *hrpc.WebSocketClient

	signer ed25519.PrivateKey

	blockHead  *chain.StatefulBlock
	blockHeadL sync.Mutex

	proposer  *hrpc.NextProposerReply
	proposerL sync.Mutex

	// ETH Chain related
	Namespace []byte // ChainID bytes

	// SEQ related
	parser         chain.Parser
	ChainID        ids.ID
	NetworkID      uint32
	NewSlotHandler func(uint64)

	stop chan struct{}
}

func (cli *SeqClient) SetNewSlotHandler(handler func(uint64)) {
	cli.NewSlotHandler = handler
}

func NewSeqClient(config *Config) (*SeqClient, error) {
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
				// trigger callback
				client.NewSlotHandler(client.blockHead.Hght)
				client.blockHeadL.Unlock()

				// query next proposer on receiving a new block, this save us time while we do compuating during round trip
				nextProposer, err := client.nextProposer(ctx)
				client.proposerL.Lock()
				if err != nil {
					client.proposer = nil // set next proposer to nil to notify the ToB block built on top of this is invalid
					log.Error("unable to fetch next proposer", "err", err)
				} else {
					client.proposer = nextProposer
				}
				log.Info("setting proposer", "proposer", nextProposer.NodeID.String())
				client.proposerL.Unlock()
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
	s.proposerL.Lock()
	defer s.proposerL.Unlock()

	return s.proposer
}

func (s *SeqClient) nextProposer(ctx context.Context) (*hrpc.NextProposerReply, error) {
	return s.hrpc.NextProposer(ctx)
}

//func (s *SeqClient) GenerateSeqTxsFromEthRaws(ctx context.Context, ethTxs []hexutil.Bytes) ([]*chain.Transaction, error) {
//	if len(s.Namespace) == 0 {
//		return nil, fmt.Errorf("namespace not set yet")
//	}
//
//	parser := s.parser
//
//	unitPrices, err := s.hrpc.UnitPrices(ctx, true)
//	if err != nil {
//		return nil, err
//	}
//
//	pubkey := s.signer.PublicKey()
//	rsender := auth.NewED25519Address(pubkey)
//
//	acts := make([]chain.Action, 0, len(ethTxs))
//	for _, ethTx := range ethTxs {
//		if len(ethTx) == 0 {
//			return nil, fmt.Errorf("provided a empty eth tx")
//		}
//		action := actions.SequencerMsg{
//			ChainID:     s.Namespace,
//			Data:        ethTx,
//			FromAddress: rsender,
//			RelayerID:   0,
//		}
//		acts = append(acts, &action)
//	}
//
//	now := time.Now().UnixMilli()
//	authFactory := auth.NewED25519Factory(s.signer)
//
//	actionRegistry, authRegistry := parser.Registry()
//
//	txs := make([]*chain.Transaction, 0, len(ethTxs))
//	for _, act := range acts {
//		maxUnits, _, err := chain.EstimateUnits(parser.Rules(now), []chain.Action{act}, authFactory)
//		if err != nil {
//			return nil, err
//		}
//		maxFee, err := fees.MulSum(unitPrices, maxUnits)
//		if err != nil {
//			return nil, err
//		}
//		base := &chain.Base{
//			Timestamp: utils.UnixRMilli(now, parser.Rules(now).GetValidityWindow()),
//			ChainID:   s.ChainID,
//			MaxFee:    maxFee,
//		}
//
//		tx := chain.NewTx(base, []chain.Action{act})
//		tx, err = tx.Sign(authFactory, actionRegistry, authRegistry)
//		if err != nil {
//			return nil, err
//		}
//
//		txs = append(txs, tx)
//	}
//
//	return txs, nil
//}

//func (s *SeqClient) GenerateTransferTx(ctx context.Context, to codec.Address, value uint64, memo []byte) (*chain.Transaction, error) {
//	if len(s.Namespace) == 0 {
//		return nil, fmt.Errorf("namespace not set yet")
//	}
//
//	parser := s.parser
//	act := &actions.Transfer{
//		To:    to,
//		Value: value,
//		Memo:  memo,
//	}
//
//	unitPrices, err := s.hrpc.UnitPrices(ctx, true)
//	if err != nil {
//		return nil, err
//	}
//
//	now := time.Now().UnixMilli()
//	authFactory := auth.NewED25519Factory(s.signer)
//	actionRegistry, authRegistry := parser.Registry()
//
//	maxUnits, _, err := chain.EstimateUnits(parser.Rules(now), []chain.Action{act}, authFactory)
//	if err != nil {
//		return nil, err
//	}
//	maxFee, err := fees.MulSum(unitPrices, maxUnits)
//	if err != nil {
//		return nil, err
//	}
//	base := &chain.Base{
//		Timestamp: utils.UnixRMilli(now, parser.Rules(now).GetValidityWindow()),
//		ChainID:   s.ChainID,
//		MaxFee:    maxFee,
//	}
//	tx := chain.NewTx(base, []chain.Action{act})
//	tx, err = tx.Sign(authFactory, actionRegistry, authRegistry)
//	if err != nil {
//		return nil, err
//	}
//	return tx, nil
//
//}
