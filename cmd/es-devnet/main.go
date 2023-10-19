// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/urfave/cli"
	"os"
	"strings"
)

//var (
//	chunkLen   *uint64
//	kvLen      *uint64
//	miner      *string
//	dumpFolder *string
//	filenames  *[]string
//
//	verbosity *int
//
//	chunkIdx *uint64
//
//	shardIdx   *uint64
//	kvSize     *uint64
//	kvEntries  *uint64
//	kvIdx      *uint64
//	chunkSize  *uint64
//	encodeType *uint64
//
//	fileNum      *uint64
//	blobNum      *uint64
//	rpcURL       *string
//	contractAddr *string
//	chainId      *string
//	privateKeys  *[]string
//)
//
//var CreateCmd = &cobra.Command{
//	Use:   "create",
//	Short: "Create a data file",
//	Run:   runCreate,
//}
//
//var BlobWriteCmd = &cobra.Command{
//	Use:   "blob_write",
//	Short: "Write a blob to a data shard",
//	Run:   runWriteBlob,
//}
//
//func init() {
//	kvLen = CreateCmd.Flags().Uint64("kv_len", 0, "kv idx len to create")
//	chunkLen = CreateCmd.Flags().Uint64("chunk_len", 0, "Chunks idx len to create")
//
//	filenames = rootCmd.PersistentFlags().StringArray("filename", []string{}, "Data filename")
//	dumpFolder = rootCmd.PersistentFlags().String("dump_folder", "", "Data dump folder")
//	miner = rootCmd.PersistentFlags().String("miner", "", "miner address")
//	verbosity = rootCmd.PersistentFlags().Int("verbosity", 3, "Logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail")
//	chunkIdx = rootCmd.PersistentFlags().Uint64("chunk_idx", 0, "Chunk idx to start/read/write")
//
//	shardIdx = rootCmd.PersistentFlags().Uint64("shard_idx", 0, "Shard idx to read/write")
//	kvSize = rootCmd.PersistentFlags().Uint64("kv_size", 4096, "Shard KV size to read/write")
//	kvIdx = rootCmd.PersistentFlags().Uint64("kv_idx", 0, "Shard KV index to read/write")
//	readStart = rootCmd.PersistentFlags().Uint64("read_start", 0, "Start index for KV reading")
//	readEnd = rootCmd.PersistentFlags().Uint64("read_end", 1, "End index for KV reading")
//	kvEntries = rootCmd.PersistentFlags().Uint64("kv_entries", 0, "Number of KV entries in the shard")
//	encodeType = rootCmd.PersistentFlags().Uint64("encode_type", 0, "Encode Type, 0=no, 1=simple, 2=ethash")
//	chunkSize = rootCmd.PersistentFlags().Uint64("chunk_size", 4096, "Chunk size to encode/decode")
//
//	readLen = rootCmd.PersistentFlags().Uint64("readlen", 0, "Bytes to read (only for unmasked read)")
//	commitString = rootCmd.PersistentFlags().String("commit", "", "encode key")
//
//	sampleIdx = rootCmd.PersistentFlags().Uint64("sample_idx", 0, "Sample idx to read")
//
//	fileNum = rootCmd.PersistentFlags().Uint64("file_num", 5, "Number of files")
//	blobNum = rootCmd.PersistentFlags().Uint64("blob_num", 1, "Number of blobs in a file")
//	// TODO: @Qiang everytime devnet update, we may need to update the following two: URL + contract address
//	rpcURL = rootCmd.PersistentFlags().String("rpc_url", "http://65.109.50.145:32773", "L1 RPC URL")
//	contractAddr = rootCmd.PersistentFlags().String("contract_addr", "0xc443DA12Ec34b2677F9a2755f3738879bEBe0db7", "L1 EthStorage contract address")
//	chainId = rootCmd.PersistentFlags().String("chain_id", "3151908", "L1 Chain Id")
//
//	privateKeys = rootCmd.PersistentFlags().StringArray("private_key", []string{}, "Private keys to upload the blobs")
//}
//
//func setupLogger() {
//	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
//	glogger.Verbosity(log.Lvl(*verbosity))
//	log.Root().SetHandler(glogger)
//
//	// setup logger
//	var ostream log.Handler
//	output := io.Writer(os.Stderr)
//
//	usecolor := (isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
//	if usecolor {
//		output = colorable.NewColorableStderr()
//	}
//	ostream = log.StreamHandler(output, log.TerminalFormat(usecolor))
//
//	glogger.SetHandler(ostream)
//}
//
//func runCreate(cmd *cobra.Command, args []string) {
//	setupLogger()
//
//	if len(*filenames) != 1 {
//		log.Crit("Must provide single filename")
//	}
//
//	if *miner == "" {
//		log.Crit("Must provide miner")
//	}
//	minerAddr := common.HexToAddress(*miner)
//
//	if *chunkSize == 0 {
//		log.Crit("Chunk size should not be 0")
//	}
//	if *kvSize%*chunkSize != 0 {
//		log.Crit("Max kv size %% chunk size should be 0")
//	}
//
//	if *chunkLen == 0 && *kvLen == 0 {
//		log.Crit("Chunk_Len or kv_Len is needed")
//	}
//	if *chunkLen > 0 && *kvLen > 0 {
//		log.Crit("Only one of chunk_Len and kv_Len is nonzero")
//	}
//	if *chunkLen == 0 {
//		chunkPerKV := *kvSize / *chunkSize
//		*chunkLen = *kvLen * chunkPerKV
//		*chunkIdx = *kvIdx * chunkPerKV
//	}
//
//	log.Info("Creating data file", "chunkIdx", *chunkIdx, "chunksLen", *chunkLen, "chunkSize", *chunkSize, "miner", minerAddr, "encodeType", *encodeType)
//
//	_, err := es.Create((*filenames)[0], *chunkIdx, *chunkLen, 0, *kvSize, *encodeType, minerAddr, *chunkSize)
//	if err != nil {
//		log.Crit("Create failed", "error", err)
//	}
//}
//
//func readInputBytes() []byte {
//	in := bufio.NewReader(os.Stdin)
//	b := make([]byte, 0)
//	for {
//		c, err := in.ReadByte()
//		if err == io.EOF {
//			break
//		}
//		b = append(b, c)
//	}
//	return b
//}
//
//func initDataShard() *es.DataShard {
//	ds := es.NewDataShard(*shardIdx, *kvSize, *kvEntries, *chunkSize)
//	for _, filename := range *filenames {
//		var err error
//		var df *es.DataFile
//		df, err = es.OpenDataFile(filename)
//		if err != nil {
//			log.Crit("Open failed", "error", err)
//		}
//		err = ds.AddDataFile(df)
//		if err != nil {
//			log.Crit("Open failed", "error", err)
//		}
//	}
//
//	if !ds.IsComplete() {
//		log.Warn("Shard is not completed")
//	}
//	return ds
//}
//
//func runWriteBlob(cmd *cobra.Command, args []string) {
//	setupLogger()
//
//	bs := readInputBytes()
//	blobs := utils.EncodeBlobs(bs)
//	commitments, _, versionedHashes, err := utils.ComputeBlobs(blobs)
//	if err != nil {
//		log.Crit("Compute versioned hash failed", "error", err)
//	} else {
//		log.Info(fmt.Sprintf("Versioned hash is %x and commitment is %x", versionedHashes[0][:], commitments[0][:]))
//	}
//
//	ds := initDataShard()
//
//	err = ds.Write(*kvIdx, blobs[0][:], versionedHashes[0])
//	if err != nil {
//		log.Crit("Write failed", "error", err)
//	}
//	log.Info("Write value", "kvIdx", *kvIdx, "bytes", len(blobs[0][:]))
//
//}
//
//func genBlobAndDump(idx int) [][]byte {
//	saveDir := fmt.Sprintf("%s/%d", *dumpFolder, idx)
//	err := os.MkdirAll(saveDir, os.ModePerm)
//	if err != nil {
//		log.Crit("Error:", err)
//	}
//
//	finalDir := fmt.Sprintf("%s/final", *dumpFolder)
//	err = os.MkdirAll(finalDir, os.ModePerm)
//	if err != nil {
//		log.Crit("Error:", err)
//	}
//
//	filesData := [][]byte{}
//	fileSize := 4096 * 32 * (*blobNum)
//
//	for i := uint64(0); i < *fileNum; i++ {
//		data := make([]byte, fileSize)
//		for j := uint64(0); j < fileSize; j += 32 {
//			scalar := genRandomCanonicalScalar()
//			copy(data[j:(j+32)], scalar[:])
//		}
//		for j := uint64(0); j < *blobNum; j++ {
//			fileName := fmt.Sprintf("%s/%s.txt", saveDir, hex.EncodeToString((data[(j * fileSize) : (j*fileSize)+5])))
//			f, err := os.Create(fileName)
//			if err != nil {
//				log.Crit("Error creating file:", err)
//			}
//
//			writer := bufio.NewWriter(f)
//			writer.Write(data[j*fileSize : (j+1)*fileSize])
//
//			writer.Flush()
//			f.Close()
//
//			log.Info("Generate a blob file", "fileName", fileName)
//
//			_, name := filepath.Split(fileName)
//			finalPath := filepath.Join(finalDir, name)
//			finalFile, err := os.Create(finalPath)
//			if err != nil {
//				log.Crit("Error creating file:", err)
//			}
//
//			srcFile, err := os.Open(fileName)
//			if err != nil {
//				log.Crit("Error creating file:", err)
//			}
//
//			_, err = io.Copy(finalFile, srcFile)
//			if err != nil {
//				log.Crit("Error coping file:", err)
//			}
//
//			finalFile.Close()
//			srcFile.Close()
//
//		}
//		filesData = append(filesData, data)
//	}
//	return filesData
//}
//
//func genRandomCanonicalScalar() [32]byte {
//	maxCanonical := new(big.Int)
//	_, success := maxCanonical.SetString("73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000000", 16)
//	if !success {
//		log.Crit("Error creating modulus")
//	}
//	randomNum, err := crand.Int(crand.Reader, maxCanonical)
//	if err != nil {
//		log.Crit("Error generate random number")
//	}
//	var res [32]byte
//	randomNum.FillBytes(res[:])
//
//	return res
//}
//
//func runUploadBlobs(cmd *cobra.Command, args []string) {
//	setupLogger()
//
//	wg := new(sync.WaitGroup)
//	wg.Add(len(*privateKeys))
//
//	for i, privateKey := range *privateKeys {
//		go func(idx int, priv string) {
//			files := genBlobAndDump(idx)
//
//			for j, file := range files {
//				selector := "0x4581a920"
//				byteArray := make([]byte, 32)
//				binary.LittleEndian.PutUint64(byteArray, uint64(j))
//				firstParam := hex.EncodeToString(byteArray)
//				otherParams := "00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000020000"
//				calldata := selector + firstParam + otherParams
//
//				utils.SendBlobTx(
//					*rpcURL,
//					common.HexToAddress(*contractAddr),
//					priv,
//					file,
//					false,
//					-1,
//					"0x0",
//					210000,
//					"",
//					"200000000",
//					"300000000",
//					*chainId, // TODO: @Qiang everytime devnet update, we may need to update it
//					calldata,
//				)
//			}
//
//			wg.Done()
//		}(i, privateKey)
//	}
//
//	wg.Wait()
//}

var (
	l1Rpc       string
	contract    string
	miner       string
	datadir     string
	shardLength int
)

var (
	log = esLog.NewLogger(esLog.DefaultCLIConfig())
)

var flags = []cli.Flag{
	cli.StringFlag{
		Name:        "l1.rpc",
		Usage:       "Address of L1 User JSON-RPC endpoint to use (eth namespace required)",
		Destination: &l1Rpc,
	},
	cli.StringFlag{
		Name:        "storage.l1contract",
		Usage:       "Storage contract address on l1",
		Destination: &contract,
	},
	cli.StringFlag{
		Name:        "storage.miner",
		Usage:       "Miner's address to encode data and receive mining rewards",
		Destination: &miner,
	},
	cli.StringFlag{
		Name:        "datadir",
		Value:       "./es-data",
		Usage:       "Data directory for the storage files, databases and keystore",
		Destination: &datadir,
	},
	cli.IntFlag{
		Name:        "shardLength",
		Value:       1,
		Usage:       "File counts",
		Destination: &shardLength,
	},
}

func main() {
	app := cli.NewApp()
	app.Version = "1.0.0"
	app.Name = "es-devnet"
	app.Usage = "Create EthStorage Test Data"
	app.Flags = flags
	//该程序执行的代码
	app.Action = EsNodeInit
	//启动
	err := app.Run(os.Args)
	if err != nil {
		log.Crit("Application failed", "message", err)
		return
	}
}

func EsNodeInit(ctx *cli.Context) error {
	cctx := context.Background()
	client, err := ethclient.DialContext(cctx, l1Rpc)
	if err != nil {
		log.Error("Failed to connect to the Ethereum client", "error", err, "l1Rpc", l1Rpc)
		return err
	}
	defer client.Close()

	l1Contract := common.HexToAddress(contract)
	storageCfg, err := initStorageConfig(cctx, client, l1Contract, common.HexToAddress(miner))
	if err != nil {
		log.Error("Failed to load storage config", "error", err)
		return err
	}
	log.Info("Storage config loaded", "storageCfg", storageCfg)

	shardIdxList := make([]uint64, shardLength)
	shardIdxList[1] = 1
	// create
	files, err := createDataFile(storageCfg, shardIdxList, datadir)
	if err != nil {
		log.Error("Failed to create data file", "error", err)
		return err
	}
	if len(files) > 0 {
		log.Info("Data files created", "files", strings.Join(files, ","))
	} else {
		log.Warn("No data files created")
	}
	return nil
}
