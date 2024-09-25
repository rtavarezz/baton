package common

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/flashbots/go-boost-utils/bls"
	"time"

	builderApiV1 "github.com/attestantio/go-builder-client/api/v1"
	"github.com/attestantio/go-eth2-client/spec/bellatrix"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/flashbots/go-boost-utils/utils"
	"github.com/sirupsen/logrus"
)

// TestLog is used to log information in the test methods
var TestLog = logrus.WithField("testing", true)

func check(err error, args ...interface{}) {
	if err != nil {
		TestLog.Error(err, args)
		panic(err)
	}
}

// _HexToAddress converts a hexadecimal string to an Ethereum address
func _HexToAddress(s string) (ret bellatrix.ExecutionAddress) {
	ret, err := utils.HexToAddress(s)
	check(err, " _HexToAddress: ", s)
	return ret
}

// _HexToPubkey converts a hexadecimal string to a BLS Public Key
func _HexToPubkey(s string) (ret phase0.BLSPubKey) {
	ret, err := utils.HexToPubkey(s)
	check(err, " _HexToPubkey: ", s)
	return ret
}

// _HexToSignature converts a hexadecimal string to a BLS Signature
func _HexToSignature(s string) (ret phase0.BLSSignature) {
	ret, err := utils.HexToSignature(s)
	check(err, " _HexToSignature: ", s)
	return ret
}

// _HexToHash converts a hexadecimal string to a Hash
func _HexToHash(s string) (ret phase0.Hash32) {
	ret, err := utils.HexToHash(s)
	check(err, " _HexToHash: ", s)
	return ret
}

var ValidPayloadRegisterValidator = builderApiV1.SignedValidatorRegistration{
	Message: &builderApiV1.ValidatorRegistration{
		FeeRecipient: _HexToAddress("0xdb65fEd33dc262Fe09D9a2Ba8F80b329BA25f941"),
		Timestamp:    time.Unix(1606824043, 0),
		GasLimit:     30000000,
		Pubkey: _HexToPubkey(
			"0x84e975405f8691ad7118527ee9ee4ed2e4e8bae973f6e29aa9ca9ee4aea83605ae3536d22acc9aa1af0545064eacf82e"),
	},
	Signature: _HexToSignature(
		"0xaf12df007a0c78abb5575067e5f8b089cfcc6227e4a91db7dd8cf517fe86fb944ead859f0781277d9b78c672e4a18c5d06368b603374673cf2007966cece9540f3a1b3f6f9e1bf421d779c4e8010368e6aac134649c7a009210780d401a778a5"),
}

func MakeRandomAnchorGetHeaderResponse(pk bls.PublicKey, slot uint64) *AnchorGetHeaderResponse {
	tobHash, err := GenerateRandomHash()
	if err != nil {
		return nil
	}

	tobBlockHashStr, err := GenerateRandomHash()
	tobBlockHash := tobBlockHashStr.String()
	if err != nil {
		return nil
	}

	robHash1, err := GenerateRandomHash()
	if err != nil {
		return nil
	}

	robBlockHashStr1, err := GenerateRandomHash()
	robBlockHash1 := robBlockHashStr1.String()
	if err != nil {
		return nil
	}

	robHash2, err := GenerateRandomHash()
	if err != nil {
		return nil
	}

	robBlockHashStr2, err := GenerateRandomHash()
	robBlockHash2 := robBlockHashStr2.String()
	if err != nil {
		return nil
	}

	tobAnchorHeader := AnchorHeader{
		Header:    tobHash,
		BlockHash: tobBlockHash,
		Value:     uint64(5),
	}

	robAnchorHeader1 := AnchorHeader{
		Header:    robHash1,
		BlockHash: robBlockHash1,
		Value:     uint64(1),
	}

	robAnchorHeader2 := AnchorHeader{
		Header:    robHash2,
		BlockHash: robBlockHash2,
		Value:     uint64(2),
	}

	robHashes := make(map[string]*AnchorHeader, 0)
	robHashes["test-chain-0"] = &robAnchorHeader1
	robHashes["test-chain-1"] = &robAnchorHeader2

	execPayloads := ExecHeadersInfo{
		ToBHash:   &tobAnchorHeader,
		RoBHashes: robHashes,
	}

	//proposerPk, err := bls.PublicKeyFromBytes(pkBytes)
	//if err != nil {
	//	return nil
	//}

	anchorBlockInfo := AnchorBlockInfo{
		Slot: slot,
		// nodeID of chunk producing validator.
		Producer:       ids.NodeID{1},
		ProposerPubkey: pk,
	}

	resp := AnchorGetHeaderResponse{
		ExecHeaders: execPayloads,
		BlockInfo:   anchorBlockInfo,
	}

	return &resp
}
