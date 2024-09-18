package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-utils/cli"
	"github.com/flashbots/go-utils/jsonrpc"
	"github.com/flashbots/mev-boost-relay/common"
)

var (
	ErrRequestClosed    = errors.New("request context closed")
	ErrSimulationFailed = errors.New("simulation failed")
	ErrJSONDecodeFailed = errors.New("json error")

	maxConcurrentBlocks = int64(cli.GetEnvInt("BLOCKSIM_MAX_CONCURRENT", 4)) // 0 for no maximum
	simRequestTimeout   = time.Duration(cli.GetEnvInt("BLOCKSIM_TIMEOUT_MS", 10000)) * time.Millisecond
)

type IBlockSimRateLimiter interface {
	//Send(context context.Context, payload *common.BuilderBlockValidationRequest, isHighPrio, fastTrack bool) (error, error)
	SimBlockAndGetGasUsedForChain(context context.Context, chainID string, req *common.BlockValidationRequest) (uint64, error, error)
	RegisterSimulator(req *SimulatorRegisterRequest) (bool, error)
	CurrentCounter() int64
}

type BlockSimulationRateLimiter struct {
	cv      *sync.Cond
	counter int64

	blockSimURLs map[string]string // chainID -> builder url
	client       http.Client

	manager *bls.PublicKey
}

func NewBlockSimulationRateLimiter(manager *bls.PublicKey) *BlockSimulationRateLimiter {
	return &BlockSimulationRateLimiter{
		cv:           sync.NewCond(&sync.Mutex{}),
		counter:      0,
		manager:      manager,
		blockSimURLs: make(map[string]string),
		client: http.Client{ //nolint:exhaustruct
			Timeout: simRequestTimeout,
		},
	}
}

type SimulatorInfo struct {
	URL string `json:"url"`
}

type SimulatorRegisterRequest struct {
	Simulator SimulatorInfo `json:"simulator"`
	Pubkey    []byte        `json:"pubkey"`
	Signature []byte        `json:"signature"`
}

type SimulatorRegisterResponse struct {
	Success bool `json:"success"`
}

func (b *BlockSimulationRateLimiter) RegisterSimulator(req *SimulatorRegisterRequest) (bool, error) {
	pubkey, err := bls.PublicKeyFromBytes(req.Pubkey)
	if err != nil {
		return false, err
	}
	if !pubkey.Equal(b.manager) {
		return false, fmt.Errorf("unpriviliged request from %s", hexutil.Encode(req.Pubkey))
	}

	sig, err := bls.SignatureFromBytes(req.Signature)
	if err != nil {
		return false, err
	}

	msg, err := json.Marshal(req.Simulator)
	if err != nil {
		return false, err
	}

	verified, err := bls.VerifySignature(sig, pubkey, msg)
	if err != nil {
		return false, err
	}

	return verified, nil
}

func (b *BlockSimulationRateLimiter) SimBlockAndGetGasUsedForChain(context context.Context, chainID string, req *common.BlockValidationRequest) (uint64, error, error) {
	if _, ok := b.blockSimURLs[chainID]; !ok {
		return 0, fmt.Errorf("unsupported chain %s", chainID), nil
	}

	return b.simBlockAndGetGasUsed(context, b.blockSimURLs[chainID], req)
}

func (b *BlockSimulationRateLimiter) simBlockAndGetGasUsed(context context.Context, simUrl string, request *common.BlockValidationRequest) (uint64, error, error) {
	b.cv.L.Lock()
	cnt := atomic.AddInt64(&b.counter, 1)
	if maxConcurrentBlocks > 0 && cnt > maxConcurrentBlocks {
		b.cv.Wait()
	}
	b.cv.L.Unlock()

	defer func() {
		b.cv.L.Lock()
		atomic.AddInt64(&b.counter, -1)
		b.cv.Signal()
		b.cv.L.Unlock()
	}()

	if err := context.Err(); err != nil {
		return 0, fmt.Errorf("%w, %w", ErrRequestClosed, err), nil
	}

	var simReq *jsonrpc.JSONRPCRequest
	var gasUsed uint64

	// Create and fire off JSON-RPC request
	simReq = jsonrpc.NewJSONRPCRequest("1", "eth_callBundle", request)
	resp, requestErr, validationErr := SendJSONRPCRequest(&b.client, *simReq, simUrl, nil)

	// read out gas used to simulate bundle
	var callBundleResult common.FlashbotsCallBundleResult
	if requestErr != nil && validationErr != nil {
		err := json.Unmarshal(resp.Result, &callBundleResult)
		if err != nil {
			log.Error("simBlockAndGetGasUsed error unmarshaling call bundle json:", err)
			return 0, fmt.Errorf("%w, %w", ErrJSONDecodeFailed, err), validationErr
		}
		gasUsed = uint64(callBundleResult.TotalGasUsed)
	}

	return gasUsed, requestErr, validationErr
}

// CurrentCounter returns the number of waiting and active requests
func (b *BlockSimulationRateLimiter) CurrentCounter() int64 {
	return atomic.LoadInt64(&b.counter)
}

// SendJSONRPCRequest sends the request to URL and returns the general JsonRpcResponse, or an error (note: not the JSONRPCError)
func SendJSONRPCRequest(client *http.Client, req jsonrpc.JSONRPCRequest, url string, headers http.Header) (res *jsonrpc.JSONRPCResponse, requestErr, validationErr error) {
	buf, err := json.Marshal(req)
	if err != nil {
		return nil, err, nil
	}

	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err, nil
	}

	// set request headers
	httpReq.Header.Add("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Add(k, v[0])
	}

	// execute request
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err, nil
	}
	defer resp.Body.Close()

	// read all resp bytes
	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response bytes: %w", err), nil
	}

	// try json parsing
	res = new(jsonrpc.JSONRPCResponse)
	if err := json.NewDecoder(bytes.NewReader(rawResp)).Decode(res); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrJSONDecodeFailed, string(rawResp[:])), nil
	}

	if res.Error != nil {
		return res, nil, fmt.Errorf("%w: %s", ErrSimulationFailed, res.Error.Message)
	}
	return res, nil, nil
}
