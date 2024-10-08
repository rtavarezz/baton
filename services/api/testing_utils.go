package api

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
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
	TestAddress         = "0x010000000000000000000000000000000000000000000000000000000000000000" // created by codec.CreateAddress(1, ids.Empty)
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
	RobChainIndex  int // only used if isTob false
	NumTxs         int

	WithTransferTx bool
	SeqChainID     ids.ID

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
		chainIndex = opts.RobChainIndex
		parentHash = opts.ParentHash

		numTxs = opts.NumTxs
		if opts.BuilderPubkey.String() != "" {
			builderPubkey = &opts.BuilderPubkey
		}
		if opts.ProposerPubkey.String() != "" {
			proposerPk = opts.ProposerPubkey
		}
	}

	txs := []*chain.Transaction{}
	var chainIDs []*big.Int
	// single chain id
	var chainID *big.Int
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
		tx := CreateHypersdkTx(opts.SeqChainID, chainID, ethTx)
		txs = append(txs, tx)
	}

	if opts.WithTransferTx {
		transferAction := CreateTestProposerTransfer(opts.SeqChainID, value)
		txs = append(txs, transferAction)
	}

	builderPubkeyBytes := builderPubkey.Bytes()
	proposerPubkeyBytes := proposerPk.Bytes()

	blockReq := common.NewSubmitNewBlockRequest()
	blockReq.BuilderPubKey = builderPubkeyBytes[:]
	blockReq.Chunk.Slot = slot
	blockReq.Chunk.ParentHash = parentHash
	blockReq.Chunk.ProposerPubkey = proposerPubkeyBytes[:]
	proposerPayment, err := hexutil.Decode(TestAddress)
	require.NoError(t, err)
	blockReq.Chunk.ProposerPayment = codec.Address(proposerPayment)

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

	anchorHeader, err := BuildHeader(&blockReq, value)
	require.NoError(t, err)

	anchorPayload, err := BuildPayload(&blockReq, blockReq.Txs(), value, nil)
	require.NoError(t, err)

	err = blockReq.Initialize()
	require.NoError(t, err)

	return &blockReq, &anchorHeader, anchorPayload
}

func CreateTestChunkSubmissionWithBuilderPubKey(
	t *testing.T,
	value uint64,
	builderPubKey bls.PublicKey,
	opts *CreateTestBlockSubmissionOpts,
) (*common.SubmitNewBlockRequest,
	*common.AnchorHeader,
	*common.AnchorPayload,
	common.BidTraceV3,
) {
	blockReq, anchorHeader, anchorPayload := CreateTestChunkSubmission(t, value, opts)
	blockReq.BuilderPubKey = bls.PublicKeyToBytes(&builderPubKey)
	//temp := blockReq.BlockHash
	testBlockHash := "0x8ae5292d1e248d987d51b58665b81848814202d7b23b343d20f2a167d12f07dcb01ca41c42fdd60b7fca9c4b90890792"
	testGasLimit := uint64(1000000)
	testGasUsed := uint64(100)
	testBlockNumber := "0xABCDABCDABCDABCD"
	testNumTxs := uint64(2)
	testChainID := "chain1"
	trace := common.BidTraceV3{
		Slot:            uint64(2),
		IsTob:           false,
		ChainID:         testChainID,
		ParentHash:      ids.Empty.String(),
		BlockHash:       testBlockHash,
		BuilderPubkey:   blockReq.BuilderPubkeyAsStr(),
		ProposerPubkey:  blockReq.ProposerPubKeyAsStr(),
		ProposerPayment: blockReq.ProposerPaymentAsStr(),
		GasLimit:        testGasLimit,
		GasUsed:         testGasUsed,
		Value:           value,
		BlockNumber:     testBlockNumber,
		NumTx:           testNumTxs,
	}
	return blockReq, anchorHeader, anchorPayload, trace
}

func GetTestChainID(i int) *big.Int {
	return big.NewInt(int64(45200 + i))
}

func GetTestChainIds(isToB bool, c int) []*big.Int {
	if isToB {
		testChainIDs := make([]*big.Int, c)
		for i := 0; i < c; i++ {
			testChainIDs[i] = GetTestChainID(i)
		}
		return testChainIDs
	}
	return []*big.Int{GetTestChainID(c)}
}

func CreateHypersdkTx(seqChainID ids.ID, chainID *big.Int, ethTx []byte) *chain.Transaction {
	chainIDu64 := chainID.Uint64()
	namespace := make([]byte, 8)
	binary.LittleEndian.PutUint64(namespace, chainIDu64)

	seqMsg := actions.SequencerMsg{
		ChainID:     namespace,
		Data:        ethTx,
		FromAddress: TestProposerPayment,
		RelayerID:   TestRelayerID,
	}
	var base = chain.Base{
		Timestamp: time.Now().UnixMilli(),
		ChainID:   seqChainID,
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

func CreateTestProposerTransfer(seqChainID ids.ID, value uint64) *chain.Transaction {
	transfer := actions.Transfer{
		To:    TestProposerPayment,
		Value: value,
	}
	base := chain.Base{
		Timestamp: time.Now().UnixMilli(),
		ChainID:   seqChainID,
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
