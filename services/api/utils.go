package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/mev-boost-relay/common"
	"strconv"

	"github.com/AnomalyFi/hypersdk/chain"
	"github.com/ethereum/go-ethereum/log"

	eth "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
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
	anchorHeader.Header = &header
	anchorHeader.BlockHash = s.BlockHash().String()
	return anchorHeader, nil
}

func BuildPayload(s *common.SubmitNewBlockRequest, txs []*chain.Transaction) (*common.AnchorPayload, error) {
	hash, err := BuildHeader(s)
	if err != nil {
		log.Error("failed to hash header")
	}

	seqTxs, err := marshalTxs(txs)
	if err != nil {
		log.Error("failed to marshal txs, err: " + err.Error())
		return nil, err
	}

	payload := common.AnchorPayload{
		Slot:         s.Chunk.Slot,
		Header:       *hash.Header,
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

// helper funcs used for verifying signatures
//func HashSha(data []byte) []byte {
//	hash := sha256.Sum256(data)
//	return hash[:]
//}

// computes the Merkle root of the given hashes
//func ComputeMerkleRoot(hashes [][]byte) []byte {
//	if len(hashes) == 0 {
//		return nil
//	}
//	for len(hashes) > 1 {
//		var newLevel [][]byte
//		// hashes in pairs
//		for i := 0; i < len(hashes); i += 2 {
//			if i+1 < len(hashes) {
//				combined := append(hashes[i], hashes[i+1]...)
//				newLevel = append(newLevel, HashSha(combined))
//			} else {
//				newLevel = append(newLevel, hashes[i])
//			}
//		}
//		hashes = newLevel
//	}
//	return hashes[0]
//}

func VerifySignature(header *common.ExecHeadersInfo, signature bls.Signature, pubKey common.PubkeyHex) error {
	//// work to combine hashes
	//var hashes [][]byte
	//if header.ToBHash != nil && header.ToBHash.Header != nil {
	//	hashes = append(hashes, HashSha(header.ToBHash.Header.Bytes()))
	//}
	//for _, robHash := range header.RoBHashes {
	//	if robHash.Header != nil {
	//		hashes = append(hashes, HashSha(robHash.Header.Bytes()))
	//	}
	//}
	//// get root for list of hashes
	//merkleRoot := ComputeMerkleRoot(hashes)
	//// verify the signature with pubkey and merkleroot and signature
	//// TODO: may need fixing( added signature and pubkey to make logic easier but need to double check if this is okay or not)
	//pubKeyBytes := []byte(pubKey)
	//if !crypto.VerifySignature(pubKeyBytes, merkleRoot, signature) {
	//	return errors.New("signature verification failed")
	//}
	return nil
}
