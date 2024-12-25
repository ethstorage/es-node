const {ethers, Contract} = require("ethers");
const crypto = require('crypto');
const {EthStorage} = require("ethstorage-sdk");
const core = require('@actions/core');
const fs = require('fs');

const dotenv = require("dotenv")
dotenv.config()
const privateKey = process.env.ES_NODE_UPLOADER_PRIVATE_KEY;
const contractAddr = process.env.ES_NODE_CONTRACT_ADDRESS;
const RPC = 'http://5.9.87.214:8545';
const contractABI = [
    "function lastKvIdx() public view returns (uint40)"
]

const provider = new ethers.JsonRpcProvider(RPC);
const contract = new Contract(contractAddr, contractABI, provider);
const MAX_BLOB = BigInt(process.argv[2]);
const BATCH_SIZE = 6n;
const NEED_WAIT = (process.argv[3] === 'true');

async function UploadBlobsForIntegrationTest() {
    // put blobs
    console.log(contractAddr)
    const es = await EthStorage.create({
        rpc: RPC,
        privateKey,
        address: contractAddr
    })
    while (true) {
        const currentIndex = await contract.lastKvIdx();
        const totalCount = MAX_BLOB - currentIndex;
        console.log("Current Number:", currentIndex, " Total Number:", totalCount, "at", new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }));
        if (totalCount <= 0) {
            break;
        }

        let keys = [];
        let blobs = [];
        for (let i = 0; i < BATCH_SIZE && i < totalCount; i++) {
            const buf = crypto.randomBytes(126976);
            keys[i] = buf.subarray(0,32).toString('hex')
            blobs[i] = buf
        }

        // write blobs
        try {
            let status = await es.writeBlobs(keys, blobs);
            if (status == false) {
                continue
            }
            console.log(status);
        } catch (e) {
            console.log("upload blob error:", e.message);
            continue
        }
        for (let i = 0; i < blobs.length; i++) {
            fs.writeFileSync(".data", blobs[i].toString('hex')+'\n', { flag: 'a+' });
        }
    }

    if (!NEED_WAIT) {
        return
    }

    let latestBlock
    try {
        latestBlock = await provider.getBlock();
        console.log("latest block number is", latestBlock.number);
    } catch (e) {
        core.setFailed(`EthStorage: get latest block failed with message: ${e.message}`);
        return
    }

    // wait for blobs finalized
    var intervalId = setInterval(async function (){
        try {
            let finalizedBlock = await provider.getBlock("finalized");
            console.log(
                "finalized block number is",
                finalizedBlock.number,
                "at",
                new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })
            );
            if (latestBlock.number < finalizedBlock.number) {
                setTimeout(() => console.log("Upload done!"), 300000)
                clearInterval(intervalId);
            }
        } catch (e) {
            console.error(`EthStorage: get finalized block failed!`, e.message);
        }
    }, 120000);
}

UploadBlobsForIntegrationTest();

