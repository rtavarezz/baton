package common

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/ssz"

	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
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
func ComputeDomain(domainType phase0.DomainType, forkVersionHex, genesisValidatorsRootHex string) (domain phase0.Domain, err error) {
	genesisValidatorsRoot := phase0.Root(ethcommon.HexToHash(genesisValidatorsRootHex))
	forkVersionBytes, err := hexutil.Decode(forkVersionHex)
	if err != nil || len(forkVersionBytes) != 4 {
		return domain, ErrInvalidForkVersion
	}
	var forkVersion [4]byte
	copy(forkVersion[:], forkVersionBytes[:4])
	return ssz.ComputeDomain(domainType, forkVersion, genesisValidatorsRoot), nil
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

func PublicKeyToByteString(pk *bls.PublicKey) string {
	pubKeyBytes := pk.Bytes()
	// pubKeyBytesAsStr := string(pubKeyBytes[:])
	pubKeyBytesAsStr := hex.EncodeToString(pubKeyBytes[:])
	return pubKeyBytesAsStr
}

func MustB64Gunzip(s string) []byte {
	b, _ := base64.StdEncoding.DecodeString(s)
	gzreader, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		panic(err)
	}
	output, err := io.ReadAll(gzreader)
	if err != nil {
		panic(err)
	}
	return output
}

// rob,boost-relay/:cache-gethead-response:1_0x3078313365363036633762336431666161643765383335303363653364656463_a2d4448cd0db7db072960cf0077332bef49d9c54850d6f54b167975c1b4598b01ddc80d05f8afd12d7fea12715bedbb5_test-chain-0
// rob,boost-relay/:cache-gethead-response:1_0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747_a2d4448cd0db7db072960cf0077332bef49d9c54850d6f54b167975c1b4598b01ddc80d05f8afd12d7fea12715bedbb5_test-chain-0
