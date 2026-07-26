package mockserver

import (
	"net"
	"net/http"
	"net/http/httptest"

	"github.com/canopy-network/canopy-rpc-mock/mockserver/internal/chain"
	"github.com/canopy-network/canopy-rpc-mock/mockserver/internal/rpc"
)

// Server wraps httptest.Server around a chain.MockChain's registered routes.
type Server struct {
	*httptest.Server
	chain *chain.MockChain
}

// Profile re-exports chain.Profile so callers can pick busy/quiet without
// reaching into internal/chain.
type Profile = chain.Profile

const (
	ProfileBusy  = chain.ProfileBusy
	ProfileQuiet = chain.ProfileQuiet
)

type serverConfig struct {
	numBlocks      int
	addr           string
	profile        *Profile
	seed           *int64
	validatorCount int
	accountCount   int
}

type Option func(*serverConfig)

func WithBlocks(n int) Option { return func(c *serverConfig) { c.numBlocks = n } }

// WithAddr binds a fixed address instead of an ephemeral port (used by the
// standalone CLI in main.go; integration tests should omit this and read
// srv.URL instead).
func WithAddr(addr string) Option { return func(c *serverConfig) { c.addr = addr } }

// WithProfile overrides the automatic chainID-based busy/quiet profile pick.
func WithProfile(p Profile) Option { return func(c *serverConfig) { c.profile = &p } }

// WithSeed overrides the RNG seed, which otherwise defaults to int64(chainID).
func WithSeed(seed int64) Option { return func(c *serverConfig) { c.seed = &seed } }

// WithValidatorCount overrides the default of 10 validators.
func WithValidatorCount(n int) Option { return func(c *serverConfig) { c.validatorCount = n } }

// WithAccountCount overrides the default of one account per validator.
func WithAccountCount(n int) Option { return func(c *serverConfig) { c.accountCount = n } }

func defaultServerConfig() serverConfig {
	return serverConfig{numBlocks: 50}
}

// New starts an in-process mock RPC server for chainID. Callers must call
// Close() when done. This is the entry point canopy-indexer's integration
// tests use in place of the old hand-rolled fakeMultiRPC interface fake —
// it exercises the real wire path (HTTPClient.Blob() -> real HTTP -> protobuf
// parse) that a Go-interface fake bypasses entirely. Note New() only imports
// chain and rpc — it never reaches into internal/gen directly, which is the
// boundary this task's split exists to enforce.
func New(chainID uint64, opts ...Option) *Server {
	cfg := defaultServerConfig()
	for _, o := range opts {
		o(&cfg)
	}
	var chainOpts []chain.Option
	if cfg.profile != nil {
		chainOpts = append(chainOpts, chain.WithProfile(*cfg.profile))
	}
	if cfg.seed != nil {
		chainOpts = append(chainOpts, chain.WithSeed(*cfg.seed))
	}
	if cfg.validatorCount > 0 {
		chainOpts = append(chainOpts, chain.WithValidatorCount(cfg.validatorCount))
	}
	if cfg.accountCount > 0 {
		chainOpts = append(chainOpts, chain.WithAccountCount(cfg.accountCount))
	}
	mc := chain.NewMockChain(cfg.numBlocks, chainID, chainOpts...)
	mux := http.NewServeMux()
	rpc.RegisterRoutes(mux, mc)

	ts := httptest.NewUnstartedServer(mux)
	if cfg.addr != "" {
		l, err := net.Listen("tcp", cfg.addr)
		if err != nil {
			panic(err)
		}
		_ = ts.Listener.Close()
		ts.Listener = l
	}
	ts.Start()
	return &Server{Server: ts, chain: mc}
}
