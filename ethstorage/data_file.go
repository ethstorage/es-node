// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package ethstorage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/detailyang/go-fallocate"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ncw/directio"
)

const (
	NO_ENCODE = iota
	ENCODE_KECCAK_256
	ENCODE_ETHASH
	ENCODE_BLOB_POSEIDON
	ENCODE_END = ENCODE_BLOB_POSEIDON

	// keccak256(b'Web3Q Large Storage')[0:8]
	MAGIC   = uint64(0xcf20bd770c22b2e1)
	VERSION = uint64(1)

	HEADER_SIZE = 4096
)

// A DataFile represents a local file for a consective chunks
type DataFile struct {
	file          *os.File
	chunkIdxStart uint64
	chunkIdxLen   uint64
	encodeType    uint64
	maxKvSize     uint64
	chunkSize     uint64
	metaSize      uint64         // per KV meta size (like commit)
	miner         common.Address // storage provider key
}

type DataFileHeader struct {
	magic         uint64
	version       uint64
	chunkIdxStart uint64
	chunkIdxLen   uint64
	encodeType    uint64
	maxKvSize     uint64
	chunkSize     uint64
	metaSize      uint64
	miner         common.Address
	status        uint64
}

// Mask the data in place.  Padding zeros to userData if the len of userData is smaller than that of maskData,
func MaskDataInPlace(maskData []byte, userData []byte) []byte {
	if len(userData) > len(maskData) {
		panic("user data can not be larger than mask data")
	}
	for i := 0; i < len(userData); i++ {
		maskData[i] = maskData[i] ^ userData[i]
	}
	return maskData
}

// Unmask the data in place
func UnmaskDataInPlace(userData []byte, maskData []byte) []byte {
	if len(userData) > len(maskData) {
		panic("user data can not be larger than mask data")
	}
	for i := 0; i < len(userData); i++ {
		maskData[i] = maskData[i] ^ userData[i]
	}
	return maskData[:len(userData)]
}

func Create(filename string, chunkIdxStart, chunkIdxLen, epoch, maxKvSize, encodeType uint64, miner common.Address, chunkSize uint64) (*DataFile, error) {
	if chunkSize > maxKvSize {
		return nil, fmt.Errorf("chunkSize must be smaller than maxKvSize")
	}
	if (chunkIdxLen*chunkSize)%maxKvSize != 0 {
		return nil, fmt.Errorf("chunkSize * chunkIdxLen must be multiple of maxKvSize")
	}
	if (chunkIdxStart*chunkSize)%maxKvSize != 0 {
		return nil, fmt.Errorf("chunkSize * chunkIdxStart must be multiple of maxKvSize")
	}
	if !isPow2n(chunkSize) || !isPow2n(maxKvSize) {
		return nil, fmt.Errorf("chunkSize and maxKvSize must be 2^n")
	}

	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}
	// actual initialization is done when synchronize
	err = fallocate.Fallocate(file, int64((chunkSize+32)*chunkIdxLen), int64(HEADER_SIZE))
	if err != nil {
		return nil, err
	}
	dataFile := &DataFile{
		file:          file,
		chunkIdxStart: chunkIdxStart,
		chunkIdxLen:   chunkIdxLen,
		encodeType:    encodeType,
		maxKvSize:     maxKvSize,
		miner:         miner,
		chunkSize:     chunkSize,
		metaSize:      32,
	}
	dataFile.writeHeader()
	return dataFile, nil
}

func OpenDataFile(filename string) (*DataFile, error) {
	file, err := os.OpenFile(filename, os.O_RDWR, 0755)
	if err != nil {
		return nil, err
	}
	dataFile := &DataFile{
		file: file,
	}
	return dataFile, dataFile.readHeader()
}

func OpenDataFileDirectIO(filename string) (*DataFile, error) {
	file, err := directio.OpenFile(filename, os.O_RDWR, 0755)
	if err != nil {
		return nil, err
	}
	dataFile := &DataFile{
		file: file,
	}
	return dataFile, dataFile.readHeader()
}

func (df *DataFile) Contains(chunkIdx uint64) bool {
	return chunkIdx >= df.chunkIdxStart && chunkIdx < df.ChunkIdxEnd()
}

func (df *DataFile) ContainsKv(kvIdx uint64) bool {
	return kvIdx >= df.KvIdxStart() && kvIdx < df.KvIdxEnd()
}

func (df *DataFile) ContainsSample(sampleIdx uint64) bool {
	return df.Contains(sampleIdx * 32 / df.chunkSize)
}

func (df *DataFile) ChunkIdxEnd() uint64 {
	return df.chunkIdxStart + df.chunkIdxLen
}

func (df *DataFile) KvIdxEnd() uint64 {
	return df.KvIdxStart() + df.chunkIdxLen*df.chunkSize/df.maxKvSize
}

func (df *DataFile) KvIdxStart() uint64 {
	return df.chunkIdxStart * df.chunkSize / df.maxKvSize
}

func (df *DataFile) Miner() common.Address {
	return df.miner
}

// Read raw chunk data from the storage file.
func (df *DataFile) Read(chunkIdx uint64, len int) ([]byte, error) {
	if !df.Contains(chunkIdx) {
		return nil, fmt.Errorf("chunk not found")
	}
	if len > int(df.chunkSize) {
		return nil, fmt.Errorf(("read too large"))
	}
	md := make([]byte, len)
	n, err := df.file.ReadAt(md, HEADER_SIZE+int64(chunkIdx-df.chunkIdxStart)*int64(df.chunkSize))
	if err != nil {
		return nil, err
	}
	if n != len {
		return nil, fmt.Errorf("not full read")
	}
	return md, nil
}

// Read raw chunk data from the storage file.
func (df *DataFile) ReadSample(sampleIdx uint64) (common.Hash, error) {
	if !df.ContainsSample(sampleIdx) {
		return common.Hash{}, fmt.Errorf("sample not found")
	}

	md := make([]byte, 32)
	n, err := df.file.ReadAt(md, HEADER_SIZE+int64(sampleIdx*32)-int64(df.chunkIdxStart*df.chunkSize))
	if err != nil {
		return common.Hash{}, err
	}
	if n != 32 {
		return common.Hash{}, fmt.Errorf("not full read")
	}
	return common.BytesToHash(md), nil
}

// Read raw sample data from the storage file using directIO.
func (df *DataFile) ReadSampleDirectly(sampleIdx uint64) (common.Hash, error) {
	if !df.ContainsSample(sampleIdx) {
		return common.Hash{}, fmt.Errorf("sample not found")
	}
	md := directio.AlignedBlock(directio.BlockSize)
	n, err := df.file.ReadAt(md, HEADER_SIZE+int64(sampleIdx*32)-int64(df.chunkIdxStart*df.chunkSize))
	if err != nil {
		return common.Hash{}, err
	}
	if n != directio.BlockSize {
		return common.Hash{}, fmt.Errorf("not full read")
	}
	return common.BytesToHash(md[:32]), nil
}

// Write the chunk bytes to the file.
func (df *DataFile) Write(chunkIdx uint64, b []byte) error {
	if !df.Contains(chunkIdx) {
		return fmt.Errorf("chunk not found")
	}

	if len(b) > int(df.chunkSize) {
		return fmt.Errorf("write data too large")
	}

	_, err := df.file.WriteAt(b, HEADER_SIZE+int64(chunkIdx-df.chunkIdxStart)*int64(df.chunkSize))
	return err
}

// Read the metadata of the kv
func (df *DataFile) ReadMeta(kvIdx uint64) ([]byte, error) {
	if !df.ContainsKv(kvIdx) {
		return nil, fmt.Errorf("kv not found")
	}

	b := make([]byte, df.metaSize)
	_, err := df.file.ReadAt(b, int64(HEADER_SIZE+df.chunkIdxLen*df.chunkSize+(kvIdx-df.KvIdxStart())*df.metaSize))
	return b, err
}

// Write the metadata of the kv
func (df *DataFile) WriteMeta(kvIdx uint64, b []byte) error {
	if !df.ContainsKv(kvIdx) {
		return fmt.Errorf("kv not found")
	}

	if len(b) > int(df.metaSize) {
		return fmt.Errorf("write meta too large")
	}

	_, err := df.file.WriteAt(b, int64(HEADER_SIZE+df.chunkIdxLen*df.chunkSize+(kvIdx-df.KvIdxStart())*df.metaSize))
	return err
}

func (df *DataFile) writeHeader() error {
	header := DataFileHeader{
		magic:         MAGIC,
		version:       VERSION,
		chunkIdxStart: df.chunkIdxStart,
		chunkIdxLen:   df.chunkIdxLen,
		encodeType:    df.encodeType,
		maxKvSize:     df.maxKvSize,
		chunkSize:     df.chunkSize,
		metaSize:      df.metaSize,
		miner:         df.miner,
		status:        0,
	}

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, header.magic); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, header.version); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, header.chunkIdxStart); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, header.chunkIdxLen); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, header.encodeType); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, header.maxKvSize); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, header.chunkSize); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, header.metaSize); err != nil {
		return err
	}
	n, err := buf.Write(header.miner[:])
	if err != nil {
		return err
	}
	if n != len(header.miner) {
		return fmt.Errorf("short write for header.miner, n=%d", n)
	}
	if err := binary.Write(buf, binary.BigEndian, header.status); err != nil {
		return err
	}
	if _, err := df.file.WriteAt(buf.Bytes(), 0); err != nil {
		return err
	}
	return nil
}

func (df *DataFile) readHeader() error {
	header := DataFileHeader{}

	b := make([]byte, HEADER_SIZE)
	n, err := df.file.ReadAt(b, 0)
	if err != nil {
		return err
	}
	if n != int(HEADER_SIZE) {
		return fmt.Errorf("not full header read")
	}

	buf := bytes.NewBuffer(b)
	if err := binary.Read(buf, binary.BigEndian, &header.magic); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &header.version); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &header.chunkIdxStart); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &header.chunkIdxLen); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &header.encodeType); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &header.maxKvSize); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &header.chunkSize); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &header.metaSize); err != nil {
		return err
	}
	n, err = buf.Read(header.miner[:])
	if err != nil {
		return err
	}
	if n != len(header.miner) {
		return fmt.Errorf("short read for header.miner, n=%d", n)
	}
	if err := binary.Read(buf, binary.BigEndian, &header.status); err != nil {
		return err
	}

	// Sanity check
	if header.magic != MAGIC {
		return fmt.Errorf("magic error")
	}
	if header.version > VERSION {
		return fmt.Errorf("unsupported version")
	}
	if header.encodeType > ENCODE_END {
		return fmt.Errorf("unknown mask type")
	}

	df.chunkIdxStart = header.chunkIdxStart
	df.chunkIdxLen = header.chunkIdxLen
	df.encodeType = header.encodeType
	df.maxKvSize = header.maxKvSize
	df.chunkSize = header.chunkSize
	df.metaSize = header.metaSize
	df.miner = header.miner

	return nil
}

func (df *DataFile) Close() error {
	if df.file != nil {
		if err := df.file.Close(); err != nil {
			return fmt.Errorf("close data file %s error: %w", df.file.Name(), err)
		}
	}
	return nil
}
