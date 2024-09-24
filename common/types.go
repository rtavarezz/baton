package common

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/exp/rand"
	"math/big"
	"os"
	"strings"

	"github.com/AnomalyFi/hypersdk/chain"
	"github.com/AnomalyFi/hypersdk/codec"
	"github.com/AnomalyFi/nodekit-seq/actions"
	apiv1 "github.com/attestantio/go-builder-client/api/v1"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	boostSsz "github.com/flashbots/go-boost-utils/ssz"
)

var (
	ErrUnknownNetwork = errors.New("unknown network")
	ErrEmptyPayload   = errors.New("empty payload")

	EthNetworkHolesky = "holesky"
	EthNetworkSepolia = "sepolia"
	EthNetworkGoerli  = "goerli"
	EthNetworkMainnet = "mainnet"
	EthNetworkCustom  = "custom"

	GenesisForkVersionHolesky = "0x01017000"
	GenesisForkVersionSepolia = "0x90000069"
	GenesisForkVersionGoerli  = "0x00001020"
	GenesisForkVersionMainnet = "0x00000000"

	GenesisValidatorsRootHolesky = "0x9143aa7c615a7f7115e2b6aac319c03529df8242ae705fba9df39b79c59fa8b1"
	GenesisValidatorsRootSepolia = "0xd8ea171f3c94aea21ebc42a1ed61052acf3f9209c00e4efbaaddac09ed9b8078"
	GenesisValidatorsRootGoerli  = "0x043db0d9a83813551ee2f33450d23797757d430911a9320530ad8a0eabc43efb"
	GenesisValidatorsRootMainnet = "0x4b363db94e286120d76eb905340fdd4e54bfe9f06bf33ff6cf5ad27f511bfe95"

	BellatrixForkVersionHolesky = "0x03017000"
	BellatrixForkVersionSepolia = "0x90000071"
	BellatrixForkVersionGoerli  = "0x02001020"
	BellatrixForkVersionMainnet = "0x02000000"

	CapellaForkVersionHolesky = "0x04017000"
	CapellaForkVersionSepolia = "0x90000072"
	CapellaForkVersionGoerli  = "0x03001020"
	CapellaForkVersionMainnet = "0x03000000"

	DenebForkVersionHolesky = "0x05017000"
	DenebForkVersionSepolia = "0x90000073"
	DenebForkVersionGoerli  = "0x04001020"
	DenebForkVersionMainnet = "0x04000000"

	ForkVersionStringBellatrix = "bellatrix"
	ForkVersionStringCapella   = "capella"
	ForkVersionStringDeneb     = "deneb"

	// this is for storing DeFi addresses for state interference checks
	DaiToken  = "dai"
	WethToken = "weth"
	WbtcToken = "wbtc"
	UsdcToken = "usdc"
	// 2 addresses are specifically in custom devnet, we have 2 pairs of Dai/Weth for arbitrage tests
	DaiWethPair1    = "dai_weth_pair_1"
	DaiWethPair2    = "dai_weth_pair_2"
	UniswapFactory1 = "uniswap_factory_1"
	UniswapFactory2 = "uniswap_factory_2"
	UniV3SwapRouter = "uniswap_v3_swap_router"

	// allow a max of 3 ToB txs excluding the payout
	MaxTobTxs          = 3
	TobGasReservations = 1000000
)

type EthNetworkDetails struct {
	Name                     string
	GenesisForkVersionHex    string
	GenesisValidatorsRootHex string
	BellatrixForkVersionHex  string
	CapellaForkVersionHex    string
	DenebForkVersionHex      string

	DomainBuilder                 phase0.Domain
	DomainBeaconProposerBellatrix phase0.Domain
	DomainBeaconProposerCapella   phase0.Domain
	DomainBeaconProposerDeneb     phase0.Domain
}

func NewEthNetworkDetails(networkName string) (ret *EthNetworkDetails, err error) {
	var genesisForkVersion string
	var genesisValidatorsRoot string
	var bellatrixForkVersion string
	var capellaForkVersion string
	var denebForkVersion string
	var domainBuilder phase0.Domain
	var domainBeaconProposerBellatrix phase0.Domain
	var domainBeaconProposerCapella phase0.Domain
	var domainBeaconProposerDeneb phase0.Domain

	switch networkName {
	case EthNetworkHolesky:
		genesisForkVersion = GenesisForkVersionHolesky
		genesisValidatorsRoot = GenesisValidatorsRootHolesky
		bellatrixForkVersion = BellatrixForkVersionHolesky
		capellaForkVersion = CapellaForkVersionHolesky
		denebForkVersion = DenebForkVersionHolesky
	case EthNetworkSepolia:
		genesisForkVersion = GenesisForkVersionSepolia
		genesisValidatorsRoot = GenesisValidatorsRootSepolia
		bellatrixForkVersion = BellatrixForkVersionSepolia
		capellaForkVersion = CapellaForkVersionSepolia
		denebForkVersion = DenebForkVersionSepolia
	case EthNetworkGoerli:
		genesisForkVersion = GenesisForkVersionGoerli
		genesisValidatorsRoot = GenesisValidatorsRootGoerli
		bellatrixForkVersion = BellatrixForkVersionGoerli
		capellaForkVersion = CapellaForkVersionGoerli
		denebForkVersion = DenebForkVersionGoerli
	case EthNetworkMainnet:
		genesisForkVersion = GenesisForkVersionMainnet
		genesisValidatorsRoot = GenesisValidatorsRootMainnet
		bellatrixForkVersion = BellatrixForkVersionMainnet
		capellaForkVersion = CapellaForkVersionMainnet
		denebForkVersion = DenebForkVersionMainnet
	case EthNetworkCustom:
		genesisForkVersion = os.Getenv("GENESIS_FORK_VERSION")
		genesisValidatorsRoot = os.Getenv("GENESIS_VALIDATORS_ROOT")
		bellatrixForkVersion = os.Getenv("BELLATRIX_FORK_VERSION")
		capellaForkVersion = os.Getenv("CAPELLA_FORK_VERSION")
		denebForkVersion = os.Getenv("DENEB_FORK_VERSION")
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownNetwork, networkName)
	}

	domainBuilder, err = ComputeDomain(boostSsz.DomainTypeAppBuilder, genesisForkVersion, phase0.Root{}.String())
	if err != nil {
		return nil, err
	}

	domainBeaconProposerBellatrix, err = ComputeDomain(boostSsz.DomainTypeBeaconProposer, bellatrixForkVersion, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}

	domainBeaconProposerCapella, err = ComputeDomain(boostSsz.DomainTypeBeaconProposer, capellaForkVersion, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}

	domainBeaconProposerDeneb, err = ComputeDomain(boostSsz.DomainTypeBeaconProposer, denebForkVersion, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}

	return &EthNetworkDetails{
		Name:                          networkName,
		GenesisForkVersionHex:         genesisForkVersion,
		GenesisValidatorsRootHex:      genesisValidatorsRoot,
		BellatrixForkVersionHex:       bellatrixForkVersion,
		CapellaForkVersionHex:         capellaForkVersion,
		DenebForkVersionHex:           denebForkVersion,
		DomainBuilder:                 domainBuilder,
		DomainBeaconProposerBellatrix: domainBeaconProposerBellatrix,
		DomainBeaconProposerCapella:   domainBeaconProposerCapella,
		DomainBeaconProposerDeneb:     domainBeaconProposerDeneb,
	}, nil
}

func (e *EthNetworkDetails) String() string {
	return fmt.Sprintf(
		`EthNetworkDetails{
	Name: %s,
	GenesisForkVersionHex: %s,
	GenesisValidatorsRootHex: %s,
	BellatrixForkVersionHex: %s,
	CapellaForkVersionHex: %s,
	DenebForkVersionHex: %s,
	DomainBuilder: %x,
	DomainBeaconProposerBellatrix: %x,
	DomainBeaconProposerCapella: %x,
	DomainBeaconProposerDeneb: %x
}`,
		e.Name,
		e.GenesisForkVersionHex,
		e.GenesisValidatorsRootHex,
		e.BellatrixForkVersionHex,
		e.CapellaForkVersionHex,
		e.DenebForkVersionHex,
		e.DomainBuilder,
		e.DomainBeaconProposerBellatrix,
		e.DomainBeaconProposerCapella,
		e.DomainBeaconProposerDeneb)
}

type PubkeyHex string

func NewPubkeyHex(pk string) PubkeyHex {
	return PubkeyHex(strings.ToLower(pk))
}

func (p PubkeyHex) String() string {
	return string(p)
}

type BuilderGetValidatorsResponseEntry struct {
	Slot           uint64 `json:"slot,string"`
	ValidatorIndex uint64 `json:"validator_index,string"`
	// seq validator info below in Entry.Message
	Entry *apiv1.SignedValidatorRegistration `json:"entry"`
}

type BidTraceV2 struct {
	apiv1.BidTrace
	BlockNumber uint64 `json:"block_number,string" db:"block_number"`
	NumTx       uint64 `json:"num_tx,string" db:"num_tx"`
}

type BidTraceV3 struct {
	Slot            uint64
	IsTob           bool
	ChainID         string
	ParentHash      string
	BlockHash       string
	BuilderPubkey   string
	ProposerPubkey  string
	ProposerPayment string
	GasLimit        uint64
	GasUsed         uint64
	Value           uint64
	BlockNumber     string
	NumTx           uint64
}

type BidTraceV2JSON struct {
	Slot                 uint64 `json:"slot,string"`
	ParentHash           string `json:"parent_hash"`
	BlockHash            string `json:"block_hash"`
	BuilderPubkey        string `json:"builder_pubkey"`
	ProposerPubkey       string `json:"proposer_pubkey"`
	ProposerFeeRecipient string `json:"proposer_fee_recipient"`
	GasLimit             uint64 `json:"gas_limit,string"`
	GasUsed              uint64 `json:"gas_used,string"`
	Value                string `json:"value"`
	NumTx                uint64 `json:"num_tx,string"`
	BlockNumber          uint64 `json:"block_number,string"`
}

func (b BidTraceV2) MarshalJSON() ([]byte, error) {
	return json.Marshal(&BidTraceV2JSON{
		Slot:                 b.Slot,
		ParentHash:           b.ParentHash.String(),
		BlockHash:            b.BlockHash.String(),
		BuilderPubkey:        b.BuilderPubkey.String(),
		ProposerPubkey:       b.ProposerPubkey.String(),
		ProposerFeeRecipient: b.ProposerFeeRecipient.String(),
		GasLimit:             b.GasLimit,
		GasUsed:              b.GasUsed,
		Value:                b.Value.ToBig().String(),
		NumTx:                b.NumTx,
		BlockNumber:          b.BlockNumber,
	})
}

func (b *BidTraceV2) UnmarshalJSON(data []byte) error {
	params := &struct {
		NumTx       uint64 `json:"num_tx,string"`
		BlockNumber uint64 `json:"block_number,string"`
	}{}
	err := json.Unmarshal(data, params)
	if err != nil {
		return err
	}
	b.NumTx = params.NumTx
	b.BlockNumber = params.BlockNumber

	bidTrace := new(apiv1.BidTrace)
	err = json.Unmarshal(data, bidTrace)
	if err != nil {
		return err
	}
	b.BidTrace = *bidTrace
	return nil
}

func (b *BidTraceV2JSON) CSVHeader() []string {
	return []string{
		"slot",
		"parent_hash",
		"block_hash",
		"builder_pubkey",
		"proposer_pubkey",
		"proposer_fee_recipient",
		"gas_limit",
		"gas_used",
		"value",
		"num_tx",
		"block_number",
	}
}

func (b *BidTraceV2JSON) ToCSVRecord() []string {
	return []string{
		fmt.Sprint(b.Slot),
		b.ParentHash,
		b.BlockHash,
		b.BuilderPubkey,
		b.ProposerPubkey,
		b.ProposerFeeRecipient,
		fmt.Sprint(b.GasLimit),
		fmt.Sprint(b.GasUsed),
		b.Value,
		fmt.Sprint(b.NumTx),
		fmt.Sprint(b.BlockNumber),
	}
}

type BidTraceV2WithTimestampJSON struct {
	BidTraceV2JSON
	Timestamp            int64 `json:"timestamp,string,omitempty"`
	TimestampMs          int64 `json:"timestamp_ms,string,omitempty"`
	OptimisticSubmission bool  `json:"optimistic_submission"`
}

func (b *BidTraceV2WithTimestampJSON) CSVHeader() []string {
	return []string{
		"slot",
		"parent_hash",
		"block_hash",
		"builder_pubkey",
		"proposer_pubkey",
		"proposer_fee_recipient",
		"gas_limit",
		"gas_used",
		"value",
		"num_tx",
		"block_number",
		"timestamp",
		"timestamp_ms",
		"optimistic_submission",
	}
}

func (b *BidTraceV2WithTimestampJSON) ToCSVRecord() []string {
	return []string{
		fmt.Sprint(b.Slot),
		b.ParentHash,
		b.BlockHash,
		b.BuilderPubkey,
		b.ProposerPubkey,
		b.ProposerFeeRecipient,
		fmt.Sprint(b.GasLimit),
		fmt.Sprint(b.GasUsed),
		b.Value,
		fmt.Sprint(b.NumTx),
		fmt.Sprint(b.BlockNumber),
		fmt.Sprint(b.Timestamp),
		fmt.Sprint(b.TimestampMs),
		fmt.Sprint(b.OptimisticSubmission),
	}
}

type SignedBeaconBlock struct {
	Bellatrix *phase0.SignedBeaconBlock
	Capella   *phase0.SignedBeaconBlock
}

func (s *SignedBeaconBlock) MarshalJSON() ([]byte, error) {
	if s.Capella != nil {
		return json.Marshal(s.Capella)
	}
	if s.Bellatrix != nil {
		return json.Marshal(s.Bellatrix)
	}
	return nil, ErrEmptyPayload
}

func (s *SignedBeaconBlock) Slot() uint64 {
	if s.Capella != nil {
		return uint64(s.Capella.Message.Slot)
	}
	if s.Bellatrix != nil {
		return uint64(s.Bellatrix.Message.Slot)
	}
	return 0
}

func (s *SignedBeaconBlock) BlockHash() string {
	if s.Capella != nil {
		return string(s.Capella.Message.Body.ETH1Data.BlockHash[:])
	}
	if s.Bellatrix != nil {
		return string(s.Bellatrix.Message.Body.ETH1Data.BlockHash[:])
	}
	return ""
}

// SubmitNewBlockRequest is the incoming message for new blocks to be added to Baton.
// Txs format is hypersdk transactions. The Eth transaction is stored in within Action.Data.
type SubmitNewBlockRequest struct {
	Chunk         BatonBlock
	Signature     bls.Signature `json:"signature" ssz-size:"96"`
	BuilderPubKey bls.PublicKey `json:"builder_pubkey" ssz-size:"48"`
}

type BatonBlock struct {
	Txs             []byte            `json:"txs"`
	Slot            uint64            `json:"slot"`
	ParentHash      common.Hash       `json:"parent_hash"`
	BlockNumber     map[string]string `json:"blocknumber"`
	BlockHash       common.Hash       `json:"block_hash" ssz-size:"32"`
	ProposerPubkey  bls.PublicKey     `json:"proposer_pubkey" ssz-size:"48"`
	ProposerPayment codec.Address     `json:"proposer_payment" ssz-size:"48"`
}

func NewSubmitNewBlockRequest() SubmitNewBlockRequest {
	return SubmitNewBlockRequest{
		Chunk:         NewBatonBlockRequest(),
		Signature:     bls.Signature{},
		BuilderPubKey: bls.PublicKey{},
	}
}

func NewBatonBlockRequest() BatonBlock {
	return BatonBlock{
		Txs:             make([]byte, 0),
		Slot:            0,
		ParentHash:      common.Hash{},
		BlockNumber:     make(map[string]string),
		BlockHash:       common.Hash{},
		ProposerPayment: codec.Address{},
		ProposerPubkey:  bls.PublicKey{},
	}
}

func (r *SubmitNewBlockRequest) FromJSON(data []byte) error {
	return json.Unmarshal(data, r)
}
func (r *SubmitNewBlockRequest) FromJSONAction(data []byte) error {
	return json.Unmarshal(data, r)
}

func (r *SubmitNewBlockRequest) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

func (r *SubmitNewBlockRequest) Slot() uint64 {
	return r.Chunk.Slot
}

func (r *SubmitNewBlockRequest) BlockHash() common.Hash {
	return r.Chunk.BlockHash
}

// TODO: can just return map without pointer, map is always passed by reference
func (r *SubmitNewBlockRequest) BlockNumber() *map[string]string {
	return &r.Chunk.BlockNumber
}

func (r *SubmitNewBlockRequest) ProposerPubKey() bls.PublicKey {
	return r.Chunk.ProposerPubkey
}

func (r *SubmitNewBlockRequest) ProposerPubKeyAsStr() string {
	pk := r.ProposerPubKey()
	proposerPubKeyBytes := (&pk).Bytes()
	proposerPubKeyBytesAsStr := hex.EncodeToString(proposerPubKeyBytes[:])
	return proposerPubKeyBytesAsStr
}

func (r *SubmitNewBlockRequest) ProposerPayment() codec.Address {
	return r.Chunk.ProposerPayment
}

func (r *SubmitNewBlockRequest) ProposerPaymentAsStr() string {
	return string(r.Chunk.ProposerPayment[:])
}

func (r *SubmitNewBlockRequest) ParentHash() common.Hash {
	return r.Chunk.ParentHash
}

func (r *SubmitNewBlockRequest) Txs() []byte {
	return r.Chunk.Txs
}

func (r *SubmitNewBlockRequest) BlockNumberAsStr() (string, error) {
	blockNumberJson, err := json.Marshal(r.BlockNumber())
	if err != nil {
		return "", errors.New("could not marshal block number into string")
	}
	return string(blockNumberJson), nil
}

// Note the value should come from the transfer action.
func Value(txs []*chain.Transaction) (*big.Int, error) {
	if len(txs) == 0 {
		return nil, errors.New("no txs found in baton block")
	}
	if len(txs) == 1 {
		return nil, errors.New("need more than 1 tx in baton block")
	}
	lastTx := txs[len(txs)-1]

	if len(lastTx.Actions) != 1 {
		return nil, errors.New("simulateBlock: transfer action had multiple txs")
	}
	for _, action := range lastTx.Actions {
		if seqMsg, ok := action.(*actions.Transfer); ok {
			return big.NewInt(int64(seqMsg.Value)), nil
		}
	}
	return nil, errors.New("simulateBlock: could not retireve value and transfer action")
}

func (r *SubmitNewBlockRequest) BuilderPubkey() bls.PublicKey {
	return r.BuilderPubKey
}

func (r *SubmitNewBlockRequest) BuilderPubkeyAsStr() string {
	pk := r.BuilderPubkey()
	builderPubKeyBytes := (&pk).Bytes()
	builderPubKeyBytesAsStr := hex.EncodeToString(builderPubKeyBytes[:])
	return builderPubKeyBytesAsStr
}

func (r *SubmitNewBlockRequest) Sig() bls.Signature {
	return r.Signature
}

// callLog is the result of LOG opCode
type CallLog struct {
	Address common.Address `json:"address"`
	Topics  []common.Hash  `json:"topics"`
	Data    hexutil.Bytes  `json:"data"`
}

type CallTrace struct {
	From         common.Address  `json:"from"`
	Gas          *hexutil.Uint64 `json:"gas"`
	GasUsed      *hexutil.Uint64 `json:"gasUsed"`
	To           *common.Address `json:"to,omitempty"`
	Input        hexutil.Bytes   `json:"input"`
	Output       hexutil.Bytes   `json:"output,omitempty"`
	Error        string          `json:"error,omitempty"`
	RevertReason string          `json:"revertReason,omitempty"`
	Calls        []CallTrace     `json:"calls,omitempty"`
	Logs         []CallLog       `json:"logs,omitempty"`
	Value        *hexutil.Big    `json:"value,omitempty"`
	// Gencodec adds overridden fields at the end
	Type string `json:"type"`
}

type CallTraceResponse struct {
	Result CallTrace `json:"result"`
}

type NetworkTobTxChecker func(CallTrace) (bool, error)

type BlockValidationRequest struct {
	Txs              []hexutil.Bytes `json:"txs"`              // Signed eth transactions
	BlockNumber      string          `json:"blockNumber"`      // hex-encoded block number for which this request is valid on
	StateBlockNumber string          `json:"stateBlockNumber"` // hex-encoded number or block tag for which state to base this simulation on. Can use "latest"
	Timestamp        uint64          `json:"timestamp"`        // Optional number. the timestamp to use for this bundle simulation
}

// Used in simulating bundle and getting gas used
type FlashbotsCallBundleResult struct {
	BundleGasPrice    string          `json:"bundleGasPrice"`
	BundleHash        string          `json:"bundleHash"`
	CoinbaseDiff      string          `json:"coinbaseDiff"`
	EthSentToCoinbase string          `json:"ethSentToCoinbase"`
	GasFees           string          `json:"gasFees"`
	Results           []hexutil.Bytes `json:"results"`
	StateBlockNumber  int64           `json:"stateBlockNumber"`
	TotalGasUsed      int64           `json:"totalGasUsed"`
}

type AnchorHeader struct {
	Header    *common.Hash `json:"header"`
	BlockHash string       `json:"block_hash"`
	Value     *big.Int     `json:"value"`
}

type AnchorPayload struct {
	Slot   uint64      `json:"slot"`
	Header common.Hash `json:"blockHash"`

	// Hypersdk txs from the submit new block request.
	Transactions []byte `json:"transactions"`

	GasUsed  uint64 `json:"gasused" db:"gas_used"`
	GasLimit uint64 `json:"gaslimit" db:"gas_limit"`
}

type AnchorGetHeaderResponse struct {
	ExecHeaders ExecHeadersInfo `json:"exec_headers"`
	BlockInfo   AnchorBlockInfo `json:"block_info"`
	ParentHash  common.Hash     `json:"parent_hash"`
	// Exec headers signed by baton's key.
	ExecHeadersSig []byte `json:"exec_headers_sig"`
}

type AnchorBlockInfo struct {
	Slot uint64 `json:"slot"`
	// nodeID of chunk producing validator.
	Producer       ids.NodeID    `json:"producer"`
	ProposerPubkey bls.PublicKey `json:"proposer_pubkey"`
}

// SEQ validator should sign this
type ExecHeadersInfo struct {
	// Make signature based off ToBHash + RoBHashes then we use this signature for Baton/Anchor to check against
	ToBHash   *AnchorHeader            `json:"tobhash"`
	RoBHashes map[string]*AnchorHeader `json:"robhashes"`
}

type AnchorGetPayloadRequest struct {
	Slot           uint64 `json:"slot"`
	ProposerPubKey []byte `json:"proposer_pubkey"`
	ParentHash     string `json:"parent_hash"`
	// Exec headers signed by validator's key. Should be [48]byte bls.signature.
	SignedHeaders []byte `json:"signed_headers"`
}

type AnchorGetPayloadResponse struct {
	Slot uint64 `json:"slot"`
	// Contains actual hypersdk txs in byte format
	ExecPayloads ExecPayloadsInfo `json:"execpayloads"`
	// Exec payloads signed by baton's private key.
	ExecPayloadsSig []byte `json:"execpayloads_sig"`
}

type ExecPayloadsInfo struct {
	ToBPayload  *ExecutionPayload           `json:"tobpayload"`
	RoBPayloads map[string]ExecutionPayload `json:"robpayloads"`
}

type ExecutionPayload struct {
	// hypersdk transactions in byte slice format
	Transactions []byte `json:"transactions"`
}

func (r *AnchorGetPayloadResponse) GetExecPayloadsSig() (*bls.Signature, error) {
	signature, err := bls.SignatureFromBytes(r.ExecPayloadsSig)
	if err != nil {
		return nil, errors.New("invalid signed headers, err: " + err.Error())
	}
	return signature, nil
}

func (r *AnchorGetPayloadRequest) GetSignedHeaders() (*bls.Signature, error) {
	if r.SignedHeaders == nil {
		return nil, errors.New("signed headers was empty")
	}

	signature, err := bls.SignatureFromBytes(r.SignedHeaders)
	if err != nil {
		return nil, errors.New("invalid signed headers, err: " + err.Error())
	}
	return signature, nil
}

func (r *AnchorGetPayloadResponse) SetExecPayloadsSig(sig *bls.Signature) {
	signatureAsBytes := sig.Bytes()
	r.ExecPayloadsSig = signatureAsBytes[:]
}

func (msg *AnchorGetPayloadResponse) IsEmpty() bool {
	return msg.ExecPayloads.ToBPayload == nil && len(msg.ExecPayloads.RoBPayloads) == 0
}

func (msg *AnchorGetPayloadResponse) NumToBTxs() int {
	if msg.ExecPayloads.ToBPayload == nil {
		return 0
	}
	return len(msg.ExecPayloads.ToBPayload.Transactions)
}

func (msg *AnchorGetPayloadResponse) NumRoBTxs() int {
	var numTxs int
	for _, txs := range msg.ExecPayloads.RoBPayloads {
		numTxs = numTxs + len(txs.Transactions)
	}
	return numTxs
}

func NewAnchorGetPayloadResponse(slot uint64, needsToB bool) AnchorGetPayloadResponse {
	var tob *ExecutionPayload
	if needsToB {
		payload := NewExecutionPayload()
		tob = &payload
	}

	execPayloads := ExecPayloadsInfo{
		ToBPayload:  tob,
		RoBPayloads: make(map[string]ExecutionPayload),
	}

	return AnchorGetPayloadResponse{
		Slot:         slot,
		ExecPayloads: execPayloads,
	}
}

func NewExecutionPayload() ExecutionPayload {
	return ExecutionPayload{
		Transactions: make([]byte, 0),
	}
}

func NewExecutionHeader() ExecHeadersInfo {
	return ExecHeadersInfo{
		RoBHashes: make(map[string]*AnchorHeader),
	}
}

// VerifyHeaderSignature verifies that the getHeader ExecHeaders have been signed with the given public key
func VerifyHeaderSignature(response *AnchorGetHeaderResponse, pubKey bls.PublicKey) (bool, error) {
	payloadHash, err := HashExecHeaders(&response.ExecHeaders)
	if err != nil {
		return false, err
	}

	payloadSignatureBytes := response.ExecHeadersSig
	pubKeyBytes := pubKey.Bytes()

	return bls.VerifySignatureBytes(payloadHash[:], payloadSignatureBytes[:], pubKeyBytes[:])
}

// VerifyPayloadSignature verifies that the getHeader ExecHeaders have been signed with the given public key
func VerifyPayloadSignature(response *AnchorGetPayloadResponse, pubKey bls.PublicKey) (bool, error) {
	payloadHash, err := HashExecPayloads(&response.ExecPayloads)
	if err != nil {
		return false, err
	}

	payloadSignatureBytes := response.ExecPayloadsSig
	pubKeyBytes := pubKey.Bytes()

	return bls.VerifySignatureBytes(payloadHash[:], payloadSignatureBytes[:], pubKeyBytes[:])
}

func GetExecHeaderSignature(headers *ExecHeadersInfo, secretKey *bls.SecretKey) (*bls.Signature, error) {
	// Step 1: Hash the ExecHeaders (ToBHash + RoBHashes) data
	payloadHash, err := HashExecHeaders(headers)
	if err != nil {
		return nil, err
	}

	// Step 2: Sign the hashed headers using the secret key
	signature := bls.Sign(secretKey, payloadHash[:])
	return signature, nil
}

func GetExecPayloadSignature(payloads *ExecPayloadsInfo, secretKey *bls.SecretKey) (*bls.Signature, error) {
	// Step 1: Hash the ExecHeaders (ToBHash + RoBHashes) data
	payloadHash, err := HashExecPayloads(payloads)
	if err != nil {
		return nil, err
	}

	// Step 2: Sign the hashed payloads using the secret key
	signature := bls.Sign(secretKey, payloadHash[:])
	return signature, nil
}
func (r *AnchorGetHeaderResponse) SetExecPayloadsSig(sig *bls.Signature) {
	signatureAsBytes := sig.Bytes()
	r.ExecHeadersSig = signatureAsBytes[:]
}

func SignAnchorGetHeaderResponse(response *AnchorGetHeaderResponse, secretKey *bls.SecretKey) error {
	signature, err := GetExecHeaderSignature(&response.ExecHeaders, secretKey)
	if err != nil {
		return errors.New("failed to sign anchor header response, err: " + err.Error())
	}

	response.SetExecPayloadsSig(signature)
	return nil
}

func SignAnchorGetPayloadResponse(response *AnchorGetPayloadResponse, secretKey *bls.SecretKey) error {
	signature, err := GetExecPayloadSignature(&response.ExecPayloads, secretKey)
	if err != nil {
		return errors.New("failed to sign anchor header response, err: " + err.Error())
	}

	response.SetExecPayloadsSig(signature)
	return nil
}

func HashExecHeaders(headers *ExecHeadersInfo) ([32]byte, error) {
	// Use JSON serialization to hash the struct
	payloadBytes, err := json.Marshal(*headers)
	if err != nil {
		return [32]byte{}, fmt.Errorf("failed to serialize ExecHeaders: %w", err)
	}

	// Use sha256 to hash the serialized ExecHeaders data
	hash := sha256.Sum256(payloadBytes)
	return hash, nil
}

func HashExecPayloads(payloads *ExecPayloadsInfo) ([32]byte, error) {
	// Use JSON serialization to hash the struct
	payloadBytes, err := json.Marshal(*payloads)
	if err != nil {
		return [32]byte{}, fmt.Errorf("failed to serialize ExecHeaders: %w", err)
	}

	// Use sha256 to hash the serialized ExecHeaders data
	hash := sha256.Sum256(payloadBytes)
	return hash, nil
}

// VerifySignedHeaders verifies that the getHeader ExecHeaders have been signed with the given public key
func VerifySignedHeaders(
	expectedHeaders *ExecHeadersInfo,
	payloadReq *AnchorGetPayloadRequest,
	pubKey bls.PublicKey,
) (bool, error) {
	payloadHash, err := HashExecHeaders(expectedHeaders)
	if err != nil {
		return false, err
	}

	payloadSignatureBytes := payloadReq.SignedHeaders
	pubKeyBytes := pubKey.Bytes()

	return bls.VerifySignatureBytes(payloadHash[:], payloadSignatureBytes[:], pubKeyBytes[:])
}

func GenerateRandomHash() (common.Hash, error) {
	// Create a 32-byte array (since common.Hash is [32]byte)
	var hashBytes [32]byte

	// Fill the array with random bytes
	_, err := rand.Read(hashBytes[:])
	if err != nil {
		return common.Hash{}, err
	}

	// Convert the random bytes to a common.Hash and return it
	return common.BytesToHash(hashBytes[:]), nil
}

func (r *AnchorGetPayloadRequest) GetPublicKey() (*bls.PublicKey, error) {
	return bls.PublicKeyFromBytes(r.ProposerPubKey)
}
