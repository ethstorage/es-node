# Build ES node in a stock Go builder container
FROM golang:1.20-alpine as builder

RUN apk add --no-cache gcc musl-dev linux-headers make

# build
ADD . /es-node
WORKDIR /es-node
RUN make

# Pull ES node into a second stage deploy alpine container
FROM node:16-alpine
COPY --from=builder /es-node/build/ /es-node/build/

# For zk proof
RUN npm install -g snarkjs@0.7.0
RUN apk add --no-cache curl grep

# Entrypoint
COPY --from=builder /es-node/run.sh /es-node/
RUN chmod +x /es-node/run.sh
WORKDIR /es-node

EXPOSE 9545 9222 9222/udp
