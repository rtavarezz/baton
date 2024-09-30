package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AnomalyFi/baton/common"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/ethereum/go-ethereum/log"
	"github.com/flashbots/go-boost-utils/bls"

	eth "github.com/ethereum/go-ethereum/common"
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

func checkBLSPublicKeyHex(pkHex string) error {
	//var proposerPubkey boostTypes.PublicKey
	//return proposerPubkey.UnmarshalText([]byte(pkHex))
	var proposerPubKey bls.PublicKey
	return proposerPubKey.Unmarshal([]byte(pkHex))
}

// ConvertPhase0ToBLSPubKey deserializes the phase0 BLSPubKey bytes to a bls.PublicKey
func ConvertPhase0ToBLSPubKey(phase0PubKey phase0.BLSPubKey) (*bls.PublicKey, error) {
	blsPubKey, err := bls.PublicKeyFromBytes(phase0PubKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize BLS public key: %w", err)
	}
	return blsPubKey, nil
}

func hasReachedFork(slot, forkEpoch uint64) bool {
	currentEpoch := slot / common.SlotsPerEpoch
	return currentEpoch >= forkEpoch
}

func Sha256ToCommonHash(data []byte) eth.Hash {
	shaHash := sha256.Sum256(data)
	return eth.BytesToHash(shaHash[:])
}

func hashHeader(s *common.SubmitNewBlockRequest) (eth.Hash, error) {
	hasher := sha256.New()

	// Serialize the struct to JSON
	structBytes, err := json.Marshal(s.Chunk)
	if err != nil {
		return eth.Hash{}, err
	}

	// Write the struct bytes to the hasher
	hasher.Write(structBytes)

	// Compute and return the hash as common.Hash
	hash := Sha256ToCommonHash(hasher.Sum(nil))
	return hash, nil
}

func BuildHeader(s *common.SubmitNewBlockRequest) (common.AnchorHeader, error) {
	header, err := hashHeader(s)
	if err != nil {
		log.Error("failed to hash header")
	}
	var anchorHeader common.AnchorHeader
	anchorHeader.Header = header
	anchorHeader.BlockHash = s.BlockHash().String()
	return anchorHeader, nil
}

func BuildPayload(s *common.SubmitNewBlockRequest, hypersdkTxs []byte) (*common.AnchorPayload, error) {
	hash, err := BuildHeader(s)
	if err != nil {
		log.Error("failed to hash header")
	}

	payload := common.AnchorPayload{
		Slot:         s.Chunk.Slot,
		Header:       hash.Header,
		Transactions: hypersdkTxs,
	}

	return &payload, nil
}
