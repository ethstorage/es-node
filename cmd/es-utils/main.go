// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"bufio"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var (
	chunkLen   *uint64
	kvLen      *uint64
	miner      *string
	dumpFolder *string
	filenames  *[]string

	verbosity *int

	chunkIdx *uint64
	readLen  *uint64

	shardIdx     *uint64
	kvSize       *uint64
	kvEntries    *uint64
	kvIdx        *uint64
	readStart    *uint64
	readEnd      *uint64
	chunkSize    *uint64
	commitString *string
	encodeType   *uint64
	readEncoded  *bool

	sampleIdx *uint64

	fileNum      *uint64
	blobNum      *uint64
	rpcURL       *string
	contractAddr *string
	chainId      *string
	privateKeys  *[]string
)

var CreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a data file",
	Run:   runCreate,
}

var ChunkReadCmd = &cobra.Command{
	Use:   "chunk_read",
	Short: "Read a chunk from a data file",
	Run:   runChunkRead,
}

var ChunkWriteCmd = &cobra.Command{
	Use:   "chunk_write",
	Short: "Write a chunk from a data file",
	Run:   runChunkWrite,
}

var ShardReadCmd = &cobra.Command{
	Use:   "shard_read",
	Short: "Read a KV from a data shard",
	Run:   runShardRead,
}

var KVReadCmd = &cobra.Command{
	Use:   "kv_read",
	Short: "Read KV range from a data shard",
	Run:   runKVRead,
}

var ShardWriteCmd = &cobra.Command{
	Use:   "shard_write",
	Short: "Write a value to a data shard",
	Run:   runShardWrite,
}

var MetaReadCmd = &cobra.Command{
	Use:   "meta_read",
	Short: "Read a KV meta from a data shard",
	Run:   runMetaRead,
}

var SampleReadCmd = &cobra.Command{
	Use:   "sample_read",
	Short: "Read a sample to a data shard",
	Run:   runSampleRead,
}

var SampleReadDirectCmd = &cobra.Command{
	Use:   "sample_read_directio",
	Short: "Read a sample to a data shard using DirectIO",
	Run:   runSampleReadDirect,
}

var BlobWriteCmd = &cobra.Command{
	Use:   "blob_write",
	Short: "Write a blob to a data shard",
	Run:   runWriteBlob,
}

var BlobUploadCmd = &cobra.Command{
	Use:   "blob_upload",
	Short: "Upload blobs",
	Run:   runUploadBlobs,
}

func init() {
	kvLen = CreateCmd.Flags().Uint64("kv_len", 0, "kv idx len to create")
	chunkLen = CreateCmd.Flags().Uint64("chunk_len", 0, "Chunks idx len to create")

	filenames = rootCmd.PersistentFlags().StringArray("filename", []string{}, "Data filename")
	dumpFolder = rootCmd.PersistentFlags().String("dump_folder", "", "Data dump folder")
	miner = rootCmd.PersistentFlags().String("miner", "", "miner address")
	verbosity = rootCmd.PersistentFlags().Int("verbosity", 3, "Logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail")
	chunkIdx = rootCmd.PersistentFlags().Uint64("chunk_idx", 0, "Chunk idx to start/read/write")

	shardIdx = rootCmd.PersistentFlags().Uint64("shard_idx", 0, "Shard idx to read/write")
	kvSize = rootCmd.PersistentFlags().Uint64("kv_size", 4096, "Shard KV size to read/write")
	kvIdx = rootCmd.PersistentFlags().Uint64("kv_idx", 0, "Shard KV index to read/write")
	readStart = rootCmd.PersistentFlags().Uint64("read_start", 0, "Start index for KV reading")
	readEnd = rootCmd.PersistentFlags().Uint64("read_end", 1, "End index for KV reading")
	kvEntries = rootCmd.PersistentFlags().Uint64("kv_entries", 0, "Number of KV entries in the shard")
	encodeType = rootCmd.PersistentFlags().Uint64("encode_type", 0, "Encode Type, 0=no, 1=simple, 2=ethash")
	chunkSize = rootCmd.PersistentFlags().Uint64("chunk_size", 4096, "Chunk size to encode/decode")

	readLen = rootCmd.PersistentFlags().Uint64("readlen", 0, "Bytes to read (only for unmasked read)")
	commitString = rootCmd.PersistentFlags().String("commit", "", "encode key")
	readEncoded = rootCmd.PersistentFlags().Bool("read_encoded", false, "Read encoded KV data")

	sampleIdx = rootCmd.PersistentFlags().Uint64("sample_idx", 0, "Sample idx to read")

	fileNum = rootCmd.PersistentFlags().Uint64("file_num", 5, "Number of files")
	blobNum = rootCmd.PersistentFlags().Uint64("blob_num", 1, "Number of blobs in a file")
	// TODO: @Qiang everytime devnet update, we may need to update the following two: URL + contract address
	rpcURL = rootCmd.PersistentFlags().String("rpc_url", "http://65.109.50.145:32773", "L1 RPC URL")
	contractAddr = rootCmd.PersistentFlags().String("contract_addr", "0xc443DA12Ec34b2677F9a2755f3738879bEBe0db7", "L1 EthStorage contract address")
	chainId = rootCmd.PersistentFlags().String("chain_id", "3151908", "L1 Chain Id")

	privateKeys = rootCmd.PersistentFlags().StringArray("private_key", []string{}, "Private keys to upload the blobs")
}

func setupLogger() {
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	glogger.Verbosity(log.Lvl(*verbosity))
	log.Root().SetHandler(glogger)

	// setup logger
	var ostream log.Handler
	output := io.Writer(os.Stderr)

	usecolor := (isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
	if usecolor {
		output = colorable.NewColorableStderr()
	}
	ostream = log.StreamHandler(output, log.TerminalFormat(usecolor))

	glogger.SetHandler(ostream)
}

func runCreate(cmd *cobra.Command, args []string) {
	setupLogger()

	if len(*filenames) != 1 {
		log.Crit("Must provide single filename")
	}

	if *encodeType != es.NO_ENCODE && *miner == "" {
		log.Crit("Must provide miner")
	}
	minerAddr := common.HexToAddress(*miner)

	if *chunkSize == 0 {
		log.Crit("Chunk size should not be 0")
	}
	if *kvSize%*chunkSize != 0 {
		log.Crit("Max kv size %% chunk size should be 0")
	}

	if *chunkLen == 0 && *kvLen == 0 {
		log.Crit("Chunk_Len or kv_Len is needed")
	}
	if *chunkLen > 0 && *kvLen > 0 {
		log.Crit("Only one of chunk_Len and kv_Len is nonzero")
	}
	if *chunkLen == 0 {
		chunkPerKV := *kvSize / *chunkSize
		*chunkLen = *kvLen * chunkPerKV
		*chunkIdx = *kvIdx * chunkPerKV
	}

	log.Info("Creating data file", "chunkIdx", *chunkIdx, "chunksLen", *chunkLen, "chunkSize", *chunkSize, "miner", minerAddr, "encodeType", *encodeType)

	_, err := es.Create((*filenames)[0], *chunkIdx, *chunkLen, 0, *kvSize, *encodeType, minerAddr, *chunkSize)
	if err != nil {
		log.Crit("Create failed", "error", err)
	}
}

func runChunkRead(cmd *cobra.Command, args []string) {
	setupLogger()

	if len(*filenames) != 1 {
		log.Crit("Must provide a filename")
	}

	var err error
	var df *es.DataFile
	df, err = es.OpenDataFile((*filenames)[0])
	if err != nil {
		log.Crit("Open failed", "error", err)
	}

	// do not have data hash, use empty hash for placeholder
	b, err := df.Read(*chunkIdx, int(*readLen))
	if err != nil {
		log.Crit("Open failed", "error", err)
	}
	os.Stdout.Write(b)
}

func readInputBytes() []byte {
	in := bufio.NewReader(os.Stdin)
	b := make([]byte, 0)
	for {
		c, err := in.ReadByte()
		if err == io.EOF {
			break
		}
		b = append(b, c)
	}
	return b
}

func runChunkWrite(cmd *cobra.Command, args []string) {
	setupLogger()

	log.Warn("Writing chunk without writing meta may corrupt the file!")

	if len(*filenames) != 1 {
		log.Crit("Must provide a filename")
	}

	var err error
	var df *es.DataFile
	df, err = es.OpenDataFile((*filenames)[0])
	if err != nil {
		log.Crit("Open failed", "error", err)
	}

	err = df.Write(*chunkIdx, readInputBytes())
	if err != nil {
		log.Crit("Write failed", "error", err)
	}
}

func runShardRead(cmd *cobra.Command, args []string) {
	setupLogger()

	ds := initDataShard()
	var b []byte
	var err error
	if *readEncoded {
		b, err = ds.ReadEncoded(*kvIdx, int(*readLen))
	} else {
		commit := common.HexToHash(*commitString)
		b, err = ds.Read(*kvIdx, int(*readLen), commit)
	}
	if err != nil {
		log.Crit("Read failed", "error", err)
	}
	os.Stdout.Write(b)
}

func runKVRead(cmd *cobra.Command, args []string) {
	setupLogger()

	ds := initDataShard()

	for i := *readStart; i < *readEnd; i++ {
		b, _, err := ds.ReadWithMeta(i, int(*readLen))
		if err != nil {
			log.Crit("Read failed", "error", err)
		}
		fileName := fmt.Sprintf("%s/%s.dat", *dumpFolder, hex.EncodeToString(b[0:5]))
		f, err := os.Create(fileName)
		if err != nil {
			log.Crit("Error creating file:", err)
		}

		writer := bufio.NewWriter(f)
		writer.Write(b)

		writer.Flush()
		f.Close()
	}
}

func runMetaRead(cmd *cobra.Command, args []string) {
	setupLogger()

	ds := initDataShard()

	b, err := ds.ReadMeta(*kvIdx)
	if err != nil {
		log.Crit("Read failed", "error", err)
	}
	os.Stdout.Write([]byte(common.Bytes2Hex(b)))
}

func initDataShard() *es.DataShard {
	ds := es.NewDataShard(*shardIdx, *kvSize, *kvEntries, *chunkSize)
	for _, filename := range *filenames {
		var err error
		var df *es.DataFile
		df, err = es.OpenDataFile(filename)
		if err != nil {
			log.Crit("Open failed", "error", err)
		}
		err = ds.AddDataFile(df)
		if err != nil {
			log.Crit("Open failed", "error", err)
		}
	}

	if !ds.IsComplete() {
		log.Warn("Shard is not completed")
	}
	return ds
}

func initDataShardDirectIO() *es.DataShard {
	ds := es.NewDataShard(*shardIdx, *kvSize, *kvEntries, *chunkSize)
	for _, filename := range *filenames {
		var err error
		var df *es.DataFile
		log.Info("Opening file with directIO", "file", filename)
		df, err = es.OpenDataFileDirectIO(filename)
		if err != nil {
			log.Crit("Open failed", "error", err)
		}
		err = ds.AddDataFile(df)
		if err != nil {
			log.Crit("Open failed", "error", err)
		}
	}

	if !ds.IsComplete() {
		log.Warn("Shard is not completed")
	}
	return ds
}

func runShardWrite(cmd *cobra.Command, args []string) {
	setupLogger()

	ds := initDataShard()

	bs := readInputBytes()

	commit := common.HexToHash(*commitString)
	encodingKey := es.CalcEncodeKey(commit, *kvIdx, ds.Miner())
	log.Info("Write info", "commit", commit, "encodingKey", encodingKey)

	err := ds.Write(*kvIdx, bs, commit)
	if err != nil {
		log.Crit("Write failed", "error", err)
	}
	log.Info("Write value", "kvIdx", *kvIdx, "bytes", len(bs))
}

func runSampleRead(cmd *cobra.Command, args []string) {
	setupLogger()

	ds := initDataShard()
	log.Info("Reading sample", "sampleIdx", *sampleIdx)
	b, err := ds.ReadSample(*sampleIdx)
	if err != nil {
		log.Crit("Read failed", "error", err)
	}
	os.Stdout.Write(b.Bytes())
}

func runSampleReadDirect(cmd *cobra.Command, args []string) {
	setupLogger()

	ds := initDataShardDirectIO()
	log.Info("Reading sample with DirectIO", "sampleIdx", *sampleIdx)
	b, err := ds.ReadSampleDirectly(*sampleIdx)
	if err != nil {
		log.Crit("Read failed", "error", err)
	}
	os.Stdout.Write(b.Bytes())
}

func runWriteBlob(cmd *cobra.Command, args []string) {
	setupLogger()

	bs := readInputBytes()
	blobs := utils.EncodeBlobs(bs)
	commitments, _, versionedHashes, err := utils.ComputeBlobs(blobs)
	if err != nil {
		log.Crit("Compute versioned hash failed", "error", err)
	} else {
		log.Info(fmt.Sprintf("Versioned hash is %x and commitment is %x", versionedHashes[0][:], commitments[0][:]))
	}

	ds := initDataShard()

	err = ds.Write(*kvIdx, blobs[0][:], versionedHashes[0])
	if err != nil {
		log.Crit("Write failed", "error", err)
	}
	log.Info("Write value", "kvIdx", *kvIdx, "bytes", len(blobs[0][:]))

}

func genBlobAndDump(idx int) [][]byte {
	saveDir := fmt.Sprintf("%s/%d", *dumpFolder, idx)
	err := os.MkdirAll(saveDir, os.ModePerm)
	if err != nil {
		log.Crit("Error:", err)
	}

	finalDir := fmt.Sprintf("%s/final", *dumpFolder)
	err = os.MkdirAll(finalDir, os.ModePerm)
	if err != nil {
		log.Crit("Error:", err)
	}

	filesData := [][]byte{}
	fileSize := 4096 * 32 * (*blobNum)

	for i := uint64(0); i < *fileNum; i++ {
		data := make([]byte, fileSize)
		for j := uint64(0); j < fileSize; j += 32 {
			scalar := genRandomCanonicalScalar()
			copy(data[j:(j+32)], scalar[:])
		}
		for j := uint64(0); j < *blobNum; j++ {
			fileName := fmt.Sprintf("%s/%s.txt", saveDir, hex.EncodeToString(data[(j*fileSize):(j*fileSize)+5]))
			f, err := os.Create(fileName)
			if err != nil {
				log.Crit("Error creating file:", err)
			}

			writer := bufio.NewWriter(f)
			writer.Write(data[j*fileSize : (j+1)*fileSize])

			writer.Flush()
			f.Close()

			log.Info("Generate a blob file", "fileName", fileName)

			_, name := filepath.Split(fileName)
			finalPath := filepath.Join(finalDir, name)
			finalFile, err := os.Create(finalPath)
			if err != nil {
				log.Crit("Error creating file:", err)
			}

			srcFile, err := os.Open(fileName)
			if err != nil {
				log.Crit("Error creating file:", err)
			}

			_, err = io.Copy(finalFile, srcFile)
			if err != nil {
				log.Crit("Error coping file:", err)
			}

			finalFile.Close()
			srcFile.Close()

		}
		filesData = append(filesData, data)
	}
	return filesData
}

func genRandomCanonicalScalar() [32]byte {
	maxCanonical := new(big.Int)
	_, success := maxCanonical.SetString("73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000000", 16)
	if !success {
		log.Crit("Error creating modulus")
	}
	randomNum, err := crand.Int(crand.Reader, maxCanonical)
	if err != nil {
		log.Crit("Error generate random number")
	}
	var res [32]byte
	randomNum.FillBytes(res[:])

	return res
}

func runUploadBlobs(cmd *cobra.Command, args []string) {
	setupLogger()

	wg := new(sync.WaitGroup)
	wg.Add(len(*privateKeys))

	for i, privateKey := range *privateKeys {
		go func(idx int, priv string) {
			files := genBlobAndDump(idx)

			for j, file := range files {
				selector := "0x4581a920"
				byteArray := make([]byte, 32)
				binary.LittleEndian.PutUint64(byteArray, uint64(j))
				firstParam := hex.EncodeToString(byteArray)
				otherParams := "00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000020000"
				calldata := selector + firstParam + otherParams

				utils.SendBlobTx(
					*rpcURL,
					common.HexToAddress(*contractAddr),
					priv,
					file,
					false,
					-1,
					"0x0",
					210000,
					"",
					"200000000",
					"300000000",
					*chainId, // TODO: @Qiang everytime devnet update, we may need to update it
					calldata,
				)
			}

			wg.Done()
		}(i, privateKey)
	}

	wg.Wait()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "es-utils",
	Short: "EthStorage utilities",
}

func init() {
	rootCmd.AddCommand(CreateCmd)
	rootCmd.AddCommand(ChunkReadCmd)
	rootCmd.AddCommand(ChunkWriteCmd)
	rootCmd.AddCommand(ShardReadCmd)
	rootCmd.AddCommand(ShardWriteCmd)
	rootCmd.AddCommand(SampleReadCmd)
	rootCmd.AddCommand(SampleReadDirectCmd)
	rootCmd.AddCommand(MetaReadCmd)
	rootCmd.AddCommand(BlobWriteCmd)
	rootCmd.AddCommand(BlobUploadCmd)
	rootCmd.AddCommand(KVReadCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
