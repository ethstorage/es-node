#!/bin/sh

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> ./run-docker.sh

container_name="es"
image_name="es-node"

# if docker container inspect -f '{{.State.Running}}' $container_name; then 
is_running=$(docker inspect -f '{{.State.Running}}' "$container_name") 

if  "$is_running" == "true" ; then
    echo "$container_name already started"
else 
    # start container if exists
    if docker ps -a --format "{{.Names}}" | grep -q "^$container_name$"; then
        docker start $container_name  
    else
        # build an image if not exist
        if ! docker images --format "{{.Repository}}" | grep -q "^$image_name$"; then
            docker build -t $image_name .
        fi
        # run container in the background
        docker run --name $container_name -v ./es-data:/es-node/es-data -e ES_NODE_STORAGE_MINER=$ES_NODE_STORAGE_MINER -e ES_NODE_PRIVATE_KEY=$ES_NODE_PRIVATE_KEY -d --entrypoint /es-node/run.sh $image_name 
    fi
fi
