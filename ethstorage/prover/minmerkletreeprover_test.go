// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func Test_MinMerkleTreeProver(test *testing.T) {
	type expectedProofsForChunk struct {
		chunkIdx       uint64
		expectedProofs []common.Hash
	}

	type testCase struct {
		data           []byte
		chunkPerKVBits uint64
		chunkSize      uint64
		expectedRoot   common.Hash
		chunkProofs    []expectedProofsForChunk
	}

	testCases := []testCase{
		{
			data:           make([]byte, 0),
			chunkPerKVBits: uint64(5),
			chunkSize:      uint64(4096),
			expectedRoot:   common.Hash{},
			chunkProofs: []expectedProofsForChunk{
				{
					chunkIdx:       uint64(0),
					expectedProofs: make([]common.Hash, 0),
				},
				{
					chunkIdx:       uint64(1),
					expectedProofs: make([]common.Hash, 0),
				},
				{
					chunkIdx:       uint64(31),
					expectedProofs: make([]common.Hash, 0),
				},
			},
		},
		{
			data:           []byte{1, 1, 1, 1},
			chunkPerKVBits: uint64(5),
			chunkSize:      uint64(4096),
			expectedRoot:   common.HexToHash("0x9edfefee6a285de13826a2f33d0056539b801642d4955a202c46835bfcad0c02"),
			chunkProofs: []expectedProofsForChunk{
				{
					chunkIdx:       uint64(0),
					expectedProofs: make([]common.Hash, 0),
				},
				{
					chunkIdx:       uint64(1),
					expectedProofs: make([]common.Hash, 0),
				},
				{
					chunkIdx:       uint64(31),
					expectedProofs: make([]common.Hash, 0),
				},
			},
		},
		{
			data:           append(make([]byte, 4096), []byte{1, 1, 1, 1}...),
			chunkPerKVBits: uint64(5),
			chunkSize:      uint64(4096),
			expectedRoot:   common.HexToHash("0x1318c29a10619d9f0fb25d52cff12a9daef407ec925c73f272e54c668ace058a"),
			chunkProofs: []expectedProofsForChunk{
				{
					chunkIdx:       uint64(0),
					expectedProofs: []common.Hash{common.HexToHash("0x9edfefee6a285de13826a2f33d0056539b801642d4955a202c46835bfcad0c02")},
				},
				{
					chunkIdx:       uint64(1),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6")},
				},
				{
					chunkIdx:       uint64(2),
					expectedProofs: make([]common.Hash, 0),
				},
				{
					chunkIdx:       uint64(31),
					expectedProofs: make([]common.Hash, 0),
				},
			},
		},
		{
			data:           append(make([]byte, 4096*2), []byte{1, 1, 1, 1}...),
			chunkPerKVBits: uint64(5),
			chunkSize:      uint64(4096),
			expectedRoot:   common.HexToHash("0xf52716afaf825c4ab08b3edf97e783ec898efda7a1d0d175f989a3ba7ba11c0c"),
			chunkProofs: []expectedProofsForChunk{
				{
					chunkIdx:       uint64(0),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x3f6b60a21968bb0bffe0588efabc9e728eda5e90afeba45d56492f9d4dd90ba5")},
				},
				{
					chunkIdx:       uint64(1),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x3f6b60a21968bb0bffe0588efabc9e728eda5e90afeba45d56492f9d4dd90ba5")},
				},
				{
					chunkIdx:       uint64(2),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62")},
				},
				{
					chunkIdx:       uint64(3),
					expectedProofs: []common.Hash{common.HexToHash("0x9edfefee6a285de13826a2f33d0056539b801642d4955a202c46835bfcad0c02"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62")},
				},
				{
					chunkIdx:       uint64(31),
					expectedProofs: make([]common.Hash, 0),
				},
			},
		},
		{
			data:           append(make([]byte, 4096*10), []byte{1, 1, 1, 1}...),
			chunkPerKVBits: uint64(5),
			chunkSize:      uint64(4096),
			expectedRoot:   common.HexToHash("0x9ea6831d655dcc710a768112da04b15a00d794c581193632eb5b48c945741f08"),
			chunkProofs: []expectedProofsForChunk{
				{
					chunkIdx:       uint64(0),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0x15216cfd3591a782767836737dbd61cc8e4011971abe750bb8569b11f0651ec1"), common.HexToHash("0x94bd48c9ae8d3859dd4fa2650dd966ba29cb89eb952958ea1ec0f516defeb7c7")},
				},
				{
					chunkIdx:       uint64(1),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0x15216cfd3591a782767836737dbd61cc8e4011971abe750bb8569b11f0651ec1"), common.HexToHash("0x94bd48c9ae8d3859dd4fa2650dd966ba29cb89eb952958ea1ec0f516defeb7c7")},
				},
				{
					chunkIdx:       uint64(10),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0xc39e39502196b06ae000fb1d95851ab467f4a69727e497d24487bb8322ba84f2")},
				},
				{
					chunkIdx:       uint64(15),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"), common.HexToHash("0xf52716afaf825c4ab08b3edf97e783ec898efda7a1d0d175f989a3ba7ba11c0c"), common.HexToHash("0xc39e39502196b06ae000fb1d95851ab467f4a69727e497d24487bb8322ba84f2")},
				},
				{
					chunkIdx:       uint64(31),
					expectedProofs: make([]common.Hash, 0),
				},
			},
		},
		{
			data:           append(make([]byte, 4096*31), []byte{1, 1, 1, 1}...),
			chunkPerKVBits: uint64(5),
			chunkSize:      uint64(4096),
			expectedRoot:   common.HexToHash("0xf6692599b6a2fef3dd4e59d9d3cf71967051eaee9d52951a9ddf29b09aedf8c0"),
			chunkProofs: []expectedProofsForChunk{
				{
					chunkIdx:       uint64(0),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0x15216cfd3591a782767836737dbd61cc8e4011971abe750bb8569b11f0651ec1"), common.HexToHash("0xc39e39502196b06ae000fb1d95851ab467f4a69727e497d24487bb8322ba84f2"), common.HexToHash("0x8ce139afc995bc9df8a240625aa2d95aacb82015307ac611b920a97b007caa63")},
				},
				{
					chunkIdx:       uint64(1),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0x15216cfd3591a782767836737dbd61cc8e4011971abe750bb8569b11f0651ec1"), common.HexToHash("0xc39e39502196b06ae000fb1d95851ab467f4a69727e497d24487bb8322ba84f2"), common.HexToHash("0x8ce139afc995bc9df8a240625aa2d95aacb82015307ac611b920a97b007caa63")},
				},
				{
					chunkIdx:       uint64(2),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0x15216cfd3591a782767836737dbd61cc8e4011971abe750bb8569b11f0651ec1"), common.HexToHash("0xc39e39502196b06ae000fb1d95851ab467f4a69727e497d24487bb8322ba84f2"), common.HexToHash("0x8ce139afc995bc9df8a240625aa2d95aacb82015307ac611b920a97b007caa63")},
				},
				{
					chunkIdx:       uint64(30),
					expectedProofs: []common.Hash{common.HexToHash("0x9edfefee6a285de13826a2f33d0056539b801642d4955a202c46835bfcad0c02"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0x15216cfd3591a782767836737dbd61cc8e4011971abe750bb8569b11f0651ec1"), common.HexToHash("0xc39e39502196b06ae000fb1d95851ab467f4a69727e497d24487bb8322ba84f2"), common.HexToHash("0xbf76e6bed66c470fb221dad020fc920c47abe5c3a9b6e8bc28865229f3a47e10")},
				},
				{
					chunkIdx:       uint64(31),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0x15216cfd3591a782767836737dbd61cc8e4011971abe750bb8569b11f0651ec1"), common.HexToHash("0xc39e39502196b06ae000fb1d95851ab467f4a69727e497d24487bb8322ba84f2"), common.HexToHash("0xbf76e6bed66c470fb221dad020fc920c47abe5c3a9b6e8bc28865229f3a47e10")},
				},
			},
		},
	}

	prover := MinMerkleTreeProver{}
	for _, tc := range testCases {
		root := prover.GetRoot(tc.data, 1<<tc.chunkPerKVBits, tc.chunkSize)
		if root != tc.expectedRoot {
			test.Error("get root fail", "expected root", tc.expectedRoot.Hex(), "root", root.Hex())
		}
		for _, stc := range tc.chunkProofs {
			proofs, err := prover.GetProof(tc.data, tc.chunkPerKVBits, stc.chunkIdx, tc.chunkSize)
			if err != nil {
				test.Error(err.Error())
			}
			if len(proofs) != len(stc.expectedProofs) {
				test.Errorf("len of proofs is different from len of expectedProofs, proofs %v, expectedProofs %v", proofs, stc.expectedProofs)
			}
			for i, proof := range proofs {
				if proof != stc.expectedProofs[i] {
					test.Error("get root fail", "expected proof", stc.expectedProofs[i].Hex(), "proof", proof.Hex())
				}
			}
			hash := common.Hash{}
			if uint64(len(tc.data)) > stc.chunkIdx*tc.chunkSize {
				data := tc.data[stc.chunkIdx*tc.chunkSize:]
				if uint64(len(data)) > tc.chunkSize {
					data = data[:tc.chunkSize]
				}
				hash = crypto.Keccak256Hash(data)
			}

			m, _ := findNChunk(uint64(len(tc.data)), tc.chunkSize)
			if m > stc.chunkIdx {
				root, err = prover.GetRootWithProof(hash, stc.chunkIdx, stc.expectedProofs)
				if err != nil {
					test.Error(err.Error())
				}
				if root != tc.expectedRoot {
					test.Error("get root with proof fail", "chunk idx", stc.chunkIdx, "expected root", tc.expectedRoot.Hex(), "root", root.Hex())
				}
			} else if bytes.Compare(hash.Bytes(), make([]byte, 32)) != 0 {
				test.Error("get root with proof fail", "chunk idx", stc.chunkIdx, "hash", hash.Hex())
			}
		}
	}
}
