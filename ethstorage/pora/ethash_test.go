// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package pora

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestMaskData(t *testing.T) {
	fullMask := GetMaskData(0, common.Hash{}, 128, nil)
	partialMask := GetMaskData(0, common.Hash{}, 7, nil)
	if !bytes.Equal(fullMask[0:len(partialMask)], partialMask) {
		t.Errorf("partial mask data wrong")
	}

	fullMask = GetMaskData(0, common.Hash{}, 4096, nil)
	partialMask = GetMaskData(0, common.Hash{}, 4095, nil)
	if !bytes.Equal(fullMask[0:len(partialMask)], partialMask) {
		t.Errorf("partial mask data wrong")
	}
}
