const {ethers, Contract} = require("ethers");
const crypto = require('crypto');
const {EthStorage} = require("ethstorage-sdk");
const core = require('@actions/core');

const dotenv = require("dotenv")
dotenv.config()
const privateKey = process.env.ES_NODE_SIGNER_PRIVATE_KEY;
const contractAddr = process.env.ES_NODE_CONTRACT_ADDRESS;
const RPC = 'http://65.109.20.29:8545';
const contractABI = [
    "function lastKvIdx() public view returns (uint40)"
]

const provider = new ethers.JsonRpcProvider(RPC);
const contract = new Contract(contractAddr, contractABI, provider);
const MAX_BLOB = 256n;

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
        console.log("Current Number:", currentIndex, " Total Number:", totalCount);
        if (totalCount <= 0) {
            break;
        }
        const buf = crypto.randomBytes(126976);
        const cost = await es.estimateCost(buf.subarray(0,32).toString('hex'), buf);
        console.log(cost)

        // write
        let status = await es.write(buf.subarray(0,32).toString('hex'), buf);
        console.log(status)
    }

    let latestBlock
    try {
        latestBlock = await provider.getBlock();
        console.log("latest block number is", latestBlock.number);
    } catch (e) {
        core.setFailed(`EthStorage: get latest block failed!` + e.message);
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

