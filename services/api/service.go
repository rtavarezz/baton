// Package api contains the API webserver for the proposer and block-builder APIs
package api

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnomalyFi/baton/common"
	"github.com/AnomalyFi/baton/database"
	"github.com/AnomalyFi/baton/datastore"
	"github.com/AnomalyFi/baton/seq"

	"github.com/AnomalyFi/hypersdk/crypto/ed25519"
	hrpc "github.com/AnomalyFi/hypersdk/rpc"

	"github.com/AnomalyFi/baton/beaconclient"
	"github.com/AnomalyFi/hypersdk/chain"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/flashbots/go-boost-utils/utils"
	"github.com/go-redis/redis/v9"

	"github.com/AnomalyFi/nodekit-seq/actions"
	srpc "github.com/AnomalyFi/nodekit-seq/rpc"
	"github.com/NYTimes/gziphandler"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	common2 "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-utils/cli"
	"github.com/flashbots/go-utils/httplogger"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	uberatomic "go.uber.org/atomic"
	"golang.org/x/exp/slices"
)

const (
	ErrBlockAlreadyKnown           = "simulation failed: block already known"
	ErrBlockRequiresReorg          = "simulation failed: block requires a reorg"
	ErrMissingTrieNode             = "missing trie node"
	SeqUnmarshalTxsInitialCapacity = 100
)

var (
	ErrMissingLogOpt              = errors.New("log parameter is nil")
	ErrMissingDatastoreOpt        = errors.New("proposer datastore is nil")
	ErrRelayPubkeyMismatch        = errors.New("baton pubkey does not match existing one")
	ErrServerAlreadyStarted       = errors.New("server was already started")
	ErrBuilderAPIWithoutSecretKey = errors.New("cannot start builder API without secret key")
	ErrMissingSeqURL              = errors.New("cannot start Baton without SEQ URL")
)

var (
	// Proposer API (builder-specs)
	pathStatus            = "/eth/v1/builder/status"
	pathRegisterValidator = "/eth/v1/builder/validators"
	pathGetHeader         = "/eth/v1/builder/header/{slot}/{parent_hash}/{pubkey}"
	pathGetPayload        = "/eth/v1/builder/blinded_blocks"

	// Block Simulator API
	pathRegisterSimulator = "/sim/v1/register"

	// Block builder API
	pathBuilderGetValidators  = "/baton/v1/builder/validators"
	pathSubmitNewBlockRequest = "/baton/v1/builder/submit"
	pathGetTobGasReservations = "/baton/v1/builder/tob_gas_reservations"

	// Data API
	pathDataProposerPayloadDelivered = "/baton/v1/data/bidtraces/proposer_payload_delivered"
	pathDataBuilderBidsReceived      = "/baton/v1/data/bidtraces/builder_blocks_received"
	pathDataValidatorRegistration    = "/baton/v1/data/validator_registration"
	pathIncludedTobTxs               = "/baton/v1/data/included_tob_txs/{slot:[0-9]+}/{parent_hash:0x[a-fA-F0-9]+}/{block_hash:0x[a-fA-F0-9]+}"

	// Internal API
	pathInternalBuilderStatus     = "/internal/v1/builder/{pubkey:0x[a-fA-F0-9]+}"
	pathInternalBuilderCollateral = "/internal/v1/builder/collateral/{pubkey:0x[a-fA-F0-9]+}"

	// Testing APIs
	// TODO - lets keep this for v0 launch for ease of testing but after that remove it.
	pathGetSlot              = "/eth/v1/baton/get_head_slot"
	pathGetParentHashForSlot = "/eth/v1/baton/get_parent_hash_for_slot/{slot:[0-9]+}"
	pathGetProposerForSlot   = "/eth/v1/baton/get_proposer_for_slot/{slot:[0-9]+}"

	// various timings
	timeoutGetPayloadRetryMs = cli.GetEnvInt("GETPAYLOAD_RETRY_TIMEOUT_MS", 100)

	// api settings
	apiReadTimeoutMs       = cli.GetEnvInt("API_TIMEOUT_READ_MS", 1500)
	apiReadHeaderTimeoutMs = cli.GetEnvInt("API_TIMEOUT_READHEADER_MS", 600)
	apiIdleTimeoutMs       = cli.GetEnvInt("API_TIMEOUT_IDLE_MS", 3_000)
	apiWriteTimeoutMs      = cli.GetEnvInt("API_TIMEOUT_WRITE_MS", 10_000)
	apiMaxHeaderBytes      = cli.GetEnvInt("API_MAX_HEADER_BYTES", 60_000)

	// api shutdown: wait time (to allow removal from load balancer before stopping http server)
	apiShutdownWaitDuration = common.GetEnvDurationSec("API_SHUTDOWN_WAIT_SEC", 30)

	// api shutdown: whether to stop sending bids during shutdown phase (only useful if running a single-instance testnet setup)
	apiShutdownStopSendingBids = os.Getenv("API_SHUTDOWN_STOP_SENDING_BIDS") == "1"

	// maximum payload bytes for a block submission to be fast-tracked (large payloads slow down other fast-tracked requests!)
	fastTrackPayloadSizeLimit = cli.GetEnvInt("FAST_TRACK_PAYLOAD_SIZE_LIMIT", 230_000)

	// user-agents which shouldn't receive bids
	apiNoHeaderUserAgents = common.GetEnvStrSlice("NO_HEADER_USERAGENTS", []string{
		"mev-boost/v1.5.0 Go-http-client/1.1", // Prysm v4.0.1 (Shapella signing issue)
	})
)

// BatonAPIOpts contains the options for a baton
type BatonAPIOpts struct {
	Log              *logrus.Entry
	SeqURL           string
	SeqChainID       ids.ID
	SeqNetworkID     uint32
	SeqSigningKey    ed25519.PrivateKey
	SeqBlockWaitTime time.Duration

	ListenAddr         string
	BlockSimURL        string
	BlockSimManager    *bls.PublicKey // bls pubkey of the sim manager
	BlockSimSigningKey *ecdsa.PrivateKey
	BlockSimDepth      int

	BeaconClient beaconclient.IMultiBeaconClient
	Datastore    *datastore.Datastore
	Redis        *datastore.RedisCache
	Memcached    *datastore.Memcached
	DB           database.IDatabaseService

	SlotSizeLimit      int // how many bytes are allowed in a slot, current SEQ upper bound for one block is 2MB, we should left space for normal SEQ txs and the overhead of wrapping a block
	FutureSlotsAllowed int

	SecretKey *bls.SecretKey // used to sign bids (getHeader responses)

	// Network specific variables
	EthNetDetails common.EthNetworkDetails

	// APIs to enable
	ProposerAPI     bool
	BlockBuilderAPI bool
	DataAPI         bool
	PprofAPI        bool
	InternalAPI     bool

	// Mock mode assists in testing and helps us skip difficult to test functionality (like simulation).
	mockMode bool
}

// type blockBuilderCacheEntry struct {
// 	status     common.BuilderStatus
// 	collateral *big.Int
// }

type blockSimResult struct {
	wasSimulated         bool
	optimisticSubmission bool
	requestErr           error
	validationErr        error
}

// BatonAPI represents a single Relay instance
type BatonAPI struct {
	opts BatonAPIOpts  `jjj:"opts"`
	log  *logrus.Entry `jjj:"log"`

	blsSk     *bls.SecretKey `jjj:"bls_sk"`
	publicKey *bls.PublicKey `jjj:"public_key"`

	srv         *http.Server    `jjj:"srv"`
	srvStarted  uberatomic.Bool `jjj:"srv_started"`
	srvShutdown uberatomic.Bool `jjj:"srv_shutdown"`

	// TODO: to be removed
	// beaconClient beaconclient.IMultiBeaconClient `jjj:"beacon_client"`
	datastore *datastore.Datastore      `jjj:"datastore"`
	redis     *datastore.RedisCache     `jjj:"redis"`
	memcached *datastore.Memcached      `jjj:"memcached"`
	db        database.IDatabaseService `jjj:"db"`

	headSlot    uberatomic.Uint64                `jjj:"head_slot"`
	genesisInfo *beaconclient.GetGenesisResponse `jjj:"genesis_info"`

	proposerDutiesLock     sync.RWMutex                                           `jjj:"proposer_duties_lock"`
	proposerDutiesResponse *[]byte                                                `jjj:"proposer_duties_response"` // raw http response
	proposerDutiesMap      map[uint64]*common.BuilderGetSEQValidatorResponseEntry `jjj:"proposer_duties_map"`

	blockSimRateLimiter IBlockSimRateLimiter `jjj:"block_sim_rate_limiter"`
	blockSimDepth       int                  `jjj:"block_sim_depth"` // combined txs range from [headSlot:headSlot-depth] to simulate, since there are situations that 1. L2 runs slower than SEQ 2. l2-builder needs time to sync with l2-geth
	tracer              ITracer              `jjj:"tracer"`

	validatorRegC chan common.SignedSEQValidatorRegistration `jjj:"validator_reg_c"`

	// used to wait on any active getPayload calls on shutdown
	getPayloadCallsInFlight sync.WaitGroup `jjj:"get_payload_calls_in_flight"`

	// Feature flags
	ffForceGetHeader204          bool `jjj:"ff_force_get_header_204"`
	ffDisableLowPrioBuilders     bool `jjj:"ff_disable_low_prio_builders"`
	ffDisablePayloadDBStorage    bool `jjj:"ff_disable_payload_db_storage"`      // disable storing the execution payloads in the database
	ffLogInvalidSignaturePayload bool `jjj:"ff_log_invalid_signature_payload"`   // log payload if getPayload signature validation fails
	ffEnableCancellations        bool `jjj:"ff_enable_cancellations"`            // whether to enable block builder cancellations
	ffRegValContinueOnInvalidSig bool `jjj:"ff_reg_val_continue_on_invalid_sig"` // whether to continue processing further validators if one fails
	ffIgnorableValidationErrors  bool `jjj:"ff_ignorable_validation_errors"`     // whether to enable ignorable validation errors
	ffMockSimulation             bool `jjj:"ff_mock_simulation"`                 // simulations always pass, intended for testing internal server functionality

	// Cache for builder statuses and collaterals.
	// blockBuildersCache map[string]*blockBuilderCacheEntry `jjj:"block_builders_cache"`
	// stores DeFi contract addresses rquired for state interference checks
	defiAddresses map[string]common2.Address `jjj:"defi_addresses"`
	// stores RoB chain IDs
	workingRoBChainIDs map[uint64][]string             `jjj:"working_rob_chain_ids"` // tracks working RoB chain_ids per slot across operational flows. Each slot entry cleared on GetPayload.
	expectedHeader     *common.AnchorGetHeaderResponse `jjj:"expected_header"`
	seqClient          seq.BaseSeqClient
	sizeTracker        *SizeTracker

	// To prevent bugs resulting from overlapping requests, for now, let's have each mutating request be processed one at a time.
	requestMu sync.Mutex

	// Mock mode assists in testing and helps us skip difficult to test functionality (like simulation).
	mockMode bool
}

func FillUpDefiAddresses(opts BatonAPIOpts) map[string]common2.Address {
	defiAddresses := make(map[string]common2.Address)

	if opts.EthNetDetails.Name == common.EthNetworkMainnet {
		// TODO - fill up mainnet defi addresses
	} else if opts.EthNetDetails.Name == common.EthNetworkGoerli {
		defiAddresses[common.WethToken] = common2.HexToAddress("0xB4FBF271143F4FBf7B91A5ded31805e42b2208d6")
		defiAddresses[common.UsdcToken] = common2.HexToAddress("0x9B2660A7BEcd0Bf3d90401D1C214d2CD36317da5")
		defiAddresses[common.WbtcToken] = common2.HexToAddress("0xC04B0d3107736C32e19F1c62b2aF67BE61d63a05")
		defiAddresses[common.DaiToken] = common2.HexToAddress("0x11fe4b6ae13d2a6055c8d9cf65c55bac32b5d844")
		defiAddresses[common.UniV3SwapRouter] = common2.HexToAddress("0xE592427A0AEce92De3Edee1F18E0157C05861564")
	} else if opts.EthNetDetails.Name == common.EthNetworkCustom {

		defiAddresses[common.DaiToken] = common2.HexToAddress("0xAb2A01BC351770D09611Ac80f1DE076D56E0487d")
		defiAddresses[common.WethToken] = common2.HexToAddress("0x4c849Ff66a6F0A954cbf7818b8a763105C2787D6")

		// this is only in custom kurtosis devnets
		defiAddresses[common.DaiWethPair1] = common2.HexToAddress("0x0D6b80a9Cefc2C58308F0Adc26586E550E4422ef")
		defiAddresses[common.UniswapFactory1] = common2.HexToAddress("0xBFF5cD0aA560e1d1C6B1E2C347860aDAe1bd8235")
		defiAddresses[common.DaiWethPair2] = common2.HexToAddress("0x2ed2B47342450C006F83913a422F7C2BDAB8377a")
		defiAddresses[common.UniswapFactory2] = common2.HexToAddress("0x6bEaE43B589D986d127Bd2BdAcF4e24C41C5C035")
	}

	return defiAddresses
}

// NewBatonAPI creates a new service. if builders is nil, allow any builder
func NewBatonAPI(opts BatonAPIOpts) (api *BatonAPI, err error) {
	if opts.Log == nil {
		return nil, ErrMissingLogOpt
	}

	// if opts.BeaconClient == nil {
	// 	return nil, ErrMissingBeaconClientOpt
	// }

	if opts.Datastore == nil {
		return nil, ErrMissingDatastoreOpt
	}

	if len(opts.SeqURL) == 0 && !opts.mockMode {
		return nil, ErrMissingSeqURL
	}

	// If block-builder API is enabled, then ensure secret key is all set
	var publicKey phase0.BLSPubKey
	var blsPubkey *bls.PublicKey
	if opts.BlockBuilderAPI {
		if opts.SecretKey == nil {
			return nil, ErrBuilderAPIWithoutSecretKey
		}

		// If using a secret key, ensure it's the correct one
		blsPubkey, err = bls.PublicKeyFromSecretKey(opts.SecretKey)
		if err != nil {
			return nil, err
		}
		publicKey, err = utils.BlsPublicKeyToPublicKey(blsPubkey)
		if err != nil {
			return nil, err
		}
		opts.Log.Infof("Using BLS key: %s", publicKey.String())

		// ensure pubkey is same across all baton instances
		_pubkey, err := opts.Redis.GetRelayConfig(datastore.RedisConfigFieldPubkey)
		if err != nil {
			return nil, err
		} else if _pubkey == "" {
			err := opts.Redis.SetRelayConfig(datastore.RedisConfigFieldPubkey, publicKey.String())
			if err != nil {
				return nil, err
			}
		} else if _pubkey != publicKey.String() {
			return nil, fmt.Errorf("%w: new=%s old=%s", ErrRelayPubkeyMismatch, publicKey.String(), _pubkey)
		}
	}
	config := seq.SeqClientConfig{
		PrivateKey:    opts.SeqSigningKey,
		URL:           opts.SeqURL,
		ChainID:       opts.SeqChainID,
		NetworkID:     opts.SeqNetworkID,
		Log:           opts.Log,
		BlockWaitTime: opts.SeqBlockWaitTime,
	}

	var seqClient seq.BaseSeqClient
	if !opts.mockMode {
		seqClient, err = seq.NewSeqClient(&config)
	} else {
		seqClient, err = seq.NewMockSeqClient(&config)
	}
	if err != nil {
		opts.Log.Error("Could not create seq client: ", err.Error())
		return nil, err
	}

	sizeTracker := NewSizeTracker(opts.SlotSizeLimit)
	opts.Log.Infof("Size tracker limit: %d\n", opts.SlotSizeLimit)

	api = &BatonAPI{
		opts:      opts,
		log:       opts.Log,
		blsSk:     opts.SecretKey,
		publicKey: blsPubkey,
		datastore: opts.Datastore,
		// beaconClient: opts.BeaconClient,
		redis:     opts.Redis,
		memcached: opts.Memcached,
		db:        opts.DB,

		proposerDutiesMap:      make(map[uint64]*common.BuilderGetSEQValidatorResponseEntry),
		proposerDutiesResponse: &[]byte{},
		blockSimRateLimiter:    NewBlockSimulationRateLimiter(opts.Log, opts.BlockSimManager, opts.BlockSimSigningKey),
		blockSimDepth:          opts.BlockSimDepth,
		tracer:                 NewTracer(opts.BlockSimURL), // TODO: check what the tracer does, since it depends on opts.BlockSimURL

		validatorRegC:      make(chan common.SignedSEQValidatorRegistration, 450_000),
		defiAddresses:      FillUpDefiAddresses(opts),
		workingRoBChainIDs: make(map[uint64][]string),
		seqClient:          seqClient,
		sizeTracker:        sizeTracker,

		// should only be true in testing
		mockMode: opts.mockMode,
	}

	// callback trigger for seq client
	api.seqClient.SetOnNewBlockHandler(func(blk *chain.StatefulBlock, nextProposer *hrpc.NextProposerReply) {
		api.onNewSeqBlock(blk, nextProposer)
	})

	if os.Getenv("FORCE_GET_HEADER_204") == "1" {
		api.log.Warn("env: FORCE_GET_HEADER_204 - forcing getHeader to always return 204")
		api.ffForceGetHeader204 = true
	}

	if os.Getenv("DISABLE_LOWPRIO_BUILDERS") == "1" {
		api.log.Warn("env: DISABLE_LOWPRIO_BUILDERS - allowing only high-level builders")
		api.ffDisableLowPrioBuilders = true
	}

	if os.Getenv("DISABLE_PAYLOAD_DATABASE_STORAGE") == "1" {
		api.log.Warn("env: DISABLE_PAYLOAD_DATABASE_STORAGE - disabling storing payloads in the database")
		api.ffDisablePayloadDBStorage = true
	}

	if os.Getenv("LOG_INVALID_GETPAYLOAD_SIGNATURE") == "1" {
		api.log.Warn("env: LOG_INVALID_GETPAYLOAD_SIGNATURE - getPayload payloads with invalid proposer signature will be logged")
		api.ffLogInvalidSignaturePayload = true
	}

	if os.Getenv("ENABLE_BUILDER_CANCELLATIONS") == "1" {
		api.log.Warn("env: ENABLE_BUILDER_CANCELLATIONS - builders are allowed to cancel submissions when using ?cancellation=1")
		api.ffEnableCancellations = true
	}

	if os.Getenv("REGISTER_VALIDATOR_CONTINUE_ON_INVALID_SIG") == "1" {
		api.log.Warn("env: REGISTER_VALIDATOR_CONTINUE_ON_INVALID_SIG - validator registration will continue processing even if one validator has an invalid signature")
		api.ffRegValContinueOnInvalidSig = true
	}

	if os.Getenv("ENABLE_IGNORABLE_VALIDATION_ERRORS") == "1" {
		api.log.Warn("env: ENABLE_IGNORABLE_VALIDATION_ERRORS - some validation errors will be ignored")
		api.ffIgnorableValidationErrors = true
	}

	if os.Getenv("ENABLE_MOCK_SIMULATION") == "1" {
		api.log.Warn("env: ENABLE_MOCK_SIMULATION - tx simulations will always pass")
		api.ffMockSimulation = true
	}

	return api, nil
}

func (api *BatonAPI) SetExpectedHeaders(msg *common.AnchorGetHeaderResponse) {
	api.expectedHeader = msg
}

func (api *BatonAPI) GetSeqClient() seq.BaseSeqClient {
	return api.seqClient
}

func (api *BatonAPI) getRouter() http.Handler {
	r := mux.NewRouter()

	r.HandleFunc("/", api.handleRoot).Methods(http.MethodGet)
	r.HandleFunc("/livez", api.handleLivez).Methods(http.MethodGet)
	r.HandleFunc("/readyz", api.handleReadyz).Methods(http.MethodGet)
	r.HandleFunc("/newslot", api.handleNewSlot).Methods(http.MethodGet)

	// Proposer API
	if api.opts.ProposerAPI {
		api.log.Info("proposer API enabled")
		r.HandleFunc(pathStatus, api.handleStatus).Methods(http.MethodGet)
		r.HandleFunc(pathRegisterValidator, api.handleRegisterValidator).Methods(http.MethodPost)
		r.HandleFunc(pathGetHeader, api.handleGetHeader).Methods(http.MethodGet)
		r.HandleFunc(pathGetPayload, api.handleGetPayload).Methods(http.MethodPost)
	}

	// Builder API
	if api.opts.BlockBuilderAPI {
		api.log.Info("block builder API enabled")
		r.HandleFunc(pathBuilderGetValidators, api.handleBuilderGetValidators).Methods(http.MethodGet)
		r.HandleFunc(pathGetTobGasReservations, api.handleGetTobGasReservations).Methods(http.MethodGet)

		// Handles both ToB and RoB submissions
		r.HandleFunc(pathSubmitNewBlockRequest, api.handleSubmitNewBlockRequest).Methods(http.MethodPost)

		// Handles sim registeration
		r.HandleFunc(pathRegisterSimulator, api.handleRegisterSimlator).Methods(http.MethodPost)
	}

	// Data API
	if api.opts.DataAPI {
		api.log.Info("data API enabled")
		r.HandleFunc(pathDataProposerPayloadDelivered, api.handleDataProposerPayloadDelivered).Methods(http.MethodGet)
		r.HandleFunc(pathDataBuilderBidsReceived, api.handleDataBuilderBidsReceived).Methods(http.MethodGet)
		r.HandleFunc(pathDataValidatorRegistration, api.handleDataValidatorRegistration).Methods(http.MethodGet)
		r.HandleFunc(pathIncludedTobTxs, api.handleIncludedTobTxs).Methods(http.MethodGet)
	}

	// Pprof
	if api.opts.PprofAPI {
		api.log.Info("pprof API enabled")
		r.PathPrefix("/debug/pprof/").Handler(http.DefaultServeMux)
	}

	// /internal/...
	if api.opts.InternalAPI {
		api.log.Info("internal API enabled")
		r.HandleFunc(pathInternalBuilderStatus, api.handleInternalBuilderStatus).Methods(http.MethodGet, http.MethodPost, http.MethodPut)
		r.HandleFunc(pathInternalBuilderCollateral, api.handleInternalBuilderCollateral).Methods(http.MethodPost, http.MethodPut)
	}

	r.HandleFunc(pathGetSlot, api.handleGetSlot).Methods(http.MethodGet)
	r.HandleFunc(pathGetParentHashForSlot, api.handleGetParentHashForSlot).Methods(http.MethodGet)
	r.HandleFunc(pathGetProposerForSlot, api.handleGetProposerForSlot).Methods(http.MethodGet)

	mresp := common.MustB64Gunzip("H4sICAtOkWQAA2EudHh0AKWVPW+DMBCGd36Fe9fIi5Mt8uqqs4dIlZiCEqosKKhVO2Txj699GBtDcEl4JwTnh/t4dS7YWom2FcVaiETSDEmIC+pWLGRVgKrD3UY0iwnSj6THofQJDomiR13BnPgjvJDqNWX+OtzH7inWEGvr76GOCGtg3Kp7Ak+lus3zxLNtmXaMUncjcj1cwbOH3xBZtJCYG6/w+hdpB6ErpnqzFPZxO4FdXB3SAEgpscoDqWeULKmJA4qyfYFg0QV+p7hD8GGDd6C8+mElGDKab1CWeUQMVVvVDTJVj6nngHmNOmSoe6yH1BM3KZIKpuRaHKrOFd/3ksQwzdK+ejdM4VTzSDfjJsY1STeVTWb0T9JWZbJs8DvsNvwaddKdUy4gzVIzWWaWk3IF8D35kyUDf3FfKipwk/DYUee2nYyWQD0xEKDHeprzeXYwVmZD/lXt1OOg8EYhFfitsmQVcwmbUutpdt3PoqWdMyd2DYHKbgcmPlEYMxPjR6HhxOfuNG52xZr7TtzpygJJKNtWS14Uf0T6XSmzBwAA")
	r.HandleFunc("/miladyz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK); w.Write(mresp) }).Methods(http.MethodGet) //nolint:errcheck

	// r.Use(mux.CORSMethodMiddleware(r))
	loggedRouter := httplogger.LoggingMiddlewareLogrus(api.log, r)
	withGz := gziphandler.GzipHandler(loggedRouter)
	return withGz
}

// StartServer starts up this API instance and HTTP server
// - First it initializes the cache and updates local information
// - Once that is done, the HTTP server is started
func (api *BatonAPI) StartServer() (err error) {
	if api.srvStarted.Swap(true) {
		return ErrServerAlreadyStarted
	}

	// Initialize block builder cache.
	// api.blockBuildersCache = make(map[string]*blockBuilderCacheEntry)

	// create and start HTTP server
	api.srv = &http.Server{
		Addr:    api.opts.ListenAddr,
		Handler: api.getRouter(),

		ReadTimeout:       time.Duration(apiReadTimeoutMs) * time.Millisecond,
		ReadHeaderTimeout: time.Duration(apiReadHeaderTimeoutMs) * time.Millisecond,
		WriteTimeout:      time.Duration(apiWriteTimeoutMs) * time.Millisecond,
		IdleTimeout:       time.Duration(apiIdleTimeoutMs) * time.Millisecond,
		MaxHeaderBytes:    apiMaxHeaderBytes,
	}
	err = api.srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (api *BatonAPI) IsReady() bool {
	// If server is shutting down, return false
	if api.srvShutdown.Load() {
		return false
	}

	// Proposer API readiness checks
	if api.opts.ProposerAPI {
		knownValidatorsUpdated := api.datastore.KnownValidatorsWasUpdated.Load()
		return knownValidatorsUpdated
	}

	// Block-builder API readiness checks
	return true
}

// StopServer gracefully shuts down the HTTP server:
// - Stop returning bids
// - Set ready /readyz to negative status
// - Wait a bit to allow removal of service from load balancer and draining of requests
func (api *BatonAPI) StopServer() (err error) {
	// avoid running this twice. setting srvShutdown to true makes /readyz switch to negative status
	if wasStopping := api.srvShutdown.Swap(true); wasStopping {
		return nil
	}

	// start server shutdown
	api.log.Info("Stopping server...")

	// stop returning bids on getHeader calls (should only be used when running a single instance)
	if api.opts.ProposerAPI && apiShutdownStopSendingBids {
		api.ffForceGetHeader204 = true
		api.log.Info("Disabled returning bids on getHeader")
	}

	// wait some time to get service removed from load balancer
	api.log.Infof("Waiting %.2f seconds before shutdown...", apiShutdownWaitDuration.Seconds())
	time.Sleep(apiShutdownWaitDuration)

	// wait for any active getPayload call to finish
	api.getPayloadCallsInFlight.Wait()

	// shutdown
	return api.srv.Shutdown(context.Background())
}

// TODO: to be updated, to return gasUsed as a map(chainID -> gasUsed)
func (api *BatonAPI) simulateL2Txs(
	ctx context.Context,
	blockNumber map[string]uint64, // chainID(hexutil.Encode(big.Int)) -> block number(0x...)
	l2Txs map[string][]hexutil.Bytes,
	log *logrus.Entry,
) (uint64, error, error) {
	t := time.Now()
	var simResL sync.Mutex
	simRes := make(map[string]struct {
		gasUsed       uint64
		requestErr    error
		validationErr error
	}, len(l2Txs))

	var wg sync.WaitGroup
	for chainID, otxs := range l2Txs {
		wg.Add(1)
		go func(chainID string, otxs []hexutil.Bytes) {
			defer wg.Done()
			blockReq := common.BlockValidationRequest{
				Txs:              otxs,
				BlockNumber:      "latest", // change to blockNumber[chainID] when we can, currently anchorPayload doesn't provide this field that originally provided by submitNewBatonBlock
				StateBlockNumber: "latest",
				// Timestamp:        uint64(time.Now().UnixMilli()),
			}

			gasUsed, requestErr, validationErr := api.blockSimRateLimiter.SimBlockAndGetGasUsedForChain(ctx, chainID, &blockReq)
			log = log.WithFields(logrus.Fields{
				"durationMs": time.Since(t).Milliseconds(),
				"numWaiting": api.blockSimRateLimiter.CurrentCounter(),
			})

			simResL.Lock()
			defer simResL.Unlock()
			simRes[chainID] = struct {
				gasUsed       uint64
				requestErr    error
				validationErr error
			}{
				gasUsed:       gasUsed,
				requestErr:    requestErr,
				validationErr: validationErr,
			}

		}(chainID, otxs)
	}

	wg.Wait()

	gasUsed := uint64(0)
	for chainID := range l2Txs {
		r := simRes[chainID]
		if r.validationErr != nil {
			if api.ffIgnorableValidationErrors {
				// Operators chooses to ignore certain validation errors
				ignoreError := r.validationErr.Error() == ErrBlockAlreadyKnown || r.validationErr.Error() == ErrBlockRequiresReorg || strings.Contains(r.validationErr.Error(), ErrMissingTrieNode)
				if ignoreError {
					log.WithError(r.validationErr).Warn("block validation failed with ignorable error")
					return uint64(0), nil, nil
				}
			}
			log.WithError(r.validationErr).Warn("block validation failed")
			return 0, nil, r.validationErr
		}

		if r.requestErr != nil {
			log.WithError(r.requestErr).Warn("request error")
			return 0, r.requestErr, nil
		}

		gasUsed += r.gasUsed
	}

	// TODO: do we return the sum of used gas?
	return gasUsed, nil, nil
}

func (api *BatonAPI) onNewSeqBlock(blk *chain.StatefulBlock, nextProposer *hrpc.NextProposerReply) {
	api.processNewSlot(blk.Hght)
	api.proposerDutiesLock.Lock()
	defer api.proposerDutiesLock.Unlock()
	blockID, err := blk.ID()
	if err != nil {
		api.log.Warn("unable to get block ID")
	}
	api.pruneOldProposerDuties()

	// reset tracker
	api.sizeTracker.SetLowestSlot(blk.Hght + 1)
	api.redis.SetSizeTracker(api.sizeTracker)

	api.proposerDutiesMap[blk.Hght+1] = &common.BuilderGetSEQValidatorResponseEntry{
		Slot: blk.Hght + 1,
		// set it to unsigned, it will be signed after the SEQ proposer sends the registration request
		Entry: &common.SignedSEQValidatorRegistration{
			Message: &common.SEQValidatorRegistration{
				Pubkey: nextProposer.PublicKey,
			},
		},

		ParentHash:     blockID.String(),
		ProposerPubkey: hexutil.Encode(nextProposer.PublicKey),
	}
	api.log.WithFields(logrus.Fields{
		"newSlot":           blk.Hght + 1,
		"newParentHash":     blockID.String(),
		"newProposerPubkey": hexutil.Encode(nextProposer.PublicKey),
	}).Info("setting proposer duty map for new slot")
}

// note: important for seq/seqclient.go
func (api *BatonAPI) processNewSlot(headSlot uint64) {
	prevHeadSlot := api.headSlot.Load()
	if headSlot <= prevHeadSlot {
		return
	}

	// If there's gaps between previous and new headslot, print the missed slots
	if prevHeadSlot > 0 {
		for s := prevHeadSlot + 1; s < headSlot; s++ {
			api.log.WithField("missedSlot", s).Warnf("missed slot: %d", s)
		}
	}

	// store the head slot
	api.headSlot.Store(headSlot)

	// log
	epoch := headSlot / common.SlotsPerEpoch
	api.log.WithFields(logrus.Fields{
		"epoch":              epoch,
		"slotHead":           headSlot,
		"slotStartNextEpoch": (epoch + 1) * common.SlotsPerEpoch,
	}).Infof("updated headSlot to %d", headSlot)
}

func (api *BatonAPI) RespondError(w http.ResponseWriter, code int, message string) {
	api.Respond(w, code, HTTPErrorResp{code, message})
}

func (api *BatonAPI) RespondOK(w http.ResponseWriter, response any) {
	api.Respond(w, http.StatusOK, response)
}

func (api *BatonAPI) RespondMsg(w http.ResponseWriter, code int, msg string) {
	api.Respond(w, code, HTTPMessageResp{msg})
}

func (api *BatonAPI) Respond(w http.ResponseWriter, code int, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if response == nil {
		return
	}

	// write the json response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		api.log.WithField("response", response).WithError(err).Error("Couldn't write response")
		http.Error(w, "", http.StatusInternalServerError)
	}
}

func (api *BatonAPI) handleStatus(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// ---------------
//  PROPOSER APIS
// ---------------

func (api *BatonAPI) handleRoot(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Baton API")
}

func (api *BatonAPI) handleGetSlot(w http.ResponseWriter, req *http.Request) {
	api.RespondOK(w, api.headSlot.Load())
}

// TODO: Fix this function later
func (api *BatonAPI) handleGetParentHashForSlot(w http.ResponseWriter, req *http.Request) {
	// slot is passed as url args
	slotStr := mux.Vars(req)["slot"]
	slot, err := strconv.ParseUint(slotStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
	}

	api.RespondError(w, 500, "not implemented for slot "+strconv.FormatUint(slot, 10))
}

func (api *BatonAPI) handleGetProposerForSlot(w http.ResponseWriter, req *http.Request) {
	// slot is passed as url args
	slotStr := mux.Vars(req)["slot"]
	slot, err := strconv.ParseUint(slotStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
	}
	log := api.log.WithFields(logrus.Fields{
		"method":        "handleGetProposerForSlot",
		"headSlot":      api.headSlot.Load(),
		"contentLength": req.ContentLength,
	})

	api.proposerDutiesLock.RLock()
	res, ok := api.proposerDutiesMap[slot]
	api.proposerDutiesLock.RUnlock()

	log.Infof("payload attributes: %+v", res)

	if !ok {
		api.RespondError(w, http.StatusNotFound, "slot proposer duties not found")
		return
	}
	api.RespondOK(w, hexutil.Encode(res.Entry.Message.FeeRecipient[:]))
}

func (api *BatonAPI) handleRegisterValidator(w http.ResponseWriter, req *http.Request) {
	api.requestMu.Lock()
	defer api.requestMu.Unlock()

	ua := req.UserAgent()
	log := api.log.WithFields(logrus.Fields{
		"method":        "registerValidator",
		"ua":            ua,
		"mevBoostV":     common.GetMevBoostVersionFromUserAgent(ua),
		"headSlot":      api.headSlot.Load(),
		"contentLength": req.ContentLength,
	})

	start := time.Now().UTC()
	numRegTotal := 0
	numRegProcessed := 0
	numRegActive := 0
	numRegNew := 0
	processingStoppedByError := false
	slot := api.headSlot.Load()

	// Setup error handling
	handleError := func(_log *logrus.Entry, code int, msg string) {
		processingStoppedByError = true
		_log.Warnf("error: %s", msg)
		api.RespondError(w, code, msg)
	}

	// Start processing
	if req.ContentLength == 0 {
		log.Info("empty request")
		api.RespondError(w, http.StatusBadRequest, "empty request")
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		log.WithError(err).WithField("contentLength", req.ContentLength).Warn("failed to read request body")
		api.RespondError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	req.Body.Close()

	var signedReg common.SignedSEQValidatorRegistration
	if err := json.Unmarshal(body, &signedReg); err != nil {
		log.WithError(err).Warn("failed to unmarshal req request")
		api.RespondError(w, http.StatusBadRequest, "failed to unmarshal body")
		return
	}
	if err := signedReg.Initialize(); err != nil {
		log.WithError(err).Warn("failed to initialize reg request")
		api.RespondError(w, http.StatusBadRequest, "failed to initialize reg req")
		return
	}
	pkHex := common.PubkeyHex(signedReg.Message.PublicKey().String())
	regLog := log.WithFields(logrus.Fields{
		"pubkey":       pkHex,
		"signature":    signedReg.Sig().String(),
		"feeRecipient": hexutil.Encode(signedReg.Message.FeeRecipient[:]),
		"timestamp":    signedReg.Message.Timestamp,
	})

	// TODO: with this check we have to add SEQ pubkeys to datastore before the reg req been made, do we need this for the permissioned system?
	isKnownValidator := api.datastore.IsKnownValidator(pkHex)
	if !isKnownValidator {
		handleError(regLog, http.StatusBadRequest, fmt.Sprintf("not a known validator: %s", pkHex.String()))
		return
	}

	// Check for a previous registration timestamp
	prevTimestamp, err := api.redis.GetValidatorRegistrationTimestamp(pkHex)
	if err != nil {
		regLog.WithError(err).Error("error getting last registration timestamp")
	} else if prevTimestamp >= uint64(signedReg.Message.Timestamp) {
		// abort if the current registration timestamp is older or equal to the last known one
		return
	}

	ok, err := signedReg.Verified(api.seqClient.GetChainID(), api.seqClient.GetNetworkID())
	if err != nil || !ok {
		regLog.WithError(err).WithField("ok", ok).Error("error verifying signature from SEQ")
		handleError(regLog, http.StatusBadRequest, fmt.Sprintf("cannot veirfy signature: %s", err))
		return
	}

	api.proposerDutiesLock.Lock()
	defer api.proposerDutiesLock.Unlock()
	if duty := api.proposerDutiesMap[slot]; duty != nil {
		if !bytes.Equal(signedReg.Message.Pubkey, duty.Entry.Message.Pubkey) {
			regLog.WithError(err).Error(fmt.Sprintf("not proposer for slot: %d", slot))
			handleError(regLog, http.StatusBadRequest, fmt.Sprintf("not proposer for slot: %d", slot))
			return
		}

		api.proposerDutiesMap[slot] = &common.BuilderGetSEQValidatorResponseEntry{
			Slot:  slot,
			Entry: &signedReg,
		}
	}

	select {
	case api.validatorRegC <- signedReg:
	default:
		regLog.Error("validator registration channel full")
	}

	log = log.WithFields(logrus.Fields{
		"timeNeededSec":             time.Since(start).Seconds(),
		"timeNeededMs":              time.Since(start).Milliseconds(),
		"numRegistrations":          numRegTotal,
		"numRegistrationsActive":    numRegActive,
		"numRegistrationsProcessed": numRegProcessed,
		"numRegistrationsNew":       numRegNew,
		"processingStoppedByError":  processingStoppedByError,
		"slot":                      slot,
	})

	if err != nil {
		handleError(log, http.StatusBadRequest, "error in traversing json")
		return
	}

	log.Info("validator registrations call processed")
	w.WriteHeader(http.StatusOK)
}

func (api *BatonAPI) FindProposerDutiesByPubKey(pk *bls.PublicKey) (*common.BuilderGetSEQValidatorResponseEntry, error) {
	api.proposerDutiesLock.RLock()
	defer api.proposerDutiesLock.RUnlock()
	for _, entry := range api.proposerDutiesMap {
		epk, err := bls.PublicKeyFromBytes(entry.Entry.Message.Pubkey)
		if err != nil {
			return nil, err
		}
		if epk.Equal(pk) {
			return entry, nil
		}
	}
	return nil, nil
}

func (api *BatonAPI) handleGetHeader(w http.ResponseWriter, req *http.Request) {
	api.requestMu.Lock()
	defer api.requestMu.Unlock()

	vars := mux.Vars(req)
	// TODO: figure out slot(block number from seq) with rollups
	slotStr := vars["slot"]
	parentHashStr := vars["parent_hash"]
	proposerPubkeyHex := vars["pubkey"] // in 0x prefixed hex
	ua := req.UserAgent()
	headSlot := api.headSlot.Load()

	parentHash, err := ids.FromString(parentHashStr)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid parent hash")
		return
	}

	slot, err := strconv.ParseUint(slotStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, common.ErrInvalidSlot.Error())
		return
	}

	requestTime := time.Now().UTC()
	// TODO: to be removed as not needed
	// slotStartTimestamp := api.genesisInfo.Data.GenesisTime + (slot * common.SecondsPerSlot)
	// msIntoSlot := requestTime.UnixMilli() - int64((slotStartTimestamp * 1000))

	log := api.log.WithFields(logrus.Fields{
		"method":           "getHeader",
		"headSlot":         headSlot,
		"slot":             slotStr,
		"parentHash":       parentHash,
		"pubkey":           proposerPubkeyHex,
		"ua":               ua,
		"mevBoostV":        common.GetMevBoostVersionFromUserAgent(ua),
		"requestTimestamp": requestTime.Unix(),
		// "slotStartSec":     slotStartTimestamp,
		// "msIntoSlot":       msIntoSlot,
	})

	log.Debug("request arrived")

	proposerPubkeyBytes, err := hexutil.Decode(proposerPubkeyHex)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, fmt.Sprintf("unable to decode proposer pubkey: %s", err.Error()))
		return
	}

	if len(proposerPubkeyBytes) != 48 {
		api.RespondError(w, http.StatusBadRequest, common.ErrInvalidPubkey.Error())
		return
	}

	if slot < headSlot {
		log.Warn("handleGetHeader: slot too old")
		api.RespondError(w, http.StatusBadRequest, "slot is too old")
		return
	}

	log.Debug("getHeader request received")

	if slices.Contains(apiNoHeaderUserAgents, ua) {
		log.Warn("handleGetHeader: rejecting getHeader ")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if api.ffForceGetHeader204 {
		log.Info("forced getHeader 204 response")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Only allow requests for the current slot until a certain cutoff time
	//if getHeaderRequestCutoffMs > 0 && msIntoSlot > 0 && msIntoSlot > int64(getHeaderRequestCutoffMs) {
	//	log.Info("getHeader sent too late")
	//	w.WriteHeader(http.StatusNoContent)
	//	return
	//}

	headers := common.NewExecutionHeader()
	var hasToB bool
	var hasRoB bool
	// TODO: bid returns an empty AnchorHeader which causes code to fail, debug needed below
	bid, err := api.redis.GetToBBestBid(slot, parentHashStr, proposerPubkeyHex)
	if err != nil {
		log.WithError(err).Error("could not get bid for ToB")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	} else {
		hasToB = bid != nil
	}

	log.Debugf("getHeader has ToB: %+v", hasToB)

	// Make sure the retrieved ToB block is valid
	if hasToB {
		if bid.Header.Big().Cmp(big.NewInt(0)) == 0 {
			log.Info("handleGetHeader ToB was removed due to header comparison")
			hasToB = false
		} else if len(bid.BlockHash) == 0 {
			log.Warning("handleGetHeader ToB was removed due to empty block hash")
			hasToB = false
		}
	}
	if hasToB {
		headers.ToBHash = bid
	}

	workingRoBChainIDs, workingRoBExists := api.GetRoBChainIDsForSlot(slot)
	if workingRoBExists {
		for _, chainID := range workingRoBChainIDs {
			bid, err := api.redis.GetRoBBestBid(slot, parentHashStr, proposerPubkeyHex, chainID)
			if err != nil {
				log.WithError(err).Error("could not get bid for RoB: " + chainID)
				api.RespondError(w, http.StatusBadRequest, err.Error())
				return
			}

			if bid == nil {
				continue
			}

			if (bid.Header.Big().Cmp(big.NewInt(0))) == 0 {
				log.Error("handleGetHeader: rob chunk had zero value")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			headers.RoBHashes[chainID] = bid
			hasRoB = true
		}
	}

	if !hasToB && !hasRoB {
		log.Info("handleGetHeader: no valid rob or tob were found")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Convert the result to a common.Hash
	var blockInfo common.AnchorBlockInfo
	blockInfo.Slot = slot

	if headers.ToBHash == nil && len(headers.RoBHashes) == 0 {
		log.Info("handleGetHeader: no chunks, nothing to do")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := common.AnchorGetHeaderResponse{
		ExecHeaders: headers,
		BlockInfo:   blockInfo,
		ParentHash:  parentHash,
	}

	err = common.SignAnchorGetHeaderResponse(&resp, api.blsSk)
	if err != nil {
		logMsg := "handleGetHeader: could not sign exec headers, err: " + err.Error()
		log.Error(logMsg)
		api.RespondError(w, http.StatusBadRequest, logMsg)
		return
	}

	// This sets the expected header we will compare against when we receive getPayloadRequest()
	api.expectedHeader = &resp

	api.RespondOK(w, resp)
}

func (api *BatonAPI) handleGetPayload(w http.ResponseWriter, req *http.Request) {
	api.requestMu.Lock()
	defer api.requestMu.Unlock()

	// note: multiple payload calls are unlikely
	api.getPayloadCallsInFlight.Add(1)
	defer api.getPayloadCallsInFlight.Done()

	ua := req.UserAgent()
	headSlot := api.headSlot.Load()
	receivedAt := time.Now().UTC()

	// TODO: update fields
	log := api.log.WithFields(logrus.Fields{
		"method":                "getPayload",
		"ua":                    ua,
		"mevBoostV":             common.GetMevBoostVersionFromUserAgent(ua),
		"contentLength":         req.ContentLength,
		"headSlot":              headSlot,
		"headSlotEpochPos":      (headSlot % common.SlotsPerEpoch) + 1,
		"idArg":                 req.URL.Query().Get("id"),
		"timestampRequestStart": receivedAt.UnixMilli(),
	})

	// Log at start and end of request
	log.Info("request initiated")
	defer func() {
		log.WithFields(logrus.Fields{
			"timestampRequestFin": time.Now().UTC().UnixMilli(),
			"requestDurationMs":   time.Since(receivedAt).Milliseconds(),
		}).Info("request finished")
	}()

	// Read the body first, so we can decode it later
	body, err := io.ReadAll(req.Body)
	if err != nil {
		if strings.Contains(err.Error(), "i/o timeout") {
			log.WithError(err).Error("getPayload request failed to decode (i/o timeout)")
			api.RespondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		log.WithError(err).Error("could not read body of request for handleGetPayload()")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Decode payload
	payload := new(common.ExecutionPayload)
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(payload); err != nil {
		log.WithError(err).Warn("failed to decode getPayload request")
		api.RespondError(w, http.StatusBadRequest, "failed to decode anchor payload request")
		return
	}

	// TODO: are below needed?
	// we reject any slots that we didn't receive proposer duty, which was managed by listening blocks from SEQ(check seq_client for more detail)
	//api.proposerDutiesLock.RLock()
	//if _, ok := api.proposerDutiesMap[payload.Slot]; !ok {
	//	log.WithField("slot", payload.Slot).Warn("proposer duty map didn't received, rejecting get payload request")
	//	api.RespondError(w, http.StatusBadRequest, fmt.Sprintf("proposer duty map didn't receive for slot %d", payload.Slot))
	//	api.proposerDutiesLock.RUnlock()
	//	return
	//}
	//api.proposerDutiesLock.RUnlock()

	// Take time after the decoding, and add to logging
	decodeTime := time.Now().UTC()
	// slotStartTimestamp := api.genesisInfo.Data.GenesisTime + (payload.Slot * common.SecondsPerSlot)
	// msIntoSlot := decodeTime.UnixMilli() - int64((slotStartTimestamp * 1000))

	log = log.WithFields(logrus.Fields{
		"slot":                 payload.Slot,
		"slotEpochPos":         (payload.Slot % common.SlotsPerEpoch) + 1,
		"epoch":                payload.Epoch,
		"parentHash":           payload.ParentHash,
		"feeRecipient":         payload.FeeRecipient,
		"stateRoot":            payload.StateRoot,
		"receiptsRoot":         payload.ReceiptsRoot,
		"prevRandao":           payload.PrevRandao,
		"blockNumber":          payload.BlockNumber,
		"gasLimit":             payload.GasLimit,
		"gasUsed":              payload.GasUsed,
		"timestampAfterDecode": decodeTime.UnixMilli(),
		"extraData":            payload.ExtraData,
		"baseFeePerGas":        payload.BaseFeePerGas,
		"blockHash":            payload.BlockHash,
	})

	//if api.expectedHeader == nil {
	//	log.WithError(err).Warn("payload request could not find expected headers (Was getHeaders() called?)")
	//	api.RespondError(w, http.StatusBadRequest, "payload request could not find expected headers (Was getHeaders() called?)")
	//	return
	//}

	// TODO: need update below on maybe verifying ,
	//ok, err := common.VerifySignedHeaders(api.seqClient.GetChainID(), api.seqClient.GetNetworkID(), &api.expectedHeader.ExecHeaders, payload, *proposerPubkey)
	//if err != nil {
	//	logMsg := "payload request failed because error occurred during signed header verification, err: " + err.Error()
	//	log.WithError(err).Warn(logMsg)
	//	api.RespondError(w, http.StatusBadRequest, logMsg)
	//	return
	//}
	//if !ok {
	//	logMsg := "payload request failed because signed header verification contained mismatch"
	//	log.WithError(err).Warn(logMsg)
	//	api.RespondError(w, http.StatusBadRequest, logMsg)
	//	return
	//}

	// Log about received payload (with a valid proposer signature)
	log = log.WithField("timestampAfterSignatureVerify", time.Now().UTC().UnixMilli())
	log.Info("getPayload request received")

	var getPayloadResp common.GetPayloadResponse
	var msNeededForPublishing uint64

	// TODO: above was updated, below needs changing.

	// Save information about delivered payload
	// TODO: next step, update below based on new payload req/resp
	defer func() {
		var bidTrace *common.BidTraceV3

		// we only need to get the bidtrace for one of the involved blocks.
		if getPayloadResp.ExecPayloads.ToBPayload != nil {
			bidTrace, err = api.redis.GetToBBidTrace(payload.Slot, common.ProposerPubKeyAsStr(proposerPubkey), payload.ParentHash)
			if err != nil {
				log.WithError(err).Error("failed to get bidTrace for delivered payload from redis")
				return
			}
		} else {
			// Get RoB bid trace. There should be at least one entry in the response rob map.
			if len(getPayloadResp.ExecPayloads.RoBPayloads) == 0 {
				log.WithError(err).Error("could not get bidtrace because no tob or rob chain ids were found")
				return
			}

			var robFirstChainID string
			for k := range getPayloadResp.ExecPayloads.RoBPayloads {
				robFirstChainID = k
			}
			bidTrace, err = api.redis.GetRoBBidTrace(payload.Slot, common.ProposerPubKeyAsStr(proposerPubkey), payload.ParentHash.String(), robFirstChainID)
			if err != nil {
				log.WithError(err).Error("failed to get bidTrace for delivered payload from redis")
				return
			}
		}

		err = api.db.SaveDeliveredAnchorPayload(bidTrace, &getPayloadResp, decodeTime, msNeededForPublishing)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"bidTrace": bidTrace,
				"payload":  payload,
			}).Error("failed to save delivered payload")
		}

		// Increment builder stats
		err = api.db.IncBlockBuilderStatsAfterGetPayload(bidTrace.BuilderPubkey)
		if err != nil {
			log.WithError(err).Error("failed to increment builder-stats after getPayload")
		}
	}()

	// Get the response - from Redis, Memcache or DB
	// note that recent mev-boost versions only send getPayload to relays that provided the bid
	var tobAnchorPayload *common.AnchorPayload
	var payloadWasFound bool

	tobAnchorPayload, err = api.datastore.GetGetToBPayloadResponse(log, payload.Slot, common.BlsPubKeyToStr(proposerPubkey), payload.ParentHash.String())
	if err != nil || tobAnchorPayload == nil {
		log.WithError(err).Warn("failed getting execution payload (1/2)")
		time.Sleep(time.Duration(timeoutGetPayloadRetryMs) * time.Millisecond)

		// Try again
		tobAnchorPayload, err = api.datastore.GetGetToBPayloadResponse(log, payload.Slot, common.BlsPubKeyToStr(proposerPubkey), payload.ParentHash.String())
		if err != nil || tobAnchorPayload == nil {
			// Still not found! Error out now.
			// TODO: Is the below still needed?
			/*
				if errors.Is(err, datastore.ErrExecutionPayloadNotFound) {
					// Couldn't find the execution payload, maybe it never was submitted to our baton! Check that now
					_, err := api.db.GetBlockSubmissionEntry(payload.Slot, proposerPubkey.String(), payload.BlockHash)
					if errors.Is(err, sql.ErrNoRows) {
						log.Warn("failed getting execution payload (2/2) - payload not found, block was never submitted to this baton")
						api.RespondError(w, http.StatusBadRequest, "no execution payload for this request - block was never seen by this baton")
					} else if err != nil {
						log.WithError(err).Error("failed getting execution payload (2/2) - payload not found, and error on checking bids")
					} else {
						log.Error("failed getting execution payload (2/2) - payload not found, but found bid in database")
					}
				} else { // some other error
					log.WithError(err).Error("failed getting execution payload (2/2) - error")
				}
			*/

			log.Info("no tob anchor execution payload for this request")
			//api.RespondError(w, http.StatusBadRequest, "no tob anchor execution payload for this request")
			//return
		} else {
			payloadWasFound = true
		}
	} else {
		payloadWasFound = true
	}

	robPayloads := make(map[string]*common.AnchorPayload)
	workingRoBChainIDs, workingRoBExists := api.GetRoBChainIDsForSlot(payload.Slot)
	if workingRoBExists {
		for _, chainID := range workingRoBChainIDs {
			robAnchorPayload, err := api.datastore.GetGetRoBPayloadResponse(log, payload.Slot, common.BlsPubKeyToStr(proposerPubkey), payload.ParentHash.String(), chainID)
			if err != nil || robAnchorPayload == nil {
				log.WithError(err).Warn("failed getting execution payload (1/2)")
				time.Sleep(time.Duration(timeoutGetPayloadRetryMs) * time.Millisecond)

				// Try again
				robAnchorPayload, err = api.datastore.GetGetRoBPayloadResponse(log, payload.Slot, common.BlsPubKeyToStr(proposerPubkey), payload.ParentHash.String(), chainID)
				if err != nil || robAnchorPayload == nil {
					// Still not found! Error out now.
					// TODO: Is the below still needed?
					/*
						if errors.Is(err, datastore.ErrExecutionPayloadNotFound) {
							// Couldn't find the execution payload, maybe it never was submitted to our baton! Check that now
							_, err := api.db.GetBlockSubmissionEntry(payload.Slot, proposerPubkey.String(), payload.BlockHash)
							if errors.Is(err, sql.ErrNoRows) {
								log.Warn("failed getting execution payload (2/2) - payload not found, block was never submitted to this baton")
								api.RespondError(w, http.StatusBadRequest, "no execution payload for this request - block was never seen by this baton")
							} else if err != nil {
								log.WithError(err).Error("failed getting execution payload (2/2) - payload not found, and error on checking bids")
							} else {
								log.Error("failed getting execution payload (2/2) - payload not found, but found bid in database")
							}
						} else { // some other error
							log.WithError(err).Error("failed getting execution payload (2/2) - error")
						}
					*/

					log.Info("no tob anchor execution payload for this request")
					//api.RespondError(w, http.StatusBadRequest, "no tob anchor execution payload for this request")
					//return
				} else {
					payloadWasFound = true
					robPayloads[chainID] = robAnchorPayload
				}
			} else {
				payloadWasFound = true
				robPayloads[chainID] = robAnchorPayload
			}
		}
	}

	if !payloadWasFound {
		log.Warn("no execution payloads were found for getPayload request")
		api.RespondError(w, http.StatusNoContent, "no execution payloads were found for getPayload request")
		return
	}

	// Now we know this baton also has the payload
	log = log.WithField("timestampAfterLoadResponse", time.Now().UTC().UnixMilli())

	// Check whether getPayload has already been called -- TODO: do we need to allow multiple submissions of one blinded block?
	err = api.redis.CheckAndSetLastSlotAndHashDelivered(payload.Slot, payload.ParentHash.String())
	log = log.WithField("timestampAfterAlreadyDeliveredCheck", time.Now().UTC().UnixMilli())
	if err != nil {
		if errors.Is(err, datastore.ErrAnotherPayloadAlreadyDeliveredForSlot) {
			// BAD VALIDATOR, 2x GETPAYLOAD FOR DIFFERENT PAYLOADS
			log.Warn("validator called getPayload twice for different payload hashes")
			api.RespondError(w, http.StatusBadRequest, "another payload for this slot was already delivered")
			return
		} else if errors.Is(err, datastore.ErrPastSlotAlreadyDelivered) {
			// BAD VALIDATOR, 2x GETPAYLOAD FOR PAST SLOT
			log.Warn("validator called getPayload for past slot")
			api.RespondError(w, http.StatusBadRequest, "payload for this slot was already delivered")
			return
		} else if errors.Is(err, redis.TxFailedErr) {
			// BAD VALIDATOR, 2x GETPAYLOAD + RACE
			log.Warn("validator called getPayload twice (race)")
			api.RespondError(w, http.StatusBadRequest, "payload for this slot was already delivered (race)")
			return
		}
		log.WithError(err).Error("redis.CheckAndSetLastSlotAndHashDelivered failed")
	}

	// TODO: to be removed as was used when there's beacon client
	// Handle early/late requests
	// if msIntoSlot < 0 {
	// 	// Wait until slot start (t=0) if still in the future
	// 	_msSinceSlotStart := time.Now().UTC().UnixMilli() - int64((slotStartTimestamp * 1000))
	// 	if _msSinceSlotStart < 0 {
	// 		delayMillis := _msSinceSlotStart * -1
	// 		log = log.WithField("delayMillis", delayMillis)
	// 		log.Info("waiting until slot start t=0")
	// 		time.Sleep(time.Duration(delayMillis) * time.Millisecond)
	// 	}
	// } else if getPayloadRequestCutoffMs > 0 && msIntoSlot > int64(getPayloadRequestCutoffMs) {
	// 	// Reject requests after cutoff time
	// 	log.Warn("getPayload sent too late")
	// 	api.RespondError(w, http.StatusBadRequest, fmt.Sprintf("sent too late - %d ms into slot", msIntoSlot))

	// 	go func() {
	// 		err := api.db.InsertTooLateGetPayload(payload.Slot, proposerPubkey.String(), payload.ParentHash, slotStartTimestamp, uint64(receivedAt.UnixMilli()), uint64(decodeTime.UnixMilli()), uint64(msIntoSlot))
	// 		if err != nil {
	// 			log.WithError(err).Error("failed to insert payload too late into db")
	// 		}
	// 	}()
	// 	return
	// }

	// fill in rest of the payload response
	getPayloadResp.Slot = payload.Slot
	getPayloadResp.ExecPayloads.RoBPayloads = make(map[string]common.ExecutionPayload)

	// TODO: Should a bad bundle block the rest of the action? Or should rest of good chunks be allowed through?
	if tobAnchorPayload != nil {
		// slots from retrieved payload should match request else abort
		if tobAnchorPayload.Slot != payload.Slot {
			log.Warn("getPayload failed because stored payload slot did not match requested slot")
			api.RespondError(w, http.StatusBadRequest, "getPayload failed because stored payload slot did not match requested slot")
			return
		}

		tobExecPayload := common.ExecutionPayload{
			Transactions: tobAnchorPayload.Transactions,
		}
		getPayloadResp.ExecPayloads.ToBPayload = &tobExecPayload
	}

	for chainID, anchorPayload := range robPayloads {
		// slots from retrieved payload should match request else abort
		if anchorPayload.Slot != payload.Slot {
			log.Warn("getPayload failed because stored payload slot did not match requested slot")
			api.RespondError(w, http.StatusBadRequest, "getPayload failed because stored payload slot did not match requested slot")
			return
		}

		robChunkExecPayload := common.ExecutionPayload{
			Transactions: anchorPayload.Transactions,
		}
		getPayloadResp.ExecPayloads.RoBPayloads[chainID] = robChunkExecPayload
	}

	err = common.SignAnchorGetPayloadResponse(&getPayloadResp, api.blsSk)
	if err != nil {
		log.Warn("payload request failed because response could not be signed")
		api.RespondError(w, http.StatusBadRequest, "payload request failed because response could not be signed")
		return
	}

	// respond to the HTTP request
	api.RespondOK(w, getPayloadResp)
	log = log.WithFields(logrus.Fields{
		"blockHash": payload.ParentHash,
	})

	api.ClearRoBChainIDsForSlot(payload.Slot)
	api.expectedHeader = nil

	log.Info("execution payload delivered")
}

// --------------------
//
//	BLOCK BUILDER APIS
//
// --------------------
func (api *BatonAPI) handleBuilderGetValidators(w http.ResponseWriter, req *http.Request) {
	api.proposerDutiesLock.RLock()
	resp := api.proposerDutiesResponse
	api.proposerDutiesLock.RUnlock()
	_, err := w.Write(*resp)
	if err != nil {
		api.log.WithError(err).Warn("failed to write response for builderGetValidators")
	}
}

// TODO: to be removed, no gas limit now
// func (api *BatonAPI) getValidatorGasLimit(
// 	w http.ResponseWriter,
// 	log *logrus.Entry,
// 	slot uint64,
// ) (uint64, bool) {
// 	api.proposerDutiesLock.RLock()
// 	slotDuty := api.proposerDutiesMap[slot]
// 	api.proposerDutiesLock.RUnlock()

// 	if slotDuty == nil {
// 		logMsg := "could not find slot duty for slot " + strconv.FormatUint(slot, 10)
// 		log.Error(logMsg)
// 		api.Respond(w, http.StatusBadRequest, logMsg)
// 		return 0, false
// 		//note type conversion
// 	}

// 	return slotDuty.Entry.Message.GasLimit, true
// }

// TODO: Below shouldn't be needed. Remove when needed.
/*
func (api *BatonAPI) checkSubmissionPayloadAttrs(w http.ResponseWriter, log *logrus.Entry, payload *common.BuilderSubmitBlockRequest) bool {
	api.payloadAttributesLock.RLock()
	attrs, ok := api.payloadAttributes[*payload.ParentHash()]
	api.payloadAttributesLock.RUnlock()
	if !ok || payload.Slot() != attrs.slot {
		log.Warn("payload attributes not (yet) known")
		api.RespondError(w, http.StatusBadRequest, "payload attributes not (yet) known")
		return false
	}

	if payload.Random() != attrs.payloadAttributes.PrevRandao {
		msg := fmt.Sprintf("incorrect prev_randao - got: %s, expected: %s", payload.Random(), attrs.payloadAttributes.PrevRandao)
		log.Info(msg)
		api.RespondError(w, http.StatusBadRequest, msg)
		return false
	}

	if hasReachedFork(payload.Slot(), api.capellaEpoch) { // Capella requires correct withdrawals
		withdrawalsRoot, err := ComputeWithdrawalsRoot(payload.Withdrawals())
		if err != nil {
			log.WithError(err).Warn("could not compute withdrawals root from payload")
			api.RespondError(w, http.StatusBadRequest, "could not compute withdrawals root")
			return false
		}

		if withdrawalsRoot != attrs.withdrawalsRoot {
			msg := fmt.Sprintf("incorrect withdrawals root - got: %s, expected: %s", withdrawalsRoot.String(), attrs.withdrawalsRoot.String())
			log.Info(msg)
			api.RespondError(w, http.StatusBadRequest, msg)
			return false
		}
	}
	return true
}
*/

func (api *BatonAPI) checkSubmissionSlotDetails(w http.ResponseWriter, log *logrus.Entry, headSlot uint64, payload *common.SubmitNewBlockRequest) bool {
	if payload.Slot() <= headSlot {
		log.Info("submitNewBlock failed: submission for past slot")
		api.RespondError(w, http.StatusBadRequest, "submission for past slot")
		return false
	}

	return true
}

// func (api *BatonAPI) checkBuilderEntry(w http.ResponseWriter, log *logrus.Entry, builderPubkey phase0.BLSPubKey) (*blockBuilderCacheEntry, bool) {
// 	builderEntry, ok := api.blockBuildersCache[builderPubkey.String()]
// 	if !ok {
// 		log.Warnf("unable to read builder: %s from the builder cache, using low-prio and no collateral", builderPubkey.String())
// 		builderEntry = &blockBuilderCacheEntry{
// 			status: common.BuilderStatus{
// 				IsHighPrio:    false,
// 				IsOptimistic:  false,
// 				IsBlacklisted: false,
// 			},
// 			collateral: big.NewInt(0),
// 		}
// 	}

// 	if builderEntry.status.IsBlacklisted {
// 		log.Info("builder is blacklisted")
// 		time.Sleep(200 * time.Millisecond)
// 		w.WriteHeader(http.StatusOK)
// 		return builderEntry, false
// 	}

// 	// In case only high-prio requests are accepted, fail others
// 	if api.ffDisableLowPrioBuilders && !builderEntry.status.IsHighPrio {
// 		log.Info("rejecting low-prio builder (ff-disable-low-prio-builders)")
// 		time.Sleep(200 * time.Millisecond)
// 		w.WriteHeader(http.StatusOK)
// 		return builderEntry, false
// 	}

// 	return builderEntry, true
// }

func (api *BatonAPI) handleGetTobGasReservations(w http.ResponseWriter, req *http.Request) {
	api.RespondOK(w, common.TobGasReservations)
}

func (api *BatonAPI) handleRegisterSimlator(w http.ResponseWriter, req *http.Request) {
	receivedAt := time.Now().UTC()

	log := api.log.WithField("method", "registerSimulator")
	log.Info("Received request")
	log = api.log.WithFields(logrus.Fields{
		"method":                "registerSimulator",
		"contentLength":         req.ContentLength,
		"timestampRequestStart": receivedAt.UnixMilli(),
	})
	var r io.Reader = req.Body
	limitReader := io.LimitReader(r, 10*1024*1024) // 10 MB
	payloadBytes, err := io.ReadAll(limitReader)
	if err != nil {
		log.WithError(err).Warn("could not read payload")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var registerReq SimulatorRegisterRequest
	err = json.Unmarshal(payloadBytes, &registerReq)
	if err != nil {
		log.WithError(err).Warn("could unmarshal payload to sim register request")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := registerReq.Initialize(); err != nil {
		log.WithError(err).Warn("could initialize sim regsiter request")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	registered, err := api.blockSimRateLimiter.RegisterSimulator(&registerReq)
	if err != nil || !registered {
		log.WithError(err).Warn("unable to register simulator")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := SimulatorRegisterResponse{
		Success: true,
	}

	api.RespondOK(w, resp)
}

// This method used for both ToB and for RoB.
// TODO: Builders need to register themeselves on Baton before submitting blocks
// TODO: rollup sequencer provide slot state and we need to make sure slot is seperated between rob and tob(this we can provide in baton, just verify that the slot matches)
func (api *BatonAPI) handleSubmitNewBlockRequest(w http.ResponseWriter, req *http.Request) {
	api.requestMu.Lock()
	defer api.requestMu.Unlock()

	var pf common.Profile
	var prevTime, nextTime time.Time
	headSlot := api.headSlot.Load()
	receivedAt := time.Now().UTC()
	prevTime = receivedAt
	if api.log == nil {
		panic("api.log is nil")
	}

	log := api.log.WithField("method", "submitNewBlockRequest")
	log.Info("Received request")
	log = api.log.WithFields(logrus.Fields{
		"method":                "submitNewBlockRequest",
		"contentLength":         req.ContentLength,
		"headSlot":              headSlot,
		"timestampRequestStart": receivedAt.UnixMilli(),
	})

	// Log at start and end of request
	log.Info("request initiated")
	defer func() {
		log.WithFields(logrus.Fields{
			"timestampRequestFin": time.Now().UTC().UnixMilli(),
			"requestDurationMs":   time.Since(receivedAt).Milliseconds(),
		}).Info("request finished")
	}()

	var err error
	var r io.Reader = req.Body

	limitReader := io.LimitReader(r, 10*1024*1024) // 10 MB
	payloadBytes, err := io.ReadAll(limitReader)
	if err != nil {
		log.WithError(err).Warn("could not read payload")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// TODO: need to verify signature from builder
	blockReq := common.NewSubmitNewBlockRequest()

	err = blockReq.FromJSON(payloadBytes)
	if err != nil {
		log.WithError(err).Warn("could not parse payload into SubmitNewBlockRequest")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := blockReq.Initialize(); err != nil {
		log.WithError(err).Warn("could not initialize SubmitNewBlockRequest")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var chainIDs map[string]struct{}
	var incomingSimBlockNumber map[string]uint64
	var txs []*chain.Transaction
	parser := srpc.Parser{}
	actionRegistry, authRegistry := parser.Registry()
	_, txs, err = chain.UnmarshalTxs(
		blockReq.Chunk.Txs,
		SeqUnmarshalTxsInitialCapacity,
		actionRegistry,
		authRegistry)
	if err != nil {
		log.WithError(err).Warn("could not unmarshal txs")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(txs) == 0 {
		log.WithError(err).Warn("txs is nil or no txs")
		api.RespondError(w, http.StatusNoContent, err.Error())
		return
	}
	if !api.mockMode {
		chainIDs = api.getChainIDsFromSEQTxs(context.TODO(), txs)
		incomingSimBlockNumber, err = api.blockSimRateLimiter.GetBlockNumber(chainIDs)
		if err != nil {
			log.WithError(err).Warn("unable to get block numbers of L2s")
			api.RespondError(w, http.StatusNoContent, err.Error())
			return
		}
	}

	nextTime = time.Now().UTC()
	pf.Decode = uint64(nextTime.Sub(prevTime).Microseconds())

	isLargeRequest := len(payloadBytes) > fastTrackPayloadSizeLimit
	slot := blockReq.Slot()

	// We only allow [opts.FutureSLotsAllowed] to enter bidding
	if (slot - headSlot) > uint64(api.opts.FutureSlotsAllowed) {
		log.Error("Slot's TOB bid not yet started!!")
		api.Respond(w, http.StatusBadRequest, "Slot's TOB bid not yet started!!")
		return
	}

	ok := api.checkSubmissionSlotDetails(w, log, headSlot, &blockReq)
	if !ok {
		log.WithError(err).Info("slot details check failed")
		api.RespondError(w, http.StatusBadRequest, "slot details check failed")
		return
	}

	// ToB will have varying chain id in txs, RoB will have uniform
	// Also verifies len(txs) >= 2
	isToB, err := api.checkBlockRequestIsToB(txs)
	if err != nil {
		log.WithError(err).Info(err.Error())
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Infof("is tob: %t", isToB)

	chainID, err := FirstChainID(txs)
	if err != nil {
		log.WithError(err).Info(err.Error())
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// checks for ToB txs count & chunk txs size
	if isToB {
		if len(txs) < common.MinTobTxs {
			msg := fmt.Sprintf("we support at least %d txs on the TOB currently, got %d", common.MinTobTxs, len(blockReq.Txs()))
			log.WithError(err).Info(msg)
			api.Respond(w, http.StatusBadRequest, msg)
			return
		}

		if err := api.sizeTracker.TryUpdate(ToBTrackerTag, slot, len(blockReq.Chunk.Txs)); err != nil {
			log.WithError(err).Info("slot full, rejecting bid")
			api.Respond(w, http.StatusBadRequest, fmt.Sprintf("slot full rejecting bid: %s", err.Error()))
			return
		}
	} else {
		if err := api.sizeTracker.TryUpdate(chainID, slot, len(blockReq.Chunk.Txs)); err != nil {
			log.WithError(err).Info("slot full, rejecting bid")
			api.Respond(w, http.StatusBadRequest, fmt.Sprintf("slot full rejecting bid: %s", err.Error()))
			return
		}
	}

	// Note this also validates slot validity
	var gasLimit uint64
	if isToB {
		gasLimit = 5_000_000
	} else {
		gasLimit = 25_000_000
	}

	var topBidValue uint64
	var hasRoB, hasToB bool
	tx := api.redis.NewTxPipeline()
	bidIsTopBid := false

	var value *big.Int
	value, err = common.Value(txs)
	if err != nil {
		log.WithError(err).Info("block req value returned err")
		api.RespondError(w, http.StatusBadRequest, "value check failed")
		return
	}

	if isToB {
		hasToB, err = api.redis.HasToBTopBidValue(context.Background(), blockReq.Slot(), blockReq.ParentHashAsStr(), *blockReq.ProposerPubKey())
		if err != nil {
			log.WithError(err).Info("could not query tob for blockReq, returned err")
			api.RespondError(w, http.StatusBadRequest, "could not query tob for blockReq, failed")
			return
		}
		if hasToB {
			topBidValue, err = api.redis.GetToBTopBidValue(context.Background(), tx, blockReq.Slot(), blockReq.ParentHashAsStr(), *blockReq.ProposerPubKey())
			if err != nil {
				log.WithError(err).Info("could not get top tob bid val for blockReq, returned err")
				api.RespondError(w, http.StatusBadRequest, "could not get top tob bid for blockReq, failed")
				return
			}
			// TODO: check conversion since we changed from big.Int to uint64 for topBidValue
			valueU64 := value.Uint64()
			bidIsTopBid = valueU64 > topBidValue
			log = log.WithFields(logrus.Fields{
				"topBidValue":    topBidValue,
				"newBidIsTopBid": bidIsTopBid,
			})
		}
	} else {
		hasRoB, err = api.redis.HasRoBTopBidValue(context.Background(), blockReq.Slot(), blockReq.ParentHashAsStr(), *blockReq.ProposerPubKey(), chainID)
		if err != nil {
			log.WithError(err).Info("could not query rob for blockReq, returned err")
			api.RespondError(w, http.StatusBadRequest, "could not query rob for blockReq, failed")
			return
		}
		if hasRoB {
			topBidValue, err = api.redis.GetRoBTopBidValue(context.Background(), tx, blockReq.Slot(), blockReq.ParentHashAsStr(), *blockReq.ProposerPubKey(), chainID)
			if err != nil {
				log.WithError(err).Info("could not get top rob bid val for blockReq, returned err")
				api.RespondError(w, http.StatusBadRequest, "could not get top rob bid for blockReq, failed")
				return
			}

			// TODO: check conversion since we changed from big.Int to uint64 for topBidValue
			valueU64 := value.Uint64()
			bidIsTopBid = valueU64 > topBidValue
			log = log.WithFields(logrus.Fields{
				"topBidValue":    topBidValue,
				"newBidIsTopBid": bidIsTopBid,
			})
		}
	}

	proposerPayment := blockReq.ProposerPayment()
	log = log.WithFields(logrus.Fields{
		"timestampAfterDecoding": time.Now().UTC().UnixMilli(),
		"slot":                   blockReq.Slot(),
		"numTx":                  len(blockReq.Txs()),
		"parentHash":             blockReq.ParentHash().String(),
		"blockHash":              hexutil.Encode(blockReq.BlockHash().Bytes()),
		"builderPubkey":          blockReq.BuilderPubkey().String(),
		"proposerPubkey":         blockReq.ProposerPubKeyAsStr(),
		"proposerPayment":        hexutil.Encode(proposerPayment[:]),
		"isLargeRequest":         isLargeRequest,
		"isToB":                  isToB,
	})

	// Build the header and payload for this block request.
	getHeader, err := BuildHeader(&blockReq, value.Uint64())
	if err != nil {
		log.WithError(err).Warn("failed to build header")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	getHeader.Value = value.Uint64()

	getPayload, err := BuildPayload(&blockReq, blockReq.Txs(), value.Uint64(), incomingSimBlockNumber)
	if err != nil {
		log.WithError(err).Warn("failed to build payload")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Perform block simulation which makes sure all txs are valid before we allow it to participate in the auction
	simStartTime := time.Now().UTC()
	simResultC := make(chan *blockSimResult, 1)

	// Once we process the sim, then we want to save the results to the redis database.
	var eligibleAt time.Time // will be set once the bid is ready
	var gasUsed uint64
	defer func() {
		savePayloadToDatabase := !api.ffDisablePayloadDBStorage
		var simResult *blockSimResult
		select {
		case simResult = <-simResultC:
		case <-time.After(10 * time.Second):
			log.Warn("timed out waiting for simulation result")
			simResult = &blockSimResult{false, false, nil, nil}
		}

		submissionEntry, err := api.db.SaveBuilderBlockSubmission(
			&blockReq,
			getPayload,
			gasUsed,
			0,
			isToB,
			value,
			chainID,
			simResult.requestErr,
			simResult.validationErr,
			receivedAt,
			eligibleAt,
			simResult.wasSimulated,
			savePayloadToDatabase,
			pf,
			simResult.optimisticSubmission,
		)
		if err != nil {
			log.WithError(err).WithField("chainID", chainID).Error("saving builder block submission to database failed")
			return
		}

		err = api.db.UpsertBlockBuilderEntryAfterSubmission(submissionEntry, isToB, chainID, simResult.validationErr != nil)
		if err != nil {
			log.WithError(err).Error("failed to upsert block-builder-entry")
		}
	}()

	var reqErr, validErr error
	if !api.mockMode {
		ctx := context.Background()
		sctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		var txs2simulate map[string][]hexutil.Bytes
		var simBlockNumber map[string]uint64
		slog := log.WithFields(logrus.Fields{
			"slot":           blockReq.Slot,
			"parentHash":     blockReq.ParentHash().String(),
			"proposerPubkey": blockReq.ProposerPubKeyAsStr(),
			"isToB":          isToB,
		})
		if isToB {
			robTxs, blockNumber, err := api.getTopRoBsTxsByChainIDs(sctx, chainIDs, blockReq.Slot(), api.blockSimDepth, slog)
			if err != nil {
				slog.WithError(err).Warn("unable to fetch top RoB txs from redis")
				api.RespondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			txs2simulate = robTxs // safe since txs2simulate is empty
			simBlockNumber = blockNumber
		} else {
			tobTxs, blockNumber, err := api.getTopToBTxsByChainID(sctx, chainID, blockReq.Slot(), api.blockSimDepth, slog)
			if err != nil {
				slog.WithError(err).Warn("unable to fetch top RoB txs from redis")
				api.RespondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			txs2simulate = tobTxs
			simBlockNumber = blockNumber
		}

		simBlockNumber = lowestHeights([]map[string]uint64{simBlockNumber, incomingSimBlockNumber})

		// in case there's no record found in cache
		if txs2simulate == nil {
			txs2simulate = make(map[string][]hexutil.Bytes)
		}

		// append txs from req
		l2txs := api.seqTxs2EthTxs(ctx, txs)
		for chainID, otxs := range l2txs {
			l := txs2simulate[chainID]
			l = append(l, otxs...)
			txs2simulate[chainID] = l

		}
		log.Debug("txs to simulate")
		// for only debugging
		for chainID, otxs := range txs2simulate {
			log.Debugf("======txs of chain======: %s\n", chainID)
			for _, otx := range otxs {
				tx := new(types.Transaction)
				if err := tx.UnmarshalBinary(otx); err != nil {
					log.Error("unable to unmarshal raw tx")
					continue
				}
				log.Debugf("txhash: %s\n", tx.Hash().Hex())
			}
		}

		gasUsed, reqErr, validErr = api.simulateL2Txs(sctx, simBlockNumber, txs2simulate, slog)
		if gasUsed != 0 && gasUsed > gasLimit {
			errMsg := "simulation failed due to gas limit exceeded, gas_used [" + strconv.FormatUint(gasUsed, 10) + "], gas_limit [" + strconv.FormatUint(gasLimit, 10) + "]"
			validErr = errors.New(errMsg)
			log.WithError(reqErr).Warn("could not simulate L2 txs on gas exceeding")
			api.RespondError(w, http.StatusBadRequest, validErr.Error())
			return
		}

		simResultC <- &blockSimResult{reqErr == nil, false, reqErr, validErr}
		if reqErr != nil {
			log.WithError(reqErr).Warn("could not simulate L2 txs on request error")
			api.RespondError(w, http.StatusBadRequest, reqErr.Error())
			return
		}
		if validErr != nil {
			log.WithError(validErr).Warn("validation error during L2 txs simulation")
			api.RespondError(w, http.StatusBadRequest, validErr.Error())
			return
		}
	} else {
		gasUsed = 0
		simResultC <- &blockSimResult{true, false, nil, nil}
	}

	simulationDuration := time.Since(simStartTime).Microseconds()

	blockNumberJSON, err := json.Marshal(blockReq.BlockNumber())
	if err != nil {
		log.WithError(err).Warn("couldn't marshal block number")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	blockNumberJSONStr := string(blockNumberJSON[:])

	trace := common.BidTraceV3{
		Slot:            slot,
		IsTob:           isToB,
		ChainID:         chainID,
		ParentHash:      blockReq.ParentHashAsStr(),
		BlockHash:       blockReq.BlockHash().String(),
		BuilderPubkey:   blockReq.BuilderPubkeyAsStr(),
		ProposerPubkey:  blockReq.ProposerPubKeyAsStr(),
		ProposerPayment: blockReq.ProposerPaymentAsStr(),
		GasLimit:        0,
		GasUsed:         gasUsed,
		Value:           value.Uint64(),
		BlockNumber:     blockNumberJSONStr,
		NumTx:           uint64(len(blockReq.Txs())),
	}

	// Save the header and payloads for this block request to Redis.
	// The auction is processed here where the best bid for the ToB or RoB namespace is saved to the database.
	var updateBidResult datastore.SaveBidAndUpdateTopBidResponse
	if isToB {
		// ToB case
		updateBidResult, err = api.redis.SaveToBBidAndUpdateTopBid(context.Background(), tx, &blockReq, value,
			getPayload, &getHeader, receivedAt, false, nil, &trace)
		if err != nil {
			log.WithError(err).Error("could not save bid and update top bids")
			api.RespondError(w, http.StatusInternalServerError, "failed saving and updating bid")
			return
		}
		log.Info("tob bid result saved")
	} else {
		//RoB case
		updateBidResult, err = api.redis.SaveRoBBidAndUpdateTopBid(context.Background(), tx, &blockReq, value,
			getPayload, &getHeader, chainID, receivedAt, false, nil, &trace)
		if err != nil {
			log.WithError(err).Error("could not save bid and update top bids for RoB")
			api.RespondError(w, http.StatusInternalServerError, "failed saving and updating bid")
			return
		}
		log.Info("rob bid result saved")
	}

	if updateBidResult.WasBidSaved {
		// Bid is eligible to win the auction
		eligibleAt = time.Now().UTC()
		log = log.WithField("timestampEligibleAt", eligibleAt.UnixMilli())

		// Save to memcache in the background
		if api.memcached != nil {
			go func() {
				if isToB {
					err = api.memcached.SaveToBAnchorPayload(blockReq.Slot(),
						blockReq.ProposerPubKeyAsStr(),
						blockReq.BlockHash().String(),
						getPayload)
				} else {
					err = api.memcached.SaveRoBAnchorPayload(blockReq.Slot(),
						blockReq.ProposerPubKeyAsStr(),
						blockReq.BlockHash().String(),
						chainID,
						getPayload)
				}
				if err != nil {
					log.WithError(err).Error("failed saving execution payload in memcached")
				}
			}()
		}

		if !isToB {
			api.AddWorkingRoBChainID(slot, chainID)
		}

		log.Infof("bid saved, slot: %+v, parnetHash: %s, proposerPubKkey: %s", blockReq.Slot(), blockReq.ParentHashAsStr(), blockReq.ProposerPubKeyAsStr())
	}
	// TODO: make a meaningful submitNewBlock response
	api.RespondOK(w, "success")

	defer func() {
		totalDuration := time.Since(receivedAt).Microseconds()
		txHashList := []string{}
		for _, tx := range txs {
			digestBytes, err := tx.Digest()
			if err != nil {
				log.WithError(err).Error("couldn't get tx digest")
				continue
			}
			txHashList = append(txHashList, hexutil.Encode(digestBytes))
		}
		txHashes := strings.Join(txHashList, ",")

		if isToB {
			err := api.db.InsertToBSubmitProfile(blockReq.Slot(), blockReq.ParentHashAsStr(), txHashes, uint64(simulationDuration), 0, uint64(totalDuration))
			if err != nil {
				log.WithError(err).Error("failed to insert tob submit profile into db")
			}
		} else {
			err := api.db.InsertRoBSubmitProfile(blockReq.Slot(), blockReq.ParentHashAsStr(), txHashes, uint64(simulationDuration), 0, uint64(totalDuration))
			if err != nil {
				log.WithError(err).Error("failed to insert rob submit profile into db")
			}
		}
	}()
}

func (api *BatonAPI) getChainIDsFromSEQTxs(ctx context.Context, txs []*chain.Transaction) map[string]struct{} {
	ret := make(map[string]struct{})
	for _, tx := range txs {
		for _, action := range tx.Actions {
			if seqMsg, ok := action.(*actions.SequencerMsg); ok {
				chainIDu64 := binary.LittleEndian.Uint64(seqMsg.ChainID)
				chainID := big.NewInt(int64(chainIDu64))
				chainIDstr := hexutil.EncodeBig(chainID)
				if _, ok := ret[chainIDstr]; !ok {
					ret[chainIDstr] = struct{}{}
				}
			}
		}
	}

	return ret
}

func (api *BatonAPI) seqTxs2EthTxs(ctx context.Context, txs []*chain.Transaction) map[string][]hexutil.Bytes {
	ret := make(map[string][]hexutil.Bytes)
	for _, tx := range txs {
		for _, action := range tx.Actions {
			if seqMsg, ok := action.(*actions.SequencerMsg); ok {
				chainIDu64 := binary.LittleEndian.Uint64(seqMsg.ChainID)
				chainID := big.NewInt(int64(chainIDu64))
				chainIDstr := hexutil.EncodeBig(chainID)

				l := ret[chainIDstr]
				l = append(l, seqMsg.Data)
				ret[chainIDstr] = l
			}
		}
	}

	return ret
}

func (api *BatonAPI) getTopToBTxsByChainID(ctx context.Context, robChainID string, slot2bid uint64, depth int, log *logrus.Entry) (map[string][]hexutil.Bytes, map[string]uint64, error) {
	// chainID str -> a list of txs
	ret := make(map[string][]hexutil.Bytes)
	blockNumbers := make([]map[string]uint64, 0, depth)

	for slot := slot2bid - uint64(depth-1); slot <= slot2bid; slot++ {
		api.proposerDutiesLock.RLock()
		slotDutyInfo, ok := api.proposerDutiesMap[slot]
		api.proposerDutiesLock.RUnlock()
		if !ok {
			// since we reject every duty map slot if the duty map for this slot didn't receive, so it's safe even we don't simulate the txs in that slot, same for below getTopRobTxs
			api.log.Warnf("proposer duty map didn't received, skipping slot: %d", slot)
			continue
		}
		parentHash := slotDutyInfo.ParentHash
		proposerPubkey := slotDutyInfo.ProposerPubkey

		header, err := api.redis.GetToBBestBid(slot, parentHash, proposerPubkey)
		if err != nil {
			return nil, nil, err
		}
		if header == nil {
			continue
		}

		var topPayload *common.AnchorPayload
		payload, err := api.datastore.GetGetToBPayloadResponse(log, slot, proposerPubkey, header.BlockHash)
		if err != nil {
			return nil, nil, err
		}
		topPayload = payload
		blockNumbers = append(blockNumbers, topPayload.BlockNumber)

		txsRaw := topPayload.Transactions
		parser := srpc.Parser{} // TODO: need to verify the registry returned is non-related to networkID and ChainID, those two are used for signing, which potentially might cause issues
		actionRegistry, authRegistry := parser.Registry()
		_, txs, err := chain.UnmarshalTxs(txsRaw, SeqUnmarshalTxsInitialCapacity, actionRegistry, authRegistry)
		if err != nil {
			return nil, nil, err
		}

		for _, tx := range txs {
			for _, action := range tx.Actions {
				if seqMsg, ok := action.(*actions.SequencerMsg); ok {
					chainIDu64 := binary.LittleEndian.Uint64(seqMsg.ChainID)
					chainID := big.NewInt(int64(chainIDu64))
					chainIDstr := hexutil.EncodeBig(chainID)
					if chainIDstr != robChainID {
						continue
					}

					l := ret[chainIDstr]
					l = append(l, seqMsg.Data)
					ret[chainIDstr] = l
				}
			}
		}
	}

	return ret, lowestHeights(blockNumbers), nil
}

func (api *BatonAPI) getTopRoBsTxsByChainIDs(ctx context.Context, chainIDs map[string]struct{}, slot2bid uint64, depth int, log *logrus.Entry) (map[string][]hexutil.Bytes, map[string]uint64, error) {
	ret := make(map[string][]hexutil.Bytes, 0)
	blockNumbers := make([]map[string]uint64, 0, len(chainIDs)*depth)
	for slot := slot2bid - uint64(depth-1); slot <= slot2bid; slot++ {
		api.proposerDutiesLock.RLock()
		slotDutyInfo, ok := api.proposerDutiesMap[slot]
		api.proposerDutiesLock.RUnlock()
		if !ok {
			api.log.Warnf("proposer duty map didn't received, skipping slot: %d", slot)
			continue
		}
		parentHash := slotDutyInfo.ParentHash
		proposerPubkey := slotDutyInfo.ProposerPubkey

		for chainID := range chainIDs {
			header, err := api.redis.GetRoBBestBid(slot, parentHash, proposerPubkey, chainID)
			if err != nil { // nil won't throw
				return nil, nil, err
			}
			var topPayload *common.AnchorPayload
			// top bid not exists, not one bid submitted for this chain
			if header == nil {
				continue
			}
			payload, err := api.datastore.GetGetRoBPayloadResponse(log, slot, proposerPubkey, header.BlockHash, chainID)
			if err != nil {
				return nil, nil, err
			}
			topPayload = payload

			txsRaw := topPayload.Transactions
			parser := srpc.Parser{} // TODO: need to verify the registry returned is non-related to networkID and ChainID, those two are used for signing, which potentially might cause issues
			actionRegistry, authRegistry := parser.Registry()
			_, txs, err := chain.UnmarshalTxs(txsRaw, SeqUnmarshalTxsInitialCapacity, actionRegistry, authRegistry)
			if err != nil {
				return nil, nil, err
			}

			blockNumbers = append(blockNumbers, topPayload.BlockNumber)

			ret[chainID] = make([]hexutil.Bytes, 0, len(txs)-1)
			for _, tx := range txs {
				for _, action := range tx.Actions {
					if seqMsg, ok := action.(*actions.SequencerMsg); ok {
						l := ret[chainID]
						l = append(l, seqMsg.Data)
						ret[chainID] = l
					}
				}
			}
		}

	}
	return ret, lowestHeights(blockNumbers), nil
}

// ---------------
//
//	INTERNAL APIS
//
// ---------------
func (api *BatonAPI) handleInternalBuilderStatus(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	builderPubkey := vars["pubkey"]
	builderEntry, err := api.db.GetBlockBuilderByPubkey(builderPubkey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			api.RespondError(w, http.StatusBadRequest, "builder not found")
			return
		}

		api.log.WithError(err).Error("could not get block builder")
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req.Method == http.MethodGet {
		api.RespondOK(w, builderEntry)
		return
	} else if req.Method == http.MethodPost || req.Method == http.MethodPut || req.Method == http.MethodPatch {
		st := common.BuilderStatus{
			IsHighPrio:    builderEntry.IsHighPrio,
			IsBlacklisted: builderEntry.IsBlacklisted,
			IsOptimistic:  builderEntry.IsOptimistic,
		}
		trueStr := "true"
		args := req.URL.Query()
		if args.Get("high_prio") != "" {
			st.IsHighPrio = args.Get("high_prio") == trueStr
		}
		if args.Get("blacklisted") != "" {
			st.IsBlacklisted = args.Get("blacklisted") == trueStr
		}
		if args.Get("optimistic") != "" {
			st.IsOptimistic = args.Get("optimistic") == trueStr
		}
		api.log.WithFields(logrus.Fields{
			"builderPubkey": builderPubkey,
			"isHighPrio":    st.IsHighPrio,
			"isBlacklisted": st.IsBlacklisted,
			"isOptimistic":  st.IsOptimistic,
		}).Info("updating builder status")
		err := api.db.SetBlockBuilderStatus(builderPubkey, st)
		if err != nil {
			err := fmt.Errorf("error setting builder: %v status: %w", builderPubkey, err)
			api.log.Error(err)
			api.RespondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.RespondOK(w, st)
	}
}

func (api *BatonAPI) handleInternalBuilderCollateral(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	builderPubkey := vars["pubkey"]
	if req.Method == http.MethodPost || req.Method == http.MethodPut {
		args := req.URL.Query()
		collateral := args.Get("collateral")
		value := args.Get("value")
		log := api.log.WithFields(logrus.Fields{
			"pubkey":     builderPubkey,
			"collateral": collateral,
			"value":      value,
		})
		log.Infof("updating builder collateral")
		if err := api.db.SetBlockBuilderCollateral(builderPubkey, collateral, value); err != nil {
			fullErr := fmt.Errorf("unable to set collateral in db for pubkey: %v: %w", builderPubkey, err)
			log.Error(fullErr.Error())
			api.RespondError(w, http.StatusInternalServerError, fullErr.Error())
			return
		}
		api.RespondOK(w, NilResponse)
	}
}

// -----------
//  DATA APIS
// -----------

// TODO: Revise me later for new functionality
func (api *BatonAPI) handleDataProposerPayloadDelivered(w http.ResponseWriter, req *http.Request) {
	var err error
	args := req.URL.Query()

	filters := database.GetPayloadsFilters{
		Limit: 200,
	}

	if args.Get("slot") != "" && args.Get("cursor") != "" {
		api.RespondError(w, http.StatusBadRequest, "cannot specify both slot and cursor")
		return
	} else if args.Get("slot") != "" {
		filters.Slot, err = strconv.ParseUint(args.Get("slot"), 10, 64)
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid slot argument")
			return
		}
	} else if args.Get("cursor") != "" {
		filters.Cursor, err = strconv.ParseUint(args.Get("cursor"), 10, 64)
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid cursor argument")
			return
		}
	}

	if args.Get("block_hash") != "" {
		// TODO: old version below
		//var hash boostTypes.Hash
		//err = hash.UnmarshalText([]byte(args.Get("block_hash")))
		var hash phase0.Hash32
		err = hash.UnmarshalJSON([]byte(args.Get("block_hash")))
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid block_hash argument")
			return
		}
		filters.BlockHash = args.Get("block_hash")
	}

	if args.Get("block_number") != "" {
		filters.BlockNumber, err = strconv.ParseUint(args.Get("block_number"), 10, 64)
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid block_number argument")
			return
		}
	}

	if args.Get("proposer_pubkey") != "" {
		if err = checkBLSPublicKeyHex(args.Get("proposer_pubkey")); err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid proposer_pubkey argument")
			return
		}
		filters.ProposerPubkey = args.Get("proposer_pubkey")
	}

	if args.Get("builder_pubkey") != "" {
		if err = checkBLSPublicKeyHex(args.Get("builder_pubkey")); err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid builder_pubkey argument")
			return
		}
		filters.BuilderPubkey = args.Get("builder_pubkey")
	}

	if args.Get("limit") != "" {
		_limit, err := strconv.ParseUint(args.Get("limit"), 10, 64)
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid limit argument")
			return
		}
		if _limit > filters.Limit {
			api.RespondError(w, http.StatusBadRequest, fmt.Sprintf("maximum limit is %d", filters.Limit))
			return
		}
		filters.Limit = _limit
	}

	if args.Get("order_by") == "value" {
		filters.OrderByValue = 1
	} else if args.Get("order_by") == "-value" {
		filters.OrderByValue = -1
	}

	deliveredPayloads, err := api.db.GetRecentDeliveredPayloads(filters)
	if err != nil {
		api.log.WithError(err).Error("error getting recent payloads")
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]common.BidTraceV2JSON, len(deliveredPayloads))

	// TODO: Fix me later
	//for i, payload := range deliveredPayloads {
	//		response[i] = database.DeliveredPayloadEntryToBidTraceV2JSON(payload)
	//	}

	api.RespondOK(w, response)
}

func (api *BatonAPI) handleDataBuilderBidsReceived(w http.ResponseWriter, req *http.Request) {
	var err error
	args := req.URL.Query()

	filters := database.GetBuilderSubmissionsFilters{
		Limit:         500,
		Slot:          0,
		BlockHash:     "",
		BlockNumber:   0,
		BuilderPubkey: "",
	}

	if args.Get("cursor") != "" {
		api.RespondError(w, http.StatusBadRequest, "cursor argument not supported")
		return
	}

	if args.Get("slot") != "" {
		filters.Slot, err = strconv.ParseUint(args.Get("slot"), 10, 64)
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid slot argument")
			return
		}
	}

	if args.Get("block_hash") != "" {
		//var hash boostTypes.Hash
		//err = hash.UnmarshalText([]byte(args.Get("block_hash")))
		var hash phase0.Hash32
		err = hash.UnmarshalJSON([]byte(args.Get("block_hash")))
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid block_hash argument")
			return
		}
		filters.BlockHash = args.Get("block_hash")
	}

	if args.Get("block_number") != "" {
		filters.BlockNumber, err = strconv.ParseUint(args.Get("block_number"), 10, 64)
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid block_number argument")
			return
		}
	}

	if args.Get("builder_pubkey") != "" {
		if err = checkBLSPublicKeyHex(args.Get("builder_pubkey")); err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid builder_pubkey argument")
			return
		}
		filters.BuilderPubkey = args.Get("builder_pubkey")
	}

	// at least one query arguments is required
	if filters.Slot == 0 && filters.BlockHash == "" && filters.BlockNumber == 0 && filters.BuilderPubkey == "" {
		api.RespondError(w, http.StatusBadRequest, "need to query for specific slot or block_hash or block_number or builder_pubkey")
		return
	}

	if args.Get("limit") != "" {
		_limit, err := strconv.ParseUint(args.Get("limit"), 10, 64)
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid limit argument")
			return
		}
		if _limit > filters.Limit {
			api.RespondError(w, http.StatusBadRequest, fmt.Sprintf("maximum limit is %d", filters.Limit))
			return
		}
		filters.Limit = _limit
	}

	blockSubmissions, err := api.db.GetBuilderSubmissions(filters)
	if err != nil {
		api.log.WithError(err).Error("error getting recent payloads")
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]common.BidTraceV2WithTimestampJSON, len(blockSubmissions))
	for i, payload := range blockSubmissions {
		response[i] = database.BuilderSubmissionEntryToBidTraceV2WithTimestampJSON(payload)
	}

	api.RespondOK(w, response)
}

func (api *BatonAPI) handleIncludedTobTxs(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	slotStr := vars["slot"]
	slot, err := strconv.ParseUint(slotStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid slot")
		return
	}
	parentHash := vars["parent_hash"]
	blockHash := vars["block_hash"]
	if parentHash == "" {
		api.RespondError(w, http.StatusBadRequest, "invalid parent_hash")
		return
	}
	if blockHash == "" {
		api.RespondError(w, http.StatusBadRequest, "invalid block_hash")
		return
	}

	tobTxs, err := api.db.GetIncludedTobTxsForGivenSlotAndParentHashAndBlockHash(slot, parentHash, blockHash)
	if err != nil {
		api.log.WithError(err).Error("error getting included tob txs")
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondOK(w, tobTxs)
}

func (api *BatonAPI) handleDataValidatorRegistration(w http.ResponseWriter, req *http.Request) {
	pkStr := req.URL.Query().Get("pubkey")
	if pkStr == "" {
		api.RespondError(w, http.StatusBadRequest, "missing pubkey argument")
		return
	}

	//var pk common.PublicKey
	//err := pk.UnmarshalText([]byte(pkStr))
	var pk bls.PublicKey
	err := pk.Unmarshal([]byte(pkStr))
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid pubkey")
		return
	}

	registrationEntry, err := api.db.GetValidatorRegistration(pkStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			api.RespondError(w, http.StatusBadRequest, "no registration found for validator "+pkStr)
			return
		}
		api.log.WithError(err).Error("error getting validator registration")
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	signedRegistration, err := registrationEntry.ToSignedValidatorRegistration()
	if err != nil {
		api.log.WithError(err).Error("error converting registration entry to signed validator registration")
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondOK(w, signedRegistration)
}

func (api *BatonAPI) handleLivez(w http.ResponseWriter, req *http.Request) {
	api.RespondMsg(w, http.StatusOK, "live")
}

func (api *BatonAPI) handleReadyz(w http.ResponseWriter, req *http.Request) {
	if api.IsReady() {
		api.RespondMsg(w, http.StatusOK, "ready")
	} else {
		api.RespondMsg(w, http.StatusServiceUnavailable, "not ready")
	}
}

func (api *BatonAPI) handleNewSlot(w http.ResponseWriter, req *http.Request) {
	//r.HandleFunc("/newslot", api.handleNewSlot).Methods(http.MethodGet) {
}

func FirstChainID(txs []*chain.Transaction) (string, error) {
	if len(txs) == 0 {
		return "", errors.New("getFirstChainID: no transactions found")
	}
	seqActions := txs[0].Actions
	if len(seqActions) == 0 {
		return "", errors.New("getFirstChainID: no actions in first tx")
	}
	if seqMsg, ok := seqActions[0].(*actions.SequencerMsg); ok {
		chainIDu64 := binary.LittleEndian.Uint64(seqMsg.ChainID)
		chainID := big.NewInt(int64(chainIDu64))
		return hexutil.EncodeBig(chainID), nil
	} else {
		return "", errors.New("could not convert seq actions to seqMsg")
	}
}

// Check if block request is ToB
func (api *BatonAPI) checkBlockRequestIsToB(txs []*chain.Transaction) (bool, error) {
	if len(txs) == 0 {
		return false, errors.New("block request has no transactions provided")
	}

	if len(txs) == 1 {
		return false, errors.New("block request needs more than one transaction provided")
	}

	// RoBs have txs with all the same chain id, ToBs has more than chain id
	firstChainID, err := FirstChainID(txs)
	if err != nil {
		return false, err
	}

	// The last action in the txs list is a transfer action so we don't want to process it.
	for i := 0; i < len(txs)-1; i++ {
		for _, action := range txs[i].Actions {
			if seqMsg, ok := action.(*actions.SequencerMsg); ok {
				chainIDu64 := binary.LittleEndian.Uint64(seqMsg.ChainID)
				chainID := big.NewInt(int64(chainIDu64))
				txChainID := hexutil.EncodeBig(chainID)
				if txChainID != firstChainID {
					return true, nil
				}
			} else {
				return false, errors.New("checkBlockRequestIsToB tx is not sequencer message")
			}
		}
	}

	return false, nil
}

func (api *BatonAPI) GetRoBChainIDs() map[uint64][]string {
	return api.workingRoBChainIDs
}

func (api *BatonAPI) GetRoBChainIDsForSlot(slot uint64) ([]string, bool) {
	values, exists := api.workingRoBChainIDs[slot]
	return values, exists
}

func (api *BatonAPI) AddWorkingRoBChainID(slot uint64, robChainID string) {
	values, exists := api.workingRoBChainIDs[slot]
	if exists {
		// add robChainID if not already present
		for _, chainID := range values {
			if chainID == robChainID {
				return
			}
		}
		api.workingRoBChainIDs[slot] = append(api.workingRoBChainIDs[slot], robChainID)
	} else {
		api.workingRoBChainIDs[slot] = append(api.workingRoBChainIDs[slot], robChainID)
	}
}

func (api *BatonAPI) ClearRoBChainIDsForSlot(slot uint64) {
	delete(api.workingRoBChainIDs, slot)
}

// this method assume the lock was already acquired
func (api *BatonAPI) pruneOldProposerDuties() {
	headSlot := api.headSlot.Load()
	if headSlot < uint64(api.opts.BlockSimDepth) {
		return
	}

	for slot := range api.proposerDutiesMap {
		if slot < headSlot-uint64(api.opts.BlockSimDepth) {
			delete(api.proposerDutiesMap, slot)
		}
	}
}
