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

	"github.com/ethereum-optimism/optimism/op-service/httputil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/gorilla/mux"
)

func NewService(cfg Config, storageMgr *ethstorage.StorageManager, l1Beacon *eth.BeaconClient, l1Source *eth.PollingClient, lg log.Logger) *APIService {
	api := NewAPI(storageMgr, l1Beacon, l1Source, lg)
	return &APIService{
		cfg: cfg,
		api: api,
		lg:  lg,
	}
}

type APIService struct {
	lg        log.Logger
	cfg       Config
	apiServer *http.Server
	api       *API
}

// blobSidecarHandler implements the /eth/v1/beacon/blob_sidecars/{id} endpoint,
// deprecated since Fusaka
func (a *APIService) blobSidecarHandler(w http.ResponseWriter, r *http.Request) {

	a.lg.Info("Blob archiver API request", "from", readUserIP(r), "url", r.RequestURI)

	id := mux.Vars(r)["id"]
	if hErr := validateBlockID(id); hErr != nil {
		hErr.write(w)
		return
	}

	indices, hErr := parseIndices(r, a.cfg.MaxBlobsPerBlock)
	if hErr != nil {
		hErr.write(w)
		return
	}

	result, hErr := a.api.queryBlobSidecars(id, indices)
	if hErr != nil {
		hErr.write(w)
		return
	}

	if len(result.Data) == 0 {
		a.lg.Info("Not stored by EthStorage", "beaconID", id)
		errBlobNotInES.write(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	encodingErr := json.NewEncoder(w).Encode(result)
	if encodingErr != nil {
		a.lg.Error("Unable to encode blob sidecars to JSON", "err", encodingErr)
		errServerError.write(w)
		return
	}
}

// blobsHandler implements the /eth/v1/beacon/blobs/{id} endpoint
func (a *APIService) blobsHandler(w http.ResponseWriter, r *http.Request) {

	a.lg.Info("Blob archiver API request", "from", readUserIP(r), "url", r.RequestURI)

	id := mux.Vars(r)["id"]
	if hErr := validateBlockID(id); hErr != nil {
		hErr.write(w)
		return
	}

	hashes, hErr := parseVersionedHashes(r)
	if hErr != nil {
		hErr.write(w)
		return
	}

	result, hErr := a.api.queryBlobs(id, hashes)
	if hErr != nil {
		hErr.write(w)
		return
	}

	if len(result.Data) == 0 {
		a.lg.Info("Not stored by EthStorage", "beaconID", id)
		errBlobNotInES.write(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	encodingErr := json.NewEncoder(w).Encode(result)
	if encodingErr != nil {
		a.lg.Error("Unable to encode blob sidecars to JSON", "err", encodingErr)
		errServerError.write(w)
		return
	}
}

func (a *APIService) Start(ctx context.Context) error {
	a.lg.Debug("Starting blob archiver API service", "address", a.cfg.ListenAddr)
	endpoint := net.JoinHostPort(a.cfg.ListenAddr, strconv.Itoa(a.cfg.ListenPort))
	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		return err
	}
	r := mux.NewRouter()
	// Deprecated by Fusaka but still used by OP Stack
	r.HandleFunc("/eth/v1/beacon/blob_sidecars/{id}", a.blobSidecarHandler)
	// Fusaka
	r.HandleFunc("/eth/v1/beacon/blobs/{id}", a.blobsHandler)

	a.apiServer = httputil.NewHttpServer(r)
	go func() {
		if err := a.apiServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.lg.Error("Start blob archiver API service failed", "err", err)
		}
	}()
	a.lg.Info("Blob archiver API service started", "address", listener.Addr().String())
	return nil
}

func (a *APIService) Stop(ctx context.Context) {
	a.lg.Debug("Stopping blob archiver API service")
	if a.apiServer != nil {
		if err := a.apiServer.Shutdown(ctx); err != nil {
			a.lg.Error("Error stopping blob archiver API service", "err", err)
		}
	}
	a.lg.Info("Blob archiver API service stopped")
}
