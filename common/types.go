package common

import (
	"encoding/json"
	"errors"
	"fmt"
	consensuscapella "github.com/attestantio/go-eth2-client/spec/capella"
	"math/big"
	"os"

	"github.com/AnomalyFi/hypersdk/chain"
	"github.com/AnomalyFi/hypersdk/codec"
	"github.com/AnomalyFi/nodekit-seq/actions"
	apiv1 "github.com/attestantio/go-builder-client/api/v1"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	boostTypes "github.com/flashbots/go-boost-utils/types"
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

	// TODO: check deneb fork version for holesky when it is out
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

	DomainBuilder                 boostTypes.Domain
	DomainBeaconProposerBellatrix boostTypes.Domain
	DomainBeaconProposerCapella   boostTypes.Domain
	DomainBeaconProposerDeneb     boostTypes.Domain
}

func NewEthNetworkDetails(networkName string) (ret *EthNetworkDetails, err error) {
	var genesisForkVersion string
	var genesisValidatorsRoot string
	var bellatrixForkVersion string
	var capellaForkVersion string
	var denebForkVersion string
	var domainBuilder boostTypes.Domain
	var domainBeaconProposerBellatrix boostTypes.Domain
	var domainBeaconProposerCapella boostTypes.Domain
	var domainBeaconProposerDeneb boostTypes.Domain

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

	domainBuilder, err = ComputeDomain(boostTypes.DomainTypeAppBuilder, genesisForkVersion, boostTypes.Root{}.String())
	if err != nil {
		return nil, err
	}

	domainBeaconProposerBellatrix, err = ComputeDomain(boostTypes.DomainTypeBeaconProposer, bellatrixForkVersion, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}

	domainBeaconProposerCapella, err = ComputeDomain(boostTypes.DomainTypeBeaconProposer, capellaForkVersion, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}

	domainBeaconProposerDeneb, err = ComputeDomain(boostTypes.DomainTypeBeaconProposer, denebForkVersion, genesisValidatorsRoot)
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

type BuilderGetValidatorsResponseEntry struct {
	Slot           uint64                                  `json:"slot,string"`
	ValidatorIndex uint64                                  `json:"validator_index,string"`
	Entry          *boostTypes.SignedValidatorRegistration `json:"entry"`
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

type ExecutionPayload struct {
	// Array of transaction objects, each object is a byte list (DATA) representing
	// TransactionType || TransactionPayload or LegacyTransaction as defined in EIP-2718
	Transactions []hexutil.Bytes `json:"transactions"`
}

type AnchorHeader struct {
	Header    *common.Hash `json:"header"`
	BlockHash string       `json:"block_hash"`
}

type SignedBeaconBlock struct {
	Bellatrix *boostTypes.SignedBeaconBlock
	Capella   *consensuscapella.SignedBeaconBlock
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
		return s.Bellatrix.Message.Slot
	}
	return 0
}

func (s *SignedBeaconBlock) BlockHash() string {
	if s.Capella != nil {
		return s.Capella.Message.Body.ExecutionPayload.BlockHash.String()
	}
	if s.Bellatrix != nil {
		return s.Bellatrix.Message.Body.ExecutionPayload.BlockHash.String()
	}
	return ""
}

type AnchorGetHeaderResponse struct {
	ExecPayloads ExecPayloadsInfo
	BlockInfo    AnchorBlockInfo
}
type AnchorBlockInfo struct {
	// note: Message should be the anchor req
	Slot uint64 `json:"slot"`
	// nodeID of chunk producing validator.
	Producer ids.NodeID `json:"producer"`
	// hash of the anchor chunks (tob + robs)
	ChunkHash common.Hash `json:"chunkhash"`
}

// SEQ validator should sign this
type ExecPayloadsInfo struct {
	// Make signature based off ToBHash + RoBHashes then we use this signature for Baton/Anchor to check against
	ToBHash   *AnchorHeader            `json:"tobhash"`
	RoBHashes map[string]*AnchorHeader `json:"robhashes"`
}

type AnchorGetPayloadRequest struct {
	Slot uint64 `json:"slot"`
	// TODO: Figure out how to verify signature(ex: actual vs expected)
	Signature     boostTypes.Signature `json:"signature"`
	ProposerIndex uint64               `json:"proposer_index"`
	BlockHash     string               `json:"block_hash"`
}

type AnchorGetPayloadResponse struct {
	Slot        uint64                      `json:"slot"`
	ToBPayload  *ExecutionPayload           `json:"tobpayload"`
	RoBPayloads map[string]ExecutionPayload `json:"robpayloads"`
}

type BuilderSubmitBlockRequest struct {
	AnchorSignature  boostTypes.Signature   `json:"signature" ssz-size:"96"`
	AnchorMessage    *SubmitNewBlockRequest `json:"message"`
	ExecutionPayload *ExecutionPayload      `json:"execution_payload"`
}

/*
func BoostBidToBidTrace(bidTrace *boostTypes.BidTrace) *apiv1.BidTrace {
	if bidTrace == nil {
		return nil
	}
	return &apiv1.BidTrace{
		BuilderPubkey:        phase0.BLSPubKey(bidTrace.BuilderPubkey),
		Slot:                 bidTrace.Slot,
		ProposerPubkey:       phase0.BLSPubKey(bidTrace.ProposerPubkey),
		ProposerFeeRecipient: consensusbellatrix.ExecutionAddress(bidTrace.ProposerFeeRecipient),
		BlockHash:            phase0.Hash32(bidTrace.BlockHash),
		Value:                U256StrToUint256(bidTrace.Value),
		ParentHash:           phase0.Hash32(bidTrace.ParentHash),
		GasLimit:             bidTrace.GasLimit,
		GasUsed:              bidTrace.GasUsed,
	}
}
*/

// type GetPayloadResponse struct {
// 	Bellatrix *boostTypes.GetPayloadResponse
// 	Capella   *api.VersionedExecutionPayload
// }

// func (p *GetPayloadResponse) UnmarshalJSON(data []byte) error {
// 	capella := new(api.VersionedExecutionPayload)
// 	err := json.Unmarshal(data, capella)
// 	if err == nil && capella.Capella != nil {
// 		p.Capella = capella
// 		return nil
// 	}
// 	bellatrix := new(boostTypes.GetPayloadResponse)
// 	err = json.Unmarshal(data, bellatrix)
// 	if err != nil {
// 		return err
// 	}
// 	p.Bellatrix = bellatrix
// 	return nil
// }

// func (p *GetPayloadResponse) MarshalJSON() ([]byte, error) {
// 	if p.Bellatrix != nil {
// 		return json.Marshal(p.Bellatrix)
// 	}
// 	if p.Capella != nil {
// 		return json.Marshal(p.Capella)
// 	}
// 	return nil, ErrEmptyPayload
// }
// noting for handleGetHeader func
// type GetHeaderResponse struct {
// 	Bellatrix *boostTypes.GetHeaderResponse
// 	Capella   *spec.VersionedSignedBuilderBid
// }

// func (p *GetHeaderResponse) UnmarshalJSON(data []byte) error {
// 	capella := new(spec.VersionedSignedBuilderBid)
// 	err := json.Unmarshal(data, capella)
// 	if err == nil && capella.Capella != nil {
// 		p.Capella = capella
// 		return nil
// 	}
// 	bellatrix := new(boostTypes.GetHeaderResponse)
// 	err = json.Unmarshal(data, bellatrix)
// 	if err != nil {
// 		return err
// 	}
// 	p.Bellatrix = bellatrix
// 	return nil
// }

// func (p *GetHeaderResponse) MarshalJSON() ([]byte, error) {
// 	if p.Capella != nil {
// 		return json.Marshal(p.Capella)
// 	}
// 	if p.Bellatrix != nil {
// 		return json.Marshal(p.Bellatrix)
// 	}
// 	return nil, ErrEmptyPayload
// }

// func (p *GetHeaderResponse) Value() *big.Int {
// 	if p.Capella != nil {
// 		return p.Capella.Capella.Message.Value.ToBig()
// 	}
// 	if p.Bellatrix != nil {
// 		return p.Bellatrix.Data.Message.Value.BigInt()
// 	}
// 	return nil
// }

// func (p *GetHeaderResponse) BlockHash() phase0.Hash32 {
// 	if p.Capella != nil {
// 		return p.Capella.Capella.Message.Header.BlockHash
// 	}
// 	if p.Bellatrix != nil {
// 		return phase0.Hash32(p.Bellatrix.Data.Message.Header.BlockHash)
// 	}
// 	return phase0.Hash32{}
// }

// func (p *GetHeaderResponse) Empty() bool {
// 	if p == nil {
// 		return true
// 	}
// 	if p.Capella != nil {
// 		return p.Capella.Capella == nil || p.Capella.Capella.Message == nil
// 	}
// 	if p.Bellatrix != nil {
// 		return p.Bellatrix.Data == nil || p.Bellatrix.Data.Message == nil
// 	}
// 	return true
// }

func encodeTransactions(txs []*types.Transaction) [][]byte {
	var enc = make([][]byte, len(txs))
	for i, tx := range txs {
		enc[i], _ = tx.MarshalBinary()
	}
	return enc
}

func DecodeTransactions(enc [][]byte) ([]*types.Transaction, error) {
	var txs = make([]*types.Transaction, len(enc))
	for i, encTx := range enc {
		var tx types.Transaction
		if err := tx.UnmarshalBinary(encTx); err != nil {
			return nil, fmt.Errorf("invalid transaction %d: %v", i, err)
		}
		txs[i] = &tx
	}
	return txs, nil
}

// note: every tx(s) on Baton and anchor should be type SEQ Tx
// note: don't have to use eth payloads, because we just need to abide by seq standards so seq/anchor/baton
type ExecutionPayloadTransactions struct {
	//the note above means that 'Transactions' needs to be []SEQTransaction from seq-sdk/types
	Transactions []*chain.Transaction
}

/*
// this struct submits to seq
// rollup -> seq
type SequencerMsgRequest struct {
	// id of seq chains to submit txs too
	ChainID string
	// tx data itself
	Data []byte
	// address of rollup submitting tx(s)
	FromAddress string
}
*/

type BatonBlock struct {
	Txs             []*chain.Transaction `json:"txs"`
	Slot            uint64               `json:"slot"`
	ParentHash      common.Hash          `json:"parent_hash"`
	BlockNumber     map[string]string    `json:"blocknumber"`
	BlockHash       common.Hash          `json:"block_hash" ssz-size:"32"`
	ProposerPubkey  boostTypes.PublicKey `json:"proposer_pubkey" ssz-size:"48"`
	ProposerPayment codec.Address        `json:"proposer_payment" ssz-size:"48"`
}

// SubmitNewBlockRequest is the incoming message for new blocks to be added to Baton.
// Txs format is hypersdk transactions. The Eth transaction is stored in within Action.Data.
type SubmitNewBlockRequest struct {
	Chunk         BatonBlock
	Signature     boostTypes.Signature `json:"signature" ssz-size:"96"`
	BuilderPubKey boostTypes.PublicKey `json:"builder_pubkey" ssz-size:"48"`
}

/*
// SubmitNewBlockRequest is the incoming message for new blocks to be added to Baton.
// Txs format is hypersdk transactions. The Eth transaction is stored in within Action.Data.
type SubmitNewBlockRequest struct {
	// @TODO: Last tx should be the proposer tx.
	// @TODO: Check last tx proposer address matches the proposer payment below. Last tx should use transfer action.
	// @TODO: Use hypersdk method to marshal and unmarshal.
	// See https://github.com/AnomalyFi/hypersdk/blob/main/chain/transaction.go#L377
	// See parser in https://github.com/AnomalyFi/anchor/blob/seq-util/seq/seq.go#L33

	// Txs []byte `json:"txs"`
	Txs             []*chain.Transaction `json:"txs"`

	Slot       uint64 `json:"slot"`
	ParentHash string `json:"parent_hash"`

	// TODO: Switch to the below because block number is L2-roll-up specific.
	// HashMap[String, String], This contains a mapping of chainIds alongside the corresponding hex encoded block number which this bundle will be valid on.
	// Corresponds to rollup blocks. When we simulate, we will simulate on this block.
	BlockNumber map[string]string `json:"blocknumber"`

	BlockHash common.Hash `json:"block_hash" ssz-size:"32"`

	// TODO: Verify this matches the proposer's address.
	ProposerPayment codec.Address

	// Builder signing off on their payload.
	// TODO: Verify using here. https://github.com/flashbots/mev-boost-relay/blob/main/services/api/service.go#L2055
	Signature boostTypes.Signature `json:"signature" ssz-size:"96"`

	BuilderPubkey  boostTypes.PublicKey `json:"builder_pubkey" ssz-size:"48"`
	ProposerPubkey boostTypes.PublicKey `json:"proposer_pubkey" ssz-size:"48"`
}
*/

func NewSubmitNewBlockRequest() SubmitNewBlockRequest {
	return SubmitNewBlockRequest{
		Chunk:         NewBatonBlockRequest(),
		Signature:     boostTypes.Signature{},
		BuilderPubKey: boostTypes.PublicKey{},
	}
}

func NewBatonBlockRequest() BatonBlock {
	return BatonBlock{
		Txs:             make([]*chain.Transaction, 0),
		Slot:            0,
		ParentHash:      common.Hash{},
		BlockNumber:     make(map[string]string),
		BlockHash:       common.Hash{},
		ProposerPayment: codec.Address{},
		ProposerPubkey:  boostTypes.PublicKey{},
	}
}

func NewBatonBlockRequest2() BatonBlock2 {
	return BatonBlock2{
		Txs:             make([]byte, 20000), // TODO: JUST A TEST. FIX ME
		Slot:            0,
		ParentHash:      common.Hash{},
		BlockNumber:     make(map[string]string),
		BlockHash:       common.Hash{},
		ProposerPayment: codec.Address{},
		ProposerPubkey:  boostTypes.PublicKey{},
	}
}

func (r *SubmitNewBlockRequest) FromJSON(data []byte) error {
	/*
	   var authCounts map[uint8]int
	   var txs []*chain.Transaction
	   var err error

	   actionRegistry, authRegistry := g.vm.Registry()

	   actionRegistry, authRegistry := g.vm.Registry()
	   authCounts, txs, err := chain.UnmarshalTxs(msg, initialCapacity, actionRegistry, authRegistry)

	   authCounts, txs, err := chain.UnmarshalTxs(
	     data,
	     initialCapacity int,
	     actionRegistry ActionRegistry,
	     authRegistry AuthRegistry)

	*/

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

func (r *SubmitNewBlockRequest) BlockNumber() *map[string]string {
	return &r.Chunk.BlockNumber
}

func (r *SubmitNewBlockRequest) ProposerPubKey() boostTypes.PublicKey {
	return r.Chunk.ProposerPubkey
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

func (r *SubmitNewBlockRequest) Txs() []*chain.Transaction {
	return r.Chunk.Txs
}

func (r *SubmitNewBlockRequest) BlockNumberAsStr() (string, error) {
	blockNumberJson, err := json.Marshal(r.BlockNumber())
	if err != nil {
		return "", errors.New("could not marshal block number into string")
	}
	return string(blockNumberJson), nil
}

// The value should come from the proposer tx.
func (r *SubmitNewBlockRequest) Value() (*big.Int, error) {
	if len(r.Chunk.Txs) == 0 {
		return nil, errors.New("no txs found in baton block")
	}
	if len(r.Chunk.Txs) == 1 {
		return nil, errors.New("need more than 1 tx in baton block")
	}
	lastTx := r.Chunk.Txs[len(r.Chunk.Txs)-1]

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

func (r *SubmitNewBlockRequest) BuilderPubkey() boostTypes.PublicKey {
	return r.BuilderPubKey
}

func (r *SubmitNewBlockRequest) Sig() boostTypes.Signature {
	return r.Signature
}

//TODO: Remove when determined not needed
/*
func (r *SubmitNewBlockRequest) DecodeTxs() ([]*chain.Transaction, error) {
  scli := srpc.NewJSONRPCClient(uri, networkID, chainID)
  parser    chain.Parser
	actionRegistry, authRegistry := parser.Registry()
  parser, err := scli.Parser(context.TODO())
  if err != nil {
    return nil, err
  }
}
*/

// @TODO: fix me SOON
func (r *SubmitNewBlockRequest) FirstChainID() (string, error) {
	if len(r.Chunk.Txs) == 0 {
		return "", errors.New("getFirstChainID: no transactions found")
	}
	seqActions := r.Chunk.Txs[0].Actions
	if len(seqActions) == 0 {
		return "", errors.New("getFirstChainID: no actions in first tx")
	}
	if seqMsg, ok := seqActions[0].(*actions.SequencerMsg); ok {
		return string(seqMsg.ChainId), nil
	} else {
		return "", errors.New("could not convert seq actions to seqMsg")
	}
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

// TODO: REMOVE ME LATER. USE VERSION FROM WITHIN ANCHOR
type AnchorPayload struct {
	Slot   uint64      `json:"slot"`
	Header common.Hash `json:"blockHash"`
	// Array of transaction objects, each object is a byte list (DATA) representing
	// TransactionType || TransactionPayload or LegacyTransaction as defined in EIP-2718
	// TODO: Change me to hypersdk tx
	Transactions []hexutil.Bytes `json:"transactions"`

	GasUsed  uint64 `json:"gasused" db:"gas_used"`
	GasLimit uint64 `json:"gaslimit" db:"gas_limit"`
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

type BidTrace2 struct {
	Slot                 uint64
	ParentHash           common.Hash          `ssz-size:"32"`
	BlockHash            common.Hash          `ssz-size:"32"`
	BuilderPubkey        boostTypes.PublicKey `ssz-size:"48"`
	ProposerPubkey       boostTypes.PublicKey `ssz-size:"48"`
	ProposerFeeRecipient codec.Address
	GasLimit             uint64
	GasUsed              uint64
	Value                *big.Int `ssz-size:"32"`
}
