#!/bin/sh

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run-docker.sh

container_name="es"
image_name="es-node" 

# check if container is running
if sudo docker ps --format "{{.Names}}" | grep -q "^$container_name$"; then
    echo "container $container_name already started"
else
    # start container if exist
    if sudo docker ps -a --format "{{.Names}}" | grep -q "^$container_name$"; then
        sudo docker start $container_name
    else
        # build an image if not exist
        if ! sudo docker images --format "{{.Repository}}" | grep -q "^$image_name$"; then
            sudo docker build -t $image_name .
            echo "image $image_name built"
        fi
        
        # check if data folder exist
        if [ ! -d "./es-data" ]; then
          sudo docker run --rm \
            -v ./es-data:/es-node/es-data \
            -v ./build/bin/snark_lib/zkey:/es-node/build/bin/snark_lib/zkey \
            -e ES_NODE_STORAGE_MINER=$ES_NODE_STORAGE_MINER \
            --entrypoint /es-node/init.sh \
            $image_name 
        fi

        # run container in the background
        sudo docker run --name $container_name \
          -v ./es-data:/es-node/es-data \
          -v ./build/bin/snark_lib/zkey:/es-node/build/bin/snark_lib/zkey \
          -e ES_NODE_STORAGE_MINER=$ES_NODE_STORAGE_MINER \
          -e ES_NODE_SIGNER_PRIVATE_KEY=$ES_NODE_SIGNER_PRIVATE_KEY \
          -p 9545:9545 \
          -p 9222:9222 \
          -p 30305:30305/udp \
          -d \
          --entrypoint /es-node/run.sh \
          $image_name
    fi
fi
