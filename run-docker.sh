#!/bin/sh

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> ./run-docker.sh

log_name="es-docker.log"
container_name="es"
image_name="es-node"

if docker ps -a --format "{{.Names}}" | grep -q "^$container_name$"; then
    docker -e ES_NODE_STORAGE_MINER=$ES_NODE_STORAGE_MINER -e ES_NODE_PRIVATE_KEY=$ES_NODE_PRIVATE_KEY start $container_name >> $log_name
else
    if ! docker images | grep -q "^$image_name$"; then
        docker build -t $image_name .
    fi
    docker run --name $container_name -v ./es-data:/es-node/es-data -e ES_NODE_STORAGE_MINER=$ES_NODE_STORAGE_MINER -e ES_NODE_PRIVATE_KEY=$ES_NODE_PRIVATE_KEY -it --entrypoint /es-node/run.sh $image_name >> $log_name
fi
