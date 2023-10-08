# es-node

Golang implementation of the EthStorage node.

## Getting started

### How to launch an es-node with binary

#### Environment

* go 1.20 or above
* node 16 or above

#### Build and run es-node 

```
git clone git@github.com:ethstorage/es-node.git

cd es-node/cmd/es-node && go build && cd ../..

chmod +x run.sh

env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> ./run.sh
```

### How to launch an es-node with Docker
#### Environment

- Docker-compose version 1.29.2 or above
- Docker version 24.0.5 or above

#### Docker compose
To start es-node with docker-compose, pull es-node source code and execute:
```sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> docker compose up 
```
or
```sh 
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> docker-compose up
```
#### Docker
To start es-node in a Docker container without docker-compose, pull es-node source code and execute:
```sh
# build image
docker build -t es-node .
# start container
docker run -v ./es-data:/es-node/es-data -e ES_NODE_STORAGE_MINER=<miner> -e ES_NODE_PRIVATE_KEY=<private_key> -p 9545:9545 -p 9222:9222 -p 30305:30305/udp -it --entrypoint /es-node/run.sh es-node
```
Where `es-node` is the name of the es-node image.

#### Docker as a background process
Or you can "build and run" a container in a single line of command which runs an es-node Docker container in the background:
```sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> ./run-docker.sh
```
Then check logs by
```sh
docker logs es | tail
```
Where `es` is the name of the es-node container.