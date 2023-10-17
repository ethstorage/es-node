# Setup a Private EthStorage Network
In this tutorial, you will deploy your storage contract on EIP-4844 devnet, and set up a private network composed of es-node instances.

Optionally, you can also set up a private EIP-4844 testnet using [Kurtosis](https://docs.kurtosis.com/).

## Setup a private EIP-4844 testnet 
This section is a quick guide to setting up a private testnet with EIP-4844 enabled. For a more detailed description, please take a look at [this document](https://notes.ethereum.org/@parithosh/kurtosis-example).

### Environment
* Docker version 24.0.5+
* Kurtosis (for EIP-4844 testnet)

#### Install Kurtosis
The following command will install the latest version of Kurtosis. 
```sh
echo "deb [trusted=yes] https://apt.fury.io/kurtosis-tech/ /" | tee /etc/apt/sources.list.d/kurtosis.list
apt update
apt install kurtosis-cli
```
### Step 1: Configuration
Save the below JSON in a file called `eth.json`.

_Note: Currently the latest version of built Docker images available is devnet-8, but it will not affect the tests._
```json
{
  "participants": [
    {
      "el_client_type": "geth",
      "el_client_image": "ethpandaops/geth:dencun-devnet-8-946a2da",
      "cl_client_type": "lighthouse",
      "cl_client_image": "sigp/lighthouse:deneb-modern",
      "count": 3
    }
  ],
  "network_params": {
    "capella_fork_epoch": 2,
    "deneb_fork_epoch": 3
  },
  "launch_additional_services": false,
  "wait_for_finalization": false,
  "wait_for_verifications": false,
  "global_client_log_level": "info",
  "snooper_enabled": true
}
```

### Step 2: Startup
Start the module with the following command:
```sh
kurtosis run --enclave eth github.com/kurtosis-tech/ethereum-package  "$(cat ./eth.json)"
```

### Step 3: Check status
The testnet will start in the background. You can use the following command to get more information:
```sh
kurtosis enclave inspect eth
```
And you can see something like this:
```
Name:            eth
UUID:            ae85030054d5
Status:          RUNNING
Creation Time:   Mon, 16 Oct 2023 07:09:47 UTC

========================================= Files Artifacts =========================================
UUID           Name
d7e7889adb60   1-lighthouse-geth-0-63
d17aad8e7e0f   2-lighthouse-geth-64-127
1fee58f6e4ec   3-lighthouse-geth-128-191
29f355f179c5   cl-genesis-data
6c32c66203c8   el-genesis-data
10c419c28385   genesis-generation-config-cl
d78e53940cf4   genesis-generation-config-el
c9cf29feb435   geth-prefunded-keys
8c0e94eac7dd   misty-cloud
af22a014fe4a   prysm-password
66a2e28b23d4   vast-tiger

========================================== User Services ==========================================
UUID           Name                                             Ports                                         Status
28ce14f05034   cl-1-lighthouse-geth                             http: 4000/tcp -> http://127.0.0.1:32787      RUNNING
                                                                metrics: 5054/tcp -> http://127.0.0.1:32786
                                                                tcp-discovery: 9000/tcp -> 127.0.0.1:32785
                                                                udp-discovery: 9000/udp -> 127.0.0.1:32771
9af3202a8e45   cl-1-lighthouse-geth-validator                   http: 5042/tcp -> 127.0.0.1:32789             RUNNING
                                                                metrics: 5064/tcp -> http://127.0.0.1:32788
53ec97c897de   cl-2-lighthouse-geth                             http: 4000/tcp -> http://127.0.0.1:32793      RUNNING
                                                                metrics: 5054/tcp -> http://127.0.0.1:32792
                                                                tcp-discovery: 9000/tcp -> 127.0.0.1:32791
                                                                udp-discovery: 9000/udp -> 127.0.0.1:32772
b0169ad0f7a2   cl-2-lighthouse-geth-validator                   http: 5042/tcp -> 127.0.0.1:32795             RUNNING
                                                                metrics: 5064/tcp -> http://127.0.0.1:32794
d9c155747205   cl-3-lighthouse-geth                             http: 4000/tcp -> http://127.0.0.1:32799      RUNNING
                                                                metrics: 5054/tcp -> http://127.0.0.1:32798
                                                                tcp-discovery: 9000/tcp -> 127.0.0.1:32797
                                                                udp-discovery: 9000/udp -> 127.0.0.1:32773
f8223fdb94c1   cl-3-lighthouse-geth-validator                   http: 5042/tcp -> 127.0.0.1:32801             RUNNING
                                                                metrics: 5064/tcp -> http://127.0.0.1:32800
e1e09cf02539   eip4788-contract-deployment                      <none>                                        RUNNING
8ffd69c8b43a   el-1-geth-lighthouse                             engine-rpc: 8551/tcp -> 127.0.0.1:32771       RUNNING
                                                                metrics: 9001/tcp -> 127.0.0.1:32770
                                                                rpc: 8545/tcp -> 127.0.0.1:32773
                                                                tcp-discovery: 30303/tcp -> 127.0.0.1:32769
                                                                udp-discovery: 30303/udp -> 127.0.0.1:32768
                                                                ws: 8546/tcp -> 127.0.0.1:32772
87e3b73ce3cf   el-2-geth-lighthouse                             engine-rpc: 8551/tcp -> 127.0.0.1:32776       RUNNING
                                                                metrics: 9001/tcp -> 127.0.0.1:32775
                                                                rpc: 8545/tcp -> 127.0.0.1:32778
                                                                tcp-discovery: 30303/tcp -> 127.0.0.1:32774
                                                                udp-discovery: 30303/udp -> 127.0.0.1:32769
                                                                ws: 8546/tcp -> 127.0.0.1:32777
c834e8faa8ae   el-3-geth-lighthouse                             engine-rpc: 8551/tcp -> 127.0.0.1:32781       RUNNING
                                                                metrics: 9001/tcp -> 127.0.0.1:32780
                                                                rpc: 8545/tcp -> 127.0.0.1:32783
                                                                tcp-discovery: 30303/tcp -> 127.0.0.1:32779
                                                                udp-discovery: 30303/udp -> 127.0.0.1:32770
                                                                ws: 8546/tcp -> 127.0.0.1:32782
bba05b580b20   prelaunch-data-generator-cl-genesis-data         <none>                                        RUNNING
67f7b8ee9782   prelaunch-data-generator-cl-validator-keystore   <none>                                        RUNNING
92ad8762bce0   prelaunch-data-generator-el-genesis-data         <none>                                        RUNNING
73db38e17ad6   snooper-1-lighthouse-geth                        http: 8561/tcp -> 127.0.0.1:32784             RUNNING
5692585af193   snooper-2-lighthouse-geth                        http: 8561/tcp -> 127.0.0.1:32790             RUNNING
30b24830afbc   snooper-3-lighthouse-geth                        http: 8561/tcp -> 127.0.0.1:32796             RUNNING
d3d57353402f   task-d5e52baf-f0e0-4fd9-90c1-d4bfb0dca4a8        <none>                                        RUNNING
```

_Note: In the above example, the available EL RPC ports are `32773`, `32778`, and `32783`; and the CL RPC ports are `32787`, `32793`, and `32799`.  The ports could be different in your environment, so please confirm the correct ports (and IP address) are used to make configurations in the next steps._

### Step 4: Update the `run.sh` file
Some flags need to be changed to launch es-node according to the operations done above. Edit the `run.sh` file with following updates:

#### Update RPC URLs
Find the flag `--l1.rpc` in `run.sh`, and replace the old value with the execution layer RPC URL of the newly deployed EIP-4844 devnet.

```sh
--l1.rpc <l1_el_rpc> \
```
Also replace the value of the `--l1.beacon` flag with the consensus layer RPC URL of the EIP-4844 devnet:
```sh
--l1.beacon <l1_cl_rpc> \
``` 

#### Update beacon time flag
Execute the following command to get the timestamp of the first slot:
```sh
curl -s http://127.0.0.1:32787/eth/v1/beacon/blocks/1 | jq -r '.data.message.body.execution_payload.timestamp'
```
And you will get an integer representing the timestamp of the first slot. Replace the old value of the `--l1.beacon-based-time` flag in the `run.sh` with this value:
```sh
--l1.beacon-based-time <timestamp> \
``` 

## Deploy storage contract

### Environment
* Node 16+
* Hardhat 2.13.1+

### Step 1: Get the source code
```sh
git clone git@github.com:ethstorage/storage-contracts-v1.git
```

### Step 2: Configure Hardhat to use the private testnet
In the top folder of `storage-contracts-v1` repo, create a `.env` file with the following content:
```sh
PRIVATE_KEY=<private_key>
EIP4844_DEVNET_URL=<l1_el_rpc>
```
Where `<private_key>` is the private key of a pre-funded Ethereum account of the EIP-4844 devnet, and `<l1_el_rpc>` is the execution layer RPC URL of the EIP-4844 devnet.

### Step 3: Deploy contract
Execute the following command to deploy the contract:
```node
npx hardhat run scripts/deploy.js --network devnet
```
And you will see the deployed storage contract address on the console with 1 ether pre-funded.

### Step 4: Update contract address
Find the flag `--storage.l1contract` in `run.sh`, and replace the old value with the newly deployed one.
```sh
  --storage.l1contract <contract_address> \
```

## Start up a private EthStorage network

To set up your own EthStorage network, you'll need first to launch a boot node and configure other es-node instances to connect to it. 

### Step 1: Launch an es-node as a boot node

Suppose you have created an es-node according to [the quick start guide](/GUIDE.md#option-3-without-docker). 
\
_Note: Currently to run a bootnode you need to set up an es-node instance using binary instead of using Docker._


To make it a boot node, open `run.sh` for editing, and add `--p2p.advertise.ip` flag as a new line under the definition of `es_node_start`: 
```sh
--p2p.advertise.ip <your_ip_address> \
```
Where `<your_ip_address>` needs to be replaced with your IP address that the other nodes can access. 

And remove the line with the `--p2p.bootnodes` flag:
```sh
--p2p.bootnodes enr:... \
```

### Step 2: Launch common es-nodes
Then, soon after the boot node is started up, you can find the base64 encoded enr value prefixed with `enr:` in the log. 

Next, you will need to replace the value of `--p2p.bootnodes` flag in other nodes' `run.sh` with the new one. 

```sh
--p2p.bootnodes <enr-value> \
```

Finally, start up the common es-nodes and they are supposed to connect to the bootnode and each other to compose a private EthStorage network. 
Enjoy!
