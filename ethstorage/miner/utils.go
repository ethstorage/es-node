// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

/*
	This ABI is generated via the following cmd under the root dir of storage-contracts-v1 repo:
	```bash
		find out -name '*.json' -type f -print0 \
		| xargs -0 jq -r -s 'map(.abi // []) | add
		| map(select(.type=="error") | {name, type, inputs})
		| unique_by(.name)
		| sort_by(.name)
		| "[\n  " + (map(@json) | join(",\n  ")) + "\n]"'
	```
*/

// From storage contract version 0.2.0-9eeaf63
const errorABI = `[
  {"name":"AccessControlBadConfirmation","type":"error","inputs":[]},
  {"name":"AccessControlUnauthorizedAccount","type":"error","inputs":[{"name":"account","type":"address","internalType":"address"},{"name":"neededRole","type":"bytes32","internalType":"bytes32"}]},
  {"name":"AddressEmptyCode","type":"error","inputs":[{"name":"target","type":"address","internalType":"address"}]},
  {"name":"BeaconInvalidImplementation","type":"error","inputs":[{"name":"implementation","type":"address","internalType":"address"}]},
  {"name":"DecentralizedKV_BeyondRangeOfKVSize","type":"error","inputs":[]},
  {"name":"DecentralizedKV_DataLenZero","type":"error","inputs":[]},
  {"name":"DecentralizedKV_DataNotExist","type":"error","inputs":[]},
  {"name":"DecentralizedKV_DataTooLarge","type":"error","inputs":[]},
  {"name":"DecentralizedKV_GetMustBeCalledOnESNode","type":"error","inputs":[]},
  {"name":"DecentralizedKV_NotEnoughBatchPayment","type":"error","inputs":[]},
  {"name":"DecentralizedKV_RemoveToUnimplemented","type":"error","inputs":[]},
  {"name":"ERC1967InvalidAdmin","type":"error","inputs":[{"name":"admin","type":"address","internalType":"address"}]},
  {"name":"ERC1967InvalidBeacon","type":"error","inputs":[{"name":"beacon","type":"address","internalType":"address"}]},
  {"name":"ERC1967InvalidImplementation","type":"error","inputs":[{"name":"implementation","type":"address","internalType":"address"}]},
  {"name":"ERC1967NonPayable","type":"error","inputs":[]},
  {"name":"EthStorageContractM1L2_NotEnoughPayment","type":"error","inputs":[]},
  {"name":"EthStorageContractM1_DecodeSampleFailed","type":"error","inputs":[]},
  {"name":"EthStorageContractM1_InvalidSamples","type":"error","inputs":[]},
  {"name":"EthStorageContractM1_LengthMismatch","type":"error","inputs":[]},
  {"name":"EthStorageContractM2L2_NotEnoughPayment","type":"error","inputs":[]},
  {"name":"EthStorageContractM2_DecodeFailed","type":"error","inputs":[]},
  {"name":"EthStorageContractM2_InvalidSamples","type":"error","inputs":[]},
  {"name":"EthStorageContractM2_LengthMismatch","type":"error","inputs":[]},
  {"name":"EthStorageContract_FailedToGetBlobHash","type":"error","inputs":[]},
  {"name":"EthStorageContract_LengthMismatch","type":"error","inputs":[]},
  {"name":"FailedCall","type":"error","inputs":[]},
  {"name":"FailedDeployment","type":"error","inputs":[]},
  {"name":"InsufficientBalance","type":"error","inputs":[{"name":"balance","type":"uint256","internalType":"uint256"},{"name":"needed","type":"uint256","internalType":"uint256"}]},
  {"name":"InvalidInitialization","type":"error","inputs":[]},
  {"name":"L2Base_ExceedsUpdateRateLimit","type":"error","inputs":[]},
  {"name":"L2Base_FailedObtainBlockhash","type":"error","inputs":[]},
  {"name":"MissingPrecompile","type":"error","inputs":[{"name":"","type":"address","internalType":"address"}]},
  {"name":"NotInitializing","type":"error","inputs":[]},
  {"name":"OwnableInvalidOwner","type":"error","inputs":[{"name":"owner","type":"address","internalType":"address"}]},
  {"name":"OwnableUnauthorizedAccount","type":"error","inputs":[{"name":"account","type":"address","internalType":"address"}]},
  {"name":"ProxyDeniedAdminAccess","type":"error","inputs":[]},
  {"name":"RandaoLib_HeaderHashMismatch","type":"error","inputs":[]},
  {"name":"SafeCastOverflowedIntDowncast","type":"error","inputs":[{"name":"bits","type":"uint8","internalType":"uint8"},{"name":"value","type":"int256","internalType":"int256"}]},
  {"name":"SafeCastOverflowedIntToUint","type":"error","inputs":[{"name":"value","type":"int256","internalType":"int256"}]},
  {"name":"SafeCastOverflowedUintDowncast","type":"error","inputs":[{"name":"bits","type":"uint8","internalType":"uint8"},{"name":"value","type":"uint256","internalType":"uint256"}]},
  {"name":"SafeCastOverflowedUintToInt","type":"error","inputs":[{"name":"value","type":"uint256","internalType":"uint256"}]},
  {"name":"StorageContract_AtLeastOneCheckpointNeeded","type":"error","inputs":[]},
  {"name":"StorageContract_BlockNumberTooOld","type":"error","inputs":[]},
  {"name":"StorageContract_DifficultyNotMet","type":"error","inputs":[]},
  {"name":"StorageContract_FailedToObtainBlockhash","type":"error","inputs":[]},
  {"name":"StorageContract_MaxKvSizeTooSmall","type":"error","inputs":[]},
  {"name":"StorageContract_MinedTsTooSmall","type":"error","inputs":[]},
  {"name":"StorageContract_MinerNotWhitelisted","type":"error","inputs":[]},
  {"name":"StorageContract_NonceTooBig","type":"error","inputs":[]},
  {"name":"StorageContract_NotEnoughBalance","type":"error","inputs":[]},
  {"name":"StorageContract_NotEnoughBatchPayment","type":"error","inputs":[]},
  {"name":"StorageContract_NotEnoughMinerReward","type":"error","inputs":[]},
  {"name":"StorageContract_NotEnoughPrepaidAmount","type":"error","inputs":[]},
  {"name":"StorageContract_ReentrancyAttempt","type":"error","inputs":[]},
  {"name":"StorageContract_ShardSizeTooSmall","type":"error","inputs":[]},
  {"name":"StringsInsufficientHexLength","type":"error","inputs":[{"name":"value","type":"uint256","internalType":"uint256"},{"name":"length","type":"uint256","internalType":"uint256"}]},
  {"name":"StringsInvalidAddressFormat","type":"error","inputs":[]},
  {"name":"StringsInvalidChar","type":"error","inputs":[]}
]`

var (
	ab abi.ABI
)

func init() {
	var err error
	ab, err = abi.JSON(strings.NewReader(errorABI))
	if err != nil {
		panic(fmt.Errorf("invalid storage ABI (errors): %w", err))
	}
}

func parseErr(err error) string {
	errMsg := err.Error()
	var dataHex string
	if rpcErr, ok := err.(rpc.DataError); ok {
		if s, ok := rpcErr.ErrorData().(string); ok {
			dataHex = s
		}
	}
	if dataHex == "" {
		return fmt.Sprintf("%s: no revert data", errMsg)
	}
	errMsg += fmt.Sprintf(": ErrorData: %v", dataHex)
	data, _ := hex.DecodeString(strings.TrimPrefix(dataHex, "0x"))
	if len(data) < 4 {
		return fmt.Sprintf("%s: invalid revert data", errMsg)
	}
	sel := [4]byte{data[0], data[1], data[2], data[3]}
	abiErr, err1 := ab.ErrorByID(sel)
	if err1 != nil {
		return fmt.Sprintf("%s: failed to parse revert data (id=0x%s): %s", errMsg, hex.EncodeToString(sel[:]), err1.Error())
	}
	if len(abiErr.Inputs) == 0 || len(data) == 4 {
		return fmt.Sprintf("%s: %s()", errMsg, abiErr.Name)
	}
	args, err2 := abiErr.Unpack(data[4:])
	if err2 != nil {
		return fmt.Sprintf("%s: %s: failed to parse args: %s", errMsg, abiErr, err2.Error())
	}
	return fmt.Sprintf("%s: %s(%v)", errMsg, abiErr.Name, args)
}

// https://github.com/ethereum/go-ethereum/issues/21221#issuecomment-805852059
func weiToEther(wei *big.Int) *big.Float {
	f := new(big.Float)
	f.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	f.SetMode(big.ToNearestEven)
	if wei == nil {
		return f.SetInt64(0)
	}
	fWei := new(big.Float)
	fWei.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	fWei.SetMode(big.ToNearestEven)
	return f.Quo(fWei.SetInt(wei), big.NewFloat(params.Ether))
}

func fmtEth(wei *big.Int) string {
	f := weiToEther(wei)
	// trim the tailing zeros
	s := f.Text('f', 18)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		s = "0"
	}
	return s
}

func fmtGwei(gasPrice *big.Int) string {
	// if gasPrice lower than 1 Gwei, show in decimal
	if gasPrice.Cmp(big.NewInt(params.GWei)) < 0 {
		f := new(big.Float).SetPrec(236)
		f.SetMode(big.ToNearestEven)
		f.Quo(f.SetInt(gasPrice), big.NewFloat(params.GWei))
		return fmt.Sprintf("%.9f", f)
	}
	// otherwise show in integer
	return fmt.Sprintf("%d", new(big.Int).Div(gasPrice, big.NewInt(params.GWei)))
}
