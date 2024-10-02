package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"testing"
	"time"

	"github.com/AnomalyFi/baton/common"
	"github.com/AnomalyFi/hypersdk/chain"
	"github.com/AnomalyFi/hypersdk/codec"
	"github.com/AnomalyFi/hypersdk/crypto/ed25519"
	"github.com/AnomalyFi/nodekit-seq/actions"
	"github.com/AnomalyFi/nodekit-seq/auth"
	srpc "github.com/AnomalyFi/nodekit-seq/rpc"
	"github.com/ava-labs/avalanchego/ids"
	eth "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/stretchr/testify/require"
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
	ParentHash     ids.ID
	BuilderPubkey  bls.PublicKey
	ProposerPubkey bls.PublicKey
	IsToB          bool
	robChainIndex  int // only used if isTob false
	numTxs         int

	withTransferTx bool

	//relaySk        bls.SecretKey
	//relayPk        types.PublicKey
	//domain         types.Domain
}

func (opts *CreateTestBlockSubmissionOpts) BuilderPubkeyAsStr() string {
	pk := opts.BuilderPubkey
	builderPubKeyBytes := (&pk).Bytes()
	builderPubKeyBytesAsStr := "0x" + hex.EncodeToString(builderPubKeyBytes[:])
	return builderPubKeyBytesAsStr
}

func (opts *CreateTestBlockSubmissionOpts) ProposerPubKeyAsStr() string {
	pk := opts.ProposerPubkey
	proposerPubKeyBytes := (&pk).Bytes()
	proposerPubKeyBytesAsStr := "0x" + hex.EncodeToString(proposerPubKeyBytes[:])
	return proposerPubKeyBytesAsStr
}

func CreateTestChunkSubmission(
	t *testing.T,
	value uint64,
	opts *CreateTestBlockSubmissionOpts,
) (*common.SubmitNewBlockRequest,
	*common.AnchorHeader,
	*common.AnchorPayload,
) {

	t.Helper()
	var err error
	var parentHash ids.ID

	slot := uint64(1)
	proposerPk := bls.PublicKey{}
	builderSecretKey, builderPubkey, err := bls.GenerateNewKeypair()
	require.NoError(t, err)
	chainIndex := 1

	numTxs := 1

	if opts != nil {
		slot = opts.Slot
		chainIndex = opts.robChainIndex
		parentHash = opts.ParentHash

		numTxs = opts.numTxs
		if opts.BuilderPubkey.String() != "" {
			builderPubkey = &opts.BuilderPubkey
		}
		if opts.ProposerPubkey.String() != "" {
			proposerPk = opts.ProposerPubkey
		}
	}

	txs := []*chain.Transaction{}
	var chainIDs []string
	// single chain id
	var chainID string
	if opts.IsToB {
		// ToB case: add however many test chain ids you want for ToB
		chainIDs = GetTestChainIds(opts.IsToB, 4)
	} else {
		// RoB case: uses rob chain index only
		chainIDs = GetTestChainIds(opts.IsToB, chainIndex)
	}

	for i := 0; i < numTxs; i++ {
		nonce := GetNextTestNonce()
		val := big.NewInt(int64(100 * i))
		gasLimit := uint64(10000000 + i)
		gasPrice := big.NewInt(int64(10000 + i))
		chainID = chainIDs[i%len(chainIDs)]

		ethTx := CreateTestEthTransactionAsTxBytes(nonce, *val, gasLimit, *gasPrice, "")
		tx := CreateHypersdkTx(chainID, ethTx)
		txs = append(txs, tx)
	}

	if opts.withTransferTx {
		transferAction := CreateTestProposerTransfer(chainID, value)
		txs = append(txs, transferAction)
	}

	builderPubkeyBytes := builderPubkey.Bytes()
	proposerPubkeyBytes := proposerPk.Bytes()

	blockReq := common.NewSubmitNewBlockRequest()
	blockReq.BuilderPubKey = builderPubkeyBytes[:]
	blockReq.Chunk.Slot = slot
	blockReq.Chunk.ParentHash = parentHash
	blockReq.Chunk.ProposerPubkey = proposerPubkeyBytes[:]
	copy(blockReq.Chunk.ProposerPayment[:], TestAddress[:])

	// blockReq.Signature = &bls.Signature{}
	chunkBytes, err := json.Marshal(blockReq.Chunk)
	require.NoError(t, err)
	chunkSig := bls.Sign(builderSecretKey, chunkBytes)
	chunkSigBytes := chunkSig.Bytes()
	blockReq.Signature = chunkSigBytes[:]

	//txsBytes, err := json.Marshal(txs)
	//var signer ed25519.PrivateKey
	txsBytes, err := chain.MarshalTxs(txs)
	require.NoError(t, err)

	blockReq.Chunk.Txs = txsBytes

	anchorHeader, err := BuildHeader(&blockReq)
	require.NoError(t, err)

	anchorPayload, err := BuildPayload(&blockReq, blockReq.Txs())
	require.NoError(t, err)

	err = blockReq.Initialize()
	require.NoError(t, err)

	return &blockReq, &anchorHeader, anchorPayload
}

func GetTestChainID(i int) string {
	return fmt.Sprintf("test-chain-%d", i)
}

func GetTestChainIds(isToB bool, c int) []string {
	if isToB {
		testChainIDs := make([]string, c)
		for i := 0; i < c; i++ {
			testChainIDs[i] = GetTestChainID(i)
		}
		return testChainIDs
	}
	return []string{GetTestChainID(c)}
}

func CreateHypersdkTx(chainID string, ethTx []byte) *chain.Transaction {
	seqMsg := actions.SequencerMsg{
		ChainID:     []byte(chainID),
		Data:        ethTx,
		FromAddress: TestProposerPayment,
		RelayerID:   TestRelayerID,
	}
	//ids := make([]ids.ID, 32)
	var id ids.ID
	copy(id[:], seqMsg.ChainID)
	var base = chain.Base{
		Timestamp: time.Now().UnixMilli(),
		ChainID:   id,
		MaxFee:    TestMaxFee,
	}
	base.Timestamp = int64(time.Now().Second() * 1000)
	pkBytes, err := hex.DecodeString(KEYHEX)
	if err != nil {
		panic(err)
	}
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
	if err != nil {
		panic(err)
	}

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
