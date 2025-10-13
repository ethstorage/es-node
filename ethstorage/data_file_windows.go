//go:build windows

//Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package ethstorage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sys/windows"
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

	SampleSizeBits = 5 // 32 bytes

	InvalidHandle = ^windows.Handle(0)
	// DefaultTimeout allows changing the default timeout used for Write() and Read() functions. Defaults to -1 (blocking)
	DefaultTimeout int = -1
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procSetFilePointerEx = kernel32.NewProc("SetFilePointerEx")
	procSetEndOfFile     = kernel32.NewProc("SetEndOfFile")
)

// preallocateFile expand file size to targetSize bytes
func preallocateFile(h windows.Handle, targetSize int64) error {
	// Move the file pointer to the target location
	var newPos int64
	r1, _, e1 := procSetFilePointerEx.Call(
		uintptr(h),
		uintptr(targetSize),
		uintptr(unsafe.Pointer(&newPos)),
		uintptr(windows.FILE_BEGIN),
	)
	if r1 == 0 {
		return fmt.Errorf("SetFilePointerEx failed: %v", e1)
	}

	// Set the current position to the end of the file
	r1, _, e1 = procSetEndOfFile.Call(uintptr(h))
	if r1 == 0 {
		return fmt.Errorf("SetEndOfFile failed: %v", e1)
	}

	return nil
}

func asyncIo(fn func(windows.Handle, []byte, *uint32, *windows.Overlapped) error, h windows.Handle, buf []byte, milliseconds int, o *windows.Overlapped) (uint32, error) {
	var n uint32
	err := fn(h, buf, &n, o)

	if err == windows.ERROR_IO_PENDING {
		if milliseconds >= 0 {
			// Honor the time limit if one is set
			if n, _ = windows.WaitForSingleObject(o.HEvent, uint32(milliseconds)); n != windows.WAIT_OBJECT_0 {
				switch n {
				case syscall.WAIT_ABANDONED:
					err = os.NewSyscallError("WaitForSingleObject", fmt.Errorf("WAIT_ABANDONED"))
				case syscall.WAIT_TIMEOUT:
					err = os.NewSyscallError("WaitForSingleObject", windows.WAIT_TIMEOUT)
				case syscall.WAIT_FAILED:
					err = os.NewSyscallError("WaitForSingleObject", fmt.Errorf("WAIT_FAILED"))
				default:
					err = os.NewSyscallError("WaitForSingleObject", fmt.Errorf("UNKNOWN ERROR"))
				}
				return 0, err
			}
		}
		if err = windows.GetOverlappedResult(h, o, &n, true); err != nil {
			if err == windows.ERROR_HANDLE_EOF {
				err = io.EOF
				return n, err
			}
			err = os.NewSyscallError("GetOverlappedResult", err)
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	return n, nil
}

func asyncReadAt(h windows.Handle, buf []byte, offset int64) (int, error) {
	var overlapped windows.Overlapped
	overlapped.Offset = uint32(offset)
	overlapped.OffsetHigh = uint32(offset >> 32)
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(event)
	overlapped.HEvent = event

	n, err := asyncIo(windows.ReadFile, h, buf, DefaultTimeout, &overlapped)
	err = os.NewSyscallError("asyncReadAt", err)
	if errors.Is(err, io.EOF) || (err == nil && n == 0 && len(buf) > 0) || (err == nil && len(buf) > int(n)) {
		err = io.EOF
	}
	return int(n), err
}

func asyncWriteAt(h windows.Handle, buf []byte, offset int64) (int, error) {

	var overlapped windows.Overlapped
	overlapped.Offset = uint32(offset)
	overlapped.OffsetHigh = uint32(offset >> 32)
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(event)
	overlapped.HEvent = event

	n, err := asyncIo(windows.WriteFile, h, buf, DefaultTimeout, &overlapped)
	return int(n), os.NewSyscallError("asyncWriteAt", err)
}

// createFile opens or creates a file on Windows with high-performance I/O flags.
func createFile(filename string, mode int) (windows.Handle, error) {
	// createmode logic is same as syscall.Open used by os.Create and os.OpenFile
	var createmode uint32
	switch {
	case mode&(syscall.O_CREAT|syscall.O_EXCL) == (syscall.O_CREAT | syscall.O_EXCL):
		createmode = syscall.CREATE_NEW
	case mode&(syscall.O_CREAT|syscall.O_TRUNC) == (syscall.O_CREAT | syscall.O_TRUNC):
		createmode = syscall.CREATE_ALWAYS
	case mode&syscall.O_CREAT == syscall.O_CREAT:
		createmode = syscall.OPEN_ALWAYS
	case mode&syscall.O_TRUNC == syscall.O_TRUNC:
		createmode = syscall.TRUNCATE_EXISTING
	default:
		createmode = syscall.OPEN_EXISTING
	}

	path, err := windows.UTF16PtrFromString(filename)
	if err != nil {
		return InvalidHandle, fmt.Errorf("invalid filename: %w", err)
	}

	h, err := windows.CreateFile(
		path,
		windows.GENERIC_READ|windows.GENERIC_WRITE,       // Allow read/write access
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, // Allow concurrent read/write access
		nil,        // Default security attributes
		createmode, // Open/create behavior
		windows.FILE_FLAG_OVERLAPPED|windows.FILE_FLAG_RANDOM_ACCESS, // High-performance async I/O
		0,
	)
	if err != nil {
		return 0, fmt.Errorf("CreateFile failed: %w", err)
	}

	return h, nil
}

func CreateFile(filename string) (windows.Handle, error) {
	return createFile(filename, syscall.O_RDWR|syscall.O_CREAT|syscall.O_TRUNC)
}
func OpenFile(filename string, mode int) (windows.Handle, error) {
	return createFile(filename, mode)
}

// A DataFile represents a local fileHandle for a consecutive chunks
type DataFile struct {
	fileHandle    windows.Handle
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

	h, err := CreateFile(filename)
	if err != nil {
		return nil, err
	}
	// actual initialization is done when synchronize
	err = preallocateFile(h, int64((chunkSize+32)*chunkIdxLen+HEADER_SIZE))
	if err != nil {
		return nil, err
	}
	dataFile := &DataFile{
		fileHandle:    h,
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
	h, err := OpenFile(filename, syscall.O_RDWR)
	if err != nil {
		return nil, err
	}
	dataFile := &DataFile{
		fileHandle: h,
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
	return df.Contains(sampleIdx << SampleSizeBits / df.chunkSize)
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

// Read raw chunk data from the storage fileHandle.
func (df *DataFile) ReadAt(b []byte, off int64) (int, error) {
	return asyncReadAt(df.fileHandle, b, off)
}

// WriteAt writes data to a file handle at the specified offset.
func (df *DataFile) WriteAt(b []byte, off int64) (n int, err error) {
	return asyncWriteAt(df.fileHandle, b, off)
}

// Read raw chunk data from the storage fileHandle.
func (df *DataFile) Read(chunkIdx uint64, len int) ([]byte, error) {
	if !df.Contains(chunkIdx) {
		return nil, fmt.Errorf("chunk not found")
	}
	if len > int(df.chunkSize) {
		return nil, fmt.Errorf("read too large")
	}
	md := make([]byte, len)
	n, err := df.ReadAt(md, HEADER_SIZE+int64(chunkIdx-df.chunkIdxStart)*int64(df.chunkSize))
	if err != nil {
		return nil, err
	}
	if n != len {
		return nil, fmt.Errorf("not full read")
	}
	return md, nil
}

// Read raw chunk data from the storage fileHandle.
func (df *DataFile) ReadSample(sampleIdx uint64) (common.Hash, error) {
	if !df.ContainsSample(sampleIdx) {
		return common.Hash{}, fmt.Errorf("sample not found")
	}
	sampleSize := 1 << SampleSizeBits
	md := make([]byte, sampleSize)
	n, err := df.ReadAt(md, HEADER_SIZE+int64(sampleIdx<<SampleSizeBits)-int64(df.chunkIdxStart*df.chunkSize))
	if err != nil {
		return common.Hash{}, err
	}
	if n != sampleSize {
		return common.Hash{}, fmt.Errorf("not full read")
	}
	return common.BytesToHash(md), nil
}

// Write the chunk bytes to the fileHandle.
func (df *DataFile) Write(chunkIdx uint64, b []byte) error {
	if !df.Contains(chunkIdx) {
		return fmt.Errorf("chunk not found")
	}

	if len(b) > int(df.chunkSize) {
		return fmt.Errorf("write data too large")
	}

	_, err := df.WriteAt(b, HEADER_SIZE+int64(chunkIdx-df.chunkIdxStart)*int64(df.chunkSize))
	return err
}

// Read the metadata of the kv
func (df *DataFile) ReadMeta(kvIdx uint64) ([]byte, error) {
	if !df.ContainsKv(kvIdx) {
		return nil, fmt.Errorf("kv not found")
	}

	b := make([]byte, df.metaSize)
	_, err := df.ReadAt(b, int64(HEADER_SIZE+df.chunkIdxLen*df.chunkSize+(kvIdx-df.KvIdxStart())*df.metaSize))
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

	_, err := df.WriteAt(b, int64(HEADER_SIZE+df.chunkIdxLen*df.chunkSize+(kvIdx-df.KvIdxStart())*df.metaSize))
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
	if _, err := df.WriteAt(buf.Bytes(), 0); err != nil {
		return err
	}
	return nil
}

func (df *DataFile) readHeader() error {
	header := DataFileHeader{}

	b := make([]byte, HEADER_SIZE)
	n, err := df.ReadAt(b, 0)
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
	if df.fileHandle != InvalidHandle {
		if err := windows.CloseHandle(df.fileHandle); err != nil {
			return fmt.Errorf("close data fileHandle error: %w", err)
		}
	}
	return nil
}
