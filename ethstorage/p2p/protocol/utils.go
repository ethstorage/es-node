// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/golang/snappy"
	"github.com/libp2p/go-libp2p/core/network"
)

const (
	streamError = 254
	clientError = 255
)

func WriteMsg(stream network.Stream, msg *Msg) error {
	_ = stream.SetWriteDeadline(time.Now().Add(clientWriteRequestTimeout))
	// write return code
	n, err := stream.Write([]byte{msg.ReturnCode})
	if err != nil {
		return err
	}

	w := snappy.NewBufferedWriter(stream)
	// write msg size
	sizeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(sizeBytes, uint32(len(msg.Payload)))
	n, err = w.Write(sizeBytes)
	if err != nil {
		return err
	}
	if n != len(sizeBytes) {
		return fmt.Errorf("not fully write")
	}

	// write page
	n, err = w.Write(msg.Payload)
	if err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to finishing writing payload to sync response: %w", err)
	}
	return nil
}

func ReadMsg(stream network.Stream) ([]byte, byte, error) {
	_ = stream.SetReadDeadline(time.Now().Add(clientReadResponseTimeout))
	var returnCode [1]byte
	if _, err := io.ReadFull(stream, returnCode[:]); err != nil {
		return nil, clientError, fmt.Errorf("failed to read result part of response: %w", err)
	}
	code := returnCode[0]
	if code != 0 {
		return nil, code, requestResultErr(code)
	}

	var r io.Reader = snappy.NewReader(stream)
	r = io.LimitReader(r, maxGossipSize)
	sizeBytes := make([]byte, 4)
	_, err := io.ReadFull(r, sizeBytes)
	if err != nil {
		return nil, code, err
	}

	size := binary.BigEndian.Uint32(sizeBytes)

	payload := make([]byte, size)
	_, err = io.ReadFull(r, payload)
	if err := stream.CloseRead(); err != nil {
		return nil, code, fmt.Errorf("failed to close reading side")
	}
	return payload, code, err
}

func Send(stream network.Stream, req interface{}) (network.Stream, error) {
	data, err := rlp.EncodeToBytes(req)
	if err != nil {
		return nil, err
	}

	err = WriteMsg(stream, &Msg{0, data})
	if err != nil {
		stream.Close()
		return nil, err
	}

	if err := stream.CloseWrite(); err != nil {
		stream.Reset()
		return nil, err
	}
	return stream, err
}

func SendRPC(stream network.Stream, req interface{}, resp interface{}) (byte, error) {
	s, err := Send(stream, req)
	if err != nil {
		return clientError, err
	}

	msg, returnCode, err := ReadMsg(s)
	if err != nil {
		return returnCode, err
	}

	return returnCode, rlp.DecodeBytes(msg, resp)
}

func ConvertToContractShards(shards map[common.Address][]uint64) []*ContractShards {
	cs := make([]*ContractShards, 0)
	for contract, shardIds := range shards {
		cs = append(cs, &ContractShards{contract, shardIds})
	}
	return cs
}

func ConvertToShardList(css []*ContractShards) map[common.Address][]uint64 {
	shards := make(map[common.Address][]uint64)
	if css != nil {
		for _, cs := range css {
			shards[cs.Contract] = cs.ShardIds
		}
	}
	return shards
}
