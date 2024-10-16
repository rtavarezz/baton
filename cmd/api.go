package cmd

import (
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AnomalyFi/baton/common"
	"github.com/AnomalyFi/baton/database"
	"github.com/AnomalyFi/baton/datastore"
	"github.com/AnomalyFi/baton/services/api"
	"github.com/AnomalyFi/hypersdk/crypto/ed25519"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	apiDefaultListenAddr = common.GetEnv("LISTEN_ADDR", "localhost:9062")
	apiDefaultBlockSim   = common.GetEnv("BLOCKSIM_URI", "http://localhost:8545")
	apiDefaultFBRPCKey   = common.GetEnv("FBRPC_KEY", "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	apiDefaultSecretKey  = common.GetEnv("SECRET_KEY", "")
	apiDefaultLogTag     = os.Getenv("LOG_TAG")
	apiDefaultSEQURI     = common.GetEnv("SEQ_URL", "http://seq.url")

	apiDefaultPprofEnabled       = os.Getenv("PPROF") == "1"
	apiDefaultInternalAPIEnabled = os.Getenv("ENABLE_INTERNAL_API") == "1"

	// Default Builder, Data, and Proposer API as true.
	apiDefaultBuilderAPIEnabled  = os.Getenv("DISABLE_BUILDER_API") != "1"
	apiDefaultDataAPIEnabled     = os.Getenv("DISABLE_DATA_API") != "1"
	apiDefaultProposerAPIEnabled = os.Getenv("DISABLE_PROPOSER_API") != "1"
	apiDefaultSimDepth           = 3 // sim with txs deep to 3 slot
	apiDefaultFutureSlotsAllowed = 3

	apiListenAddr          string
	apiPprofEnabled        bool
	apiSecretKey           string
	apiBlockSimURL         string
	apiBlockSimFbRPCKey    string
	apiBlockSimDepth       int
	apiDebug               bool
	apiBuilderAPI          bool
	apiDataAPI             bool
	apiInternalAPI         bool
	apiProposerAPI         bool
	apiLogTag              string
	apiSEQURI              string
	apiSEQChainID          string
	apiSEQNetworkID        uint32
	apiSEQSigningKey       string
	apiSEQBlockWaitTimeout time.Duration
	apiSizeTrackerLimit    int
	apiFutureSlotsAllowed  int
)

func init() {
	rootCmd.AddCommand(apiCmd)
	apiCmd.Flags().BoolVar(&logJSON, "json", defaultLogJSON, "log in JSON format instead of text")
	apiCmd.Flags().StringVar(&logLevel, "loglevel", defaultLogLevel, "log-level: trace, debug, info, warn/warning, error, fatal, panic")
	apiCmd.Flags().StringVar(&apiLogTag, "log-tag", apiDefaultLogTag, "if set, a 'tag' field will be added to all log entries")
	apiCmd.Flags().BoolVar(&apiDebug, "debug", false, "debug logging")

	apiCmd.Flags().StringVar(&apiListenAddr, "listen-addr", apiDefaultListenAddr, "listen address for webserver")
	apiCmd.Flags().StringSliceVar(&beaconNodeURIs, "beacon-uris", defaultBeaconURIs, "beacon endpoints")
	apiCmd.Flags().StringVar(&redisURI, "redis-uri", defaultRedisURI, "redis uri")
	apiCmd.Flags().StringVar(&redisReadonlyURI, "redis-readonly-uri", defaultRedisReadonlyURI, "redis readonly uri")
	apiCmd.Flags().StringVar(&postgresDSN, "db", defaultPostgresDSN, "PostgreSQL DSN")
	apiCmd.Flags().StringSliceVar(&memcachedURIs, "memcached-uris", defaultMemcachedURIs,
		"Enable memcached, typically used as secondary backup to Redis for redundancy")
	apiCmd.Flags().StringVar(&apiSecretKey, "secret-key", apiDefaultSecretKey, "secret key for signing bids")
	apiCmd.Flags().StringVar(&apiBlockSimURL, "blocksim", apiDefaultBlockSim, "URL for block simulator")
	apiCmd.Flags().StringVar(&apiBlockSimFbRPCKey, "fbrpc-key", apiDefaultFBRPCKey, "fb rpc signing key")
	apiCmd.Flags().IntVar(&apiBlockSimDepth, "sim-depth", apiDefaultSimDepth, "simulation txs range from [headSlot:headSlot-depth]")
	apiCmd.Flags().StringVar(&network, "network", defaultNetwork, "Which network to use")
	apiCmd.Flags().IntVar(&apiSizeTrackerLimit, "slot-sizelim", api.DefaultSizeLimit, "simulation txs range from [headSlot:headSlot-depth]")
	apiCmd.Flags().IntVar(&apiFutureSlotsAllowed, "slot-future", apiDefaultFutureSlotsAllowed, "")

	apiCmd.Flags().BoolVar(&apiPprofEnabled, "pprof", apiDefaultPprofEnabled, "enable pprof API")
	apiCmd.Flags().BoolVar(&apiBuilderAPI, "builder-api", apiDefaultBuilderAPIEnabled, "enable builder API (/builder/...)")
	apiCmd.Flags().BoolVar(&apiDataAPI, "data-api", apiDefaultDataAPIEnabled, "enable data API (/data/...)")
	apiCmd.Flags().BoolVar(&apiInternalAPI, "internal-api", apiDefaultInternalAPIEnabled, "enable internal API (/internal/...)")
	apiCmd.Flags().BoolVar(&apiProposerAPI, "proposer-api", apiDefaultProposerAPIEnabled, "enable proposer API (/proposer/...)")
	apiCmd.Flags().StringVar(&apiSEQURI, "seq-uri", apiDefaultSEQURI, "SEQ rpc url")
	apiCmd.Flags().Uint32Var(&apiSEQNetworkID, "seq-network-id", uint32(1337), "SEQ rpc url")
	apiCmd.Flags().StringVar(&apiSEQChainID, "seq-chain-id", "2bJKVCnNxcpPHaHxtacZS9rwPL9NdyYnBuJjusfZgYBTE5ptSG", "SEQ rpc url")
	apiCmd.Flags().StringVar(&apiSEQSigningKey, "seq-key", "0x3851d590082e2dcf4d4a772ec43b47069c1236ab7a038e5b647cf0c2dc3d40014d24a0435169f5bb470dc00061435ad87f7fb7770f43df7bffd55f16627f83af", "SEQ signing key")
	apiCmd.Flags().DurationVar(&apiSEQBlockWaitTimeout, "seq-timeout", 3*time.Second, "timeout to wait a block & proposer info")
}

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Start the API server",
	Run: func(cmd *cobra.Command, args []string) {
		var err error

		if apiDebug {
			logLevel = "debug"
		}

		log := common.LogSetup(logJSON, logLevel).WithFields(logrus.Fields{
			"service": "relay/api",
			"version": Version,
		})
		if apiLogTag != "" {
			log = log.WithField("tag", apiLogTag)
		}
		log.Infof("boost-relay %s", Version)

		networkInfo, err := common.NewEthNetworkDetails(network)
		if err != nil {
			log.WithError(err).Fatalf("error getting network details")
		}
		log.Infof("Using network: %s", networkInfo.Name)
		log.Debug(networkInfo.String())

		// Connect to Redis
		if redisReadonlyURI == "" {
			log.Infof("Connecting to Redis at %s ...", redisURI)
		} else {
			log.Infof("Connecting to Redis at %s / readonly: %s ...", redisURI, redisReadonlyURI)
		}
		redis, err := datastore.NewRedisCache(networkInfo.Name, redisURI, redisReadonlyURI)
		if err != nil {
			log.WithError(err).Fatalf("Failed to connect to Redis at %s", redisURI)
		}

		// Connect to Memcached if it exists
		var mem *datastore.Memcached
		if len(memcachedURIs) > 0 {
			log.Infof("Connecting to Memcached at %s ...", strings.Join(memcachedURIs, ", "))
			mem, err = datastore.NewMemcached(networkInfo.Name, memcachedURIs...)
			if err != nil {
				log.WithError(err).Fatalf("Failed to connect to Memcached")
			}
		}

		// Connect to Postgres
		dbURL, err := url.Parse(postgresDSN)
		if err != nil {
			log.WithError(err).Fatalf("couldn't read db URL")
		}
		log.Infof("Connecting to Postgres database at %s%s ...", dbURL.Host, dbURL.Path)
		db, err := database.NewDatabaseService(postgresDSN)
		if err != nil {
			log.WithError(err).Fatalf("Failed to connect to Postgres database at %s%s", dbURL.Host, dbURL.Path)
		}

		log.Info("Setting up datastore...")
		ds, err := datastore.NewDatastore(redis, mem, db)
		if err != nil {
			log.WithError(err).Fatalf("Failed setting up prod datastore")
		}

		seqChainID, err := ids.FromString(apiSEQChainID)
		if err != nil {
			log.WithError(err).Fatalf("failed to parse seq chain id")
		}
		seqSkBytes, err := hexutil.Decode(apiSEQSigningKey)
		if err != nil {
			log.WithError(err).Fatalf("failed to parse seq secret key")
		}
		var seqSk ed25519.PrivateKey
		copy(seqSk[:], seqSkBytes)

		fbSkHex := strings.TrimLeft(apiBlockSimFbRPCKey, "0x")
		fbSk, err := crypto.HexToECDSA(fbSkHex)
		if err != nil {
			log.WithError(err).Fatalf("failed to parse fb rpc signing key")
		}

		opts := api.BatonAPIOpts{
			Log:        log,
			ListenAddr: apiListenAddr,
			// BeaconClient:  beaconClient,
			Datastore:          ds,
			Redis:              redis,
			Memcached:          mem,
			DB:                 db,
			EthNetDetails:      *networkInfo,
			BlockSimURL:        apiBlockSimURL,
			BlockSimSigningKey: fbSk,
			BlockSimDepth:      apiBlockSimDepth,
			SeqURL:             apiSEQURI,
			SeqChainID:         seqChainID,
			SeqNetworkID:       apiSEQNetworkID,
			SeqSigningKey:      seqSk,
			SeqBlockWaitTime:   apiSEQBlockWaitTimeout,

			BlockBuilderAPI:    apiBuilderAPI,
			DataAPI:            apiDataAPI,
			InternalAPI:        apiInternalAPI,
			ProposerAPI:        apiProposerAPI,
			PprofAPI:           apiPprofEnabled,
			SlotSizeLimit:      apiSizeTrackerLimit,
			FutureSlotsAllowed: apiFutureSlotsAllowed,
		}

		// Decode the private key
		if apiSecretKey == "" {
			log.Warn("No secret key specified, block builder API is disabled")
			opts.BlockBuilderAPI = false
		} else {
			envSkBytes, err := hexutil.Decode(apiSecretKey)
			if err != nil {
				log.WithError(err).Fatal("incorrect secret key provided")
			}
			opts.SecretKey, err = bls.SecretKeyFromBytes(envSkBytes[:])
			if err != nil {
				log.WithError(err).Fatal("unable to read secret key from bytes")
			}
			// assume the manager is the key holder of this baton instance
			pk, err := bls.PublicKeyFromSecretKey(opts.SecretKey)
			if err != nil {
				log.WithError(err).Fatal("unable to derive pubkey")
			}
			opts.BlockSimManager = pk
		}

		// Create the relay service
		log.Info("Setting up relay service...")
		srv, err := api.NewBatonAPI(opts)
		if err != nil {
			log.WithError(err).Fatal("failed to create service")
		}

		// Create a signal handler
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigs
			log.Infof("signal received: %s", sig)
			err := srv.StopServer()
			if err != nil {
				log.WithError(err).Fatal("error stopping server")
			}
		}()

		// Start the server
		log.Infof("Webserver starting on %s ...", apiListenAddr)
		err = srv.StartServer()
		if err != nil {
			log.WithError(err).Fatal("server error")
		}
		log.Info("bye")
	},
}
