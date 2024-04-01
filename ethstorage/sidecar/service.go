// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package sidecar

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"

	"github.com/ethereum-optimism/optimism/op-service/httputil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
)

func NewService(cfg Config, downloader *downloader.Downloader, storageMgr *ethstorage.StorageManager, l1Beacon *eth.BeaconClient, l log.Logger) *APIService {
	retriever := &Retriever{
		downloader: downloader,
		storageMgr: storageMgr,
		log:        l,
	}
	api := NewAPI(l1Beacon, retriever, l)
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
	a.log.Debug("starting API server", "address", a.cfg.ListenAddr)

	endpoint := net.JoinHostPort(a.cfg.ListenAddr, strconv.Itoa(a.cfg.ListenPort))

	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		return err
	}
	a.apiServer = httputil.NewHttpServer(a.api.router)
	go func() {
		if err := a.apiServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) { // todo improve error handling
			a.log.Error("Http server failed", "err", err)
		}
	}()
	a.log.Info("API server started", "address", listener.Addr().String())
	return nil
}

func (a *APIService) Stop(ctx context.Context) error {
	a.log.Info("Stopping API")

	if a.apiServer != nil {
		if err := a.apiServer.Shutdown(ctx); err != nil {
			return err
		}
	}

	return nil
}
