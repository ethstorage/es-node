// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/httputil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/gorilla/mux"
)

func NewService(cfg Config, storageMgr *ethstorage.StorageManager, l1Beacon *eth.BeaconClient, l1Source *eth.PollingClient, l log.Logger) *APIService {
	api := NewAPI(storageMgr, l1Beacon, l1Source, l)
	return &APIService{
		cfg:    cfg,
		api:    api,
		logger: l,
	}
}

type APIService struct {
	logger    log.Logger
	cfg       Config
	apiServer *http.Server
	api       *API
}

// blobSidecarHandler implements the /eth/v1/beacon/blob_sidecars/{id} endpoint,
// but allows clients to fetch expired blobs.
func (a *APIService) blobSidecarHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func(start time.Time) {
		dur := time.Since(start)
		a.logger.Info("Blob archiver API request handled", "took(s)", dur.Seconds())
	}(start)

	a.logger.Info("Blob archiver API request", "url", r.RequestURI)
	id := mux.Vars(r)["id"]
	var indices []uint64
	indicesStr := r.URL.Query().Get("indices")
	if indicesStr != "" {
		splits := strings.Split(indicesStr, ",")
		for _, index := range splits {
			parsedInt, err := strconv.ParseUint(index, 10, 64)
			if err != nil {
				// beacon API will ignore invalid indices and return all blobs
				break
			}
			indices = append(indices, parsedInt)
		}
	}
	if hErr := validateBlockID(id); hErr != nil {
		hErr.write(w)
		return
	}
	result, hErr := a.api.queryBlobSidecars(id, indices)
	if hErr != nil {
		hErr.write(w)
		return
	}

	if len(result.Data) == 0 {
		a.logger.Info("Not stored by EthStorage", "beaconID", id)
		errBlobNotInES.write(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	encodingErr := json.NewEncoder(w).Encode(result)
	if encodingErr != nil {
		a.logger.Error("Unable to encode blob sidecars to JSON", "err", encodingErr)
		errServerError.write(w)
		return
	}
}

func (a *APIService) Start(ctx context.Context) error {
	a.logger.Debug("Starting blob archiver API service", "address", a.cfg.ListenAddr)
	endpoint := net.JoinHostPort(a.cfg.ListenAddr, strconv.Itoa(a.cfg.ListenPort))
	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		return err
	}
	r := mux.NewRouter()
	r.HandleFunc("/eth/v1/beacon/blob_sidecars/{id}", a.blobSidecarHandler)

	a.apiServer = httputil.NewHttpServer(r)
	go func() {
		if err := a.apiServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("Start blob archiver API service failed", "err", err)
		}
	}()
	a.logger.Info("Blob archiver API service started", "address", listener.Addr().String())
	return nil
}

func (a *APIService) Stop(ctx context.Context) {
	a.logger.Debug("Stopping blob archiver API service")
	if a.apiServer != nil {
		if err := a.apiServer.Shutdown(ctx); err != nil {
			a.logger.Error("Error stopping blob archiver API service", "err", err)
		}
	}
	a.logger.Info("Blob archiver API service stopped")
}
