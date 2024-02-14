package api

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/attestantio/go-eth2-client/spec/capella"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/flashbots/go-utils/cli"
	"github.com/flashbots/go-utils/jsonrpc"
	"github.com/flashbots/mev-boost-relay/common"
	"github.com/flashbots/mev-boost-relay/contracts"
	"github.com/flashbots/mev-boost-relay/services/api/hashes"
	"github.com/flashbots/mev-boost-relay/services/api/incremental"

	ethbind "github.com/ethereum/go-ethereum/accounts/abi/bind"
	//"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var assemblyRequestTimeout = time.Duration(cli.GetEnvInt("BLOCKASSEMBLY_TIMEOUT_MS", 60000)) * time.Millisecond

type IBlockAssembler interface {
	Send(context context.Context, request *common.BlockAssemblerRequest) (*capella.ExecutionPayload, error, error)
}

type BlockAssembler struct {
	cv               *sync.Cond
	counter          int64
	blockAssemblyURL string
	client           http.Client
}

func NewBlockAssembler(blockAssemblyURL string) *BlockAssembler {
	return &BlockAssembler{
		cv:               sync.NewCond(&sync.Mutex{}),
		blockAssemblyURL: blockAssemblyURL,
		client: http.Client{ //nolint:exhaustruct
			Timeout: assemblyRequestTimeout,
		},
	}
}

// TODO should be able to specify ChainId here and have some kind of enum so we can send it off to different clients for different types of simulations
// Only debate is whether that should be in here or have some kind of middleware reverse proxy like dshackle instead
// It'll probably be within this method to start and then change in the future
func (b *BlockAssembler) Send(context context.Context, request *common.BlockAssemblerRequest) (payload *capella.ExecutionPayload, requestErr, validationErr error) {
	b.cv.L.Lock()
	cnt := atomic.AddInt64(&b.counter, 1)
	if maxConcurrentBlocks > 0 && cnt > maxConcurrentBlocks {
		b.cv.Wait()
	}
	b.cv.L.Unlock()

	defer func() {
		b.cv.L.Lock()
		atomic.AddInt64(&b.counter, -1)
		b.cv.Signal()
		b.cv.L.Unlock()
	}()

	if err := context.Err(); err != nil {
		return nil, fmt.Errorf("%w, %w", ErrRequestClosed, err), nil
	}

	var assembleReq *jsonrpc.JSONRPCRequest
	if request.RobPayload.Capella == nil {
		return nil, ErrNoCapellaPayload, nil
	}

	// Prepare headers
	headers := http.Header{}
	headers.Add("X-Request-ID", fmt.Sprintf("%d/%s", request.RobPayload.Slot(), request.RobPayload.BlockHash()))

	// Create and fire off JSON-RPC request
	assembleReq = jsonrpc.NewJSONRPCRequest("1", "flashbots_blockAssembler", request)
	resp, requestErr, validationErr := SendJSONRPCRequest(&b.client, *assembleReq, b.blockAssemblyURL, headers)

	// decode the response to engine.ExecutionPayloadEnvelope
	if resp != nil {
		payload = &capella.ExecutionPayload{}
		err := json.Unmarshal(resp.Result, payload)
		if err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err), nil
		}
	}

	return payload, requestErr, validationErr
}

// TODO we want to take the first half of the bundle and simulate it on the first rollup.
// Then we use the data we get from that to create a new transaction on the second rollup.
// Both transactions end up included in the TOB atomic bundle
func HyperlaneDispatch(remoteDomain uint32, recipient ethcommon.Address, body []byte, vm_id int64, priv *ecdsa.PrivateKey) error {
	//TODO create Enums for contract addresses and vm ids eventually. It'll simplify adding more chains

	gethAddr := "https://devnet.nodekit.xyz"

	contractAddr := "0x"

	conn, err := ethclient.Dial(gethAddr)

	if err != nil {
		return err
	}
	// remoteDomain := 2

	//recipient := ethcommon.HexToAddress("0x")

	//body := make([]byte, 0)

	//vm_id := int64(1)

	// p := "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

	// priv, err := crypto.HexToECDSA(p)
	// if err != nil {
	// 	fmt.Println(fmt.Errorf("Failed to convert from hex to ECDSA: %v", err))
	// 	return err
	// }

	auth, err := ethbind.NewKeyedTransactorWithChainID(priv, big.NewInt(vm_id))

	if err != nil {
		fmt.Println(fmt.Errorf("Failed to create authorized transactor: %v", err))
		return err
	}

	//Added to prevent sending to mempool
	auth.NoSend = true

	mailbox, err := contracts.NewMailbox(ethcommon.HexToAddress(contractAddr), conn)

	quote, err := mailbox.QuoteDispatch0(&ethbind.CallOpts{Pending: true}, uint32(remoteDomain), [32]byte(recipient.Bytes()), body)

	if err != nil {
		return err
	}

	auth.Value = quote

	id, err := mailbox.Dispatch1(auth, uint32(remoteDomain), [32]byte(recipient.Bytes()), body)

	if err != nil {
		return err
	}

	fmt.Println(id)

	merkle, err := contracts.NewMerkleTreeHook(ethcommon.HexToAddress(contractAddr), conn)

	//TODO new to announce the validator first before running this program

	//TODO make these configurable
	i := uint64(1001)
	f := &ethbind.FilterOpts{Start: 1000, End: &i}

	iter, err := merkle.FilterInsertedIntoTree(f)

	inserts := make([]*incremental.MerkleTreeInsertion, 0)

	incrementalTree := incremental.NewIncrementalMerkle()

	for iter.Next() {
		temp := &incremental.MerkleTreeInsertion{Leaf_index: iter.Event.Index, Message_id: iter.Event.MessageId}

		inserts = append(inserts, temp)

		incrementalTree.Ingest(iter.Event.MessageId)

		test := hashes.H256{0}

		rollup_id := uint32(101)

		c := incremental.Checkpoint{
			Root:                     incrementalTree.Root(),
			Index:                    incrementalTree.Index(),
			Merkle_tree_hook_address: test,
			Mailbox_domain:           rollup_id,
		}

		cm := incremental.CheckpointWithMessageId{
			Checkpoint: c,
			Message_id: iter.Event.MessageId,
		}

		//TODO sign and submit the CheckpointWithMessageId to the Mailbox on the destination chain
	}

	//TODO change Merkle tree hook address to not be of type bytes

	//TODO the above is all we basically need for the relayer except we also need to simulate this transaction but that happens in Javelin instead.
	//Also we need some way of getting the MerkleTree but that should come from simulating the first transaction on rollup 1
	// We'll use that to create the second transaction and simulate on the second rollup

	return nil

}
