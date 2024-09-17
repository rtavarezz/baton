package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	apiv1 "github.com/attestantio/go-builder-client/api/v1"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	srpc "github.com/AnomalyFi/nodekit-seq/rpc"
	// "github.com/AnomalyFi/hypersdk/state"
	"github.com/alicebob/miniredis/v2"
	eth "github.com/ethereum/go-ethereum/common"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/flashbots/mev-boost-relay/beaconclient"
	"github.com/flashbots/mev-boost-relay/common"
	"github.com/flashbots/mev-boost-relay/database"
	"github.com/flashbots/mev-boost-relay/datastore"
	"github.com/stretchr/testify/require"
)

const (
	testGasLimit        = uint64(30000000)
	testSlot            = uint64(42)
	testParentHash      = "0xbd3291854dc822b7ec585925cda0e18f06af28fa2886e15f52d52dd4b6f94ed6"
	testWithdrawalsRoot = "0x7f6d156912a4cb1e74ee37e492ad883f7f7ac856d987b3228b517e490aa0189e"
	testPrevRandao      = "0x9962816e9d0a39fd4c80935338a741dc916d1545694e41eb5a505e1a3098f9e4"
	testBuilderPubkey   = "0xfa1ed37c3553d0ce1e9349b2c5063cf6e394d231c8d3e0df75e9462257c081543086109ffddaacc0aa76f33dc9661c83"
	testProposerKey     = "0xda1ed37c3553d0ce1e9349b2c5063cf6e394d231c8d3e0df75e9462257c081543086109ffddaacc0aa76f33dc9661c83"
)

var (
	builderSigningDomain = phase0.Domain([32]byte{0, 0, 0, 1, 245, 165, 253, 66, 209, 106, 32, 48, 39, 152, 239, 110, 211, 9, 151, 155, 67, 0, 61, 35, 32, 217, 240, 232, 234, 152, 49, 169})
	testAddress          = eth.Address([20]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19})
	testAddress2         = eth.Address([20]byte{1, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19})
)

type testBackend struct {
	t         require.TestingT
	relay     *BatonAPI
	datastore *datastore.Datastore
	redis     *datastore.RedisCache
}

func newTestBackend(t *testing.T, numBeaconNodes int, network string) *testBackend {
	redisClient, err := miniredis.Run()
	require.NoError(t, err)

	redisCache, err := datastore.NewRedisCache("", redisClient.Addr(), "")
	require.NoError(t, err)

	db := database.MockDB{
		ExecPayloads:     map[string]*database.ExecutionPayloadEntry{},
		BlockSubmissions: map[string]*database.BuilderBlockSubmissionEntry{},
		Builders:         map[string]*database.BlockBuilderEntry{},
		Demotions:        map[string]bool{},
		IncludedTobTxs:   map[string][]*database.IncludedTobTxEntry{},
		TobSubmitProfile: map[string]*database.ToBSubmitProfileEntry{},
		RobSubmitProfile: map[string]*database.RoBSubmitProfileEntry{},
	}

	ds, err := datastore.NewDatastore(redisCache, nil, db)
	require.NoError(t, err)

	sk, _, err := bls.GenerateNewKeypair()
	require.NoError(t, err)

	if network == common.EthNetworkCustom {
		t.Setenv("GENESIS_FORK_VERSION", types.GenesisForkVersionMainnet)
		t.Setenv("GENESIS_VALIDATORS_ROOT", types.GenesisValidatorsRootMainnet)
		t.Setenv("BELLATRIX_FORK_VERSION", types.BellatrixForkVersionMainnet)
		t.Setenv("CAPELLA_FORK_VERSION", common.CapellaForkVersionMainnet)
		t.Setenv("DENEB_FORK_VERSION", common.DenebForkVersionMainnet)
	}

	mainnetDetails, err := common.NewEthNetworkDetails(network)
	require.NoError(t, err)

	opts := BatonAPIOpts{
		Log:             common.TestLog,
		ListenAddr:      "localhost:12345",
		BeaconClient:    &beaconclient.MultiBeaconClient{},
		Datastore:       ds,
		Redis:           redisCache,
		DB:              db,
		EthNetDetails:   *mainnetDetails,
		SecretKey:       sk,
		ProposerAPI:     true,
		BlockBuilderAPI: true,
		DataAPI:         true,
		InternalAPI:     true,
		mockMode:        true,
	}

	relay, err := NewBatonAPI(opts)
	require.NoError(t, err)

	relay.genesisInfo = &beaconclient.GetGenesisResponse{
		Data: beaconclient.GetGenesisResponseData{
			GenesisTime: 1606824023,
		},
	}

	backend := testBackend{
		t:         t,
		relay:     relay,
		datastore: ds,
		redis:     redisCache,
	}
	return &backend
}

func (be *testBackend) GetRedis() *datastore.RedisCache {
	return be.redis
}

func (be *testBackend) requestBytes(method, path string, payload []byte, headers map[string]string) *httptest.ResponseRecorder {
	var req *http.Request
	var err error

	req, err = http.NewRequest(method, path, bytes.NewReader(payload))
	require.NoError(be.t, err)

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// lfg
	rr := httptest.NewRecorder()
	be.relay.getRouter().ServeHTTP(rr, req)
	return rr
}

func (be *testBackend) request(method, path string, payload any) *httptest.ResponseRecorder {
	var req *http.Request
	var err error

	if payload == nil {
		req, err = http.NewRequest(method, path, bytes.NewReader(nil))
	} else {
		payloadBytes, err2 := json.Marshal(payload)
		require.NoError(be.t, err2)
		req, err = http.NewRequest(method, path, bytes.NewReader(payloadBytes))
	}
	require.NoError(be.t, err)

	// lfg
	rr := httptest.NewRecorder()
	be.relay.getRouter().ServeHTTP(rr, req)
	return rr
}

func (be *testBackend) requestWithUA(method, path, userAgent string, payload any) *httptest.ResponseRecorder {
	var req *http.Request
	var err error

	if payload == nil {
		req, err = http.NewRequest(method, path, bytes.NewReader(nil))
	} else {
		payloadBytes, err2 := json.Marshal(payload)
		require.NoError(be.t, err2)
		req, err = http.NewRequest(method, path, bytes.NewReader(payloadBytes))
	}
	req.Header.Set("User-Agent", userAgent)

	require.NoError(be.t, err)
	rr := httptest.NewRecorder()
	be.relay.getRouter().ServeHTTP(rr, req)
	return rr
}

// func generateSignedValidatorRegistration(sk *bls.SecretKey, feeRecipient types.Address, timestamp uint64) (*types.SignedValidatorRegistration, error) {
// 	var err error
// 	if sk == nil {
// 		sk, _, err = bls.GenerateNewKeypair()
// 		if err != nil {
// 			return nil, err
// 		}
// 	}

// 	blsPubKey, _ := bls.PublicKeyFromSecretKey(sk)

// 	var pubKey types.PublicKey
// 	err = pubKey.FromSlice(bls.PublicKeyToBytes(blsPubKey))
// 	if err != nil {
// 		return nil, err
// 	}
// 	msg := &types.RegisterValidatorRequestMessage{
// 		FeeRecipient: feeRecipient,
// 		Timestamp:    timestamp,
// 		Pubkey:       pubKey,
// 		GasLimit:     278234191203,
// 	}

// 	sig, err := types.SignMessage(msg, builderSigningDomain, sk)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &types.SignedValidatorRegistration{
// 		Message:   msg,
// 		Signature: sig,
// 	}, nil
// }

func TestWebserver(t *testing.T) {
	t.Run("errors when webserver is already existing", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)
		backend.relay.srvStarted.Store(true)
		err := backend.relay.StartServer()
		require.Error(t, err)
	})
}

func TestWebserverRootHandler(t *testing.T) {
	backend := newTestBackend(t, 1, common.EthNetworkMainnet)
	rr := backend.request(http.MethodGet, "/", nil)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestStatus(t *testing.T) {
	backend := newTestBackend(t, 1, common.EthNetworkMainnet)
	path := "/eth/v1/builder/status"
	rr := backend.request(http.MethodGet, path, apiv1.ValidatorRegistration{})
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestLivez(t *testing.T) {
	backend := newTestBackend(t, 1, common.EthNetworkMainnet)
	path := "/livez"
	rr := backend.request(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "{\"message\":\"live\"}\n", rr.Body.String())
}

func TestRegisterValidator(t *testing.T) {
	path := "/eth/v1/builder/validators"

	// t.Run("Normal function", func(t *testing.T) {
	// 	backend := newTestBackend(t, 1)
	// 	pubkeyHex := common.ValidPayloadRegisterValidator.Message.Pubkey.PubkeyHex()
	// 	index := uint64(17)
	// 	err := backend.redis.SetKnownValidator(pubkeyHex, index)
	// 	require.NoError(t, err)

	// 	// Update datastore
	// 	_, err = backend.datastore.RefreshKnownValidators()
	// 	require.NoError(t, err)
	// 	require.True(t, backend.datastore.IsKnownValidator(pubkeyHex))
	// 	pkH, ok := backend.datastore.GetKnownValidatorPubkeyByIndex(index)
	// 	require.True(t, ok)
	// 	require.Equal(t, pubkeyHex, pkH)

	// 	payload := []types.SignedValidatorRegistration{common.ValidPayloadRegisterValidator}
	// 	rr := backend.request(http.MethodPost, path, payload)
	// 	require.Equal(t, http.StatusOK, rr.Code)
	// 	time.Sleep(20 * time.Millisecond) // registrations are processed asynchronously

	// 	isKnown := backend.datastore.IsKnownValidator(pubkeyHex)
	// 	require.True(t, isKnown)
	// })

	t.Run("not a known validator", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)

		rr := backend.request(http.MethodPost, path, []apiv1.SignedValidatorRegistration{})
		require.Equal(t, http.StatusBadRequest, rr.Code)
	})

	// t.Run("Reject registration for >10sec into the future", func(t *testing.T) {
	// 	backend := newTestBackend(t, 1)

	// 	// Allow +10 sec
	// 	td := uint64(time.Now().Unix())
	// 	payload, err := generateSignedValidatorRegistration(nil, types.Address{1}, td+10)
	// 	require.NoError(t, err)
	// 	err = backend.redis.SetKnownValidator(payload.Message.Pubkey.PubkeyHex(), 1)
	// 	require.NoError(t, err)
	// 	_, err = backend.datastore.RefreshKnownValidators()
	// 	require.NoError(t, err)

	// 	rr := backend.request(http.MethodPost, path, []types.SignedValidatorRegistration{*payload})
	// 	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	// 	// Disallow +11 sec
	// 	td = uint64(time.Now().Unix())
	// 	payload, err = generateSignedValidatorRegistration(nil, types.Address{1}, td+12)
	// 	require.NoError(t, err)
	// 	err = backend.redis.SetKnownValidator(payload.Message.Pubkey.PubkeyHex(), 1)
	// 	require.NoError(t, err)
	// 	_, err = backend.datastore.RefreshKnownValidators()
	// 	require.NoError(t, err)

	// 	rr = backend.request(http.MethodPost, path, []types.SignedValidatorRegistration{*payload})
	// 	require.Equal(t, http.StatusBadRequest, rr.Code)
	// 	require.Contains(t, rr.Body.String(), "timestamp too far in the future")
	// })
}

func TestGetHeader(t *testing.T) {
	// Setup backend with headSlot and genesisTime
	backend := newTestBackend(t, 1, common.EthNetworkMainnet)
	backend.relay.genesisInfo = &beaconclient.GetGenesisResponse{
		Data: beaconclient.GetGenesisResponseData{
			GenesisTime: uint64(time.Now().UTC().Unix()),
		},
	}

	slot := uint64(1)
	backend.relay.headSlot.Store(slot)

	parentHash := eth.HexToHash("0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747")
	proposerPubkey := "0x6ae5932d1e248d987d51b58665b81848814202d7b23b343d20f2a167d12f07dcb01ca41c42fdd60b7fca9c4b90890792"

	//bid, err := api.redis.GetBestRoBBid(slot, parentHashHex, proposerPubkeyHex, chainID)
	//bid, err := api.redis.GetBestToBBid(slot, parentHashHex, proposerPubkeyHex)

	t.Run("Run valid base case, just tob", func(t *testing.T) {
    redis := backend.GetRedis()

    // Populate redis cache with expected headers
    redis.
    //bid, err := api.redis.GetBestRoBBid(slot, parentHashHex, proposerPubkeyHex, chainID)

		rr := httptest.NewRecorder()

		requestPath := fmt.Sprintf("/eth/v1/builder/header/%s/%s/%s", strconv.FormatUint(uint64(1), 10), parentHash, proposerPubkey)
		require.Equal(t, "/eth/v1/builder/header/1/0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747/0x6ae5932d1e248d987d51b58665b81848814202d7b23b343d20f2a167d12f07dcb01ca41c42fdd60b7fca9c4b90890792", requestPath)

		httpReq := httptest.NewRequest(http.MethodGet, requestPath, nil)
		backend.relay.getRouter().ServeHTTP(rr, httpReq)

		require.Equal(t, http.StatusOK, rr.Code)
	})
}

func createBackendHelper(t *testing.T) *testBackend {
	backend := newTestBackend(t, 1, common.EthNetworkMainnet)
	backend.relay.genesisInfo = &beaconclient.GetGenesisResponse{
		Data: beaconclient.GetGenesisResponseData{
			GenesisTime: uint64(time.Now().UTC().Unix()),
		},
	}
	return backend
}

// @TODO: Finish/fix handle test function below. Can either hard code which is copying most logic of actual function
// or create a fake request to call the function which is the approach taken below.
func TestHandleSubmitNewBlockRequest(t *testing.T) {
	//logger := logrus.New()
	//logEntry := logrus.NewEntry(logger)
	slot := uint64(1)
	var err error

	// Build hypersdk registry
	var cli = srpc.Parser{}
	_, _ = cli.Registry()

	// Build test builder keys
	testBuilderSecretKey, err := bls.GenerateRandomSecretKey()
	require.NoError(t, err)
	testBuilderPublicKey, err := bls.PublicKeyFromSecretKey(testBuilderSecretKey)
	require.NoError(t, err)

	// Build test proposer keys
	testProposerSecretKey, err := bls.GenerateRandomSecretKey()
	require.NoError(t, err)
	testProposerPublicKey, err := bls.PublicKeyFromSecretKey(testProposerSecretKey)
	require.NoError(t, err)

	// Default test block for use in tests
	// Do not overwrite! Make your own copy for each test
	defaultOpts := CreateTestBlockSubmissionOpts{
		Slot:           slot,
		ParentHash:     "0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747",
		BuilderPubkey:  *testBuilderPublicKey,
		ProposerPubkey: *testProposerPublicKey,
		IsToB:          false,
		robChainIndex:  0,
		numTxs:         1,
	}
	defaultBlockValue := uint64(2)
	defaultBlockReq, _, _, err := CreateTestChunkSubmission(t, defaultBlockValue, &defaultOpts)
	require.NoError(t, err)

	// note: mock db to set expected header from CreateTestChunkSubmission
	// TODO: look at anchor unit test for test improvements

	// Helper for processing block requests to the backend. Returns the status code of the request.
	processBlockRequest := func(backend *testBackend, blockReq *common.SubmitNewBlockRequest) int {
		// marshal the req body
		requestBodyBytes, err := json.Marshal(blockReq)
		require.NoError(t, err)

		// new HTTP req
		httpReq := httptest.NewRequest(http.MethodPost, "/relay/v1/builder/submit", bytes.NewReader(requestBodyBytes))
		httpReq.Header.Set("Content-Type", "application/json")

		// Capture the response
		rr := httptest.NewRecorder()

		// Process the request
		backend.relay.getRouter().ServeHTTP(rr, httpReq)

		return rr.Code
	}

	t.Run("Run valid base case, just rob", func(t *testing.T) {
		backend := createBackendHelper(t)

		// TODO: CHANGE ME LATER
		justRoBBlock := defaultBlockReq

		rrCode := processBlockRequest(backend, justRoBBlock)
		require.Equal(t, http.StatusOK, rrCode)
	})

	t.Run("Run valid base case, just tob", func(t *testing.T) {
		backend := createBackendHelper(t)
		rrCode := processBlockRequest(backend, defaultBlockReq)
		require.Equal(t, http.StatusOK, rrCode)
	})
}

// TODO: fix me soon
func TestBuilderApiGetValidators(t *testing.T) {
	path := "/relay/v1/builder/validators"

	backend := newTestBackend(t, 1, common.EthNetworkMainnet)
	duties := []common.BuilderGetValidatorsResponseEntry{
		{
			Slot:  1,
			Entry: &apiv1.SignedValidatorRegistration{},
		},
	}
	responseBytes, err := json.Marshal(duties)
	require.NoError(t, err)
	backend.relay.proposerDutiesResponse = &responseBytes

	rr := backend.request(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rr.Code)

	resp := []common.BuilderGetValidatorsResponseEntry{}
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp))
	require.Equal(t, uint64(1), resp[0].Slot)
	require.Equal(t, apiv1.ValidatorRegistration{}, *resp[0].Entry)
}

func TestDataApiGetDataProposerPayloadDelivered(t *testing.T) {
	path := "/relay/v1/data/bidtraces/proposer_payload_delivered"

	t.Run("Accept valid block_hash", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)

		validBlockHash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		rr := backend.request(http.MethodGet, path+"?block_hash="+validBlockHash, nil)
		require.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Reject invalid block_hash", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)

		invalidBlockHashes := []string{
			// One character too long.
			"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaab",
			// One character too short.
			"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			// Missing the 0x prefix.
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			// Has an invalid hex character ('z' at the end).
			"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaz",
		}

		for _, invalidBlockHash := range invalidBlockHashes {
			rr := backend.request(http.MethodGet, path+"?block_hash="+invalidBlockHash, nil)
			t.Log(invalidBlockHash)
			require.Equal(t, http.StatusBadRequest, rr.Code)
			require.Contains(t, rr.Body.String(), "invalid block_hash argument")
		}
	})
}

// @TODO: Fix me
func TestCheckSubmissionFeeRecipient(t *testing.T) {
	/*
		cases := []struct {
			description    string
			slotDuty       *common.BuilderGetValidatorsResponseEntry
			payload        *common.BuilderSubmitBlockRequest
			expectOk       bool
			expectGasLimit uint64
		}{
			{
				description: "success",
				slotDuty: &common.BuilderGetValidatorsResponseEntry{
					Entry: &types.SignedValidatorRegistration{
						Message: &types.RegisterValidatorRequestMessage{
							FeeRecipient: testAddress,
							GasLimit:     testGasLimit,
						},
					},
				},
				payload: &common.BuilderSubmitBlockRequest{
					Capella: &builderCapella.SubmitBlockRequest{
						Message: &v1.BidTrace{
							Slot:                 testSlot,
							ProposerFeeRecipient: bellatrix.ExecutionAddress(testAddress),
						},
					},
				},
				expectOk:       true,
				expectGasLimit: testGasLimit,
			},
			{
				description: "failure_nil_slot_duty",
				slotDuty:    nil,
				payload: &common.BuilderSubmitBlockRequest{
					Capella: &builderCapella.SubmitBlockRequest{
						Message: &v1.BidTrace{
							Slot: testSlot,
						},
					},
				},
				expectOk:       false,
				expectGasLimit: 0,
			},
			{
				description: "failure_diff_fee_recipient",
				slotDuty: &common.BuilderGetValidatorsResponseEntry{
					Entry: &types.SignedValidatorRegistration{
						Message: &types.RegisterValidatorRequestMessage{
							FeeRecipient: testAddress,
							GasLimit:     testGasLimit,
						},
					},
				},
				payload: &common.BuilderSubmitBlockRequest{
					Capella: &builderCapella.SubmitBlockRequest{
						Message: &v1.BidTrace{
							Slot:                 testSlot,
							ProposerFeeRecipient: bellatrix.ExecutionAddress(testAddress2),
						},
					},
				},
				expectOk:       false,
				expectGasLimit: 0,
			},
		}
		for _, tc := range cases {
			t.Run(tc.description, func(t *testing.T) {
				_, _, backend := startTestBackend(t, common.EthNetworkMainnet)
				backend.relay.proposerDutiesLock.RLock()
				backend.relay.proposerDutiesMap[tc.payload.Slot()] = tc.slotDuty
				backend.relay.proposerDutiesLock.RUnlock()

				w := httptest.NewRecorder()
				logger := logrus.New()
				log := logrus.NewEntry(logger)
				gasLimit, ok := backend.relay.checkSubmissionFeeRecipient(w, log, tc.payload)
				require.Equal(t, tc.expectGasLimit, gasLimit)
				require.Equal(t, tc.expectOk, ok)
			})
		}
	*/
}

// TODO: Fix me later
func TestCheckSubmissionSlotDetails(t *testing.T) {
	/*
		cases := []struct {
			description string
			payload     *common.BuilderSubmitBlockRequest
			expectOk    bool
		}{
			{
				description: "success",
				payload: &common.BuilderSubmitBlockRequest{
					Capella: &builderCapella.SubmitBlockRequest{
						ExecutionPayload: &capella.ExecutionPayload{
							Timestamp: testSlot * common.SecondsPerSlot,
						},
						Message: &v1.BidTrace{
							Slot: testSlot,
						},
					},
				},
				expectOk: true,
			},
			{
				description: "failure_nil_capella",
				payload: &common.BuilderSubmitBlockRequest{
					Capella: nil, // nil to cause error
				},
				expectOk: false,
			},
			{
				description: "failure_past_slot",
				payload: &common.BuilderSubmitBlockRequest{
					Capella: &builderCapella.SubmitBlockRequest{
						Message: &v1.BidTrace{
							Slot: testSlot - 1, // use old slot to cause error
						},
					},
				},
				expectOk: false,
			},
			{
				description: "failure_wrong_timestamp",
				payload: &common.BuilderSubmitBlockRequest{
					Capella: &builderCapella.SubmitBlockRequest{
						ExecutionPayload: &capella.ExecutionPayload{
							Timestamp: testSlot*common.SecondsPerSlot - 1, // use wrong timestamp to cause error
						},
						Message: &v1.BidTrace{
							Slot: testSlot,
						},
					},
				},
				expectOk: false,
			},
		}
		for _, tc := range cases {
			t.Run(tc.description, func(t *testing.T) {
				_, _, backend := startTestBackend(t, common.EthNetworkMainnet)

				headSlot := testSlot - 1
				w := httptest.NewRecorder()
				logger := logrus.New()
				log := logrus.NewEntry(logger)
				ok := backend.relay.checkSubmissionSlotDetails(w, log, headSlot, tc.payload)
				require.Equal(t, tc.expectOk, ok)
			})
		}
	*/
}

// @TODO: Fix me
func TestCheckBuilderEntry(t *testing.T) {
	/*
		builderPubkeyByte, err := hexutil.Decode(testBuilderPubkey)
		require.NoError(t, err)
		builderPubkey := phase0.BLSPubKey(builderPubkeyByte)
		diffPubkey := builderPubkey
		diffPubkey[0] = 0xff
		cases := []struct {
			description string
			entry       *blockBuilderCacheEntry
			pk          phase0.BLSPubKey
			expectOk    bool
		}{
			{
				description: "success",
				entry: &blockBuilderCacheEntry{
					status: common.BuilderStatus{
						IsHighPrio: true,
					},
				},
				pk:       builderPubkey,
				expectOk: true,
			},
			{
				description: "failure_blacklisted",
				entry: &blockBuilderCacheEntry{
					status: common.BuilderStatus{
						IsBlacklisted: true, // set blacklisted to true to cause failure
					},
				},
				pk:       builderPubkey,
				expectOk: false,
			},
			{
				description: "failure_low_prio",
				entry: &blockBuilderCacheEntry{
					status: common.BuilderStatus{
						IsHighPrio: false, // set low-prio to cause failure
					},
				},
				pk:       builderPubkey,
				expectOk: false,
			},
			{
				description: "failure_nil_entry_low_prio",
				entry:       nil,
				pk:          diffPubkey, // set to different pubkey, so no entry is found
				expectOk:    false,
			},
		}
		for _, tc := range cases {
			t.Run(tc.description, func(t *testing.T) {
				_, _, backend := startTestBackend(t, common.EthNetworkMainnet)
				backend.relay.blockBuildersCache[tc.pk.String()] = tc.entry
				backend.relay.ffDisableLowPrioBuilders = true
				w := httptest.NewRecorder()
				logger := logrus.New()
				log := logrus.NewEntry(logger)
				_, ok := backend.relay.checkBuilderEntry(w, log, builderPubkey)
				require.Equal(t, tc.expectOk, ok)
			})
		}
	*/
}

// @TODO: FIX ME
/*
func TestUpdateRedis(t *testing.T) {
	cases := []struct {
		description          string
		cancellationsEnabled bool
		floorValue           string
		payload              *common.BuilderSubmitBlockRequest
		expectOk             bool
	}{
		{
			description: "success",
			floorValue:  "10",
			payload: &common.BuilderSubmitBlockRequest{
				Capella: &builderCapella.SubmitBlockRequest{
					Message: &v1.BidTrace{
						Slot:  testSlot,
						Value: uint256.NewInt(1),
					},
					ExecutionPayload: &capella.ExecutionPayload{},
				},
			},
			expectOk: true,
		},
		{
			description: "failure_no_payload",
			floorValue:  "10",
			payload:     nil,
			expectOk:    false,
		},
		{
			description: "failure_encode_failure_too_long_extra_data",
			floorValue:  "10",
			payload: &common.BuilderSubmitBlockRequest{
				Capella: &builderCapella.SubmitBlockRequest{
					Message: &v1.BidTrace{
						Slot:  testSlot,
						Value: uint256.NewInt(1),
					},
					ExecutionPayload: &capella.ExecutionPayload{
						ExtraData: make([]byte, 33), // Max extra data length is 32.
					},
				},
			},
			expectOk: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			_, _, backend := startTestBackend(t, common.EthNetworkMainnet)
			w := httptest.NewRecorder()
			logger := logrus.New()
			log := logrus.NewEntry(logger)
			tx := backend.redis.NewTxPipeline()

			floorValue := new(big.Int)
			floorValue, ok := floorValue.SetString(tc.floorValue, 10)
			require.True(t, ok)
			rOpts := redisUpdateBidOpts{
				w:                    w,
				tx:                   tx,
				log:                  log,
				cancellationsEnabled: tc.cancellationsEnabled,
				floorBidValue:        floorValue,
				payload:              tc.payload,
			}
			updateResp, getPayloadResp, ok := backend.relay.updateRedisBid(rOpts)
			require.Equal(t, tc.expectOk, ok)
			if ok {
				require.NotNil(t, updateResp)
				require.NotNil(t, getPayloadResp)
			}
		})
	}
}
*/

func gzipBytes(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(b)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}
