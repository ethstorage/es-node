#!/bin/sh

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> ./run-docker.sh

container="es"
if [[ $(docker ps -aq -f name=$container) ]]; then
    docker -e ES_NODE_STORAGE_MINER=$ES_NODE_STORAGE_MINER -e ES_NODE_PRIVATE_KEY=$ES_NODE_PRIVATE_KEY start $container >> es-docker.log
else
    image="es-node"
    if ! docker images | grep -q $image; then
        docker build -t $image .
    fi
    docker run -v ./es-data:/es-node/es-data -e ES_NODE_STORAGE_MINER=$ES_NODE_STORAGE_MINER -e ES_NODE_PRIVATE_KEY=$ES_NODE_PRIVATE_KEY -it --entrypoint /es-node/run.sh $image >> es-docker.log
fi
