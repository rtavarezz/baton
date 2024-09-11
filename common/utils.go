package common

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
	boostTypes "github.com/flashbots/go-boost-utils/types"
	"github.com/holiman/uint256"
)

var (
	ErrInvalidForkVersion = errors.New("invalid fork version")
	ErrHTTPErrorResponse  = errors.New("got an HTTP error response")
	ErrIncorrectLength    = errors.New("incorrect length")
)

// SlotPos returns the slot's position in the epoch (1-based, i.e. 1..32)
func SlotPos(slot uint64) uint64 {
	return (slot % SlotsPerEpoch) + 1
}

func makeRequest(ctx context.Context, client http.Client, method, url string, payload any) (*http.Response, error) {
	var req *http.Request
	var err error

	if payload == nil {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	} else {
		payloadBytes, err2 := json.Marshal(payload)
		if err2 != nil {
			return nil, err2
		}
		req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payloadBytes))
	}
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode > 299 {
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return resp, fmt.Errorf("%w: %d / %s", ErrHTTPErrorResponse, resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// ComputeDomain computes the signing domain
func ComputeDomain(domainType types.DomainType, forkVersionHex, genesisValidatorsRootHex string) (domain types.Domain, err error) {
	genesisValidatorsRoot := types.Root(ethcommon.HexToHash(genesisValidatorsRootHex))
	forkVersionBytes, err := hexutil.Decode(forkVersionHex)
	if err != nil || len(forkVersionBytes) != 4 {
		return domain, ErrInvalidForkVersion
	}
	var forkVersion [4]byte
	copy(forkVersion[:], forkVersionBytes[:4])
	return types.ComputeDomain(domainType, forkVersion, genesisValidatorsRoot), nil
}

func GetEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

func GetSliceEnv(key string, defaultValue []string) []string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.Split(value, ",")
	}
	return defaultValue
}

func GetIPXForwardedFor(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		if strings.Contains(forwarded, ",") { // return first entry of list of IPs
			return strings.Split(forwarded, ",")[0]
		}
		return forwarded
	}
	return r.RemoteAddr
}

// GetMevBoostVersionFromUserAgent returns the mev-boost version from an user agent string
// Example ua: "mev-boost/1.0.1 go-http-client" -> returns "1.0.1". If no version is found, returns "-"
func GetMevBoostVersionFromUserAgent(ua string) string {
	parts := strings.Split(ua, " ")
	if strings.HasPrefix(parts[0], "mev-boost") {
		parts2 := strings.Split(parts[0], "/")
		if len(parts2) == 2 {
			return parts2[1]
		}
	}
	return "-"
}

func U256StrToUint256(s types.U256Str) *uint256.Int {
	i := new(uint256.Int)
	i.SetBytes(reverse(s[:]))
	return i
}

func reverse(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	for i := len(dst)/2 - 1; i >= 0; i-- {
		opp := len(dst) - 1 - i
		dst[i], dst[opp] = dst[opp], dst[i]
	}
	return dst
}

// GetEnvStrSlice returns a slice of strings from a comma-separated env var
func GetEnvStrSlice(key string, defaultValue []string) []string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.Split(value, ",")
	}
	return defaultValue
}

func StrToPhase0Pubkey(s string) (ret phase0.BLSPubKey, err error) {
	pubkeyBytes, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return ret, err
	}
	if len(pubkeyBytes) != phase0.PublicKeyLength {
		return ret, ErrIncorrectLength
	}
	copy(ret[:], pubkeyBytes)
	return ret, nil
}

func StrToPhase0Hash(s string) (ret phase0.Hash32, err error) {
	hashBytes, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return ret, err
	}
	if len(hashBytes) != phase0.Hash32Length {
		return ret, ErrIncorrectLength
	}
	copy(ret[:], hashBytes)
	return ret, nil
}

// GetEnvDurationSec returns the value of the environment variable as duration in seconds,
// or defaultValue if the environment variable doesn't exist or is not a valid integer
func GetEnvDurationSec(key string, defaultValueSec int) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		val, err := strconv.Atoi(value)
		if err != nil {
			return time.Duration(val) * time.Second
		}
	}
	return time.Duration(defaultValueSec) * time.Second
}

func GetMethodArgs(data []byte, method string, contractAbi *abi.ABI) (interface{}, error) {
	abiMethod := contractAbi.Methods[method]
	res, err := abiMethod.Inputs.Unpack(data[4:])
	if err != nil {
		return nil, err
	}

	return res[0], nil
}

// Generic function to convert map values to a slice
func MapValuesToSlice[K comparable, V any](m map[K]V) []V {
	values := make([]V, 0, len(m)) // Preallocate slice with the map's length
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

func GetSignatureForChunk(block *BatonBlock, privateKeyHex string) (*boostTypes.Signature, error) {
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, err
	}

	blockJson, err := json.Marshal(*block)
	if err != nil {
		return nil, err
	}

	return SignMessage(blockJson, privateKey)
}

func VerifySignatureForChunk(
	block *BatonBlock,
	signature *boostTypes.Signature,
	expectedPublicKey *boostTypes.PublicKey,
) (bool, error) {
	blockJson, err := json.Marshal(*block)
	if err != nil {
		return false, err
	}

	return VerifySignature(blockJson, signature, expectedPublicKey)
}

// SignMessage signs a message using the given private key and returns a [96]byte signature.
func SignMessage(message []byte, privateKey *ecdsa.PrivateKey) (*boostTypes.Signature, error) {
	var flashbotsSignature boostTypes.Signature

	// Hash the message using Keccak256
	hash := crypto.Keccak256Hash(message)

	// Sign the hash with the private key
	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		return nil, err
	}

	copy(flashbotsSignature[:65], signature)
	return &flashbotsSignature, nil
}

func VerifySignature(
	message []byte,
	signature *boostTypes.Signature,
	flashbotsPubKey *boostTypes.PublicKey,
) (bool, error) {
	// Hash the message using Keccak256
	hash := crypto.Keccak256Hash(message)

	// Extract the first 65 bytes, which contain the actual ECDSA signature
	ecdsaSignature := signature[:65]

	// Ensure the v value is in the correct range (27 or 28)
	if ecdsaSignature[64] < 27 {
		ecdsaSignature[64] += 27
	}

	// Decompress the [48]byte Flashbots public key to get X, Y coordinates
	x, y := secp256k1.DecompressPubkey(flashbotsPubKey[:])
	if x == nil || y == nil {
		return false, fmt.Errorf("failed to decompress public key")
	}

	// Convert the X, Y coordinates into an *ecdsa.PublicKey
	pubKey := ConvertDecompressedPubKey(x, y)

	// Recover the public key from the signature
	recoveredPubKey, err := crypto.SigToPub(hash.Bytes(), ecdsaSignature[:])
	if err != nil {
		return false, fmt.Errorf("failed to recover public key from signature: %w", err)
	}

	// Compare the recovered public key with the provided public key
	match := bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(recoveredPubKey))

	return match, nil
}

func ConvertDecompressedPubKey(x, y *big.Int) *ecdsa.PublicKey {
	return &ecdsa.PublicKey{
		Curve: secp256k1.S256(),
		X:     x,
		Y:     y,
	}
}

func GetURI(url *url.URL, path string) string {
	u2 := *url
	u2.User = nil
	u2.Path = path
	return u2.String()
}
