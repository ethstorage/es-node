#!/bin/sh

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> ./run-docker.sh

container_name="es"
image_name="es-node" 

# check if container is running
if docker ps --format "{{.Names}}" | grep -q "^$container_name$"; then
    echo "$container_name already started"
else
    # start container if exist
    if docker ps -a --format "{{.Names}}" | grep -q "^$container_name$"; then
        docker start $container_name
        echo "$container_name started"
    else
        # build an image if not exist
        if ! docker images --format "{{.Repository}}" | grep -q "^$image_name$"; then
            docker build -t $image_name .
            echo "image $image_name built"
        fi
    # run container in the background
    docker run --name $container_name -v ./es-data:/es-node/es-data -e ES_NODE_STORAGE_MINER=$ES_NODE_STORAGE_MINER -e ES_NODE_PRIVATE_KEY=$ES_NODE_PRIVATE_KEY -p 9545:9545 -p 9222:9222 -p 30305:30305/udp -d --entrypoint /es-node/run.sh $image_name
    echo "$container_name started"
    fi
fi
