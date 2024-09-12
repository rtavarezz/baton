package api

import (
	"encoding/hex"
	"fmt"
	"github.com/AnomalyFi/hypersdk/chain"
	"github.com/AnomalyFi/hypersdk/codec"
	"github.com/AnomalyFi/hypersdk/crypto/ed25519"
	"github.com/AnomalyFi/nodekit-seq/actions"
	"github.com/AnomalyFi/nodekit-seq/auth"
	_ "github.com/AnomalyFi/nodekit-seq/auth"
	srpc "github.com/AnomalyFi/nodekit-seq/rpc"
	"github.com/ava-labs/avalanchego/ids"
	eth "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	boostTypes "github.com/flashbots/go-boost-utils/types"
	"github.com/flashbots/mev-boost-relay/common"
	"github.com/stretchr/testify/require"
	"log"
	"math/big"
	"testing"
	"time"
)

const (
	TestAddress         = "0xDEADBEAF"
	TestPrivateKeyValue = "77619a19a837f894fa5c90e58ee3e3d69e382936d323d987bbde923da92a5ac5"
	TestAddressValue    = "0x59131f2c045f70Be0dDA50D86b6ED2b18C5012cf"
	TestRelayerID       = 111
	TestMaxFee          = 100000
)

var TestProposerPayment codec.Address
var testNonce uint64

const KEYHEX = "323b1d8f4eed5f0da9da93071b034f2dce9d2d22692c172f3cb252a64ddfafd01b057de320297c29ad0c1f589ea216869cf1938d88c9fbd70d6748323dbf2fa7"

func init() {
	testAddr := "0x59131f2c045f70Be0dDA"
	copy(TestProposerPayment[:], testAddr)
	testNonce = 1
}

func GetNextTestNonce() uint64 {
	nonce := testNonce
	testNonce++
	return nonce
}

type CreateTestBlockSubmissionOpts struct {
	Slot           uint64
	ParentHash     string
	BuilderPubkey  string
	ProposerPubkey boostTypes.PublicKey
	IsToB          bool
	robChainIndex  int // only used if isTob false
	numTxs         int

	//relaySk        bls.SecretKey
	//relayPk        types.PublicKey
	//domain         types.Domain
}

// @TODO: Expand for ToB
func CreateTestChunkSubmission(
	t *testing.T,
	value uint64,
	opts *CreateTestBlockSubmissionOpts,
) (*common.SubmitNewBlockRequest,
	*common.AnchorHeader,
	*common.AnchorPayload,
	error) {
	t.Helper()
	var err error

	slot := opts.Slot
	proposerPk := boostTypes.PublicKey{}
	parentHash := eth.Hash{}
	builderPubkey := boostTypes.PublicKey{}
	chainIndex := 1

	numTxs := 1

	if opts != nil {
		slot = opts.Slot
		chainIndex = opts.robChainIndex

		numTxs = opts.numTxs
		copy(builderPubkey[:], opts.BuilderPubkey)

		if opts.ProposerPubkey.String() != "" {
			proposerPk = opts.ProposerPubkey
		}

		if opts.ParentHash != "" {
			copy(parentHash[:], opts.ParentHash)
		}
	}

	txs := []*chain.Transaction{}
	chainID := GetTestChainId(chainIndex)

	for i := 0; i < numTxs; i++ {
		nonce := GetNextTestNonce()
		val := big.NewInt(int64(100 * i))
		gasLimit := uint64(10000000 + i)
		gasPrice := big.NewInt(int64(10000 + i))

		ethTx := CreateTestEthTransactionAsTxBytes(nonce, *val, gasLimit, *gasPrice, "")
		tx := CreateHypersdkTx(chainID, ethTx)
		txs = append(txs, tx)
	}

	transferAction := CreateTestProposerTransfer(chainID, value)
	txs = append(txs, transferAction)

	blockReq := common.NewSubmitNewBlockRequest()
	blockReq.BuilderPubKey = builderPubkey
	blockReq.Chunk.Slot = slot
	blockReq.Chunk.ParentHash = parentHash
	blockReq.Chunk.ProposerPubkey = proposerPk
	copy(blockReq.Chunk.ProposerPayment[:], TestAddress[:])

	//txsBytes, err := json.Marshal(txs)
	//var signer ed25519.PrivateKey
	txsBytes, err := chain.MarshalTxs(txs)
	if err != nil {
		return nil, nil, nil, err
	}

	blockReq.Chunk.Txs = txsBytes

	anchorHeader, err := BuildHeader(&blockReq)
	require.NoError(t, err)

	anchorPayload, err := BuildPayload(&blockReq, txs)
	require.NoError(t, err)

	return &blockReq, &anchorHeader, anchorPayload, nil
}

func GetTestChainId(i int) string {
	return fmt.Sprintf("test-chain-%d", i)
}

func CreateHypersdkTx(chainID string, ethTx []byte) *chain.Transaction {
	seqMsg := actions.SequencerMsg{
		ChainId:     []byte(chainID),
		Data:        ethTx,
		FromAddress: TestProposerPayment,
		RelayerID:   TestRelayerID,
	}
	//ids := make([]ids.ID, 32)
	var id ids.ID
	copy(id[:], seqMsg.ChainId)
	var base = chain.Base{
		Timestamp: time.Now().UnixMilli(),
		ChainID:   id,
		MaxFee:    TestMaxFee,
	}
	base.Timestamp = int64(time.Now().Second() * 1000)
	pkBytes, err := hex.DecodeString(KEYHEX)
	pk := ed25519.PrivateKey(pkBytes)
	authFactory := auth.NewED25519Factory(pk)
	actionList := []chain.Action{&seqMsg}
	tx := chain.NewTx(&base, actionList)
	var parser = srpc.Parser{}
	actionRegistry, authRegistry := parser.Registry()
	txSign, err := tx.Sign(authFactory, actionRegistry, authRegistry)
	if err != nil {
		panic(err)
	}
	return txSign
}

func CreateTestProposerTransfer(chainID string, value uint64) *chain.Transaction {
	transfer := actions.Transfer{
		To:    TestProposerPayment,
		Value: value,
	}
	var id ids.ID
	copy(id[:], chainID)
	base := chain.Base{
		Timestamp: time.Now().UnixMilli(),
		ChainID:   id,
		MaxFee:    TestMaxFee,
	}
	base.Timestamp = int64(time.Now().Second() * 1000)
	pkBytes, err := hex.DecodeString(KEYHEX)
	pk := ed25519.PrivateKey(pkBytes)
	authFactory := auth.NewED25519Factory(pk)
	var parser = srpc.Parser{}
	actionRegistry, authRegistry := parser.Registry()
	actionList := []chain.Action{&transfer}
	tx := chain.NewTx(&base, actionList)
	txSign, err := tx.Sign(authFactory, actionRegistry, authRegistry)
	if err != nil {
		panic(err)
	}
	return txSign
}

func CreateTestEthTransaction(nonce uint64, value big.Int, gasLimit uint64, gasPrice big.Int, data string) *types.Transaction {
	toAddress := eth.HexToAddress(TestAddressValue)
	_, err := crypto.HexToECDSA(TestPrivateKeyValue)
	if err != nil {
		log.Fatalf("Failed to load private key: %v", err)
	}

	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &toAddress,
		Value:    &value,
		Gas:      gasLimit,
		GasPrice: &gasPrice,
		Data:     []byte(data),
	})

	return tx
}

func CreateTestEthTransactionAsTxBytes(nonce uint64, value big.Int, gasLimit uint64, gasPrice big.Int, data string) hexutil.Bytes {
	privateKey, err := crypto.HexToECDSA(TestPrivateKeyValue)
	if err != nil {
		log.Fatalf("Failed to load private key: %v", err)
	}

	tx := CreateTestEthTransaction(nonce, value, gasLimit, gasPrice, data)

	chainID := big.NewInt(3) // Ropsten
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		log.Fatalf("Failed to sign transaction: %v", err)
	}

	rawTxBytes, err := signedTx.MarshalBinary()
	if err != nil {
		log.Fatalf("Failed to serialize transaction: %v", err)
	}

	return rawTxBytes
}

func TestTxEquals(t *testing.T, tx *types.Transaction, nonce uint64, value big.Int, gasLimit uint64, gasPrice big.Int, data string) {
	require.Equal(t, nonce, tx.Nonce())
	require.Equal(t, value, *tx.Value())
	require.Equal(t, gasLimit, tx.Gas())
	require.Equal(t, gasPrice, *tx.GasPrice())
	require.Equal(t, data, string(tx.Data()))
}
