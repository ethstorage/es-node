// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package db

type Config struct {
	DatabaseHandles int `toml:"-"`
	DatabaseCache   int
	DatabaseFreezer string
	NameSpace       string
	Name            string
}

func DefaultDBConfig() *Config {
	return &Config{
		DatabaseHandles: 8196,
		DatabaseCache:   2048,
		DatabaseFreezer: "",
		NameSpace:       "eth/db/ethstoragedata/",
		Name:            "ethstoragedata",
	}
}
