package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/AnomalyFi/baton/common"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/ethereum/go-ethereum/log"
	"github.com/flashbots/go-boost-utils/bls"

	eth "github.com/ethereum/go-ethereum/common"
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

func BuildHeader(s *common.SubmitNewBlockRequest, value uint64) (common.AnchorHeader, error) {
	header, err := hashHeader(s)
	if err != nil {
		log.Error("failed to hash header")
	}
	var anchorHeader common.AnchorHeader
	anchorHeader.Header = header
	anchorHeader.BlockHash = s.BlockHash().String()
	anchorHeader.Value = value
	return anchorHeader, nil
}

func BuildPayload(s *common.SubmitNewBlockRequest, hypersdkTxs []byte, value uint64) (*common.AnchorPayload, error) {
	hash, err := BuildHeader(s, value)
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
