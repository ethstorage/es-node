// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package downloader

type Config struct {
    DownloadStart int64 // which block should we download the blobs from
    DownloadDump  string // where to dump the download blobs
}
