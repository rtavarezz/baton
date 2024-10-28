package common

/*
import (
"encoding/json"
errors "errors"
"fmt"
"math/big"
"os"

"github.com/AnomalyFi/hypersdk/codec"
actions "github.com/AnomalyFi/seq-sdk/types"
"github.com/attestantio/go-builder-client/api"
"github.com/attestantio/go-builder-client/api/capella"
apiv1 "github.com/attestantio/go-builder-client/api/v1"
"github.com/attestantio/go-builder-client/spec"
apiv1capella "github.com/attestantio/go-eth2-client/api/v1/capella"
consensusspec "github.com/attestantio/go-eth2-client/spec"
consensuscapella "github.com/attestantio/go-eth2-client/spec/capella"
"github.com/attestantio/go-eth2-client/spec/phase0"
"github.com/ava-labs/avalanchego/ids"
"github.com/ethereum/go-ethereum/common"
"github.com/ethereum/go-ethereum/common/hexutil"
"github.com/ethereum/go-ethereum/core/types"
ssz "github.com/ferranbt/fastssz"
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
  BlockHash common.Hash `json:"blockHash"`
  // Array of transaction objects, each object is a byte list (DATA) representing
  // TransactionType || TransactionPayload or LegacyTransaction as defined in EIP-2718
  Transactions []*actions.SEQTransaction `json:"transactions"`
}
type AnchorGetHeaderResponse struct {
  // note: Message should be the anchor req
  Slot uint64 `json:"slot"`
  // nodeID of chunk producing validator.
  Producer ids.NodeID `json:"producer"`
  // block builder address
  PriorityFeeReceiverAddr codec.Address `json:"priorityfeereceiveraddr"`
  // hash of the anchor chunks (tob + robs)
  ParentHash common.Hash            `json:"chunkhash"`
  ToBHash   common.Hash            `json:"tobhash"`
  RoBHashes map[string]common.Hash `json:"robhashes"`
}
type GetPayloadResponse struct {
  Slot        uint64                      `json:"slot"`
  ToBPayload  ExecutionPayload            `json:"tobpayload"`
  RoBPayloads map[string]ExecutionPayload `json:"robpayloads"`
}

// note: likely not needed since we can define our own formats and dealing with SEQ.
type SignedBlindedBeaconBlock struct {
  Bellatrix *boostTypes.SignedBlindedBeaconBlock
  Capella   *apiv1capella.SignedBlindedBeaconBlock
}

func (s *SignedBlindedBeaconBlock) MarshalJSON() ([]byte, error) {
  if s.Capella != nil {
    return json.Marshal(s.Capella)
  }
  if s.Bellatrix != nil {
    return json.Marshal(s.Bellatrix)
  }
  return nil, ErrEmptyPayload
}

func (s *SignedBlindedBeaconBlock) Slot() uint64 {
  if s.Capella != nil {
    return uint64(s.Capella.Message.Slot)
  }
  if s.Bellatrix != nil {
    return s.Bellatrix.Message.Slot
  }
  return 0
}

func (s *SignedBlindedBeaconBlock) BlockHash() string {
  if s.Capella != nil {
    return s.Capella.Message.Body.ExecutionPayloadHeader.BlockHash.String()
  }
  if s.Bellatrix != nil {
    return s.Bellatrix.Message.Body.ExecutionPayloadHeader.BlockHash.String()
  }
  return ""
}

func (s *SignedBlindedBeaconBlock) BlockNumber() uint64 {
  if s.Capella != nil {
    return s.Capella.Message.Body.ExecutionPayloadHeader.BlockNumber
  }
  if s.Bellatrix != nil {
    return s.Bellatrix.Message.Body.ExecutionPayloadHeader.BlockNumber
  }
  return 0
}

func (s *SignedBlindedBeaconBlock) ProposerIndex() uint64 {
  if s.Capella != nil {
    return uint64(s.Capella.Message.ProposerIndex)
  }
  if s.Bellatrix != nil {
    return s.Bellatrix.Message.ProposerIndex
  }
  return 0
}

func (s *SignedBlindedBeaconBlock) Signature() []byte {
  if s.Capella != nil {
    return s.Capella.Signature[:]
  }
  if s.Bellatrix != nil {
    return s.Bellatrix.Signature[:]
  }
  return nil
}

//nolint:nolintlint,ireturn
func (s *SignedBlindedBeaconBlock) Message() boostTypes.HashTreeRoot {
  if s.Capella != nil {
    return s.Capella.Message
  }
  if s.Bellatrix != nil {
    return s.Bellatrix.Message
  }
  return nil
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

type VersionedExecutionPayload struct {
  Bellatrix *boostTypes.GetPayloadResponse
  Capella   *api.VersionedExecutionPayload
}

func (e *VersionedExecutionPayload) MarshalJSON() ([]byte, error) {
  if e.Capella != nil {
    return json.Marshal(e.Capella)
  }
  if e.Bellatrix != nil {
    return json.Marshal(e.Bellatrix)
  }

  return nil, ErrEmptyPayload
}

func (e *VersionedExecutionPayload) UnmarshalJSON(data []byte) error {
  capella := new(api.VersionedExecutionPayload)
  err := json.Unmarshal(data, capella)
  if err == nil && capella.Capella != nil {
    e.Capella = capella
    return nil
  }
  bellatrix := new(boostTypes.GetPayloadResponse)
  err = json.Unmarshal(data, bellatrix)
  if err != nil {
    return err
  }
  e.Bellatrix = bellatrix
  return nil
}

func (e *VersionedExecutionPayload) NumTx() int {
  if e.Capella != nil {
    return len(e.Capella.Capella.Transactions)
  }
  if e.Bellatrix != nil {
    return len(e.Bellatrix.Data.Transactions)
  }
  return 0
}

// note: maybe used for db?
type BuilderSubmitBlockRequest struct {
  AnchorSignature  boostTypes.Signature   `json:"signature" ssz-size:"96"`
  AnchorMessage    *SubmitNewBlockRequest `json:"message"`
  ExecutionPayload *ExecutionPayload      `json:"execution_payload"`
}

func (b *BuilderSubmitBlockRequest) MarshalJSON() ([]byte, error) {
  if b.Capella != nil {
    return json.Marshal(b.Capella)
  }
  if b.Bellatrix != nil {
    return json.Marshal(b.Bellatrix)
  }
  return nil, ErrEmptyPayload
}

func (b *BuilderSubmitBlockRequest) UnmarshalJSON(data []byte) error {
  capella := new(capella.SubmitBlockRequest)
  err := json.Unmarshal(data, capella)
  if err == nil {
    b.Capella = capella
    return nil
  }
  bellatrix := new(boostTypes.BuilderSubmitBlockRequest)
  err = json.Unmarshal(data, bellatrix)
  if err != nil {
    return err
  }
  b.Bellatrix = bellatrix
  return nil
}

func (b *BuilderSubmitBlockRequest) HasExecutionPayload() bool {
  if b.Capella != nil {
    return b.Capella.ExecutionPayload != nil
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.ExecutionPayload != nil
  }
  return false
}

func (b *BuilderSubmitBlockRequest) ExecutionPayloadResponse() (*GetPayloadResponse, error) {
  if b.Bellatrix != nil {
    return &GetPayloadResponse{ //nolint:exhaustruct
      Bellatrix: &boostTypes.GetPayloadResponse{
        Version: boostTypes.VersionString(consensusspec.DataVersionBellatrix.String()),
        Data:    b.Bellatrix.ExecutionPayload,
      },
    }, nil
  }

  if b.Capella != nil {
    return &GetPayloadResponse{ //nolint:exhaustruct
      Capella: &api.VersionedExecutionPayload{ //nolint:exhaustruct
        Version: consensusspec.DataVersionCapella,
        Capella: b.Capella.ExecutionPayload,
      },
    }, nil
  }

  return nil, ErrEmptyPayload
}

func (b *BuilderSubmitBlockRequest) Slot() uint64 {
  if b.Capella != nil {
    return b.Capella.Message.Slot
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.Message.Slot
  }
  return 0
}

func (b *BuilderSubmitBlockRequest) BlockHash() string {
  if b.Capella != nil {
    return b.Capella.Message.BlockHash.String()
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.Message.BlockHash.String()
  }
  return ""
}

func (b *BuilderSubmitBlockRequest) ExecutionPayloadBlockHash() string {
  if b.Capella != nil {
    return b.Capella.ExecutionPayload.BlockHash.String()
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.ExecutionPayload.BlockHash.String()
  }
  return ""
}

func (b *BuilderSubmitBlockRequest) BuilderPubkey() phase0.BLSPubKey {
  if b.Capella != nil {
    return b.Capella.Message.BuilderPubkey
  }
  if b.Bellatrix != nil {
    return phase0.BLSPubKey(b.Bellatrix.Message.BuilderPubkey)
  }
  return phase0.BLSPubKey{}
}

func (b *BuilderSubmitBlockRequest) ProposerFeeRecipient() string {
  if b.Capella != nil {
    return b.Capella.Message.ProposerFeeRecipient.String()
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.Message.ProposerFeeRecipient.String()
  }
  return ""
}

func (b *BuilderSubmitBlockRequest) Timestamp() uint64 {
  if b.Capella != nil {
    return b.Capella.ExecutionPayload.Timestamp
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.ExecutionPayload.Timestamp
  }
  return 0
}

func (b *BuilderSubmitBlockRequest) ProposerPubkey() string {
  if b.Capella != nil {
    return b.Capella.Message.ProposerPubkey.String()
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.Message.ProposerPubkey.String()
  }
  return ""
}

func (b *BuilderSubmitBlockRequest) ParentHash() string {
  if b.Capella != nil {
    return b.Capella.Message.ParentHash.String()
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.Message.ParentHash.String()
  }
  return ""
}

func (b *BuilderSubmitBlockRequest) ExecutionPayloadParentHash() string {
  if b.Capella != nil {
    return b.Capella.ExecutionPayload.ParentHash.String()
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.ExecutionPayload.ParentHash.String()
  }
  return ""
}

func (b *BuilderSubmitBlockRequest) Value() *big.Int {
  if b.Capella != nil {
    return b.Capella.Message.Value.ToBig()
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.Message.Value.BigInt()
  }
  return nil
}

func (b *BuilderSubmitBlockRequest) NumTx() int {
  if b.Capella != nil {
    return len(b.Capella.ExecutionPayload.Transactions)
  }
  if b.Bellatrix != nil {
    return len(b.Bellatrix.ExecutionPayload.Transactions)
  }
  return 0
}

func (b *BuilderSubmitBlockRequest) BlockNumber() uint64 {
  if b.Capella != nil {
    return b.Capella.ExecutionPayload.BlockNumber
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.ExecutionPayload.BlockNumber
  }
  return 0
}

func (b *BuilderSubmitBlockRequest) GasUsed() uint64 {
  if b.Capella != nil {
    return b.Capella.ExecutionPayload.GasUsed
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.ExecutionPayload.GasUsed
  }
  return 0
}

func (b *BuilderSubmitBlockRequest) GasLimit() uint64 {
  if b.Capella != nil {
    return b.Capella.ExecutionPayload.GasLimit
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.ExecutionPayload.GasLimit
  }
  return 0
}

func (b *BuilderSubmitBlockRequest) Signature() phase0.BLSSignature {
  if b.Capella != nil {
    return b.Capella.Signature
  }
  if b.Bellatrix != nil {
    return phase0.BLSSignature(b.Bellatrix.Signature)
  }
  return phase0.BLSSignature{}
}

func (b *BuilderSubmitBlockRequest) Random() string {
  if b.Capella != nil {
    return fmt.Sprintf("%#x", b.Capella.ExecutionPayload.PrevRandao)
  }
  if b.Bellatrix != nil {
    return b.Bellatrix.ExecutionPayload.Random.String()
  }
  return ""
}

func (b *BuilderSubmitBlockRequest) Message() *apiv1.BidTrace {
  if b.Capella != nil {
    return b.Capella.Message
  }
  if b.Bellatrix != nil {
    return BoostBidToBidTrace(b.Bellatrix.Message)
  }
  return nil
}

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

type GetPayloadResponse struct {
  Bellatrix *boostTypes.GetPayloadResponse
  Capella   *api.VersionedExecutionPayload
}

func (p *GetPayloadResponse) UnmarshalJSON(data []byte) error {
  capella := new(api.VersionedExecutionPayload)
  err := json.Unmarshal(data, capella)
  if err == nil && capella.Capella != nil {
    p.Capella = capella
    return nil
  }
  bellatrix := new(boostTypes.GetPayloadResponse)
  err = json.Unmarshal(data, bellatrix)
  if err != nil {
    return err
  }
  p.Bellatrix = bellatrix
  return nil
}

func (p *GetPayloadResponse) MarshalJSON() ([]byte, error) {
  if p.Bellatrix != nil {
    return json.Marshal(p.Bellatrix)
  }
  if p.Capella != nil {
    return json.Marshal(p.Capella)
  }
  return nil, ErrEmptyPayload
}

type GetHeaderResponse struct {
  Bellatrix *boostTypes.GetHeaderResponse
  Capella   *spec.VersionedSignedBuilderBid
}

func (p *GetHeaderResponse) UnmarshalJSON(data []byte) error {
  capella := new(spec.VersionedSignedBuilderBid)
  err := json.Unmarshal(data, capella)
  if err == nil && capella.Capella != nil {
    p.Capella = capella
    return nil
  }
  bellatrix := new(boostTypes.GetHeaderResponse)
  err = json.Unmarshal(data, bellatrix)
  if err != nil {
    return err
  }
  p.Bellatrix = bellatrix
  return nil
}

func (p *GetHeaderResponse) MarshalJSON() ([]byte, error) {
  if p.Capella != nil {
    return json.Marshal(p.Capella)
  }
  if p.Bellatrix != nil {
    return json.Marshal(p.Bellatrix)
  }
  return nil, ErrEmptyPayload
}

func (p *GetHeaderResponse) Value() *big.Int {
  if p.Capella != nil {
    return p.Capella.Capella.Message.Value.ToBig()
  }
  if p.Bellatrix != nil {
    return p.Bellatrix.Data.Message.Value.BigInt()
  }
  return nil
}

func (p *GetHeaderResponse) BlockHash() phase0.Hash32 {
  if p.Capella != nil {
    return p.Capella.Capella.Message.Header.BlockHash
  }
  if p.Bellatrix != nil {
    return phase0.Hash32(p.Bellatrix.Data.Message.Header.BlockHash)
  }
  return phase0.Hash32{}
}

func (p *GetHeaderResponse) Empty() bool {
  if p == nil {
    return true
  }
  if p.Capella != nil {
    return p.Capella.Capella == nil || p.Capella.Capella.Message == nil
  }
  if p.Bellatrix != nil {
    return p.Bellatrix.Data == nil || p.Bellatrix.Data.Message == nil
  }
  return true
}

func (b *BuilderSubmitBlockRequest) Withdrawals() []*consensuscapella.Withdrawal {
  if b.Capella != nil {
    return b.Capella.ExecutionPayload.Withdrawals
  }
  return nil
}

/*
SubmitBlockRequestV2Optimistic is the v2 request from the builder to submit
a block. The message must be SSZ encoded. The first three fields are at most
944 bytes, which fit into a single 1500 MTU ethernet packet. The
`UnmarshalSSZHeaderOnly` function just parses the first three fields,
which is sufficient data to set the bid of the builder. The `Transactions`
and `Withdrawals` fields are required to construct the full SignedBeaconBlock
and are parsed asynchronously.

Header only layout:
[000-236) = Message   (236 bytes)
[236-240) = offset1   (  4 bytes)
[240-336) = Signature ( 96 bytes)
[336-340) = offset2   (  4 bytes)
[340-344) = offset3   (  4 bytes)
[344-944) = EPH       (600 bytes)
type SubmitBlockRequestV2Optimistic struct {
  Message                *apiv1.BidTrace
  ExecutionPayloadHeader *consensuscapella.ExecutionPayloadHeader
  Signature              phase0.BLSSignature              `ssz-size:"96"`
  Transactions           []consensusbellatrix.Transaction `ssz-max:"1048576,1073741824" ssz-size:"?,?"`
  Withdrawals            []*consensuscapella.Withdrawal   `ssz-max:"16"`
}

// MarshalSSZ ssz marshals the SubmitBlockRequestV2Optimistic object
func (s *SubmitBlockRequestV2Optimistic) MarshalSSZ() ([]byte, error) {
  return ssz.MarshalSSZ(s)
}

// UnmarshalSSZ ssz unmarshals the SubmitBlockRequestV2Optimistic object
func (s *SubmitBlockRequestV2Optimistic) UnmarshalSSZ(buf []byte) error {
  var err error
  size := uint64(len(buf))
  if size < 344 {
    return ssz.ErrSize
  }

  tail := buf
  var o1, o3, o4 uint64

  // Field (0) 'Message'
  if s.Message == nil {
    s.Message = new(apiv1.BidTrace)
  }
  if err = s.Message.UnmarshalSSZ(buf[0:236]); err != nil {
    return err
  }

  // Offset (1) 'ExecutionPayloadHeader'
  if o1 = ssz.ReadOffset(buf[236:240]); o1 > size {
    return ssz.ErrOffset
  }

  if o1 < 344 {
    return ssz.ErrInvalidVariableOffset
  }

  // Field (2) 'Signature'
  copy(s.Signature[:], buf[240:336])

  // Offset (3) 'Transactions'
  if o3 = ssz.ReadOffset(buf[336:340]); o3 > size || o1 > o3 {
    return ssz.ErrOffset
  }

  // Offset (4) 'Withdrawals'
  if o4 = ssz.ReadOffset(buf[340:344]); o4 > size || o3 > o4 {
    return ssz.ErrOffset
  }

  // Field (1) 'ExecutionPayloadHeader'
  {
    buf = tail[o1:o3]
    if s.ExecutionPayloadHeader == nil {
      s.ExecutionPayloadHeader = new(consensuscapella.ExecutionPayloadHeader)
    }
    if err = s.ExecutionPayloadHeader.UnmarshalSSZ(buf); err != nil {
      return err
    }
  }

  // Field (3) 'Transactions'
  {
    buf = tail[o3:o4]
    num, err := ssz.DecodeDynamicLength(buf, 1073741824)
    if err != nil {
      return err
    }
    s.Transactions = make([]consensusbellatrix.Transaction, num)
    err = ssz.UnmarshalDynamic(buf, num, func(indx int, buf []byte) (err error) {
      if len(buf) > 1073741824 {
        return ssz.ErrBytesLength
      }
      if cap(s.Transactions[indx]) == 0 {
        s.Transactions[indx] = consensusbellatrix.Transaction(make([]byte, 0, len(buf)))
      }
      s.Transactions[indx] = append(s.Transactions[indx], buf...)
      return nil
    })
    if err != nil {
      return err
    }
  }

  // Field (4) 'Withdrawals'
  {
    buf = tail[o4:]
    num, err := ssz.DivideInt2(len(buf), 44, 16)
    if err != nil {
      return err
    }
    s.Withdrawals = make([]*consensuscapella.Withdrawal, num)
    for ii := 0; ii < num; ii++ {
      if s.Withdrawals[ii] == nil {
        s.Withdrawals[ii] = new(consensuscapella.Withdrawal)
      }
      if err = s.Withdrawals[ii].UnmarshalSSZ(buf[ii*44 : (ii+1)*44]); err != nil {
        return err
      }
    }
  }
  return err
}

// UnmarshalSSZHeaderOnly ssz unmarshals the first 3 fields of the SubmitBlockRequestV2Optimistic object
func (s *SubmitBlockRequestV2Optimistic) UnmarshalSSZHeaderOnly(buf []byte) error {
  var err error
  size := uint64(len(buf))
  if size < 344 {
    return ssz.ErrSize
  }

  tail := buf
  var o1, o3 uint64

  // Field (0) 'Message'
  if s.Message == nil {
    s.Message = new(apiv1.BidTrace)
  }
  if err = s.Message.UnmarshalSSZ(buf[0:236]); err != nil {
    return err
  }

  // Offset (1) 'ExecutionPayloadHeader'
  if o1 = ssz.ReadOffset(buf[236:240]); o1 > size {
    return ssz.ErrOffset
  }

  if o1 < 344 {
    return ssz.ErrInvalidVariableOffset
  }

  // Field (2) 'Signature'
  copy(s.Signature[:], buf[240:336])

  // Offset (3) 'Transactions'
  if o3 = ssz.ReadOffset(buf[336:340]); o3 > size || o1 > o3 {
    return ssz.ErrOffset
  }

  // Field (1) 'ExecutionPayloadHeader'
  {
    buf = tail[o1:o3]
    if s.ExecutionPayloadHeader == nil {
      s.ExecutionPayloadHeader = new(consensuscapella.ExecutionPayloadHeader)
    }
    if err = s.ExecutionPayloadHeader.UnmarshalSSZ(buf); err != nil {
      return err
    }
  }
  return err
}

// MarshalSSZTo ssz marshals the SubmitBlockRequestV2Optimistic object to a target array
func (s *SubmitBlockRequestV2Optimistic) MarshalSSZTo(buf []byte) (dst []byte, err error) {
  dst = buf
  offset := int(344)

  // Field (0) 'Message'
  if s.Message == nil {
    s.Message = new(apiv1.BidTrace)
  }
  if dst, err = s.Message.MarshalSSZTo(dst); err != nil {
    return
  }

  // Offset (1) 'ExecutionPayloadHeader'
  dst = ssz.WriteOffset(dst, offset)
  if s.ExecutionPayloadHeader == nil {
    s.ExecutionPayloadHeader = new(consensuscapella.ExecutionPayloadHeader)
  }
  offset += s.ExecutionPayloadHeader.SizeSSZ()

  // Field (2) 'Signature'
  dst = append(dst, s.Signature[:]...)

  // Offset (3) 'Transactions'
  dst = ssz.WriteOffset(dst, offset)
  for ii := 0; ii < len(s.Transactions); ii++ {
    offset += 4
    offset += len(s.Transactions[ii])
  }

  // Offset (4) 'Withdrawals'
  dst = ssz.WriteOffset(dst, offset)

  // Field (1) 'ExecutionPayloadHeader'
  if dst, err = s.ExecutionPayloadHeader.MarshalSSZTo(dst); err != nil {
    return
  }

  // Field (3) 'Transactions'
  if size := len(s.Transactions); size > 1073741824 {
    err = ssz.ErrListTooBigFn("SubmitBlockRequestV2Optimistic.Transactions", size, 1073741824)
    return
  }
  {
    offset = 4 * len(s.Transactions)
    for ii := 0; ii < len(s.Transactions); ii++ {
      dst = ssz.WriteOffset(dst, offset)
      offset += len(s.Transactions[ii])
    }
  }
  for ii := 0; ii < len(s.Transactions); ii++ {
    if size := len(s.Transactions[ii]); size > 1073741824 {
      err = ssz.ErrBytesLengthFn("SubmitBlockRequestV2Optimistic.Transactions[ii]", size, 1073741824)
      return
    }
    dst = append(dst, s.Transactions[ii]...)
  }

  // Field (4) 'Withdrawals'
  if size := len(s.Withdrawals); size > 16 {
    err = ssz.ErrListTooBigFn("SubmitBlockRequestV2Optimistic.Withdrawals", size, 16)
    return
  }
  for ii := 0; ii < len(s.Withdrawals); ii++ {
    if dst, err = s.Withdrawals[ii].MarshalSSZTo(dst); err != nil {
      return
    }
  }
  return dst, nil
}

// SizeSSZ returns the ssz encoded size in bytes for the SubmitBlockRequestV2Optimistic object
func (s *SubmitBlockRequestV2Optimistic) SizeSSZ() (size int) {
  size = 344

  // Field (1) 'ExecutionPayloadHeader'
  if s.ExecutionPayloadHeader == nil {
    s.ExecutionPayloadHeader = new(consensuscapella.ExecutionPayloadHeader)
  }
  size += s.ExecutionPayloadHeader.SizeSSZ()

  // Field (3) 'Transactions'
  for ii := 0; ii < len(s.Transactions); ii++ {
    size += 4
    size += len(s.Transactions[ii])
  }

  // Field (4) 'Withdrawals'
  size += len(s.Withdrawals) * 44

  return
}

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
  Transactions [][]byte
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

type SubmitNewBlockRequest struct {
  Txs             []*actions.SEQTransaction `json:"txs"`
  Slot            uint64
  ParentHash      string
  BlockHash       common.Hash `json:"block_hash" ssz-size:"32"`
  ProposerPayment codec.Address
  Signature       boostTypes.Signature `json:"signature" ssz-size:"96"`
  BuilderPubkey   boostTypes.PublicKey `json:"builder_pubkey" ssz-size:"48"`
  ProposerPubkey  boostTypes.PublicKey `json:"proposer_pubkey" ssz-size:"48"`
  Value           *big.Int
}

func NewSubmitNewBlockRequest() SubmitNewBlockRequest {
  return SubmitNewBlockRequest{
    Txs: make([]*actions.SEQTransaction, 0),
  }
}

func (r *SubmitNewBlockRequest) FromJSON(data []byte) error {
  return json.Unmarshal(data, r)
}

func (r *SubmitNewBlockRequest) ToJSON() ([]byte, error) {
  return json.Marshal(r)
}

/* DEPRECATED
type ToBTxsSubmitRequest struct {
	ToBTxs          []*actions.SEQTransaction `json:"txs"`
	Slot            uint64
	ParentHash      string
	BlockHash       common.Hash `json:"block_hash" ssz-size:"32"`
	ProposerPayment codec.Address
	Signature       boostTypes.Signature `json:"signature" ssz-size:"96"`
	BuilderPubkey   boostTypes.PublicKey `json:"builder_pubkey" ssz-size:"48"`
	ProposerPubkey  boostTypes.PublicKey `json:"proposer_pubkey" ssz-size:"48"`
}

func NewToBTxsSubmitRequest() ToBTxsSubmitRequest {
	return ToBTxsSubmitRequest{
		ToBTxs: make([]*actions.SEQTransaction, 0),
	}
}

type RoBTxsSubmitRequest struct {
	RoBTxs          []*actions.SEQTransaction `json:"txs"`
	Slot            uint64
	ParentHash      string
	BlockHash       common.Hash `json:"block_hash" ssz-size:"32"`
	ProposerPayment codec.Address
	Signature       boostTypes.Signature `json:"signature" ssz-size:"96"`
	BuilderPubkey   boostTypes.PublicKey `json:"builder_pubkey" ssz-size:"48"`
	ProposerPubkey  boostTypes.PublicKey `json:"proposer_pubkey" ssz-size:"48"`
}
*/

/* DEPRECATED
type IntermediateTobTxsSubmitRequest struct {
	TobTxs     []byte `json:"tobTxs"`
	Slot       uint64 `json:"slot"`
	ParentHash string `json:"parentHash"`
}

// TODO: REVISIT LATER
func (t *ToBTxsSubmitRequest) MarshalJSON() ([]byte, error) {
	txBytes, err := json.Marshal(t.ToBTxs)
	if err != nil {
		return nil, err
	}

	return json.Marshal(IntermediateTobTxsSubmitRequest{
		TobTxs:     txBytes,
		Slot:       t.Slot,
		ParentHash: t.ParentHash,
	})
}

// TODO: REVISIT LATER
func (t *ToBTxsSubmitRequest) UnmarshalJSON(data []byte) error {
	var intermediateJson IntermediateTobTxsSubmitRequest
	err := json.Unmarshal(data, &intermediateJson)
	if err != nil {
		return err
	}

	err = t.ToBTxs.UnmarshalSSZ(intermediateJson.TobTxs)
	if err != nil {
		return err
	}
	t.Slot = intermediateJson.Slot
	t.ParentHash = intermediateJson.ParentHash

	return nil
}

type BlockAssemblerRequest struct {
  TobTxs             ExecutionPayloadTransactions `json:"tob_txs"`
  RobPayload         BuilderSubmitBlockRequest    `json:"rob_payload"`
  RegisteredGasLimit uint64                       `json:"registered_gas_limit,string"`
}

type IntermediateBlockAssemblerRequest struct {
  TobTxs             []byte `json:"tob_txs"`
  RobPayload         []byte `json:"rob_payload"`
  RegisteredGasLimit uint64 `json:"registered_gas_limit,string"`
}

func (r *BlockAssemblerRequest) MarshalJSON() ([]byte, error) {
  sszedTobTxs, err := r.TobTxs.MarshalSSZ()
  if err != nil {
    return nil, err
  }
  encodedRobPayload, err := r.RobPayload.MarshalJSON()
  if err != nil {
    return nil, err
  }
  intermediateStruct := IntermediateBlockAssemblerRequest{
    TobTxs:             sszedTobTxs,
    RobPayload:         encodedRobPayload,
    RegisteredGasLimit: r.RegisteredGasLimit,
  }

  return json.Marshal(intermediateStruct)
}

func (b *BlockAssemblerRequest) UnmarshalJSON(data []byte) error {
  var intermediateJson IntermediateBlockAssemblerRequest
  err := json.Unmarshal(data, &intermediateJson)
  if err != nil {
    return err
  }
  err = b.TobTxs.UnmarshalSSZ(intermediateJson.TobTxs)
  if err != nil {
    return err
  }
  b.RegisteredGasLimit = intermediateJson.RegisteredGasLimit
  blockRequest := new(BuilderSubmitBlockRequest)
  err = json.Unmarshal(intermediateJson.RobPayload, &blockRequest)
  if err != nil {
    return err
  }
  b.RobPayload = *blockRequest

  return nil
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

type TobValidationRequest struct {
  TobTxs               []*actions.SEQTransaction
  ParentHash           string
  ProposerFeeRecipient string
  TobGasLimit          uint64
}

type IntermediateTobValidationRequest struct {
  TobTxs               []byte `json:"tob_txs"`
  ParentHash           string `json:"parent_hash"`
  ProposerFeeRecipient string `json:"proposer_fee_recipient"`
  TobGasLimit          uint64 `json:"tob_gas_limit,string"`
}

func (t *TobValidationRequest) MarshalJson() ([]byte, error) {
  sszedTobTxs, err := t.TobTxs.MarshalSSZ()
  if err != nil {
    return nil, err
  }

  intermediateStruct := IntermediateTobValidationRequest{
    TobTxs:               sszedTobTxs,
    ParentHash:           t.ParentHash,
    ProposerFeeRecipient: t.ProposerFeeRecipient,
    TobGasLimit:          t.TobGasLimit,
  }

  return json.Marshal(intermediateStruct)
}

func (t *TobValidationRequest) UnmarshalJson(data []byte) error {
  var intermediateJson IntermediateTobValidationRequest
  err := json.Unmarshal(data, &intermediateJson)
  if err != nil {
    return err
  }

  err = t.ToBTxs.UnmarshalSSZ(intermediateJson.TobTxs)
  if err != nil {
    return err
  }
  t.ParentHash = intermediateJson.ParentHash
  t.ProposerFeeRecipient = intermediateJson.ProposerFeeRecipient
  t.TobGasLimit = intermediateJson.TobGasLimit

  return nil
}

// TODO: REMOVE ME LATER. USE VERSION FROM WITHIN ANCHOR
type AnchorPayload struct {
  Slot   uint64      `json:"slot"`
  Header common.Hash `json:"blockHash"`
  // Array of transaction objects, each object is a byte list (DATA) representing
  // TransactionType || TransactionPayload or LegacyTransaction as defined in EIP-2718
  Transactions []hexutil.Bytes `json:"seqtransactions"`
}
*/
