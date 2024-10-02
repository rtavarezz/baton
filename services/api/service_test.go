package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AnomalyFi/baton/seq"
	"github.com/ava-labs/avalanchego/ids"
	abls "github.com/ava-labs/avalanchego/utils/crypto/bls"
	"github.com/ava-labs/avalanchego/vms/platformvm/warp"

	apiv1 "github.com/attestantio/go-builder-client/api/v1"
	"golang.org/x/exp/rand"

	"github.com/AnomalyFi/hypersdk/chain"
	"github.com/AnomalyFi/hypersdk/codec"
	hrpc "github.com/AnomalyFi/hypersdk/rpc"
	srpc "github.com/AnomalyFi/nodekit-seq/rpc"

	// "github.com/AnomalyFi/hypersdk/state"
	"github.com/AnomalyFi/baton/beaconclient"
	"github.com/AnomalyFi/baton/common"
	"github.com/AnomalyFi/baton/database"
	"github.com/AnomalyFi/baton/datastore"
	"github.com/alicebob/miniredis/v2"
	eth "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/stretchr/testify/require"
)

const (
	testGasLimit         = uint64(30000000)
	testSlot             = uint64(42)
	testWithdrawalsRoot  = "0x7f6d156912a4cb1e74ee37e492ad883f7f7ac856d987b3228b517e490aa0189e"
	testPrevRandao       = "0x9962816e9d0a39fd4c80935338a741dc916d1545694e41eb5a505e1a3098f9e4"
	testManagerSecretKey = "0x3fae9bafcf1572be9a4d4b7f8e6cb1d0c4bca8ad1e6f75d3d1286ad0e3e5fba1"
	testParentHash       = "0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747"
	testProposerPubkey   = "0x6ae5932d1e248d987d51b58665b81848814202d7b23b343d20f2a167d12f07dcb01ca41c42fdd60b7fca9c4b90890792"
	testBuilderPubKey    = "0x6ae7109d1e248d987d51b58665b81848814202d7b23b343d20f2a167d12f07dcb01ca41c42fdd60b7fca9c4b90891234"
	test2ProposerPubkey  = "0x84e975405f8691ad7118527ee9ee4ed2e4e8bae973f6e29aa9ca9ee4aea83605ae3536d22acc9aa1af0545064eacf82e"
	mockSecretKeyHex     = "0x4e343a647c5a5c44d76c2c58b63f02cdf3a9a0ec40f102ebc26363b4b1b95033"
	testHeaderHash       = "0x67d105493936e93431c7e42ff60e7c81405a4fe2e6877993996122fe07830a0c"
)

var (
	skBytes, _       = hexutil.Decode(mockSecretKeyHex)
	mockSecretKey, _ = bls.SecretKeyFromBytes(skBytes)
	mockPublicKey, _ = bls.PublicKeyFromSecretKey(mockSecretKey)
	testChainID      = GetTestChainID(0)
	emptyPublicKey   = bls.PublicKey{}
)

type testBackend struct {
	t          require.TestingT
	baton      *BatonAPI
	datastore  *datastore.Datastore
	redis      *datastore.RedisCache
	simManager *bls.SecretKey
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

	managerSkBytes, err := hexutil.Decode(testManagerSecretKey)
	require.NoError(t, err)
	managerSk, err := bls.SecretKeyFromBytes(managerSkBytes)
	require.NoError(t, err)
	managerPub, err := bls.PublicKeyFromSecretKey(managerSk)
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
		BlockSimManager: managerPub,
		ProposerAPI:     true,
		BlockBuilderAPI: true,
		DataAPI:         true,
		InternalAPI:     true,
		mockMode:        true,
	}

	baton, err := NewBatonAPI(opts)
	require.NoError(t, err)

	baton.genesisInfo = &beaconclient.GetGenesisResponse{
		Data: beaconclient.GetGenesisResponseData{
			GenesisTime: 1606824023,
		},
	}

	backend := testBackend{
		t:          t,
		baton:      baton,
		datastore:  ds,
		redis:      redisCache,
		simManager: managerSk,
	}

	// Add a single known test validator
	mockPublicKeyBytes := bls.PublicKeyToBytes(mockPublicKey)
	mockPublicKeyHex := hex.EncodeToString(mockPublicKeyBytes[:])
	backend.datastore.SetKnownValidator("0x"+common.PubkeyHex(mockPublicKeyHex), 0)
	return &backend
}

func (be *testBackend) GetRedis() *datastore.RedisCache {
	return be.redis
}

func (be *testBackend) GetMockSeqClient() *seq.MockSeqClient {
	seqClient := be.baton.GetSeqClient()
	mockSeqClient, ok := seqClient.(*seq.MockSeqClient)
	if !ok {
		panic("backend baton did not have mock seq client")
	}
	return mockSeqClient
}

func (be *testBackend) TriggerNextSlot(slot uint64) {
	nextBlk := chain.StatefulBlock{
		Hght: slot,
	}
	proposerReply := hrpc.NextProposerReply{}
	mockSeqClient := be.GetMockSeqClient()
	mockSeqClient.TriggerOnNextBlock(&nextBlk, &proposerReply)
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
	be.baton.getRouter().ServeHTTP(rr, req)
	return rr
}

func TestWebserver(t *testing.T) {
	t.Run("errors when webserver is already existing", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)
		backend.baton.srvStarted.Store(true)
		err := backend.baton.StartServer()
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

func TestRegisterSimulator(t *testing.T) {
	path := "/sim/v1/register"

	t.Run("can register with correct secret key", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)

		sk := backend.simManager
		sim := SimulatorInfo{
			URL: "someurl",
		}
		msg, err := json.Marshal(sim)
		require.NoError(t, err)
		sig := bls.Sign(sk, msg)
		pub, err := bls.PublicKeyFromSecretKey(sk)
		require.NoError(t, err)
		sigBytes := sig.Bytes()
		pubBytes := pub.Bytes()

		rr := backend.request(http.MethodPost, path, SimulatorRegisterRequest{
			Simulator: sim,
			Pubkey:    pubBytes[:],
			Signature: sigBytes[:],
		})
		require.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("wrong key cannot register", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)

		sk, err := bls.GenerateRandomSecretKey()
		require.NoError(t, err)
		sim := SimulatorInfo{
			URL: "someurl",
		}
		msg, err := json.Marshal(sim)
		require.NoError(t, err)
		sig := bls.Sign(sk, msg)
		pub, err := bls.PublicKeyFromSecretKey(sk)
		require.NoError(t, err)
		sigBytes := sig.Bytes()
		pubBytes := pub.Bytes()

		rr := backend.request(http.MethodPost, path, SimulatorRegisterRequest{
			Simulator: sim,
			Pubkey:    pubBytes[:],
			Signature: sigBytes[:],
		})
		require.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

// TODO: to be updated, fix me
func TestRegisterValidator(t *testing.T) {
	path := "/eth/v1/builder/validators"

	t.Run("not a known validator", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)

		rr := backend.request(http.MethodPost, path, []apiv1.SignedValidatorRegistration{common.ValidPayloadRegisterValidator})
		require.Equal(t, http.StatusBadRequest, rr.Code)
	})

	// TODO: Fix me
	/*
		t.Run("known validator", func(t *testing.T) {
			backend := newTestBackend(t, 1, common.EthNetworkMainnet)

			msg := common.ValidPayloadRegisterValidator
			backend.datastore.SetKnownValidator(common.PubkeyHex(msg.Message.Pubkey.String()), 1)

			rr := backend.request(http.MethodPost, path, []apiv1.SignedValidatorRegistration{common.ValidPayloadRegisterValidator})
			require.Equal(t, http.StatusOK, rr.Code)

			// wait for the both channel notifications
			select {
			case val := <-backend.baton.validatorRegC:
				require.Equal(t, val.Message.Pubkey, msg.Message.Pubkey)
			default:
			}
		})
	*/
}

// @TODO: Create test cases below, cover ALL cases
func TestGetHeader(t *testing.T) {
	// Setup backend with headSlot and genesisTime
	backend := newTestBackend(t, 1, common.EthNetworkMainnet)
	backend.baton.genesisInfo = &beaconclient.GetGenesisResponse{
		Data: beaconclient.GetGenesisResponseData{
			GenesisTime: uint64(time.Now().UTC().Unix()),
		},
	}
	slot := uint64(1)
	backend.baton.headSlot.Store(slot)

	backend.baton.robChainIDs[testChainID] = struct{}{}
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
	// TODO: change to ToB base case
	t.Run("Run valid base case, just tob", func(t *testing.T) {
		redis := backend.GetRedis()
		headerHash, err := common.GenerateRandomHash()
		if err != nil {
			t.Error(err)
		}

		header := common.AnchorHeader{
			Header:    headerHash,
			BlockHash: "0x8ae5292d1e248d987d51b58665b81848814202d7b23b343d20f2a167d12f07dcb01ca41c42fdd60b7fca9c4b90890792",
			Value:     uint64(2),
		}
		// Populate redis cache with expected headers
		err = redis.SetToBBid(slot, testParentHash, testProposerPubkey, header)
		if err != nil {
			t.Error(err)
		}
		keyTopBidValue := redis.KeyLatestToBBidByBuilder(slot, testParentHash, testProposerPubkey, testBuilderPubKey)

		err = redis.GetClient().Set(context.Background(), keyTopBidValue, header, 0).Err()
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		requestPath := fmt.Sprintf("/eth/v1/builder/header/%s/%s/%s", strconv.FormatUint(slot, 10), testParentHash, testProposerPubkey)
		require.Equal(t, "/eth/v1/builder/header/1/0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747/0x6ae5932d1e248d987d51b58665b81848814202d7b23b343d20f2a167d12f07dcb01ca41c42fdd60b7fca9c4b90890792", requestPath)

		httpReq := httptest.NewRequest(http.MethodGet, requestPath, nil)
		backend.baton.getRouter().ServeHTTP(rr, httpReq)

		require.Equal(t, http.StatusOK, rr.Code)
	})

	// note: base RoB case
	t.Run("Run valid base case, just rob", func(t *testing.T) {
		redis := backend.GetRedis()
		headerHash, err := common.GenerateRandomHash()
		if err != nil {
			t.Error(err)
		}

		header := common.AnchorHeader{
			Header:    headerHash,
			BlockHash: "0x8ae5292d1e248d987d51b58665b81848814202d7b23b343d20f2a167d12f07dcb01ca41c42fdd60b7fca9c4b90890792",
			Value:     uint64(2),
		}
		// Populate redis cache with expected headers
		err = redis.SetRoBBid(slot, testParentHash, testProposerPubkey, testChainID, header)
		if err != nil {
			t.Error(err)
		}
		keyTopBidValue := redis.KeyLatestRoBBidByBuilder(slot, testParentHash, testProposerPubkey, testBuilderPubKey, testChainID)

		err = redis.GetClient().Set(context.Background(), keyTopBidValue, header, 0).Err()
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		requestPath := fmt.Sprintf("/eth/v1/builder/header/%s/%s/%s", strconv.FormatUint(slot, 10), testParentHash, testProposerPubkey)
		require.Equal(t, "/eth/v1/builder/header/1/0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747/0x6ae5932d1e248d987d51b58665b81848814202d7b23b343d20f2a167d12f07dcb01ca41c42fdd60b7fca9c4b90890792", requestPath)

		httpReq := httptest.NewRequest(http.MethodGet, requestPath, nil)
		backend.baton.getRouter().ServeHTTP(rr, httpReq)

		require.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("1 bid per slot. > 1 slot", func(t *testing.T) {
		redis := backend.GetRedis()
		// state how many bids you'd like below
		numBids := 10
		// no more than 3 ToBs
		numToBs := 3
		toBCount := 0
		bidsPerSlot := make(map[uint64][]common.AnchorHeader)
		toBBidsPerSlot := make(map[uint64][]common.AnchorHeader)
		roBBidsPerSlot := make(map[uint64][]common.AnchorHeader)
		for i := 0; i < numBids; i++ {
			headerHash, err := common.GenerateRandomHash()
			if err != nil {
				t.Error(err)
			}
			header := common.AnchorHeader{
				Header:    headerHash,
				BlockHash: generateRandomBlockHash64(),
				Value:     uint64(i + 1),
			}

			slot := uint64(i + 1)
			bidsPerSlot[slot] = append(bidsPerSlot[slot], header)
			if i%2 == 0 || toBCount >= numToBs {
				// Set RoB bid
				roBBidsPerSlot[slot] = append(roBBidsPerSlot[slot], header)
				err = redis.SetRoBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), testChainID, header)
				if err != nil {
					t.Error(err)
				}

				keyTopBidValue := redis.KeyLatestRoBBidByBuilder(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), common.BuilderPubkeyAsStr(testBuilderPublicKey), testChainID)

				err = backend.redis.GetClient().Set(context.Background(), keyTopBidValue, header, 0).Err()
				if err != nil {
					t.Error(err)
				}
			} else {
				// Set ToB bid
				if len(toBBidsPerSlot[slot]) < 3 {
					toBBidsPerSlot[slot] = append(toBBidsPerSlot[slot], header)
					err = redis.SetToBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), header)
					if err != nil {
						t.Error(err)
					}

					keyTopBidValue := redis.KeyLatestToBBidByBuilder(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), common.BuilderPubkeyAsStr(testBuilderPublicKey))

					err = backend.redis.GetClient().Set(context.Background(), keyTopBidValue, header, 0).Err()
					if err != nil {
						t.Error(err)
					}
					toBCount++
				}
			}
		}
		rr := httptest.NewRecorder()
		requestPath := fmt.Sprintf("/eth/v1/builder/header/%s/%s/%s", strconv.FormatUint(1, 10), testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey))
		require.Equal(t, "/eth/v1/builder/header/1/0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747/"+common.ProposerPubKeyAsStr(testProposerPublicKey), requestPath)

		for slot := range toBBidsPerSlot {
			_, err := redis.GetBestToBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey))
			if err != nil {
				t.Error(err)
			}
		}

		for slot := range roBBidsPerSlot {
			_, err := redis.GetBestRoBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), testChainID)
			if err != nil {
				t.Error(err)
			}
		}
		httpReq := httptest.NewRequest(http.MethodGet, requestPath, nil)
		backend.baton.getRouter().ServeHTTP(rr, httpReq)
		require.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("multiple bids per slot. 2 slots only", func(t *testing.T) {
		redis := backend.GetRedis()
		// state how many bids you'd like below
		numBids := 10
		// no more than 3 ToBs
		numToBs := 3
		toBCount := 0
		bidsPerSlot := make(map[uint64][]common.AnchorHeader)
		toBBidsPerSlot := make(map[uint64][]common.AnchorHeader)
		roBBidsPerSlot := make(map[uint64][]common.AnchorHeader)
		for i := 0; i < numBids; i++ {
			headerHash, err := common.GenerateRandomHash()
			if err != nil {
				t.Error(err)
			}
			header := common.AnchorHeader{
				Header:    headerHash,
				BlockHash: generateRandomBlockHash64(),
				Value:     uint64(i + 1),
			}
			var slot uint64
			if i%2 == 0 || toBCount >= numToBs {
				// RoB slot
				slot = 2
				// Set RoB bid
				roBBidsPerSlot[slot] = append(roBBidsPerSlot[slot], header)
				err = redis.SetRoBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), testChainID, header)
				if err != nil {
					t.Error(err)
				}

				keyTopBidValue := redis.KeyLatestRoBBidByBuilder(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), common.BuilderPubkeyAsStr(testBuilderPublicKey), testChainID)

				err = backend.redis.GetClient().Set(context.Background(), keyTopBidValue, header, 0).Err()
				if err != nil {
					t.Error(err)
				}
			} else {
				// ToB slot
				slot = 1
				// Set ToB bid
				if len(toBBidsPerSlot[slot]) < 3 {
					toBBidsPerSlot[slot] = append(toBBidsPerSlot[slot], header)
					err = redis.SetToBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), header)
					if err != nil {
						t.Error(err)
					}

					keyTopBidValue := redis.KeyLatestToBBidByBuilder(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), common.BuilderPubkeyAsStr(testBuilderPublicKey))

					err = backend.redis.GetClient().Set(context.Background(), keyTopBidValue, header, 0).Err()
					if err != nil {
						t.Error(err)
					}
					toBCount++
				}
			}
			bidsPerSlot[slot] = append(bidsPerSlot[slot], header)
		}
		rr := httptest.NewRecorder()
		requestPath := fmt.Sprintf("/eth/v1/builder/header/%s/%s/%s", strconv.FormatUint(1, 10), testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey))
		require.Equal(t, "/eth/v1/builder/header/1/0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747/"+common.ProposerPubKeyAsStr(testProposerPublicKey), requestPath)

		for slot, headers := range toBBidsPerSlot {
			fmt.Printf("Slot: %d, ToB Bids: %v\n", slot, headers)

			topToBBid, err := redis.GetBestToBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey))
			if err != nil {
				t.Error(err)
			}
			fmt.Printf("Top ToB bid for slot %d: %v\n", slot, topToBBid)
		}

		for slot, headers := range roBBidsPerSlot {
			fmt.Printf("Slot: %d, RoB Bids: %v\n", slot, headers)

			topRoBBid, err := redis.GetBestRoBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), testChainID)
			if err != nil {
				t.Error(err)
			}
			fmt.Printf("Top RoB bid for slot %d: %v\n", slot, topRoBBid)
		}
		httpReq := httptest.NewRequest(http.MethodGet, requestPath, nil)
		backend.baton.getRouter().ServeHTTP(rr, httpReq)
		require.Equal(t, http.StatusOK, rr.Code)
	})
	t.Run("multiple bids, 1 slot only", func(t *testing.T) {
		redis := backend.GetRedis()
		// state how many bids you'd like below
		numBids := 10
		// no more than 3 ToBs
		numToBs := 3
		toBCount := 0
		bidsPerSlot := make(map[uint64][]common.AnchorHeader)
		toBBidsPerSlot := make(map[uint64][]common.AnchorHeader)
		roBBidsPerSlot := make(map[uint64][]common.AnchorHeader)
		for i := 0; i < numBids; i++ {
			headerHash, err := common.GenerateRandomHash()
			if err != nil {
				t.Error(err)
			}
			header := common.AnchorHeader{
				Header:    headerHash,
				BlockHash: generateRandomBlockHash64(),
				Value:     uint64(i + 1),
			}
			const slot = 1
			if i%2 == 0 || toBCount >= numToBs {
				// Set RoB bid
				roBBidsPerSlot[slot] = append(roBBidsPerSlot[slot], header)
				err = redis.SetRoBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), testChainID, header)
				if err != nil {
					t.Error(err)
				}

				keyTopBidValue := redis.KeyLatestRoBBidByBuilder(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), common.BuilderPubkeyAsStr(testBuilderPublicKey), testChainID)

				err = backend.redis.GetClient().Set(context.Background(), keyTopBidValue, header, 0).Err()
				if err != nil {
					t.Error(err)
				}
			} else {
				// Set ToB bid
				if len(toBBidsPerSlot[slot]) < 3 {
					toBBidsPerSlot[slot] = append(toBBidsPerSlot[slot], header)
					err = redis.SetToBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), header)
					if err != nil {
						t.Error(err)
					}

					keyTopBidValue := redis.KeyLatestToBBidByBuilder(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), common.BuilderPubkeyAsStr(testBuilderPublicKey))

					err = backend.redis.GetClient().Set(context.Background(), keyTopBidValue, header, 0).Err()
					if err != nil {
						t.Error(err)
					}
					toBCount++
				}
			}
			bidsPerSlot[slot] = append(bidsPerSlot[slot], header)
		}
		rr := httptest.NewRecorder()
		requestPath := fmt.Sprintf("/eth/v1/builder/header/%s/%s/%s", strconv.FormatUint(1, 10), testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey))
		require.Equal(t, "/eth/v1/builder/header/1/0x13e606c7b3d1faad7e83503ce3dedce4c6bb89b0c28ffb240d713c7b110b9747/"+common.ProposerPubKeyAsStr(testProposerPublicKey), requestPath)

		fmt.Printf("Slot: %d, All Bids: %v\n", slot, bidsPerSlot[slot])
		topToBBid, err := redis.GetBestToBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey))
		if err != nil {
			t.Error(err)
		}
		fmt.Printf("Top ToB bid for slot %d: %v\n", slot, topToBBid)

		topRoBBid, err := redis.GetBestRoBBid(slot, testParentHash, common.ProposerPubKeyAsStr(testProposerPublicKey), testChainID)
		if err != nil {
			t.Error(err)
		}
		fmt.Printf("Top RoB bid for slot %d: %v\n", slot, topRoBBid)
		httpReq := httptest.NewRequest(http.MethodGet, requestPath, nil)
		backend.baton.getRouter().ServeHTTP(rr, httpReq)
		require.Equal(t, http.StatusOK, rr.Code)
	})
}

func generateRandomBlockHash(length int) string {
	const charset = "abcdef0123456789"
	result := make([]byte, length)
	rand.Seed(uint64(time.Now().UnixNano()))
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return "0x" + string(result)
}

func generateRandomBlockHash32() string {
	return generateRandomBlockHash(32)
}

func generateRandomBlockHash64() string {
	return generateRandomBlockHash(64)
}

func createBackendHelper(t *testing.T) *testBackend {
	backend := newTestBackend(t, 1, common.EthNetworkMainnet)
	backend.baton.genesisInfo = &beaconclient.GetGenesisResponse{
		Data: beaconclient.GetGenesisResponseData{
			GenesisTime: uint64(time.Now().UTC().Unix()),
		},
	}
	return backend
}

func TestHandleSubmitNewBlockRequest(t *testing.T) {
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

	// Default rob test block for use in tests
	// Do not overwrite! Make your own copy for each test
	robBlockOpts := CreateTestBlockSubmissionOpts{
		Slot:           slot,
		ParentHash:     ids.Empty,
		BuilderPubkey:  *testBuilderPublicKey,
		ProposerPubkey: *testProposerPublicKey,
		IsToB:          false,
		robChainIndex:  0,
		numTxs:         1,
		withTransferTx: true,
	}
	robBlockValue := uint64(2)
	robBlockReq, _, _ := CreateTestChunkSubmission(t, robBlockValue, &robBlockOpts)

	// Default tob test block for use in tests
	// Do not overwrite! Make your own copy for each test
	tobBlockOpts := CreateTestBlockSubmissionOpts{
		Slot:           slot,
		ParentHash:     ids.Empty,
		BuilderPubkey:  *testBuilderPublicKey,
		ProposerPubkey: *testProposerPublicKey,
		IsToB:          true,
		robChainIndex:  0,
		numTxs:         2,
		withTransferTx: true,
	}
	tobBlockValue := uint64(5)
	tobBlockReq, _, _ := CreateTestChunkSubmission(t, tobBlockValue, &tobBlockOpts)
	require.NoError(t, err)

	// Default rob test block for use in tests
	// Do not overwrite! Make your own copy for each test
	robBlockOpts2 := CreateTestBlockSubmissionOpts{
		Slot:           slot,
		ParentHash:     ids.Empty,
		BuilderPubkey:  *testBuilderPublicKey,
		ProposerPubkey: *testProposerPublicKey,
		IsToB:          false,
		robChainIndex:  2,
		numTxs:         1,
		withTransferTx: true,
	}
	robBlockValue2 := uint64(5)
	robBlockReq2, _, _ := CreateTestChunkSubmission(t, robBlockValue2, &robBlockOpts2)

	// Helper for processing block requests to the backend. Returns the status code of the request.
	processBlockRequest := func(backend *testBackend, blockReq *common.SubmitNewBlockRequest) int {
		// marshal the req body
		requestBodyBytes, err := json.Marshal(blockReq)
		require.NoError(t, err)

		// new HTTP req
		httpReq := httptest.NewRequest(http.MethodPost, "/baton/v1/builder/submit", bytes.NewReader(requestBodyBytes))
		httpReq.Header.Set("Content-Type", "application/json")

		// Capture the response
		rr := httptest.NewRecorder()

		// Process the request
		backend.baton.getRouter().ServeHTTP(rr, httpReq)

		return rr.Code
	}

	t.Run("Run valid base case, just RoB", func(t *testing.T) {
		backend := createBackendHelper(t)
		rrCode := processBlockRequest(backend, robBlockReq)
		require.Equal(t, http.StatusOK, rrCode)
	})

	t.Run("Run valid base case, just ToB", func(t *testing.T) {
		backend := createBackendHelper(t)

		rrCode := processBlockRequest(backend, tobBlockReq)
		require.Equal(t, http.StatusOK, rrCode)
	})

	t.Run("Run valid base case, just ToB and multiple RoB", func(t *testing.T) {
		backend := createBackendHelper(t)

		rrCode := processBlockRequest(backend, tobBlockReq)
		require.Equal(t, http.StatusOK, rrCode)

		rrCode = processBlockRequest(backend, robBlockReq)
		require.Equal(t, http.StatusOK, rrCode)

		rrCode = processBlockRequest(backend, robBlockReq2)
		require.Equal(t, http.StatusOK, rrCode)
	})

	t.Run("RoB block with no txs should reject", func(t *testing.T) {
		backend := createBackendHelper(t)

		robBlockReqNoTx := robBlockReq
		robBlockReqNoTx.Chunk.Txs = nil

		rrCode := processBlockRequest(backend, robBlockReq)

		require.Equal(t, http.StatusBadRequest, rrCode)
	})

	t.Run("RoB block with slot equal to head slot", func(t *testing.T) {
		backend := createBackendHelper(t)

		headSlot := uint64(5)
		backend.TriggerNextSlot(headSlot)

		robBlockReqSlotHeadEqual := robBlockReq
		robBlockReqSlotHeadEqual.Chunk.Slot = headSlot

		rrCode := processBlockRequest(backend, robBlockReq)

		require.Equal(t, http.StatusBadRequest, rrCode)
	})

	t.Run("RoB block with slot too far head compared to head slot", func(t *testing.T) {
		backend := createBackendHelper(t)

		headSlot := uint64(5)
		backend.TriggerNextSlot(headSlot)

		robBlockReqSlotHeadEqual := robBlockReq
		robBlockReqSlotHeadEqual.Chunk.Slot = headSlot + 2

		rrCode := processBlockRequest(backend, robBlockReq)

		require.Equal(t, http.StatusBadRequest, rrCode)
	})

	t.Run("Run invalid RoB without transfer tx", func(t *testing.T) {
		backend := createBackendHelper(t)

		opts := CreateTestBlockSubmissionOpts{
			Slot:           slot,
			ParentHash:     ids.Empty,
			BuilderPubkey:  *testBuilderPublicKey,
			ProposerPubkey: *testProposerPublicKey,
			IsToB:          false,
			robChainIndex:  0,
			numTxs:         2,
			withTransferTx: false,
		}

		robReq, _, _ := CreateTestChunkSubmission(t, 100, &opts)
		require.NoError(t, err)

		rrCode := processBlockRequest(backend, robReq)
		require.Equal(t, http.StatusBadRequest, rrCode)
	})

	t.Run("ToB has too many txs", func(t *testing.T) {
		backend := createBackendHelper(t)

		opts := CreateTestBlockSubmissionOpts{
			Slot:           slot,
			ParentHash:     ids.Empty,
			BuilderPubkey:  *testBuilderPublicKey,
			ProposerPubkey: *testProposerPublicKey,
			IsToB:          true,
			robChainIndex:  0,
			numTxs:         common.MaxTobTxs + 1,
			withTransferTx: true,
		}

		baseValue := uint64(100)
		request, _, _ := CreateTestChunkSubmission(t, baseValue, &opts)
		require.NoError(t, err)

		rrCode := processBlockRequest(backend, request)
		require.Equal(t, http.StatusBadRequest, rrCode)
	})

	t.Run("ToB with number txs equal to max allowed is okay", func(t *testing.T) {
		backend := createBackendHelper(t)

		opts := CreateTestBlockSubmissionOpts{
			Slot:           slot,
			ParentHash:     ids.Empty,
			BuilderPubkey:  *testBuilderPublicKey,
			ProposerPubkey: *testProposerPublicKey,
			IsToB:          true,
			robChainIndex:  0,
			numTxs:         common.MaxTobTxs,
			withTransferTx: true,
		}

		baseValue := uint64(100)
		request, _, _ := CreateTestChunkSubmission(t, baseValue, &opts)
		require.NoError(t, err)

		rrCode := processBlockRequest(backend, request)
		require.Equal(t, http.StatusOK, rrCode)
	})

	t.Run("RoB has no tx limit enforcement", func(t *testing.T) {
		backend := createBackendHelper(t)

		opts := CreateTestBlockSubmissionOpts{
			Slot:           slot,
			ParentHash:     ids.Empty,
			BuilderPubkey:  *testBuilderPublicKey,
			ProposerPubkey: *testProposerPublicKey,
			IsToB:          false,
			robChainIndex:  0,
			numTxs:         common.MaxTobTxs + 1,
			withTransferTx: true,
		}

		baseValue := uint64(100)
		request, _, _ := CreateTestChunkSubmission(t, baseValue, &opts)
		require.NoError(t, err)

		rrCode := processBlockRequest(backend, request)
		require.Equal(t, http.StatusOK, rrCode)
	})

	t.Run("RoB block with bad builder key should reject", func(t *testing.T) {
		backend := createBackendHelper(t)

		epbBytes := emptyPublicKey.Bytes()
		robBlockReqNoTx := robBlockReq
		robBlockReqNoTx.BuilderPubKey = epbBytes[:]

		rrCode := processBlockRequest(backend, robBlockReq)

		require.Equal(t, http.StatusBadRequest, rrCode)
	})

	t.Run("Run valid RoBs for race condition", func(t *testing.T) {
		backend := createBackendHelper(t)

		opts := CreateTestBlockSubmissionOpts{
			Slot:           slot,
			ParentHash:     ids.Empty,
			BuilderPubkey:  *testBuilderPublicKey,
			ProposerPubkey: *testProposerPublicKey,
			IsToB:          false,
			robChainIndex:  0,
			numTxs:         1,
			withTransferTx: true,
		}

		baseValue := uint64(100)
		baseRobReq, _, _ := CreateTestChunkSubmission(t, baseValue, &opts)
		require.NoError(t, err)

		rrCode := processBlockRequest(backend, baseRobReq)
		require.Equal(t, http.StatusOK, rrCode)

		numRoBs := 1000
		robReqs := make([]*common.SubmitNewBlockRequest, 0, numRoBs) // all are higher bids than the base one
		for i := 0; i < numRoBs; i++ {
			req, _, _ := CreateTestChunkSubmission(t, baseValue+uint64(i), &opts)
			robReqs = append(robReqs, req)
		}
		highestBid := robReqs[len(robReqs)-1] // save the highest bid
		// shuffle bids
		for i := range robReqs {
			j := rand.Intn(i + 1)
			robReqs[i], robReqs[j] = robReqs[j], robReqs[i]
		}

		// concurrently bids
		var wg sync.WaitGroup
		for _, req := range robReqs {
			wg.Add(1)
			go func(req *common.SubmitNewBlockRequest) {
				defer wg.Done()
				rrCode := processBlockRequest(backend, req)
				require.Equal(t, http.StatusOK, rrCode)
			}(req)
		}
		wg.Wait()

		chainIDs := GetTestChainIds(opts.IsToB, opts.robChainIndex)
		header, err := backend.redis.GetBestRoBBid(opts.Slot, common.ParentHashToStr(opts.ParentHash), opts.ProposerPubKeyAsStr(), chainIDs[0])
		require.NoError(t, err)
		require.NotNil(t, header)
		require.Equal(t, header.BlockHash, highestBid.BlockHash().Hex())
	})
}

func TestRegisterSEQValidator(t *testing.T) {
	path := "/eth/v1/builder/validators"

	sk1, _, err := bls.GenerateNewKeypair()
	require.NoError(t, err)
	sk2, _, err := bls.GenerateNewKeypair()
	require.NoError(t, err)

	sk1Bytes := sk1.Bytes()
	sk2Bytes := sk2.Bytes()

	t.Run("Run can register SEQ with valid signature and existing pubkey in datastore", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)
		chainID := backend.baton.seqClient.GetChainID()
		networkID := backend.baton.seqClient.GetNetworkID()

		sk, err := abls.SecretKeyFromBytes(sk1Bytes[:])
		require.NoError(t, err)
		pk := abls.PublicFromSecretKey(sk)
		pkBytes := pk.Compress()

		reqMsg := common.SEQValidatorRegistration{
			FeeRecipient: codec.EmptyAddress,
			Timestamp:    time.Now().UnixMilli(),
			Pubkey:       pkBytes,
		}

		warpSigner := warp.NewSigner(sk, networkID, chainID)
		msgBytes, err := json.Marshal(reqMsg)
		require.NoError(t, err)
		uwm, err := warp.NewUnsignedMessage(networkID, chainID, msgBytes)
		require.NoError(t, err)
		sig, err := warpSigner.Sign(uwm)
		require.NoError(t, err)

		req := common.SignedSEQValidatorRegistration{
			Message:   &reqMsg,
			Signature: sig,
		}

		err = req.Initialize()
		require.NoError(t, err)

		backend.datastore.SetKnownValidator(common.PubkeyHex(req.Message.PublicKey().String()), 0)

		rr := backend.request(http.MethodPost, path, req)
		require.Equal(t, http.StatusOK, rr.Code)

		select {
		case val := <-backend.baton.validatorRegC:
			require.Equal(t, val.Message.Pubkey, req.Message.Pubkey)
		default:
		}
	})

	t.Run("Run cannot register SEQ with invalid signature and existing pubkey in datastore", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)
		chainID := backend.baton.seqClient.GetChainID()
		networkID := backend.baton.seqClient.GetNetworkID()

		sk, err := abls.SecretKeyFromBytes(sk2Bytes[:])
		require.NoError(t, err)
		pk := abls.PublicFromSecretKey(sk)
		pkBytes := pk.Compress()

		reqMsg := common.SEQValidatorRegistration{
			FeeRecipient: codec.EmptyAddress,
			Timestamp:    time.Now().UnixMilli(),
			Pubkey:       pkBytes,
		}

		warpSigner := warp.NewSigner(sk, networkID, chainID)
		wrongMsg := make([]byte, 200)
		_, err = rand.Read(wrongMsg)
		require.NoError(t, err)

		uwm, err := warp.NewUnsignedMessage(networkID, chainID, wrongMsg)
		require.NoError(t, err)
		sig, err := warpSigner.Sign(uwm)
		require.NoError(t, err)

		req := common.SignedSEQValidatorRegistration{
			Message:   &reqMsg,
			Signature: sig,
		}

		err = req.Initialize()
		require.NoError(t, err)

		backend.datastore.SetKnownValidator(common.PubkeyHex(req.Message.PublicKey().String()), 0)

		rr := backend.request(http.MethodPost, path, req)
		require.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Run cannot register SEQ with valid signature and non-existing pubkey in datastore", func(t *testing.T) {
		backend := newTestBackend(t, 1, common.EthNetworkMainnet)
		chainID := backend.baton.seqClient.GetChainID()
		networkID := backend.baton.seqClient.GetNetworkID()

		sk, err := abls.SecretKeyFromBytes(sk2Bytes[:])
		require.NoError(t, err)
		pk := abls.PublicFromSecretKey(sk)
		pkBytes := pk.Compress()

		reqMsg := common.SEQValidatorRegistration{
			FeeRecipient: codec.EmptyAddress,
			Timestamp:    time.Now().UnixMilli(),
			Pubkey:       pkBytes,
		}

		warpSigner := warp.NewSigner(sk, networkID, chainID)
		msgBytes, err := json.Marshal(reqMsg)
		require.NoError(t, err)
		uwm, err := warp.NewUnsignedMessage(networkID, chainID, msgBytes)
		require.NoError(t, err)
		sig, err := warpSigner.Sign(uwm)
		require.NoError(t, err)

		req := common.SignedSEQValidatorRegistration{
			Message:   &reqMsg,
			Signature: sig,
		}

		err = req.Initialize()
		require.NoError(t, err)

		rr := backend.request(http.MethodPost, path, req)
		require.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestGetCachedL2Txs(t *testing.T) {
	randomSEQTxMsg := func(chainID string, limit int) *chain.Transaction {
		minLength := 1
		txRawLength := rand.Intn(limit-minLength) + minLength
		txRaw := make([]byte, txRawLength)
		_, err := rand.Read(txRaw)
		require.NoError(t, err)

		return CreateHypersdkTx(chainID, txRaw)
	}

	randomSEQTxsMsgForChains := func(chainIDs []*big.Int, numTxs int, limit int) []byte {
		txs := make([]*chain.Transaction, 0, numTxs)
		numTxsPerChain := numTxs / len(chainIDs)
		if numTxsPerChain <= 0 {
			numTxsPerChain = 1
		}
		for _, chainID := range chainIDs {
			chainIDstr := hexutil.EncodeBig(chainID)
			for i := 0; i < numTxsPerChain; i++ {
				tx := randomSEQTxMsg(chainIDstr, limit)
				txs = append(txs, tx)
			}
		}

		// we have a 2MB bound for the packer used in this method, also the chain.UnmarshalTxs
		txsRaw, err := chain.MarshalTxs(txs)
		require.NoError(t, err)
		return txsRaw
	}

	proposerPubkey := test2ProposerPubkey
	var blockHash eth.Hash
	slot := uint64(1)
	overheadPackingTx := 512 // byte

	t.Run("benchmark extracting small(high computation load) L2 txs from raw chain.Transactions in ToB payload stored in cache", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping benchmark test in small mode(high computation load)")
		}

		backend := newTestBackend(t, 1, common.EthNetworkMainnet)
		redis := backend.GetRedis()

		sizeLimitPerL2Tx := 20 + overheadPackingTx // some overhead for packing one tx is added
		numTxsPerPayload := (2 * 1024 * 1024) / sizeLimitPerL2Tx
		numChains := 10
		tobChainIDs := make([]*big.Int, 0, numChains)
		for i := 0; i < numChains; i++ {
			chainID := big.NewInt(int64(45200 + i))
			tobChainIDs = append(tobChainIDs, chainID)
		}

		fmt.Printf("numChains: %d, numTxsPerPayload: %d, sizeLimitPerL2Tx: %d Bytes\n", numChains, numTxsPerPayload, sizeLimitPerL2Tx-overheadPackingTx)

		// ToB payload
		payload := common.AnchorPayload{
			Slot:         slot,
			Header:       blockHash,
			Transactions: randomSEQTxsMsgForChains(tobChainIDs, numTxsPerPayload, sizeLimitPerL2Tx-overheadPackingTx),
			GasUsed:      0,
			GasLimit:     0,
		}
		pipeline := redis.NewPipeline()
		err := redis.SaveExecutionToBAnchorPayload(context.TODO(), pipeline, payload.Slot, proposerPubkey, testParentHash, &payload)
		require.NoError(t, err)

		start := time.Now()
		_, err = backend.baton.getTopToBTxsByChainID(context.TODO(), hexutil.EncodeBig(tobChainIDs[0]), slot, testParentHash, testProposerPubkey, backend.baton.log)
		require.NoError(t, err)

		fmt.Printf("Used %f seconds to get ToB %d txs out of %d txs among %d chains\n", time.Since(start).Seconds(), numTxsPerPayload/numChains, numTxsPerPayload, numChains)
	})

	t.Run("benchmark extracting large(high network load) L2 txs from raw chain.Transactions in ToB payload stored in cache", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping benchmark test in large mode(high network load)")
		}

		backend := newTestBackend(t, 1, common.EthNetworkMainnet)
		redis := backend.GetRedis()

		sizeLimitPerL2Tx := 400*1024 + overheadPackingTx
		numTxsPerPayload := (2 * 1024 * 1024) / sizeLimitPerL2Tx
		numChains := 1
		tobChainIDs := make([]*big.Int, 0, numChains)
		for i := 0; i < numChains; i++ {
			chainID := big.NewInt(int64(45200 + i))
			tobChainIDs = append(tobChainIDs, chainID)
		}

		fmt.Printf("numChains: %d, numTxsPerPayload: %d, sizeLimitPerL2Tx: %d Bytes\n", numChains, numTxsPerPayload, sizeLimitPerL2Tx-overheadPackingTx)

		// ToB payload
		payload := common.AnchorPayload{
			Slot:         slot,
			Header:       blockHash,
			Transactions: randomSEQTxsMsgForChains(tobChainIDs, numTxsPerPayload, sizeLimitPerL2Tx),
			GasUsed:      0,
			GasLimit:     0,
		}
		pipeline := redis.NewPipeline()
		err := redis.SaveExecutionToBAnchorPayload(context.TODO(), pipeline, payload.Slot, proposerPubkey, testParentHash, &payload)
		require.NoError(t, err)

		start := time.Now()
		_, err = backend.baton.getTopToBTxsByChainID(context.TODO(), hexutil.EncodeBig(tobChainIDs[0]), slot, testParentHash, testProposerPubkey, backend.baton.log)
		require.NoError(t, err)

		fmt.Printf("Used %f seconds to get ToB %d txs out of %d txs among %d chains\n", time.Since(start).Seconds(), numTxsPerPayload/numChains, numTxsPerPayload, numChains)
	})

	t.Run("benchmark extracting small(high computation load) L2 txs from raw chain.Transactions in RoB payload stored in cache", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping benchmark test in small(high computation load) mode")
		}

		backend := newTestBackend(t, 1, common.EthNetworkMainnet)
		redis := backend.GetRedis()

		sizeLimitPerL2Tx := 20 + overheadPackingTx
		numTxsPerPayload := (2 * 1024 * 1024) / sizeLimitPerL2Tx
		numChains := 50
		robChainIDs := make([]*big.Int, 0, numChains)

		fmt.Printf("numChains: %d, numTxsPerPayload: %d, sizeLimitPerL2Tx: %d Bytes\n", 1, numTxsPerPayload, sizeLimitPerL2Tx)

		for i := 0; i < numChains; i++ {
			chainID := big.NewInt(int64(45200 + i))
			robChainIDs = append(robChainIDs, chainID)
			// ToB payload
			payload := common.AnchorPayload{
				Slot:         slot,
				Header:       blockHash,
				Transactions: randomSEQTxsMsgForChains([]*big.Int{chainID}, numTxsPerPayload, sizeLimitPerL2Tx-overheadPackingTx),
				GasUsed:      0,
				GasLimit:     0,
			}
			pipeline := redis.NewPipeline()
			err := redis.SaveExecutionRoBAnchorPayload(context.TODO(), pipeline, payload.Slot, proposerPubkey, testParentHash, &payload, hexutil.EncodeBig(chainID))
			require.NoError(t, err)
		}

		chainIDsMap := make(map[string]struct{})
		for _, chainID := range robChainIDs {
			chainIDsMap[hexutil.EncodeBig(chainID)] = struct{}{}
		}

		start := time.Now()
		_, err := backend.baton.getTopRoBsTxsByChainIDs(context.TODO(), chainIDsMap, slot, testParentHash, testProposerPubkey, backend.baton.log)
		require.NoError(t, err)

		fmt.Printf("Used %f seconds to extract %d RoB txs of %d chains\n", time.Since(start).Seconds(), numTxsPerPayload*numChains, numChains)
	})

	t.Run("benchmark extracting large(high network load) L2 txs from raw chain.Transactions in RoB payload stored in cache", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping benchmark test in large mode(high network load)")
		}

		backend := newTestBackend(t, 1, common.EthNetworkMainnet)
		redis := backend.GetRedis()

		sizeLimitPerL2Tx := 400*1024 + overheadPackingTx
		numTxsPerPayload := (2 * 1024 * 1024) / sizeLimitPerL2Tx
		numChains := 50
		robChainIDs := make([]*big.Int, 0, numChains)

		fmt.Printf("numChains: %d, numTxsPerPayload: %d, sizeLimitPerL2Tx: %d Bytes\n", 1, numTxsPerPayload, sizeLimitPerL2Tx-overheadPackingTx)

		for i := 0; i < numChains; i++ {
			chainID := big.NewInt(int64(45200 + i))
			robChainIDs = append(robChainIDs, chainID)
			// ToB payload
			payload := common.AnchorPayload{
				Slot:         slot,
				Header:       blockHash,
				Transactions: randomSEQTxsMsgForChains([]*big.Int{chainID}, numTxsPerPayload, sizeLimitPerL2Tx),
				GasUsed:      0,
				GasLimit:     0,
			}
			pipeline := redis.NewPipeline()
			err := redis.SaveExecutionRoBAnchorPayload(context.TODO(), pipeline, payload.Slot, proposerPubkey, testParentHash, &payload, hexutil.EncodeBig(chainID))
			require.NoError(t, err)
		}

		chainIDsMap := make(map[string]struct{})
		for _, chainID := range robChainIDs {
			chainIDsMap[hexutil.EncodeBig(chainID)] = struct{}{}
		}

		start := time.Now()
		_, err := backend.baton.getTopRoBsTxsByChainIDs(context.TODO(), chainIDsMap, slot, testParentHash, testProposerPubkey, backend.baton.log)
		require.NoError(t, err)

		fmt.Printf("Used %f seconds to extract %d RoB txs of %d chains\n", time.Since(start).Seconds(), numTxsPerPayload*numChains, numChains)
	})

}

// TODO: Fix the below
/*
func TestBuilderApiGetValidators(t *testing.T) {
	path := "/baton/v1/builder/validators"

	backend := newTestBackend(t, 1, common.EthNetworkMainnet)
	duties := []common.BuilderGetValidatorsResponseEntry{
		{
			Slot:  1,
			Entry: &apiv1.SignedValidatorRegistration{},
		},
	}
	responseBytes, err := json.Marshal(duties)
	require.NoError(t, err)
	backend.baton.proposerDutiesResponse = &responseBytes

	rr := backend.request(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rr.Code)

	resp := []common.BuilderGetValidatorsResponseEntry{}
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp))
	require.Equal(t, uint64(1), resp[0].Slot)
	require.Equal(t, apiv1.ValidatorRegistration{}, *resp[0].Entry)
}
*/

func TestGetPayload(t *testing.T) {
	// Setup backend with headSlot and genesisTime
	backend := newTestBackend(t, 1, common.EthNetworkMainnet)
	backend.baton.genesisInfo = &beaconclient.GetGenesisResponse{
		Data: beaconclient.GetGenesisResponseData{
			GenesisTime: uint64(time.Now().UTC().Unix()),
		},
	}
	slot := uint64(1)
	backend.baton.headSlot.Store(slot)
	requestPath := "/eth/v1/builder/blinded_blocks"
	headerHash := eth.Hash([]byte(testHeaderHash))
	seqChainID := backend.GetMockSeqClient().GetChainID()
	seqNetworkID := backend.GetMockSeqClient().GetNetworkID()

	robIDs := backend.baton.GetRoBChainIDs()
	(*robIDs)[testChainID] = struct{}{}

	// This is a default AnchorGetHeaderResp that can be used in our base case testing.
	anchorGetHeaderResp := common.MakeRandomAnchorGetHeaderResponse(*mockPublicKey, slot)
	anchorGetHeaderResp.ParentHash = common.StrToParentHash(testHeaderHash)
	err := common.SignAnchorGetHeaderResponse(seqChainID, seqNetworkID, anchorGetHeaderResp, mockSecretKey)
	if err != nil {
		t.Error(err)
	}
	signedHeaders, err := common.GetExecHeaderSignature(seqChainID, seqNetworkID, &anchorGetHeaderResp.ExecHeaders, mockSecretKey)
	if err != nil {
		t.Error(err)
	}
	signedHeaderBytes := signedHeaders.Bytes()
	backend.baton.SetExpectedHeaders(anchorGetHeaderResp)

	// helper for populating tob payloads
	populateToBPayloadFromHeader := func(header *common.AnchorHeader, blockHash eth.Hash, redis *datastore.RedisCache) {
		rpipe := backend.redis.NewTxPipeline()

		randHeader, err := common.GenerateRandomHash()
		require.NoError(t, err)
		txBytes := randHeader.Bytes()

		payload := common.AnchorPayload{
			Slot:   1,
			Header: header.Header,
			//Header:       eth.Hash([]byte(testHeaderHash)),
			Transactions: txBytes,
			GasUsed:      uint64(1),
			GasLimit:     uint64(10000),
		}

		err = redis.SaveExecutionToBAnchorPayload(
			context.Background(),
			rpipe,
			1,
			common.BlsPubKeyToStr(mockPublicKey),
			string(blockHash.Bytes()),
			&payload)
		require.NoError(t, err)

		_, err = rpipe.Exec(context.Background())
		require.NoError(t, err)
	}

	// helper for populating rob payloads
	populateRoBPayloadFromHeader := func(header *common.AnchorHeader, blockHash eth.Hash, redis *datastore.RedisCache, chainID string) {
		rpipe := backend.redis.NewTxPipeline()

		randHeader, err := common.GenerateRandomHash()
		require.NoError(t, err)
		txBytes := randHeader.Bytes()

		payload := common.AnchorPayload{
			Slot:   1,
			Header: header.Header,
			//Header:       eth.Hash([]byte(testHeaderHash)),
			Transactions: txBytes,
			GasUsed:      uint64(1),
			GasLimit:     uint64(10000),
		}

		err = redis.SaveExecutionRoBAnchorPayload(
			context.Background(),
			rpipe,
			1,
			common.BlsPubKeyToStr(mockPublicKey),
			string(blockHash.Bytes()),
			//blockHash.String(),
			&payload,
			chainID)
		require.NoError(t, err)

		_, err = rpipe.Exec(context.Background())
		require.NoError(t, err)
	}

	populatePayloadsFromHeaderResp := func(headerResp *common.AnchorGetHeaderResponse, blockHash eth.Hash, redis *datastore.RedisCache) {
		if headerResp.ExecHeaders.ToBHash != nil {
			populateToBPayloadFromHeader(headerResp.ExecHeaders.ToBHash, blockHash, redis)
		}

		for chainID, header := range headerResp.ExecHeaders.RoBHashes {
			populateRoBPayloadFromHeader(header, blockHash, redis, chainID)
		}
	}
	pk := mockPublicKey.Bytes()

	t.Run("Run valid base case, just tob", func(t *testing.T) {
		populatePayloadsFromHeaderResp(anchorGetHeaderResp, headerHash, backend.redis)

		payloadReq := common.AnchorGetPayloadRequest{
			Slot:           uint64(1),
			ProposerPubKey: pk[:],
			// Hash of exec headers. Must match the value sent by AnchorGetHeaderResponse.
			ParentHash: string(headerHash.Bytes()),
			// Exec headers signed by validator's key. Should be [48]byte bls.signature.
			SignedHeaders: signedHeaderBytes[:],
		}
		payloadReq.GetPublicKey()
		rr := backend.request(http.MethodPost, requestPath, payloadReq)
		require.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Run case with no valid content available", func(t *testing.T) {
		//redis := backend.GetRedis()
		payloadReq := common.AnchorGetPayloadRequest{
			Slot:           uint64(1),
			ProposerPubKey: pk[:],
			// Hash of exec headers. Must match the value sent by AnchorGetHeaderResponse.
			ParentHash: testHeaderHash,
			// Exec headers signed by validator's key. Should be [48]byte bls.signature.
			SignedHeaders: signedHeaderBytes[:],
		}

		backend.baton.SetExpectedHeaders(anchorGetHeaderResp)

		rr := backend.request(http.MethodPost, requestPath, payloadReq)
		require.Equal(t, http.StatusNoContent, rr.Code)
	})

	t.Run("Requesting getPayloads() but without call to getHeaders()", func(t *testing.T) {
		//redis := backend.GetRedis()
		payloadReq := common.AnchorGetPayloadRequest{
			Slot:           uint64(1),
			ProposerPubKey: pk[:],
			// Hash of exec headers. Must match the value sent by AnchorGetHeaderResponse.
			ParentHash: testHeaderHash,
			// Exec headers signed by validator's key. Should be [48]byte bls.signature.
			SignedHeaders: signedHeaderBytes[:],
		}

		backend.baton.SetExpectedHeaders(nil)

		rr := backend.request(http.MethodPost, requestPath, payloadReq)
		require.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestOverallBasicFlow(t *testing.T) {
	expectedSlot := uint64(1)
	var err error

	// Build hypersdk registry
	var cli = srpc.Parser{}
	_, _ = cli.Registry()

	var parentHash ids.ID
	testParentHash := []byte(generateRandomBlockHash32())
	copy(parentHash[:], testParentHash)

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

	// Create a RoB chunk
	robBlockOpts := CreateTestBlockSubmissionOpts{
		Slot:           expectedSlot,
		ParentHash:     parentHash,
		BuilderPubkey:  *testBuilderPublicKey,
		ProposerPubkey: *testProposerPublicKey,
		IsToB:          false,
		robChainIndex:  0,
		numTxs:         1,
		withTransferTx: true,
	}
	defaultRoBBlockValue := uint64(2)
	robBlockReq, _, _ := CreateTestChunkSubmission(t, defaultRoBBlockValue, &robBlockOpts)

	// Create a ToB chunk
	tobBlockOpts := CreateTestBlockSubmissionOpts{
		Slot:           expectedSlot,
		ParentHash:     parentHash,
		BuilderPubkey:  *testBuilderPublicKey,
		ProposerPubkey: *testProposerPublicKey,
		IsToB:          true,
		robChainIndex:  0,
		numTxs:         2,
		withTransferTx: true,
	}
	defaultToBBlockValue := uint64(2)
	tobBlockReq, _, _ := CreateTestChunkSubmission(t, defaultToBBlockValue, &tobBlockOpts)

	// Helper for processing block requests to the backend. Returns the status code of the request.
	processBlockRequest := func(backend *testBackend, blockReq *common.SubmitNewBlockRequest) int {
		// marshal the req body
		requestBodyBytes, err := json.Marshal(blockReq)
		require.NoError(t, err)

		// new HTTP req
		httpReq := httptest.NewRequest(http.MethodPost, "/baton/v1/builder/submit", bytes.NewReader(requestBodyBytes))
		httpReq.Header.Set("Content-Type", "application/json")

		// Capture the response
		rr := httptest.NewRecorder()

		// Process the request
		backend.baton.getRouter().ServeHTTP(rr, httpReq)

		return rr.Code
	}

	backend := createBackendHelper(t)
	seqClient := backend.GetMockSeqClient()
	proposerPubKeyStr := robBlockReq.ProposerPubKeyAsStr()
	// Submit block requests (one rob, one tob)
	rrCode := processBlockRequest(backend, robBlockReq)
	require.Equal(t, http.StatusOK, rrCode)

	rrCode = processBlockRequest(backend, tobBlockReq)
	require.Equal(t, http.StatusOK, rrCode)

	// process new blk and proposer
	seqHead := chain.NewGenesisBlock(ids.Empty)
	seqHead.Hght = 1
	nextProposerInfo := hrpc.NextProposerReply{
		PublicKey: robBlockReq.ProposerPubKeyAsBytes(),
	}
	seqClient.TriggerOnNextBlock(seqHead, &nextProposerInfo)

	// Now test getHeader()
	rr := httptest.NewRecorder()

	getHeaderRequestPath := fmt.Sprintf("/eth/v1/builder/header/%s/%s/%s", strconv.FormatUint(expectedSlot, 10), common.ParentHashToStr(parentHash), proposerPubKeyStr)

	httpReq := httptest.NewRequest(http.MethodGet, getHeaderRequestPath, nil)
	backend.baton.getRouter().ServeHTTP(rr, httpReq)

	require.Equal(t, http.StatusOK, rr.Code)

	// Now test getPayload()
	resp := new(common.AnchorGetHeaderResponse)
	resp.ExecHeaders = common.NewExecutionHeader()
	err = json.Unmarshal(rr.Body.Bytes(), resp)
	require.NoError(t, err)

	seqChainID := seqClient.GetChainID()
	seqNetworkID := seqClient.GetNetworkID()
	signedHeaders, err := common.GetExecHeaderSignature(seqChainID, seqNetworkID, &resp.ExecHeaders, testProposerSecretKey)
	if err != nil {
		t.Error(err)
	}
	resp.SetExecPayloadsSig(signedHeaders)
	signedHeadersBytes := signedHeaders.Bytes()
	proposerPubKeyBytes := robBlockReq.ProposerPubKeyAsBytes()
	payloadReq := common.AnchorGetPayloadRequest{
		Slot:           uint64(1),
		ProposerPubKey: proposerPubKeyBytes,
		// Hash of exec headers. Must match the value sent by AnchorGetHeaderResponse.
		ParentHash: common.ParentHashToStr(parentHash),
		// Exec headers signed by validator's key. Should be [48]byte bls.signature.
		SignedHeaders: signedHeadersBytes[:],
	}
	payloadReq.GetPublicKey()
	requestPath := "/eth/v1/builder/blinded_blocks"
	rr = backend.request(http.MethodPost, requestPath, payloadReq)
	require.Equal(t, http.StatusOK, rr.Code)
}

// TODO: Fix me
/*
func TestDataApiGetDataProposerPayloadDelivered(t *testing.T) {
	path := "/baton/v1/data/bidtraces/proposer_payload_delivered"

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
*/

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
				backend.baton.proposerDutiesLock.RLock()
				backend.baton.proposerDutiesMap[tc.payload.Slot()] = tc.slotDuty
				backend.baton.proposerDutiesLock.RUnlock()

				w := httptest.NewRecorder()
				logger := logrus.New()
				log := logrus.NewEntry(logger)
				gasLimit, ok := backend.baton.checkSubmissionFeeRecipient(w, log, tc.payload)
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
				ok := backend.baton.checkSubmissionSlotDetails(w, log, headSlot, tc.payload)
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
				backend.baton.blockBuildersCache[tc.pk.String()] = tc.entry
				backend.baton.ffDisableLowPrioBuilders = true
				w := httptest.NewRecorder()
				logger := logrus.New()
				log := logrus.NewEntry(logger)
				_, ok := backend.baton.checkBuilderEntry(w, log, builderPubkey)
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
			updateResp, getPayloadResp, ok := backend.baton.updateRedisBid(rOpts)
			require.Equal(t, tc.expectOk, ok)
			if ok {
				require.NotNil(t, updateResp)
				require.NotNil(t, getPayloadResp)
			}
		})
	}
}
*/
