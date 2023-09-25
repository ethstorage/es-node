// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package prover

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func Test_MerkleProver(test *testing.T) {
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
			expectedRoot:   common.HexToHash("0x5b1ece23efcb64436060f1196af7f4e563f88580317a80219f059be0143c77f7"),
			chunkProofs: []expectedProofsForChunk{
				{
					chunkIdx:       uint64(0),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(1),
					expectedProofs: []common.Hash{common.HexToHash("0x9edfefee6a285de13826a2f33d0056539b801642d4955a202c46835bfcad0c02"), common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(31),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0x310954b48993f8691a5c56b2ccd12b3b961034c242fee15fd214010292a0a0e9")},
				},
			},
		},
		{
			data:           append(make([]byte, 4096), []byte{1, 1, 1, 1}...),
			chunkPerKVBits: uint64(5),
			chunkSize:      uint64(4096),
			expectedRoot:   common.HexToHash("0x2fb98e5ba32e13328ffe482f6941e50916b15a98bbef192fec7914ba9a6679cb"),
			chunkProofs: []expectedProofsForChunk{
				{
					chunkIdx:       uint64(0),
					expectedProofs: []common.Hash{common.HexToHash("0x9edfefee6a285de13826a2f33d0056539b801642d4955a202c46835bfcad0c02"), common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(1),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(2),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0x1318c29a10619d9f0fb25d52cff12a9daef407ec925c73f272e54c668ace058a"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(31),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0xdc93eb972e7d0c74ad1e0f2b7ca67047dcf7af893f279a842d19ff0bc6529ea7")},
				},
			},
		},
		{
			data:           append(make([]byte, 4096*2), []byte{1, 1, 1, 1}...),
			chunkPerKVBits: uint64(5),
			chunkSize:      uint64(4096),
			expectedRoot:   common.HexToHash("0x97127aaf96faa87f2fea4a7b8e23775a7975e009650f1647d549240c359db7b3"),
			chunkProofs: []expectedProofsForChunk{
				{
					chunkIdx:       uint64(0),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x3f6b60a21968bb0bffe0588efabc9e728eda5e90afeba45d56492f9d4dd90ba5"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(1),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x3f6b60a21968bb0bffe0588efabc9e728eda5e90afeba45d56492f9d4dd90ba5"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(2),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(3),
					expectedProofs: []common.Hash{common.HexToHash("0x9edfefee6a285de13826a2f33d0056539b801642d4955a202c46835bfcad0c02"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(31),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0x7d8be77dfe322d6a8bbe19049367f03dd99687bcd59f3429df29e76dabcf8c69")},
				},
			},
		},
		{
			data:           append(make([]byte, 4096*10), []byte{1, 1, 1, 1}...),
			chunkPerKVBits: uint64(5),
			chunkSize:      uint64(4096),
			expectedRoot:   common.HexToHash("0x43dd8637b351884c4b69aaabaa2de07cb514c5f5043d7ac5afd1e8ffaa937f2d"),
			chunkProofs: []expectedProofsForChunk{
				{
					chunkIdx:       uint64(0),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0x15216cfd3591a782767836737dbd61cc8e4011971abe750bb8569b11f0651ec1"), common.HexToHash("0x94bd48c9ae8d3859dd4fa2650dd966ba29cb89eb952958ea1ec0f516defeb7c7"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(1),
					expectedProofs: []common.Hash{common.HexToHash("0xa8bae11751799de4dbe638406c5c9642c0e791f2a65e852a05ba4fdf0d88e3e6"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0x15216cfd3591a782767836737dbd61cc8e4011971abe750bb8569b11f0651ec1"), common.HexToHash("0x94bd48c9ae8d3859dd4fa2650dd966ba29cb89eb952958ea1ec0f516defeb7c7"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(10),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0x25cda0619b5fab4aa01fc080d4ea4cd4bbc2009bccf50c81a5daacb15ce78c62"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0xc39e39502196b06ae000fb1d95851ab467f4a69727e497d24487bb8322ba84f2"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(15),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"), common.HexToHash("0xf52716afaf825c4ab08b3edf97e783ec898efda7a1d0d175f989a3ba7ba11c0c"), common.HexToHash("0xc39e39502196b06ae000fb1d95851ab467f4a69727e497d24487bb8322ba84f2"), common.HexToHash("0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344")},
				},
				{
					chunkIdx:       uint64(31),
					expectedProofs: []common.Hash{common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"), common.HexToHash("0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"), common.HexToHash("0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85"), common.HexToHash("0x9ea6831d655dcc710a768112da04b15a00d794c581193632eb5b48c945741f08")},
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

	prover := MerkleProver{}
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

			root, err = prover.GetRootWithProof(hash, stc.chunkIdx, stc.expectedProofs)
			if err != nil {
				test.Error(err.Error())
			}
			if root != tc.expectedRoot {
				test.Error("get root with proof fail", "expected root", tc.expectedRoot.Hex(), "root", root.Hex())
			}
		}
	}
}
