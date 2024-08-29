package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/AnomalyFi/hypersdk/chain"
	actions "github.com/AnomalyFi/seq-sdk/types"
	"github.com/ethereum/go-ethereum/log"

	"github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	utilcapella "github.com/attestantio/go-eth2-client/util/capella"
	eth "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	boostTypes "github.com/flashbots/go-boost-utils/types"
	"github.com/flashbots/mev-boost-relay/common"
)

var (
	ErrBlockHashMismatch  = errors.New("blockHash mismatch")
	ErrParentHashMismatch = errors.New("parentHash mismatch")

	ErrNoPayloads               = errors.New("no payloads")
	ErrNoWithdrawals            = errors.New("no withdrawals")
	ErrPayloadMismatchBellatrix = errors.New("bellatrix beacon-block but no bellatrix payload")
	ErrPayloadMismatchCapella   = errors.New("capella beacon-block but no capella payload")
	ErrHeaderHTRMismatch        = errors.New("beacon-block and payload header mismatch")
)

func SanityCheckBuilderBlockSubmission(payload *common.BuilderSubmitBlockRequest) error {
	if payload.BlockHash() != payload.ExecutionPayloadBlockHash() {
		return ErrBlockHashMismatch
	}

	if payload.ParentHash() != payload.ExecutionPayloadParentHash() {
		return ErrParentHashMismatch
	}

	return nil
}

func ComputeWithdrawalsRoot(w []*capella.Withdrawal) (phase0.Root, error) {
	if w == nil {
		return phase0.Root{}, ErrNoWithdrawals
	}
	withdrawals := utilcapella.ExecutionPayloadWithdrawals{Withdrawals: w}
	return withdrawals.HashTreeRoot()
}

func EqExecutionPayloadToHeader(bb *common.SignedBlindedBeaconBlock, payload *common.VersionedExecutionPayload) error {
	if bb.Bellatrix != nil { // process Bellatrix beacon block
		if payload.Bellatrix == nil {
			return ErrPayloadMismatchBellatrix
		}
		bbHeaderHtr, err := bb.Bellatrix.Message.Body.ExecutionPayloadHeader.HashTreeRoot()
		if err != nil {
			return err
		}

		payloadHeader, err := boostTypes.PayloadToPayloadHeader(payload.Bellatrix.Data)
		if err != nil {
			return err
		}
		payloadHeaderHtr, err := payloadHeader.HashTreeRoot()
		if err != nil {
			return err
		}

		if bbHeaderHtr != payloadHeaderHtr {
			return ErrHeaderHTRMismatch
		}

		// bellatrix block and payload are equal
		return nil
	}

	if bb.Capella != nil { // process Capella beacon block
		if payload.Capella == nil {
			return ErrPayloadMismatchCapella
		}

		bbHeaderHtr, err := bb.Capella.Message.Body.ExecutionPayloadHeader.HashTreeRoot()
		if err != nil {
			return err
		}

		payloadHeader, err := common.CapellaPayloadToPayloadHeader(payload.Capella.Capella)
		if err != nil {
			return err
		}
		payloadHeaderHtr, err := payloadHeader.HashTreeRoot()
		if err != nil {
			return err
		}

		if bbHeaderHtr != payloadHeaderHtr {
			return ErrHeaderHTRMismatch
		}

		// capella block and payload are equal
		return nil
	}

	return ErrNoPayloads
}

func checkBLSPublicKeyHex(pkHex string) error {
	var proposerPubkey boostTypes.PublicKey
	return proposerPubkey.UnmarshalText([]byte(pkHex))
}

func hasReachedFork(slot, forkEpoch uint64) bool {
	currentEpoch := slot / common.SlotsPerEpoch
	return currentEpoch >= forkEpoch
}

func ConvertTxBytesToTransaction(data hexutil.Bytes) (*types.Transaction, error) {
	rawBytes := []byte(data)
	tx := &types.Transaction{}
	if err := tx.UnmarshalBinary(rawBytes); err != nil {
		return nil, errors.New("ConvertTxBytesToTransaction() could convert bytes to transaction")
	}
	return tx, nil
}

func Sha256ToCommonHash(data []byte) eth.Hash {
	shaHash := sha256.Sum256(data)
	return eth.BytesToHash(shaHash[:])
}

func hashHeader(s *common.SubmitNewBlockRequest) (eth.Hash, error) {
	hasher := sha256.New()

	// Serialize the struct to JSON
	structBytes, err := json.Marshal(s)
	if err != nil {
		return eth.Hash{}, err
	}

	// Write the struct bytes to the hasher
	hasher.Write(structBytes)

	// Compute and return the hash as common.Hash
	hash := Sha256ToCommonHash(hasher.Sum(nil))
	return hash, nil
}

func buildHeader(s *common.SubmitNewBlockRequest) (eth.Hash, error) {
	header, err := hashHeader(s)
	if err != nil {
		log.Error("failed to hash header")
	}
	return header, nil
}

//	type AnchorPayload struct {
//		Slot      uint64      `json:"slot"`
//		Header common.Hash `json:"blockHash"`
//		// Array of transaction objects, each object is a byte list (DATA) representing
//		// TransactionType || TransactionPayload or LegacyTransaction as defined in EIP-2718
//		Transactions []*SEQTransaction `json:"transactions"`
//	}
func buildPayload(s *common.SubmitNewBlockRequest) (*common.AnchorPayload, error) {
	hash, err := buildHeader(s)
	if err != nil {
		log.Error("failed to hash header")
	}

	seqTxs, err := marshalTxs(s.Txs)
	if err != nil {
		log.Error("failed to marshal txs, err: " + err.Error())
		return nil, err
	}

	payload := common.AnchorPayload{
		Slot:         s.Slot,
		Header:       hash,
		Transactions: seqTxs,
	}

	return &payload, nil
}

func marshalTxs(txs []*chain.Transaction) ([]hexutil.Bytes, error) {
	ret := make([]hexutil.Bytes, len(txs))
	for i, _ := range txs {
		seqTxBytes, err := json.Marshal(txs[i])
		if err != nil {
			logMsg := "failed to marshal seq tx with index " + strconv.Itoa(i) + ": " + err.Error()
			log.Error(logMsg)
			return nil, err
		}
		ret[i] = seqTxBytes
	}
	return ret, nil
}
