// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package node

import (
	"context"
	"errors"
	"math/big"
	"net"
	"net/http"
	"strconv"

	ophttp "github.com/ethereum-optimism/optimism/op-service/httputil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
)

type rpcServer struct {
	endpoint   string
	apis       []rpc.API
	httpServer *http.Server
	appVersion string
	listenAddr net.Addr
	log        log.Logger
}

func newRPCServer(
	ctx context.Context,
	rpcCfg *RPCConfig,
	l2ChainId *big.Int,
	sm *ethstorage.StorageManager,
	dl *downloader.Downloader,
	log log.Logger,
	appVersion string,
) (*rpcServer, error) {
	esAPI := NewESAPI(rpcCfg, sm, dl, log)
	ethApi := NewETHAPI(rpcCfg, l2ChainId, log)

	endpoint := net.JoinHostPort(rpcCfg.ListenAddr, strconv.Itoa(rpcCfg.ListenPort))
	r := &rpcServer{
		endpoint: endpoint,
		apis: []rpc.API{
			{
				Namespace:     "es",
				Service:       esAPI,
				Authenticated: false,
			},
			{
				Namespace:     "eth",
				Service:       ethApi,
				Authenticated: false,
			},
		},
		appVersion: appVersion,
		log:        log,
	}
	return r, nil
}

func (s *rpcServer) Start() error {
	srv := rpc.NewServer()
	if err := node.RegisterApis(s.apis, nil, srv); err != nil {
		return err
	}

	// The CORS and VHosts arguments below must be set in order for
	// other services to connect to the node. VHosts in particular
	// defaults to localhost, which will prevent containers from
	// calling into the node without an "invalid host" error.
	nodeHandler := node.NewHTTPHandlerStack(srv, []string{"*"}, []string{"*"}, nil)

	mux := http.NewServeMux()
	mux.Handle("/", nodeHandler)
	mux.HandleFunc("/healthz", healthzHandler(s.appVersion))

	listener, err := net.Listen("tcp", s.endpoint)
	if err != nil {
		return err
	}
	s.listenAddr = listener.Addr()

	s.httpServer = ophttp.NewHttpServer(mux)
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) { // todo improve error handling
			s.log.Error("Http server failed", "err", err)
		}
	}()
	return nil
}

func (r *rpcServer) Stop() {
	_ = r.httpServer.Shutdown(context.Background())
}

func healthzHandler(appVersion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(appVersion))
	}
}
