#!/bin/sh

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run-docker.sh

if [ -z "$ES_NODE_STORAGE_MINER" ]; then
  echo "Please provide 'ES_NODE_STORAGE_MINER' as an environment variable"
  exit 1
fi

if [ ${#ES_NODE_STORAGE_MINER} -ne 42 ] || case $ES_NODE_STORAGE_MINER in 0x*) false;; *) true;; esac; then
  echo "Error: ES_NODE_STORAGE_MINER should be prefixed with '0x' and have a total length of 42"
  exit 1
fi

if [ -z "$ES_NODE_SIGNER_PRIVATE_KEY" ]; then
  echo "Please provide 'ES_NODE_SIGNER_PRIVATE_KEY' as an environment variable"
  exit 1
fi

if [ ${#ES_NODE_SIGNER_PRIVATE_KEY} -ne 64 ]; then
  echo "Error: ES_NODE_SIGNER_PRIVATE_KEY should have a length of 64"
  exit 1
fi

container_name="es"
image_name="es-node" 

# check if container is running
if docker ps --format "{{.Names}}" | grep -q "^$container_name$"; then
    echo "container $container_name already started"
else
    # start container if exist
    if docker ps -a --format "{{.Names}}" | grep -q "^$container_name$"; then
        docker start $container_name
        echo "container $container_name started"
    else
        # build an image if not exist
        if ! docker images --format "{{.Repository}}" | grep -q "^$image_name$"; then
            docker build -t $image_name .
            echo "image $image_name built"
        fi
        # run container in the background
       docker run --name $container_name \
            -v ./es-data:/es-node/es-data \
            -e ES_NODE_STORAGE_MINER=$ES_NODE_STORAGE_MINER \
            -e ES_NODE_SIGNER_PRIVATE_KEY=$ES_NODE_SIGNER_PRIVATE_KEY \
            -p 9545:9545 \
            -p 9222:9222 \
            -p 30305:30305/udp \
            -d \
            --entrypoint /bin/bash /es-node/run.sh \
            $image_name
        echo "container $container_name started"
    fi
fi
