// Package api contains the API webserver for the proposer and block-builder APIs
package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/AnomalyFi/nodekit-seq/actions"
	"github.com/NYTimes/gziphandler"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/buger/jsonparser"
	common2 "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/flashbots/go-boost-utils/bls"
	boostTypes "github.com/flashbots/go-boost-utils/types"
	"github.com/flashbots/go-utils/cli"
	"github.com/flashbots/go-utils/httplogger"
	"github.com/flashbots/mev-boost-relay/beaconclient"
	"github.com/flashbots/mev-boost-relay/common"
	"github.com/flashbots/mev-boost-relay/contracts"
	"github.com/flashbots/mev-boost-relay/database"
	"github.com/flashbots/mev-boost-relay/datastore"
	"github.com/go-redis/redis/v9"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	uberatomic "go.uber.org/atomic"
	"golang.org/x/exp/slices"
)

const (
	ErrBlockAlreadyKnown  = "simulation failed: block already known"
	ErrBlockRequiresReorg = "simulation failed: block requires a reorg"
	ErrMissingTrieNode    = "missing trie node"
)

var (
	ErrMissingLogOpt              = errors.New("log parameter is nil")
	ErrMissingBeaconClientOpt     = errors.New("beacon-client is nil")
	ErrMissingDatastoreOpt        = errors.New("proposer datastore is nil")
	ErrRelayPubkeyMismatch        = errors.New("relay pubkey does not match existing one")
	ErrServerAlreadyStarted       = errors.New("server was already started")
	ErrBuilderAPIWithoutSecretKey = errors.New("cannot start builder API without secret key")
)

var (
	// Proposer API (builder-specs)
	pathStatus            = "/eth/v1/builder/status"
	pathRegisterValidator = "/eth/v1/builder/validators"
	pathGetHeader         = "/eth/v1/builder/header/{slot:[0-9]+}/{parent_hash:0x[a-fA-F0-9]+}/{pubkey:0x[a-fA-F0-9]+}"
	pathGetPayload        = "/eth/v1/builder/blinded_blocks"

	// Block builder API
	pathBuilderGetValidators  = "/relay/v1/builder/validators"
	pathSubmitNewBlockRequest = "/relay/v1/builder/submit"
	pathSubmitNewBlock        = "/relay/v1/builder/blocks"
	pathSubmitNewRoBBlock     = "/relay/v1/builder/rob_blocks"
	pathSubmitNewToBTxs       = "/relay/v1/builder/tob_txs"
	pathGetTobGasReservations = "/relay/v1/builder/tob_gas_reservations"

	// Data API
	pathDataProposerPayloadDelivered = "/relay/v1/data/bidtraces/proposer_payload_delivered"
	pathDataBuilderBidsReceived      = "/relay/v1/data/bidtraces/builder_blocks_received"
	pathDataValidatorRegistration    = "/relay/v1/data/validator_registration"
	pathIncludedTobTxs               = "/relay/v1/data/included_tob_txs/{slot:[0-9]+}/{parent_hash:0x[a-fA-F0-9]+}/{block_hash:0x[a-fA-F0-9]+}"

	// Internal API
	pathInternalBuilderStatus     = "/internal/v1/builder/{pubkey:0x[a-fA-F0-9]+}"
	pathInternalBuilderCollateral = "/internal/v1/builder/collateral/{pubkey:0x[a-fA-F0-9]+}"

	// Testing APIs
	// TODO - lets keep this for v0 launch for ease of testing but after that remove it.
	pathGetSlot              = "/eth/v1/relay/get_head_slot"
	pathGetParentHashForSlot = "/eth/v1/relay/get_parent_hash_for_slot/{slot:[0-9]+}"
	pathGetProposerForSlot   = "/eth/v1/relay/get_proposer_for_slot/{slot:[0-9]+}"

	// number of goroutines to save active validator
	numValidatorRegProcessors = cli.GetEnvInt("NUM_VALIDATOR_REG_PROCESSORS", 10)

	// various timings
	timeoutGetPayloadRetryMs  = cli.GetEnvInt("GETPAYLOAD_RETRY_TIMEOUT_MS", 100)
	getHeaderRequestCutoffMs  = cli.GetEnvInt("GETHEADER_REQUEST_CUTOFF_MS", 3000)
	getPayloadRequestCutoffMs = cli.GetEnvInt("GETPAYLOAD_REQUEST_CUTOFF_MS", 4000)
	getPayloadResponseDelayMs = cli.GetEnvInt("GETPAYLOAD_RESPONSE_DELAY_MS", 1000)

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

// BatonAPIOpts contains the options for a relay
type BatonAPIOpts struct {
	Log *logrus.Entry

	ListenAddr  string
	BlockSimURL string

	BeaconClient beaconclient.IMultiBeaconClient
	Datastore    *datastore.Datastore
	Redis        *datastore.RedisCache
	Memcached    *datastore.Memcached
	DB           database.IDatabaseService

	SecretKey *bls.SecretKey // used to sign bids (getHeader responses)

	// Network specific variables
	EthNetDetails common.EthNetworkDetails

	// APIs to enable
	ProposerAPI     bool
	BlockBuilderAPI bool
	DataAPI         bool
	PprofAPI        bool
	InternalAPI     bool
}

type payloadAttributesHelper struct {
	slot              uint64
	parentHash        string
	withdrawalsRoot   phase0.Root
	payloadAttributes beaconclient.PayloadAttributes
}

// Data needed to issue a block validation request.
type blockSimOptions struct {
	isHighPrio bool
	fastTrack  bool
	log        *logrus.Entry
	builder    *blockBuilderCacheEntry
	req        *common.BuilderBlockValidationRequest
}

// Like the above but
type blockSimOptions2 struct {
	isHighPrio         bool
	fastTrack          bool
	log                *logrus.Entry
	builder            *blockBuilderCacheEntry
	RegisteredGasLimit uint64
	req                *common.SubmitNewBlockRequest
	//req        *common.BuilderBlockValidationRequest
}

type blockBuilderCacheEntry struct {
	status     common.BuilderStatus
	collateral *big.Int
}

type blockSimResult struct {
	wasSimulated         bool
	optimisticSubmission bool
	requestErr           error
	validationErr        error
}

type tracerOptions struct {
	log *logrus.Entry
	tx  *types.Transaction
}

type tracerResult struct {
	tracerResponse *common.CallTraceResponse
	tracerError    error
}

// BatonAPI represents a single Relay instance
type BatonAPI struct {
	opts BatonAPIOpts
	log  *logrus.Entry

	blsSk     *bls.SecretKey
	publicKey *boostTypes.PublicKey

	srv         *http.Server
	srvStarted  uberatomic.Bool
	srvShutdown uberatomic.Bool

	beaconClient beaconclient.IMultiBeaconClient
	datastore    *datastore.Datastore
	redis        *datastore.RedisCache
	memcached    *datastore.Memcached
	db           database.IDatabaseService

	headSlot     uberatomic.Uint64
	genesisInfo  *beaconclient.GetGenesisResponse
	capellaEpoch uint64
	denebEpoch   uint64

	proposerDutiesLock       sync.RWMutex
	proposerDutiesResponse   *[]byte // raw http response
	proposerDutiesMap        map[uint64]*common.BuilderGetValidatorsResponseEntry
	proposerDutiesSlot       uint64
	isUpdatingProposerDuties uberatomic.Bool

	blockSimRateLimiter IBlockSimRateLimiter
	blockAssembler      IBlockAssembler
	tracer              ITracer

	validatorRegC chan boostTypes.SignedValidatorRegistration

	// used to wait on any active getPayload calls on shutdown
	getPayloadCallsInFlight sync.WaitGroup

	// Feature flags
	ffForceGetHeader204          bool
	ffDisableLowPrioBuilders     bool
	ffDisablePayloadDBStorage    bool // disable storing the execution payloads in the database
	ffLogInvalidSignaturePayload bool // log payload if getPayload signature validation fails
	ffEnableCancellations        bool // whether to enable block builder cancellations
	ffRegValContinueOnInvalidSig bool // whether to continue processing further validators if one fails
	ffIgnorableValidationErrors  bool // whether to enable ignorable validation errors
	ffMockSimulation             bool // simulations always pass, intended for testing internal server functionality

	payloadAttributes       map[string]payloadAttributesHelper // key:parentBlockHash
	payloadAttributesBySlot map[uint64]payloadAttributesHelper // key:slot
	payloadAttributesLock   sync.RWMutex

	// The slot we are currently optimistically simulating.
	optimisticSlot uberatomic.Uint64
	// The number of optimistic blocks being processed (only used for logging).
	optimisticBlocksInFlight uberatomic.Uint64
	// Wait group used to monitor status of per-slot optimistic processing.
	optimisticBlocksWG sync.WaitGroup
	// Cache for builder statuses and collaterals.
	blockBuildersCache map[string]*blockBuilderCacheEntry
	// stores DeFi contract addresses rquired for state interference checks
	defiAddresses map[string]common2.Address
	// stores RoB chain IDs
	robChainIDs map[string]struct{}
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

	if opts.BeaconClient == nil {
		return nil, ErrMissingBeaconClientOpt
	}

	if opts.Datastore == nil {
		return nil, ErrMissingDatastoreOpt
	}

	// If block-builder API is enabled, then ensure secret key is all set
	var publicKey boostTypes.PublicKey
	if opts.BlockBuilderAPI {
		if opts.SecretKey == nil {
			return nil, ErrBuilderAPIWithoutSecretKey
		}

		// If using a secret key, ensure it's the correct one
		blsPubkey, err := bls.PublicKeyFromSecretKey(opts.SecretKey)
		if err != nil {
			return nil, err
		}
		publicKey, err = boostTypes.BlsPublicKeyToPublicKey(blsPubkey)
		if err != nil {
			return nil, err
		}
		opts.Log.Infof("Using BLS key: %s", publicKey.String())

		// ensure pubkey is same across all relay instances
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

	api = &BatonAPI{
		opts:         opts,
		log:          opts.Log,
		blsSk:        opts.SecretKey,
		publicKey:    &publicKey,
		datastore:    opts.Datastore,
		beaconClient: opts.BeaconClient,
		redis:        opts.Redis,
		memcached:    opts.Memcached,
		db:           opts.DB,

		payloadAttributes:       make(map[string]payloadAttributesHelper),
		payloadAttributesBySlot: make(map[uint64]payloadAttributesHelper),

		proposerDutiesResponse: &[]byte{},
		blockSimRateLimiter:    NewBlockSimulationRateLimiter(opts.BlockSimURL),
		blockAssembler:         NewBlockAssembler(opts.BlockSimURL),
		tracer:                 NewTracer(opts.BlockSimURL),

		validatorRegC: make(chan boostTypes.SignedValidatorRegistration, 450_000),
		defiAddresses: FillUpDefiAddresses(opts),
		robChainIDs:   make(map[string]struct{}),
	}

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

func (api *BatonAPI) getRouter() http.Handler {
	r := mux.NewRouter()

	r.HandleFunc("/", api.handleRoot).Methods(http.MethodGet)
	r.HandleFunc("/livez", api.handleLivez).Methods(http.MethodGet)
	r.HandleFunc("/readyz", api.handleReadyz).Methods(http.MethodGet)

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

	log := api.log.WithField("method", "StartServer")

	// Get best beacon-node status by head slot, process current slot and start slot updates
	syncStatus, err := api.beaconClient.BestSyncStatus()
	if err != nil {
		return err
	}
	currentSlot := syncStatus.HeadSlot

	// Initialize block builder cache.
	api.blockBuildersCache = make(map[string]*blockBuilderCacheEntry)

	// Get genesis info
	api.genesisInfo, err = api.beaconClient.GetGenesis()
	if err != nil {
		return err
	}
	log.Infof("genesis info: %d", api.genesisInfo.Data.GenesisTime)

	// Get and prepare fork schedule
	forkSchedule, err := api.beaconClient.GetForkSchedule()
	if err != nil {
		return err
	}

	var foundCapellaEpoch, foundDenebEpoch bool
	for _, fork := range forkSchedule.Data {
		log.Infof("forkSchedule: version=%s / epoch=%d", fork.CurrentVersion, fork.Epoch)
		switch fork.CurrentVersion {
		case api.opts.EthNetDetails.CapellaForkVersionHex:
			foundCapellaEpoch = true
			api.capellaEpoch = fork.Epoch
		case api.opts.EthNetDetails.DenebForkVersionHex:
			foundDenebEpoch = true
			api.denebEpoch = fork.Epoch
		}
	}

	// Print fork version information
	if foundDenebEpoch && hasReachedFork(currentSlot, api.denebEpoch) {
		log.Infof("deneb fork detected (currentEpoch: %d / denebEpoch: %d)", common.SlotToEpoch(currentSlot), api.denebEpoch)
	} else if foundCapellaEpoch && hasReachedFork(currentSlot, api.capellaEpoch) {
		log.Infof("capella fork detected (currentEpoch: %d / capellaEpoch: %d)", common.SlotToEpoch(currentSlot), api.capellaEpoch)
	}

	// start proposer API specific things
	if api.opts.ProposerAPI {
		// Update known validators (which can take 10-30 sec). This is a requirement for service readiness, because without them,
		// getPayload() doesn't have the information it needs (known validators), which could lead to missed slots.
		go api.datastore.RefreshKnownValidators(api.log, api.beaconClient, currentSlot)

		// Start the validator registration db-save processor
		api.log.Infof("starting %d validator registration processors", numValidatorRegProcessors)
		for i := 0; i < numValidatorRegProcessors; i++ {
			go api.startValidatorRegistrationDBProcessor()
		}
	}

	// start block-builder API specific things
	if api.opts.BlockBuilderAPI {
		// Get current proposer duties blocking before starting, to have them ready
		api.updateProposerDuties(syncStatus.HeadSlot)

		// TODO: We shouldn't need payload attributes event. Remove when absolutely sure.
		/*
			// Subscribe to payload attributes events (only for builder-api)
			go func() {
				c := make(chan beaconclient.PayloadAttributesEvent)
				api.beaconClient.SubscribeToPayloadAttributesEvents(c)
				for {
					payloadAttributes := <-c
					api.processPayloadAttributes(payloadAttributes)
				}
			}()
		*/
	}

	// Process current slot
	api.processNewSlot(currentSlot)

	// Start regular slot updates
	go func() {
		c := make(chan beaconclient.HeadEventData)
		api.beaconClient.SubscribeToHeadEvents(c)
		for {
			headEvent := <-c
			api.processNewSlot(headEvent.Slot)
		}
	}()

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

func (api *BatonAPI) startValidatorRegistrationDBProcessor() {
	for valReg := range api.validatorRegC {
		err := api.datastore.SaveValidatorRegistration(valReg)
		if err != nil {
			api.log.WithError(err).WithFields(logrus.Fields{
				"reg_pubkey":       valReg.Message.Pubkey,
				"reg_feeRecipient": valReg.Message.FeeRecipient,
				"reg_gasLimit":     valReg.Message.GasLimit,
				"reg_timestamp":    valReg.Message.Timestamp,
			}).Error("error saving validator registration")
		}
	}
}

func (api *BatonAPI) TobTxChecks(trace *common.CallTrace) (bool, error) {
	if api.opts.EthNetDetails.Name == common.EthNetworkCustom {
		return api.TraceChecker(trace, api.IsTraceToWEthDaiPair)
	} else if api.opts.EthNetDetails.Name == common.EthNetworkGoerli {
		return api.TraceChecker(trace, api.IsTraceUniV3EthUsdcSwap)
	}

	return false, fmt.Errorf("state interference checks not implemented for %s", api.opts.EthNetDetails.Name)
}

// just check if it goes to the DaiWethPair with a swap tx
func (api *BatonAPI) TraceChecker(trace *common.CallTrace, f common.NetworkTobTxChecker) (bool, error) {
	stack := []common.CallTrace{*trace}

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		res, err := f(current)
		if err != nil {
			return false, err
		}
		if res {
			return true, nil
		}

		for _, call := range current.Calls {
			stack = append(stack, call)
		}
	}

	return false, nil
}

func (api *BatonAPI) BaseTraceChecks(callTrace common.CallTrace) (bool, error) {
	if callTrace.To == nil {
		return false, nil
	}
	if callTrace.Type == "STATICCALL" {
		return false, nil
	}
	if len(callTrace.Input) < 4 {
		return false, nil
	}

	return true, nil
}

func (api *BatonAPI) IsTraceUniV3EthUsdcSwap(callTrace common.CallTrace) (bool, error) {
	isValid, err := api.BaseTraceChecks(callTrace)
	if err != nil {
		return false, err
	}
	if !isValid {
		return false, nil
	}

	if *callTrace.To != api.defiAddresses[common.UniV3SwapRouter] {
		return false, nil
	}

	uniV3SwapRouterAbi, err := contracts.UniswapV3SwapRouterMetaData.GetAbi()
	if err != nil {
		return false, err
	}
	exactInputSingleId := uniV3SwapRouterAbi.Methods["exactInputSingle"].ID
	if !bytes.Equal(callTrace.Input[:4], exactInputSingleId) {
		return false, nil
	}

	// unpack the args
	args, err := common.GetMethodArgs(callTrace.Input, "exactInputSingle", uniV3SwapRouterAbi)
	if err != nil {
		return false, err
	}
	argBytes, err := json.Marshal(args)
	swapRouterParams := new(contracts.ISwapRouterExactInputSingleParams)
	err = json.Unmarshal(argBytes, swapRouterParams)
	if err != nil {
		return false, err
	}

	validTokenPairs := [][2]common2.Address{
		{api.defiAddresses[common.WethToken], api.defiAddresses[common.UsdcToken]},
		{api.defiAddresses[common.WethToken], api.defiAddresses[common.WbtcToken]},
		{api.defiAddresses[common.WethToken], api.defiAddresses[common.DaiToken]},
	}

	// check if (tokenIn, tokenOut) == (WETH, USDC) or (USDC, WETH) or (WETH, WBTC) or (WBTC, WETH) or (WETH, DAI) or (DAI, WETH)
	for _, tokenPairs := range validTokenPairs {
		if swapRouterParams.TokenIn == tokenPairs[0] && swapRouterParams.TokenOut == tokenPairs[1] {
			return true, nil
		}
		if swapRouterParams.TokenIn == tokenPairs[1] && swapRouterParams.TokenOut == tokenPairs[0] {
			return true, nil
		}
	}

	return false, nil
}

// This will change based on the state interference check
func (api *BatonAPI) IsTraceToWEthDaiPair(callTrace common.CallTrace) (bool, error) {
	isValid, err := api.BaseTraceChecks(callTrace)
	if err != nil {
		return false, err
	}
	if !isValid {
		return false, nil
	}

	uniswapDaiWethAddress1 := api.defiAddresses[common.DaiWethPair1]
	uniswapDaiWethAddress2 := api.defiAddresses[common.DaiWethPair2]
	if *callTrace.To != uniswapDaiWethAddress1 && *callTrace.To != uniswapDaiWethAddress2 {
		return false, nil
	}

	// this will be the same across all environments
	uniswapPairAbi, err := contracts.UniswapPairMetaData.GetAbi()
	if err != nil {
		return false, err
	}
	swapId := uniswapPairAbi.Methods["swap"].ID
	if !bytes.Equal(callTrace.Input[:4], swapId) {
		return false, nil
	}

	return true, nil
}

func (api *BatonAPI) getTraces(ctx context.Context, opts tracerOptions) (*common.CallTraceResponse, error) {
	t := time.Now()
	res, err := api.tracer.TraceTx(ctx, opts.tx)
	log := opts.log.WithFields(logrus.Fields{
		"durationMs": time.Since(t).Milliseconds(),
	})
	if err != nil {
		log.WithError(err).Warn("tracer failed")
		return nil, err
	}
	log.Info("tracer successful")
	return res, nil
}

// simulateBlock sends a request for a block simulation to blockSimRateLimiter.
// @TODO: Fix me to work with new submit block msg format
func (api *BatonAPI) simulateBlock(
	ctx context.Context,
	req *common.SubmitNewBlockRequest,
	log *logrus.Entry,
) (gasUsed uint64, requestErr, validationErr error) {
	t := time.Now()

	var txs []hexutil.Bytes
	txsByNamespace := make(map[string][]hexutil.Bytes)
	txs = make([]hexutil.Bytes, 0)
	for _, tx := range req.Chunk.Txs {
		for _, action := range tx.Actions {
			if seqMsg, ok := action.(*actions.SequencerMsg); ok {
				txs = append(txs, seqMsg.Data)
				// @TODO: grouping txs with same namespace into groups, check over logic
				ns := string(seqMsg.ChainId)
				txsByNamespace[ns] = append(txsByNamespace[ns], seqMsg.ChainId)
			} else {
				log.Error("simulateBlock: tx is not sequencer message")
				return 0, errors.New("simulateBlock: tx is not sequencer message"), nil
			}
		}
	}
	blockNumber := req.BlockNumber()
	if blockNumber == nil {
		log.Error("simulateBlock: BlockNumber is nil")
		return 0, errors.New("simulateBlock: BlockNumber is nil"), nil
	}
	// Extract the block number from the map
	var blockNumberStr string
	for _, v := range *blockNumber {
		blockNumberStr = v
		break
	}
	// @TODO: Fix me to work with new submit block msg format
	blockReq := common.BlockValidationRequest{
		Txs:              txs,
		BlockNumber:      blockNumberStr,
		StateBlockNumber: "latest",
		Timestamp:        uint64(time.Now().UnixMilli()),
	}

	gasUsed, requestErr, validationErr = api.blockSimRateLimiter.SimBlockAndGetGasUsed(ctx, &blockReq)
	log = log.WithFields(logrus.Fields{
		"durationMs": time.Since(t).Milliseconds(),
		"numWaiting": api.blockSimRateLimiter.CurrentCounter(),
	})
	if validationErr != nil {
		if api.ffIgnorableValidationErrors {
			// Operators chooses to ignore certain validation errors
			ignoreError := validationErr.Error() == ErrBlockAlreadyKnown || validationErr.Error() == ErrBlockRequiresReorg || strings.Contains(validationErr.Error(), ErrMissingTrieNode)
			if ignoreError {
				log.WithError(validationErr).Warn("block validation failed with ignorable error")
				return uint64(0), nil, nil
			}
		}
		log.WithError(validationErr).Warn("block validation failed")
		return 0, nil, validationErr
	}
	if requestErr != nil {
		log.WithError(requestErr).Warn("block validation failed: request error")
		return 0, requestErr, nil
	}
	log.Info("block validation successful")
	return 0, nil, nil
}

/*
func (api *BatonAPI) simulateBlockTxs(
	ctx context.Context,
	opts blockSimOptions2,
) (requestErr, validationErr error) {
	if api.ffMockSimulation {
		return nil, nil
	}

	t := time.Now()
	requestErr, validationErr = api.blockSimRateLimiter.Send(ctx, opts.req, opts.isHighPrio, opts.fastTrack)
	log := opts.log.WithFields(logrus.Fields{
		"durationMs": time.Since(t).Milliseconds(),
		"numWaiting": api.blockSimRateLimiter.CurrentCounter(),
	})
	if validationErr != nil {
		if api.ffIgnorableValidationErrors {
			// Operators chooses to ignore certain validation errors
			ignoreError := validationErr.Error() == ErrBlockAlreadyKnown || validationErr.Error() == ErrBlockRequiresReorg || strings.Contains(validationErr.Error(), ErrMissingTrieNode)
			if ignoreError {
				log.WithError(validationErr).Warn("block validation failed with ignorable error")
				return nil, nil
			}
		}
		log.WithError(validationErr).Warn("block validation failed")
		return nil, validationErr
	}
	if requestErr != nil {
		log.WithError(requestErr).Warn("block validation failed: request error")
		return requestErr, nil
	}
	log.Info("block validation successful")
	return nil, nil
}
*/

// TODO: Verify if this is needed. I suspect not since it is part of the Eth 2.0 consensus layer.
/*
func (api *BatonAPI) processPayloadAttributes(payloadAttributes beaconclient.PayloadAttributesEvent) {
	apiHeadSlot := api.headSlot.Load()
	payloadAttrSlot := payloadAttributes.Data.ProposalSlot

	// require proposal slot in the future
	if payloadAttrSlot <= apiHeadSlot {
		return
	}
	log := api.log.WithFields(logrus.Fields{
		"headSlot":          apiHeadSlot,
		"payloadAttrSlot":   payloadAttrSlot,
		"payloadAttrParent": payloadAttributes.Data.ParentBlockHash,
	})

	// discard payload attributes if already known
	api.payloadAttributesLock.RLock()
	_, ok := api.payloadAttributes[payloadAttributes.Data.ParentBlockHash]
	api.payloadAttributesLock.RUnlock()

	if ok {
		return
	}

	var withdrawalsRoot phase0.Root
	var err error
	if hasReachedFork(payloadAttrSlot, api.capellaEpoch) {
		withdrawalsRoot, err = ComputeWithdrawalsRoot(payloadAttributes.Data.PayloadAttributes.Withdrawals)
		log = log.WithField("withdrawalsRoot", withdrawalsRoot.String())
		if err != nil {
			log.WithError(err).Error("error computing withdrawals root")
			return
		}
	}

	api.payloadAttributesLock.Lock()
	defer api.payloadAttributesLock.Unlock()

	// Step 1: clean up old ones
	for parentBlockHash, attr := range api.payloadAttributes {
		if attr.slot < apiHeadSlot {
			delete(api.payloadAttributes, parentBlockHash)
		}
	}

	// Step 2: save new one
	api.payloadAttributes[payloadAttributes.Data.ParentBlockHash] = payloadAttributesHelper{
		slot:              payloadAttrSlot,
		parentHash:        payloadAttributes.Data.ParentBlockHash,
		withdrawalsRoot:   withdrawalsRoot,
		payloadAttributes: payloadAttributes.Data.PayloadAttributes,
	}

	api.payloadAttributesBySlot[payloadAttrSlot] = payloadAttributesHelper{
		slot:              payloadAttrSlot,
		parentHash:        payloadAttributes.Data.ParentBlockHash,
		withdrawalsRoot:   withdrawalsRoot,
		payloadAttributes: payloadAttributes.Data.PayloadAttributes,
	}

	log.WithFields(logrus.Fields{
		"randao":    payloadAttributes.Data.PayloadAttributes.PrevRandao,
		"timestamp": payloadAttributes.Data.PayloadAttributes.Timestamp,
	}).Info("updated payload attributes")
}
*/

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

	// only for builder-api
	if api.opts.BlockBuilderAPI || api.opts.ProposerAPI {
		// update proposer duties in the background
		go api.updateProposerDuties(headSlot)

		// update the optimistic slot
		go api.prepareBuildersForSlot(headSlot)
	}

	if api.opts.ProposerAPI {
		go api.datastore.RefreshKnownValidators(api.log, api.beaconClient, headSlot)
	}

	// log
	epoch := headSlot / common.SlotsPerEpoch
	api.log.WithFields(logrus.Fields{
		"epoch":              epoch,
		"slotHead":           headSlot,
		"slotStartNextEpoch": (epoch + 1) * common.SlotsPerEpoch,
	}).Infof("updated headSlot to %d", headSlot)
}

func (api *BatonAPI) updateProposerDuties(headSlot uint64) {
	// Ensure only one updating is running at a time
	if api.isUpdatingProposerDuties.Swap(true) {
		return
	}
	defer api.isUpdatingProposerDuties.Store(false)

	// Update once every 8 slots (or more, if a slot was missed)
	if headSlot%8 != 0 && headSlot-api.proposerDutiesSlot < 8 {
		return
	}

	// Load upcoming proposer duties from Redis
	duties, err := api.redis.GetProposerDuties()
	if err != nil {
		api.log.WithError(err).Error("failed getting proposer duties from redis")
		return
	}

	// Prepare raw bytes for HTTP response
	respBytes, err := json.Marshal(duties)
	if err != nil {
		api.log.WithError(err).Error("error marshalling duties")
	}

	// Prepare the map for lookup by slot
	dutiesMap := make(map[uint64]*common.BuilderGetValidatorsResponseEntry)
	for index, duty := range duties {
		dutiesMap[duty.Slot] = &duties[index]
	}

	// Update
	api.proposerDutiesLock.Lock()
	if len(respBytes) > 0 {
		api.proposerDutiesResponse = &respBytes
	}
	api.proposerDutiesMap = dutiesMap
	api.proposerDutiesSlot = headSlot
	api.proposerDutiesLock.Unlock()

	// pretty-print
	_duties := make([]string, len(duties))
	for i, duty := range duties {
		_duties[i] = fmt.Sprint(duty.Slot)
	}
	sort.Strings(_duties)
	api.log.Infof("proposer duties updated: %s", strings.Join(_duties, ", "))
}

func (api *BatonAPI) prepareBuildersForSlot(headSlot uint64) {
	// Wait until there are no optimistic blocks being processed. Then we can
	// safely update the slot.
	api.optimisticBlocksWG.Wait()
	api.optimisticSlot.Store(headSlot + 1)

	builders, err := api.db.GetBlockBuilders()
	if err != nil {
		api.log.WithError(err).Error("unable to read block builders from db, not updating builder cache")
		return
	}
	api.log.Debugf("Updating builder cache with %d builders from database", len(builders))

	newCache := make(map[string]*blockBuilderCacheEntry)
	for _, v := range builders {
		entry := &blockBuilderCacheEntry{ //nolint:exhaustruct
			status: common.BuilderStatus{
				IsHighPrio:    v.IsHighPrio,
				IsBlacklisted: v.IsBlacklisted,
				IsOptimistic:  v.IsOptimistic,
			},
		}
		// Try to parse builder collateral string to big int.
		builderCollateral, ok := big.NewInt(0).SetString(v.Collateral, 10)
		if !ok {
			api.log.WithError(err).Errorf("could not parse builder collateral string %s", v.Collateral)
			entry.collateral = big.NewInt(0)
		} else {
			entry.collateral = builderCollateral
		}
		newCache[v.BuilderPubkey] = entry
	}
	api.blockBuildersCache = newCache
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
	fmt.Fprintf(w, "MEV-Boost Relay API")
}

func (api *BatonAPI) handleGetSlot(w http.ResponseWriter, req *http.Request) {
	api.RespondOK(w, api.headSlot.Load())
}

func (api *BatonAPI) handleGetParentHashForSlot(w http.ResponseWriter, req *http.Request) {
	// slot is passed as url args
	slotStr := mux.Vars(req)["slot"]
	slot, err := strconv.ParseUint(slotStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
	}

	// get parent hash
	res, ok := api.payloadAttributesBySlot[slot]
	if !ok {
		api.RespondError(w, http.StatusNotFound, "slot payload attributes not found")
		return
	}
	api.RespondOK(w, res.parentHash)
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
	api.RespondOK(w, res.Entry.Message.FeeRecipient.String())
}

func (api *BatonAPI) handleRegisterValidator(w http.ResponseWriter, req *http.Request) {
	ua := req.UserAgent()
	log := api.log.WithFields(logrus.Fields{
		"method":        "registerValidator",
		"ua":            ua,
		"mevBoostV":     common.GetMevBoostVersionFromUserAgent(ua),
		"headSlot":      api.headSlot.Load(),
		"contentLength": req.ContentLength,
	})

	start := time.Now().UTC()
	registrationTimestampUpperBound := start.Unix() + 10 // 10 seconds from now

	numRegTotal := 0
	numRegProcessed := 0
	numRegActive := 0
	numRegNew := 0
	processingStoppedByError := false

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

	parseRegistration := func(value []byte) (reg *boostTypes.SignedValidatorRegistration, err error) {
		// Pubkey
		_pubkey, err := jsonparser.GetUnsafeString(value, "message", "pubkey")
		if err != nil {
			return nil, fmt.Errorf("registration message error (pubkey): %w", err)
		}

		pubkey, err := boostTypes.HexToPubkey(_pubkey)
		if err != nil {
			return nil, fmt.Errorf("registration message error (pubkey): %w", err)
		}

		// Timestamp
		_timestamp, err := jsonparser.GetUnsafeString(value, "message", "timestamp")
		if err != nil {
			return nil, fmt.Errorf("registration message error (timestamp): %w", err)
		}

		timestamp, err := strconv.ParseUint(_timestamp, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp: %w", err)
		}

		// GasLimit
		_gasLimit, err := jsonparser.GetUnsafeString(value, "message", "gas_limit")
		if err != nil {
			return nil, fmt.Errorf("registration message error (gasLimit): %w", err)
		}

		gasLimit, err := strconv.ParseUint(_gasLimit, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid gasLimit: %w", err)
		}

		// FeeRecipient
		_feeRecipient, err := jsonparser.GetUnsafeString(value, "message", "fee_recipient")
		if err != nil {
			return nil, fmt.Errorf("registration message error (fee_recipient): %w", err)
		}

		feeRecipient, err := boostTypes.HexToAddress(_feeRecipient)
		if err != nil {
			return nil, fmt.Errorf("registration message error (fee_recipient): %w", err)
		}

		// Signature
		_signature, err := jsonparser.GetUnsafeString(value, "signature")
		if err != nil {
			return nil, fmt.Errorf("registration message error (signature): %w", err)
		}

		signature, err := boostTypes.HexToSignature(_signature)
		if err != nil {
			return nil, fmt.Errorf("registration message error (signature): %w", err)
		}

		// Construct and return full registration object
		reg = &boostTypes.SignedValidatorRegistration{
			Message: &boostTypes.RegisterValidatorRequestMessage{
				FeeRecipient: feeRecipient,
				GasLimit:     gasLimit,
				Timestamp:    timestamp,
				Pubkey:       pubkey,
			},
			Signature: signature,
		}

		return reg, nil
	}

	// Iterate over the registrations
	_, err = jsonparser.ArrayEach(body, func(value []byte, dataType jsonparser.ValueType, offset int, _err error) {
		numRegTotal += 1
		if processingStoppedByError {
			return
		}
		numRegProcessed += 1
		regLog := log.WithFields(logrus.Fields{
			"numRegistrationsSoFar":     numRegTotal,
			"numRegistrationsProcessed": numRegProcessed,
		})

		// Extract immediately necessary registration fields
		signedValidatorRegistration, err := parseRegistration(value)
		if err != nil {
			handleError(regLog, http.StatusBadRequest, err.Error())
			return
		}

		// Add validator pubkey to logs
		pkHex := signedValidatorRegistration.Message.Pubkey.PubkeyHex()
		regLog = regLog.WithFields(logrus.Fields{
			"pubkey":       pkHex,
			"signature":    signedValidatorRegistration.Signature.String(),
			"feeRecipient": signedValidatorRegistration.Message.FeeRecipient.String(),
			"gasLimit":     signedValidatorRegistration.Message.GasLimit,
			"timestamp":    signedValidatorRegistration.Message.Timestamp,
		})

		// Ensure a valid timestamp (not too early, and not too far in the future)
		registrationTimestamp := int64(signedValidatorRegistration.Message.Timestamp)
		if registrationTimestamp < int64(api.genesisInfo.Data.GenesisTime) {
			handleError(regLog, http.StatusBadRequest, "timestamp too early")
			return
		} else if registrationTimestamp > registrationTimestampUpperBound {
			handleError(regLog, http.StatusBadRequest, "timestamp too far in the future")
			return
		}

		// Check if a real validator
		isKnownValidator := api.datastore.IsKnownValidator(pkHex)
		if !isKnownValidator {
			handleError(regLog, http.StatusBadRequest, fmt.Sprintf("not a known validator: %s", pkHex.String()))
			return
		}

		// Check for a previous registration timestamp
		prevTimestamp, err := api.redis.GetValidatorRegistrationTimestamp(pkHex)
		if err != nil {
			regLog.WithError(err).Error("error getting last registration timestamp")
		} else if prevTimestamp >= signedValidatorRegistration.Message.Timestamp {
			// abort if the current registration timestamp is older or equal to the last known one
			return
		}

		// Verify the signature
		ok, err := boostTypes.VerifySignature(signedValidatorRegistration.Message, api.opts.EthNetDetails.DomainBuilder, signedValidatorRegistration.Message.Pubkey[:], signedValidatorRegistration.Signature[:])
		if err != nil {
			regLog.WithError(err).Error("error verifying registerValidator signature")
			return
		} else if !ok {
			regLog.Info("invalid validator signature")
			if api.ffRegValContinueOnInvalidSig {
				return
			} else {
				handleError(regLog, http.StatusBadRequest, fmt.Sprintf("failed to verify validator signature for %s", signedValidatorRegistration.Message.Pubkey.String()))
				return
			}
		}

		// Now we have a new registration to process
		numRegNew += 1

		// Save to database
		select {
		case api.validatorRegC <- *signedValidatorRegistration:
		default:
			regLog.Error("validator registration channel full")
		}
	})

	log = log.WithFields(logrus.Fields{
		"timeNeededSec":             time.Since(start).Seconds(),
		"timeNeededMs":              time.Since(start).Milliseconds(),
		"numRegistrations":          numRegTotal,
		"numRegistrationsActive":    numRegActive,
		"numRegistrationsProcessed": numRegProcessed,
		"numRegistrationsNew":       numRegNew,
		"processingStoppedByError":  processingStoppedByError,
	})

	if err != nil {
		handleError(log, http.StatusBadRequest, "error in traversing json")
		return
	}

	log.Info("validator registrations call processed")
	w.WriteHeader(http.StatusOK)
}

func (api *BatonAPI) handleGetHeader(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	slotStr := vars["slot"]
	parentHashHex := vars["parent_hash"]
	proposerPubkeyHex := vars["pubkey"]
	ua := req.UserAgent()
	headSlot := api.headSlot.Load()

	slot, err := strconv.ParseUint(slotStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, common.ErrInvalidSlot.Error())
		return
	}

	requestTime := time.Now().UTC()
	slotStartTimestamp := api.genesisInfo.Data.GenesisTime + (slot * common.SecondsPerSlot)
	msIntoSlot := requestTime.UnixMilli() - int64((slotStartTimestamp * 1000))

	log := api.log.WithFields(logrus.Fields{
		"method":           "getHeader",
		"headSlot":         headSlot,
		"slot":             slotStr,
		"parentHash":       parentHashHex,
		"pubkey":           proposerPubkeyHex,
		"ua":               ua,
		"mevBoostV":        common.GetMevBoostVersionFromUserAgent(ua),
		"requestTimestamp": requestTime.Unix(),
		"slotStartSec":     slotStartTimestamp,
		"msIntoSlot":       msIntoSlot,
	})

	if len(proposerPubkeyHex) != 98 {
		api.RespondError(w, http.StatusBadRequest, common.ErrInvalidPubkey.Error())
		return
	}

	if len(parentHashHex) != 66 {
		api.RespondError(w, http.StatusBadRequest, common.ErrInvalidHash.Error())
		return
	}

	if slot < headSlot {
		api.RespondError(w, http.StatusBadRequest, "slot is too old")
		return
	}

	log.Debug("getHeader request received")

	if slices.Contains(apiNoHeaderUserAgents, ua) {
		log.Info("rejecting getHeader by user agent")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if api.ffForceGetHeader204 {
		log.Info("forced getHeader 204 response")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Only allow requests for the current slot until a certain cutoff time
	if getHeaderRequestCutoffMs > 0 && msIntoSlot > 0 && msIntoSlot > int64(getHeaderRequestCutoffMs) {
		log.Info("getHeader sent too late")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// @TODO: double check if helper funcs or funcs in general need to return AnchorHeader instead of common.Hash
	var resp common.HeaderInfo
	bid, err := api.redis.GetBestToBBid(slot, parentHashHex, proposerPubkeyHex)
	if err != nil {
		log.WithError(err).Error("could not get bid for ToB")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// think of more cases for hash if possible
	if (bid.Header.Big().Cmp(big.NewInt(0))) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(bid.BlockHash) != 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp.ToBHash = bid
	combined := resp.ToBHash.Header.Bytes()

	for chainID, _ := range api.robChainIDs {
		bid, err := api.redis.GetBestRoBBid(slot, parentHashHex, proposerPubkeyHex, chainID)
		if err != nil {
			log.WithError(err).Error("could not get bid for RoB: " + chainID)
			api.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		// think of more cases for hash if possible
		if (bid.Header.Big().Cmp(big.NewInt(0))) == 0 {
			log.Error("handleGetHeader: rob chunk had zero value")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		resp.RoBHashes[chainID] = bid
		combined = append(combined, bid.Header.Bytes()...)
	}

	// Hash the concatenated result to get a third hash
	chunkHashBytes := sha256.Sum256(combined)

	// Convert the result to a common.Hash
	var headerReq common.AnchorGetHeaderResponse
	headerReq.ChunkHash = common2.BytesToHash(chunkHashBytes[:])
	headerReq.Slot = slot

	if resp.ToBHash == nil && len(resp.RoBHashes) == 0 {
		log.Info("handleGetHeader: no chunks, nothing to do")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var final common.Header
	final.Info = resp
	final.Resp = headerReq

	api.RespondOK(w, final)
}

func (api *BatonAPI) handleGetPayload(w http.ResponseWriter, req *http.Request) {
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

		log.WithError(err).Error("could not read body of request from the beacon node")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Decode payload
	payload := new(common.AnchorGetPayloadRequest)
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(payload); err != nil {
		log.WithError(err).Warn("failed to decode getPayload request")
		api.RespondError(w, http.StatusBadRequest, "failed to decode anchor payload request")
		return
	}

	// Take time after the decoding, and add to logging
	decodeTime := time.Now().UTC()
	slotStartTimestamp := api.genesisInfo.Data.GenesisTime + (payload.Slot * common.SecondsPerSlot)
	msIntoSlot := decodeTime.UnixMilli() - int64((slotStartTimestamp * 1000))
	log = log.WithFields(logrus.Fields{
		"slot":                 payload.Slot,
		"slotEpochPos":         (payload.Slot % common.SlotsPerEpoch) + 1,
		"slotStartSec":         slotStartTimestamp,
		"msIntoSlot":           msIntoSlot,
		"timestampAfterDecode": decodeTime.UnixMilli(),
		"proposerIndex":        payload.ProposerIndex,
	})

	// Ensure the proposer index is expected
	api.proposerDutiesLock.RLock()
	slotDuty := api.proposerDutiesMap[payload.Slot]
	api.proposerDutiesLock.RUnlock()
	if slotDuty == nil {
		log.Warn("could not find slot duty")
	} else {
		log = log.WithField("feeRecipient", slotDuty.Entry.Message.FeeRecipient)
		if slotDuty.ValidatorIndex != payload.ProposerIndex {
			log.WithField("expectedProposerIndex", slotDuty.ValidatorIndex).Warn("not the expected proposer index")
			api.RespondError(w, http.StatusBadRequest, "not the expected proposer index")
			return
		}
	}

	// Get the proposer pubkey based on the validator index from the payload
	proposerPubkey, found := api.datastore.GetKnownValidatorPubkeyByIndex(payload.ProposerIndex)
	if !found {
		log.Errorf("could not find proposer pubkey for index %d", payload.ProposerIndex)
		api.RespondError(w, http.StatusBadRequest, "could not match proposer index to pubkey")
		return
	}

	// Add proposer pubkey to logs
	log = log.WithField("proposerPubkey", proposerPubkey.String())

	// Create a BLS pubkey from the hex pubkey
	//pk, err := boostTypes.HexToPubkey(proposerPubkey.String())
	_, err = boostTypes.HexToPubkey(proposerPubkey.String())
	if err != nil {
		log.WithError(err).Warn("could not convert pubkey to types.PublicKey")
		api.RespondError(w, http.StatusBadRequest, "could not convert pubkey to types.PublicKey")
		return
	}

	// Validate proposer signature
	// TODO: figure out how to use AnchorPayloadRequest to verify signature.
	// SEQ needs to keep it in byte form when signing(signatures).
	// 1.) define SEQ signatures
	// 2.) figure out how to pass header of HeaderInfo or if it's even needed 
	seqSig := payload.Signature 
	var header *common.HeaderInfo
	// can possibly use index of proposer to get the pubkey
	ok := VerifySignature(header, seqSig, proposerPubkey)
	if ok != nil {
		if api.ffLogInvalidSignaturePayload {
			txt, _ := json.Marshal(payload) //nolint:errchkjson
			fmt.Println("payload_invalid_sig_capella: ", string(txt), "pubkey:", proposerPubkey.String())
		}
		log.WithError(err).Warn("could not verify capella payload signature")
		api.RespondError(w, http.StatusBadRequest, "could not verify payload signature")
		return
	}

	// Log about received payload (with a valid proposer signature)
	log = log.WithField("timestampAfterSignatureVerify", time.Now().UTC().UnixMilli())
	log.Info("getPayload request received")

	var getPayloadResp common.AnchorGetPayloadResponse
	var msNeededForPublishing uint64

	// Save information about delivered payload
	//TODO: figure out Anchor Payload Response logic below
	defer func() {
		var bidTrace *common.BidTraceV3

		// we only need to get the bidtrace for one of the involved blocks.
		if getPayloadResp.ToBPayload != nil {
			bidTrace, err = api.redis.GetToBBidTrace(payload.Slot, proposerPubkey.String(), payload.BlockHash)
			if err != nil {
				log.WithError(err).Error("failed to get bidTrace for delivered payload from redis")
				return
			}
		} else {
			// Get RoB bid trace. There should be at least one entry in the response rob map.
			if len(getPayloadResp.RoBPayloads) == 0 {
				log.WithError(err).Error("could not get bidtrace because no tob or rob chain ids were found")
				return
			}

			var robFirstChainID string
			for k, _ := range getPayloadResp.RoBPayloads {
				robFirstChainID = k
			}
			bidTrace, err = api.redis.GetRoBBidTrace(payload.Slot, proposerPubkey.String(), payload.BlockHash, robFirstChainID)
			if err != nil {
				log.WithError(err).Error("failed to get bidTrace for delivered payload from redis")
				return
			}
		}

		// TODO: Review below delivered payload
		// note needs sharding
		err = api.db.SaveDeliveredAnchorPayload(&getPayloadResp, decodeTime, msNeededForPublishing)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"bidTrace": bidTrace,
				"payload":  payload,
			}).Error("failed to save delivered payload")
		}

		// TODO: Fix the below
		/*
			// Increment builder stats
			err = api.db.IncBlockBuilderStatsAfterGetPayload(bidTrace.BuilderPubkey.String())
			if err != nil {
				log.WithError(err).Error("failed to increment builder-stats after getPayload")
			}
		*/

		// TODO: We don't use optimistic blocks. Make sure this is ok.
		// Wait until optimistic blocks are complete.
		//api.optimisticBlocksWG.Wait()

		// TODO: What's this builder demotion thing for?
		// note: finds winning block, makes sure that block is sound and complete if so then success,
		// otherwise, block demotion case occurs. I feel like this is an important case to keep.
		// Check if there is a demotion for the winning block.
		//	_, err = api.db.GetBuilderDemotion(bidTrace)
		//	// If demotion not found, we are done!
		//	if errors.Is(err, sql.ErrNoRows) {
		//		log.Info("no demotion in getPayload, successful block proposal")
		//		return
		//	}
		//	if err != nil {
		//		log.WithError(err).Error("failed to read demotion table in getPayload")
		//		return
		//	}
		//	// Demotion found, update the demotion table with refund data.
		//	builderPubkey := bidTrace.BuilderPubkey.String()
		//	log = log.WithFields(logrus.Fields{
		//		"builderPubkey": builderPubkey,
		//		"slot":          bidTrace.Slot,
		//		"blockHash":     bidTrace.BlockHash,
		//	})
		//	log.Warn("demotion found in getPayload, inserting refund justification")

		// TODO: What is this signed beacon block for?
		// Prepare refund data.
		//	signedBeaconBlock := common.SignedBlindedBeaconBlockToBeaconBlock(payload, getPayloadResp)

		// TODO: Add the below when builder demotion is needed
		//  registrationEntry, err := api.db.GetValidatorRegistration(proposerPubkey.String())
		//  if err != nil {
		//  if errors.Is(err, sql.ErrNoRows) {
		//  		log.WithError(err).Error("no registration found for validator " + proposerPubkey.String())
		//  	} else {
		//  		log.WithError(err).Error("error reading validator registration")
		//  	}
		//  }
		//  var signedRegistration *boostTypes.SignedValidatorRegistration
		//  if registrationEntry != nil {
		//	  signedRegistration, err = registrationEntry.ToSignedValidatorRegistration()
		//	  if err != nil {
		//		  log.WithError(err).Error("error converting registration to signed registration")
		//	  }
		//  }
		//	err = api.db.UpdateBuilderDemotion(bidTrace, signedBeaconBlock, signedRegistration)
		//	if err != nil {
		//		log.WithFields(logrus.Fields{
		//			"errorWritingRefundToDB": true,
		//			"bidTrace":               bidTrace,
		//			"signedBeaconBlock":      signedBeaconBlock,
		//			"signedRegistration":     signedRegistration,
		//		}).WithError(err).Error("unable to update builder demotion with refund justification")
		//	}
	}()

	// @TODO: Continue fixing the below

	// Get the response - from Redis, Memcache or DB
	// note that recent mev-boost versions only send getPayload to relays that provided the bid
	var tobAnchorPayload *common.AnchorPayload
	var payloadWasFound bool

	tobAnchorPayload, err = api.datastore.GetGetToBPayloadResponse(log, payload.Slot, proposerPubkey.String(), payload.BlockHash)
	if err != nil || tobAnchorPayload == nil {
		log.WithError(err).Warn("failed getting execution payload (1/2)")
		time.Sleep(time.Duration(timeoutGetPayloadRetryMs) * time.Millisecond)

		// Try again
		tobAnchorPayload, err = api.datastore.GetGetToBPayloadResponse(log, payload.Slot, proposerPubkey.String(), payload.BlockHash)
		if err != nil || tobAnchorPayload == nil {
			// Still not found! Error out now.
			// TODO: Is the below still needed?
			/*
				if errors.Is(err, datastore.ErrExecutionPayloadNotFound) {
					// Couldn't find the execution payload, maybe it never was submitted to our relay! Check that now
					_, err := api.db.GetBlockSubmissionEntry(payload.Slot, proposerPubkey.String(), payload.BlockHash)
					if errors.Is(err, sql.ErrNoRows) {
						log.Warn("failed getting execution payload (2/2) - payload not found, block was never submitted to this relay")
						api.RespondError(w, http.StatusBadRequest, "no execution payload for this request - block was never seen by this relay")
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
	}

	robPayloads := map[string]*common.AnchorPayload{}
	for chainID, _ := range api.robChainIDs {
		robAnchorPayload, err := api.datastore.GetGetRoBPayloadResponse(log, payload.Slot, proposerPubkey.String(), payload.BlockHash, chainID)
		if err != nil || robAnchorPayload == nil {
			log.WithError(err).Warn("failed getting execution payload (1/2)")
			time.Sleep(time.Duration(timeoutGetPayloadRetryMs) * time.Millisecond)

			// Try again
			robAnchorPayload, err = api.datastore.GetGetRoBPayloadResponse(log, payload.Slot, proposerPubkey.String(), payload.BlockHash, chainID)
			if err != nil || robAnchorPayload == nil {
				// Still not found! Error out now.
				// TODO: Is the below still needed?
				/*
					if errors.Is(err, datastore.ErrExecutionPayloadNotFound) {
						// Couldn't find the execution payload, maybe it never was submitted to our relay! Check that now
						_, err := api.db.GetBlockSubmissionEntry(payload.Slot, proposerPubkey.String(), payload.BlockHash)
						if errors.Is(err, sql.ErrNoRows) {
							log.Warn("failed getting execution payload (2/2) - payload not found, block was never submitted to this relay")
							api.RespondError(w, http.StatusBadRequest, "no execution payload for this request - block was never seen by this relay")
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
		}
	}

	if payloadWasFound == false {
		log.Warn("no execution payloads were found for getPayload request")
		api.RespondError(w, http.StatusBadRequest, "no execution payloads were found for getPayload request")
		return
	}

	// Now we know this relay also has the payload
	log = log.WithField("timestampAfterLoadResponse", time.Now().UTC().UnixMilli())

	// Check whether getPayload has already been called -- TODO: do we need to allow multiple submissions of one blinded block?
	err = api.redis.CheckAndSetLastSlotAndHashDelivered(payload.Slot, payload.BlockHash)
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

	// Handle early/late requests
	if msIntoSlot < 0 {
		// Wait until slot start (t=0) if still in the future
		_msSinceSlotStart := time.Now().UTC().UnixMilli() - int64((slotStartTimestamp * 1000))
		if _msSinceSlotStart < 0 {
			delayMillis := _msSinceSlotStart * -1
			log = log.WithField("delayMillis", delayMillis)
			log.Info("waiting until slot start t=0")
			time.Sleep(time.Duration(delayMillis) * time.Millisecond)
		}
	} else if getPayloadRequestCutoffMs > 0 && msIntoSlot > int64(getPayloadRequestCutoffMs) {
		// Reject requests after cutoff time
		log.Warn("getPayload sent too late")
		api.RespondError(w, http.StatusBadRequest, fmt.Sprintf("sent too late - %d ms into slot", msIntoSlot))

		go func() {
			err := api.db.InsertTooLateGetPayload(payload.Slot, proposerPubkey.String(), payload.BlockHash, slotStartTimestamp, uint64(receivedAt.UnixMilli()), uint64(decodeTime.UnixMilli()), uint64(msIntoSlot))
			if err != nil {
				log.WithError(err).Error("failed to insert payload too late into db")
			}
		}()
		return
	}

	// TODO: Verify not needed. Looks to be Eth 2.0 related.
	/*
		// Check that ExecutionPayloadHeader fields (sent by the proposer) match our known ExecutionPayload
		err = EqExecutionPayloadToHeader(payload, getPayloadResp)
		if err != nil {
			log.WithError(err).Warn("ExecutionPayloadHeader not matching known ExecutionPayload")
			api.RespondError(w, http.StatusBadRequest, "invalid execution payload header")
			return
		}
	*/

	// @TODO: below prob not needed?
	/*
		// Publish the signed beacon block via beacon-node
		timeBeforePublish := time.Now().UTC().UnixMilli()
		log = log.WithField("timestampBeforePublishing", timeBeforePublish)
		signedBeaconBlock := common.SignedBlindedBeaconBlockToBeaconBlock(payload, getPayloadResp)
		code, err := api.beaconClient.PublishBlock(signedBeaconBlock) // errors are logged inside
		if err != nil || code != http.StatusOK {
			log.WithError(err).WithField("code", code).Error("failed to publish block")
			api.RespondError(w, http.StatusBadRequest, "failed to publish block")
			return
		}
		timeAfterPublish := time.Now().UTC().UnixMilli()
		msNeededForPublishing = uint64(timeAfterPublish - timeBeforePublish)
		log = log.WithField("timestampAfterPublishing", timeAfterPublish)
		log.WithField("msNeededForPublishing", msNeededForPublishing).Info("block published through beacon node")

		// give the beacon network some time to propagate the block
		time.Sleep(time.Duration(getPayloadResponseDelayMs) * time.Millisecond)
	*/

	// fill in rest of the payload response
	getPayloadResp.Slot = payload.Slot
	getPayloadResp.RoBPayloads = make(map[string]common.ExecutionPayload)

	if tobAnchorPayload != nil {
		tobExecPayload := common.ExecutionPayload{
			Transactions: tobAnchorPayload.Transactions,
		}
		getPayloadResp.ToBPayload = &tobExecPayload
	}

	for chainID, anchorPayload := range robPayloads {
		robChunkExecPayload := common.ExecutionPayload{
			Transactions: anchorPayload.Transactions,
		}
		getPayloadResp.RoBPayloads[chainID] = robChunkExecPayload
	}

	// respond to the HTTP request
	api.RespondOK(w, getPayloadResp)
	log = log.WithFields(logrus.Fields{
		"blockHash": payload.BlockHash,
	})
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

func (api *BatonAPI) getValidatorGasLimit(
	w http.ResponseWriter,
	log *logrus.Entry,
	slot uint64,
) (uint64, bool) {
	api.proposerDutiesLock.RLock()
	slotDuty := api.proposerDutiesMap[slot]
	api.proposerDutiesLock.RUnlock()

	if slotDuty == nil {
		logMsg := "could not find slot duty for slot " + strconv.FormatUint(slot, 10)
		log.Error(logMsg)
		api.Respond(w, http.StatusBadRequest, logMsg)
		return 0, false
		//note type conversion
	}

	return slotDuty.Entry.Message.GasLimit, true
}

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

func (api *BatonAPI) checkBuilderEntry(w http.ResponseWriter, log *logrus.Entry, builderPubkey phase0.BLSPubKey) (*blockBuilderCacheEntry, bool) {
	builderEntry, ok := api.blockBuildersCache[builderPubkey.String()]
	if !ok {
		log.Warnf("unable to read builder: %s from the builder cache, using low-prio and no collateral", builderPubkey.String())
		builderEntry = &blockBuilderCacheEntry{
			status: common.BuilderStatus{
				IsHighPrio:    false,
				IsOptimistic:  false,
				IsBlacklisted: false,
			},
			collateral: big.NewInt(0),
		}
	}

	if builderEntry.status.IsBlacklisted {
		log.Info("builder is blacklisted")
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		return builderEntry, false
	}

	// In case only high-prio requests are accepted, fail others
	if api.ffDisableLowPrioBuilders && !builderEntry.status.IsHighPrio {
		log.Info("rejecting low-prio builder (ff-disable-low-prio-builders)")
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		return builderEntry, false
	}

	return builderEntry, true
}

// Checks the quality of the TOB txs, if it is the txs expected in a TOB
func (api *BatonAPI) checkTobTxsStateInterference(txs []*types.Transaction, log *logrus.Entry) error {
	var wg sync.WaitGroup

	//// get traces
	tracerErrors := make([]error, len(txs))
	validationErrors := make([]error, len(txs))
	for i, tx := range txs {
		// some sanity checks
		if tx.To() == nil {
			return fmt.Errorf("contract creation cannot be a TOB tx")
		}
		if i == len(txs)-1 {
			continue
		}

		threadIndex := i
		threadTx := tx
		wg.Add(1)
		go func() {
			defer wg.Done()
			txTraces, err := api.getTraces(context.Background(), tracerOptions{
				log: log,
				tx:  threadTx,
			})
			if err != nil {
				tracerErrors[threadIndex] = fmt.Errorf("failed to get traces: %s", err.Error())
				return
			}

			res, err := api.TobTxChecks(&txTraces.Result)
			if err != nil {
				validationErrors[threadIndex] = fmt.Errorf("state interference checks failed with: %s", err.Error())
				return
			}
			if !res {
				validationErrors[threadIndex] = fmt.Errorf("not a valid tob tx")
				return
			}
		}()
	}

	wg.Wait()

	//if len(tracerErrors) > 0 {
	//	return fmt.Errorf("failed to get traces")
	//}
	for _, err := range tracerErrors {
		if err != nil {
			return fmt.Errorf("failed to get traces")
		}
	}

	for _, err := range validationErrors {
		if err != nil {
			return fmt.Errorf("not a valid tob tx")
		}
	}

	//if len(validationErrors) > 0 {
	//	return fmt.Errorf("not a valid tob tx")
	//}

	return nil
}

func (api *BatonAPI) handleGetTobGasReservations(w http.ResponseWriter, req *http.Request) {
	api.RespondOK(w, common.TobGasReservations)
}

type redisUpdateBidOpts struct {
	w                    http.ResponseWriter
	tx                   redis.Pipeliner
	log                  *logrus.Entry
	cancellationsEnabled bool
	receivedAt           time.Time
	floorBidValue        *big.Int
	payload              *common.BuilderSubmitBlockRequest
}

/*
func (api *BatonAPI) updateRedisBid(opts redisUpdateBidOpts) (*datastore.SaveBidAndUpdateTopBidResponse, *common.GetPayloadResponse, bool) {
	// Prepare the response data
	getHeaderResponse, err := common.BuildGetHeaderResponse(opts.payload, api.blsSk, api.publicKey, api.opts.EthNetDetails.DomainBuilder)
	if err != nil {
		opts.log.WithError(err).Error("could not sign builder bid")
		api.RespondError(opts.w, http.StatusBadRequest, err.Error())
		return nil, nil, false
	}

	getPayloadResponse, err := common.BuildGetPayloadResponse(opts.payload)
	if err != nil {
		opts.log.WithError(err).Error("could not build getPayload response")
		api.RespondError(opts.w, http.StatusBadRequest, err.Error())
		return nil, nil, false
	}

	bidTrace := common.BidTraceV2{
		BidTrace:    *opts.payload.Message(),
		BlockNumber: opts.payload.BlockNumber(),
		NumTx:       uint64(opts.payload.NumTx()),
	}

	//
	// Save to Redis
	//
	updateBidResult, err := api.redis.SaveBidAndUpdateTopBid(context.Background(), opts.tx, &bidTrace, opts.payload, getPayloadResponse, getHeaderResponse, opts.receivedAt, opts.cancellationsEnabled, opts.floorBidValue)
	if err != nil {
		opts.log.WithError(err).Error("could not save bid and update top bids")
		api.RespondError(opts.w, http.StatusInternalServerError, "failed saving and updating bid")
		return nil, nil, false
	}
	return &updateBidResult, getPayloadResponse, true
}
*/

// This method used for both ToB and for RoB.
func (api *BatonAPI) handleSubmitNewBlockRequest(w http.ResponseWriter, req *http.Request) {
	var pf common.Profile
	var prevTime, nextTime time.Time
	headSlot := api.headSlot.Load()
	receivedAt := time.Now().UTC()
	prevTime = receivedAt

	log := api.log.WithFields(logrus.Fields{
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

	blockReq := common.NewSubmitNewBlockRequest()
	err = blockReq.FromJSON(payloadBytes)
	if err != nil {
		log.WithError(err).Warn("could not parse payload into SubmitNewBlockRequest")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	nextTime = time.Now().UTC()
	pf.Decode = uint64(nextTime.Sub(prevTime).Microseconds())
	prevTime = nextTime

	isLargeRequest := len(payloadBytes) > fastTrackPayloadSizeLimit
	slot := blockReq.Slot()

	if slot < headSlot {
		log.Error("TOB tx request for past slot!")
		api.Respond(w, http.StatusBadRequest, "Submitted TOB tx request for past slot!")
		return
	}

	// We only allow bidding for block 1 slot prior
	if (slot - headSlot) > 1 {
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

	if len(blockReq.Txs()) > common.MaxTobTxs+1 {
		msg := fmt.Sprintf("we support only %d txs on the TOB currently, got %d", common.MaxTobTxs, len(blockReq.Txs()))
		log.WithError(err).Info(msg)
		api.Respond(w, http.StatusBadRequest, msg)
		return
	}

	// ToB will have varying chain id in txs, RoB will have uniform
	// Also verifies len(txs) >= 2
	// TODO: Might need fixing
	isToB, err := api.checkBlockRequestIsToB(&blockReq)
	if err != nil {
		log.WithError(err).Info(err.Error())
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	// TODO: Fix me SOON
	// chainID, err := blockReq.FirstChainID()
	chainID := ""

	builderPubkey := blockReq.BuilderPubkey()
	builderEntry, ok := api.checkBuilderEntry(w, log, phase0.BLSPubKey(builderPubkey))
	if !ok {
		log.WithError(err).Info("builder entry check failed")
		api.RespondError(w, http.StatusBadRequest, "builder entry check failed")
		return
	}

	log = log.WithFields(logrus.Fields{
		"builderIsHighPrio":     builderEntry.status.IsHighPrio,
		"timestampAfterChecks1": time.Now().UTC().UnixMilli(),
	})

	// Note this also validates slot validity
	gasLimit, ok := api.getValidatorGasLimit(w, log, slot)
	if !ok {
		log.WithError(err).Info("fee recipient check failed")
		api.RespondError(w, http.StatusBadRequest, "fee recipient check failed")
		return
	}

	var topBidValue *big.Int
	tx := api.redis.NewTxPipeline()
	bidIsTopBid := false
	// @TODO fill me in later
	var value *big.Int
	if isToB {
		topBidValue, err = api.redis.GetTopToBBidValue(context.Background(), tx, blockReq.Slot(), blockReq.ParentHash(), blockReq.ProposerPubKey())
	} else {
		// TODO: resolved when chainID is fixed
		topBidValue, err = api.redis.GetTopRoBBidValue(context.Background(), tx, blockReq.Slot(), blockReq.ParentHash(), blockReq.ProposerPubKey(), chainID)
	}
	if err != nil {
		log.WithError(err).Error("failed to get top bid value from redis")
		api.RespondError(w, http.StatusBadRequest, "failed to get top bid value from redis")
		return
	} else {
		// TODO: value needs to be fixed
		bidIsTopBid = value.Cmp(topBidValue) == 1
		log = log.WithFields(logrus.Fields{
			"topBidValue":    topBidValue.String(),
			"newBidIsTopBid": bidIsTopBid,
		})
	}

	var validErr error
	log = log.WithFields(logrus.Fields{
		"timestampAfterDecoding": time.Now().UTC().UnixMilli(),
		"slot":                   blockReq.Slot,
		"numTx":                  len(blockReq.Txs()),
		"parentHash":             blockReq.ParentHash,
		"blockHash":              blockReq.BlockHash,
		"builderPubkey":          blockReq.BuilderPubKey.String(),
		"proposerPubkey":         blockReq.ProposerPubKey().String(),
		"proposerPayment":        blockReq.ProposerPayment,
		"signature":              blockReq.Signature,
		"isLargeRequest":         isLargeRequest,
		"isToB":                  isToB,
	})

	// Build the header and payload for this block request.
	getHeader, err := buildHeader(&blockReq)
	if err != nil {
		log.WithError(err).Warn("failed to build header")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	getPayload, err := buildPayload(&blockReq)
	if err != nil {
		log.WithError(err).Warn("failed to build payload")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Perform block simulation which makes sure all txs are valid before we allow it to participate in the auction
	simStartTime := time.Now().UTC()
	var reqErr error

	simResultC := make(chan *blockSimResult, 1)

	// Once we process the sim, then we want to save the results to the redis database.
	var eligibleAt time.Time // will be set once the bid is ready

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
			gasLimit,
			simResult.requestErr,
			simResult.validationErr,
			receivedAt,
			eligibleAt,
			simResult.wasSimulated,
			savePayloadToDatabase,
			pf,
			simResult.optimisticSubmission,
			isToB,
			chainID)
		if err != nil {
			log.WithError(err).WithField("payload", blockReq).Error("saving builder block submission to database failed")
			return
		}

		err = api.db.UpsertBlockBuilderEntryAfterSubmission2(submissionEntry, isToB, chainID, simResult.validationErr != nil)
		if err != nil {
			log.WithError(err).Error("failed to upsert block-builder-entry")
		}
	}()

	// Simulate the block synchronously. If it simulates successfully, then we know we have a valid chunk.
	// @TODO: add logic to group txs together(ex: 2 polygon txs and 2 optimism txs are grouped separately and then we simulate the txs in those groups)
	gasUsed, reqErr, validErr := api.simulateBlock(context.Background(), &blockReq, log)
	// @TODO: figure out exact gas limit later
	if gasUsed != 0 && gasUsed > gasLimit {
		errMsg := "simulation failed due to gas limit exceeded, gas_used [" + strconv.FormatUint(gasUsed, 10) + "], gas_limit [" + strconv.FormatUint(gasLimit, 10) + "]"
		validErr = errors.New(errMsg)
	}

	simResultC <- &blockSimResult{reqErr == nil, false, reqErr, validErr}
	if reqErr != nil {
		log.WithError(err).Warn("could not simulate TOB txs")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if validErr != nil {
		log.WithError(validErr).Warn("validation error during TOB txs simulation")
		api.RespondError(w, http.StatusBadRequest, validErr.Error())
		return
	}

	simulationDuration := time.Since(simStartTime).Microseconds()

	blockNumberJson, err := json.Marshal(blockReq.BlockNumber())
	if err != nil {
		log.WithError(err).Warn("couldn't marshal block number")
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	blockNumberJsonStr := string(blockNumberJson[:])

	trace := common.BidTraceV3{
		Slot:            slot,
		IsTob:           isToB,
		ChainID:         chainID,
		ParentHash:      blockReq.ParentHash().String(),
		BlockHash:       blockReq.BlockHash().String(),
		BuilderPubkey:   blockReq.BuilderPubkey().String(),
		ProposerPubkey:  blockReq.ProposerPubKey().String(),
		ProposerPayment: blockReq.ProposerPaymentAsStr(),
		GasLimit:        gasLimit,
		GasUsed:         gasUsed,
		Value:           value.Uint64(),
		BlockNumber:     blockNumberJsonStr,
		NumTx:           uint64(len(blockReq.Txs())),
	}

	// Save the header and payloads for this block request to Redis.
	// The auction is processed here where the best bid for the ToB or RoB namespace is saved to the database.
	var updateBidResult datastore.SaveBidAndUpdateTopBidResponse
	if isToB {
		// ToB case
		updateBidResult, err = api.redis.SaveToBBidAndUpdateTopBid(context.Background(), tx, &blockReq,
			getPayload, &getHeader, receivedAt, false, nil, &trace)
		if err != nil {
			log.WithError(err).Error("could not save bid and update top bids")
			api.RespondError(w, http.StatusInternalServerError, "failed saving and updating bid")
			return
		}
	} else {
		//RoB case
		updateBidResult, err = api.redis.SaveRoBBidAndUpdateTopBid(context.Background(), tx, &blockReq,
			getPayload, &getHeader, chainID, receivedAt, false, nil, &trace)
		if err != nil {
			log.WithError(err).Error("could not save bid and update top bids for RoB")
			api.RespondError(w, http.StatusInternalServerError, "failed saving and updating bid")
			return
		}
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
						blockReq.ProposerPubKey().String(),
						blockReq.BlockHash().String(),
						getPayload)
				} else {
					err = api.memcached.SaveRoBAnchorPayload(blockReq.Slot(),
						blockReq.ProposerPubKey().String(),
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
			api.robChainIDs[chainID] = struct{}{}
		}
	}

	defer func() {
		totalDuration := time.Since(receivedAt).Microseconds()
		txHashList := []string{}
		// TODO: figure logic out or if this is needed
		for _, tx := range blockReq.Txs() {
			txHashList = append(txHashList, string(tx.Bytes()))
		}
		txHashes := strings.Join(txHashList, ",")

		if isToB {
			err := api.db.InsertToBSubmitProfile(blockReq.Slot(), blockReq.ParentHash(), txHashes, uint64(simulationDuration), 0, uint64(totalDuration))
			if err != nil {
				log.WithError(err).Error("failed to insert tob submit profile into db")
			}
		} else {
			err := api.db.InsertRoBSubmitProfile(blockReq.Slot(), blockReq.ParentHash(), txHashes, uint64(simulationDuration), 0, uint64(totalDuration))
			if err != nil {
				log.WithError(err).Error("failed to insert tob submit profile into db")
			}
		}
	}()

	// FOR TOMORROW: go to commented out SubmitToB function as well as look at GetHeader() and GetPayload() functions.
	// this is on hold: think of special cases like in the original function handleSubmitNewTobTxs
	// respond ok if all passes
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
		var hash boostTypes.Hash
		err = hash.UnmarshalText([]byte(args.Get("block_hash")))
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
	for i, payload := range deliveredPayloads {
		response[i] = database.DeliveredPayloadEntryToBidTraceV2JSON(payload)
	}

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
		var hash boostTypes.Hash
		err = hash.UnmarshalText([]byte(args.Get("block_hash")))
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

	var pk boostTypes.PublicKey
	err := pk.UnmarshalText([]byte(pkStr))
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

// TODO: Fix this once we figure out hypersdk txs
// Check if block request is ToB
func (api *BatonAPI) checkBlockRequestIsToB(req *common.SubmitNewBlockRequest) (bool, error) {
	if len(req.Txs) == 0 {
		return false, errors.New("block request has no transactions provided")
	}

	if len(req.Txs) == 1 {
		return false, errors.New("block request needs more than one transaction provided")
	}

	// RoBs have txs with all the same chain id, ToBs has more than chain id
	firstChainID, err := req.FirstChainID()
	if err != nil {
		return false, err
	}

	for i := 0; i < len(req.Txs); i++ {
		for _, action := range req.Txs[i].Actions {
			if seqMsg, ok := action.(*actions.SequencerMsg); ok {
				if string(seqMsg.ChainId) != firstChainID {
					return false, nil
				}
			} else {
				return false, errors.New("checkBlockRequestIsToB tx is not sequencer message")
			}
		}
	}

	return true, nil
}
