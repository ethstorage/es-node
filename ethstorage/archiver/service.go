// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"

	"github.com/ethereum-optimism/optimism/op-service/httputil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/gorilla/mux"
)

func NewService(cfg Config, storageMgr *ethstorage.StorageManager, l1Beacon *eth.BeaconClient, l1Source *eth.PollingClient, l log.Logger) *APIService {
	api := NewAPI(storageMgr, l1Beacon, l1Source, l)
	return &APIService{
		cfg: cfg,
		api: api,
		log: l,
	}
}

type APIService struct {
	log       log.Logger
	cfg       Config
	apiServer *http.Server
	api       *API
}

func (a *APIService) Start(ctx context.Context) error {
	a.log.Debug("starting blob archiver API server", "address", a.cfg.ListenAddr)
	endpoint := net.JoinHostPort(a.cfg.ListenAddr, strconv.Itoa(a.cfg.ListenPort))
	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		return err
	}
	r := mux.NewRouter()
	r.HandleFunc("/eth/v1/beacon/blob_sidecars/{id}", a.api.blobSidecarHandler)

	a.apiServer = httputil.NewHttpServer(r)
	go func() {
		if err := a.apiServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Error("Http server failed", "err", err)
		}
	}()
	a.log.Info("Blob archiver API server started", "address", listener.Addr().String())
	return nil
}

func (a *APIService) Stop(ctx context.Context) {
	a.log.Debug("Stopping blob archiver API")
	if a.apiServer != nil {
		if err := a.apiServer.Shutdown(ctx); err != nil {
			a.log.Error("Error stopping blob archiver API server", "err", err)
		}
	}
	a.log.Info("Blob archiver API server stopped")
}
