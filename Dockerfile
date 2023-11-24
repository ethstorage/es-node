# Build ES node in a stock Go builder container
FROM golang:1.20-alpine as builder

RUN apk add --no-cache gcc musl-dev linux-headers make

# Get dependencies - will also be cached if we won't change go.mod/go.sum
COPY go.mod /es-node/
COPY go.sum /es-node/
RUN cd /es-node && go mod download

ADD . /es-node
RUN make

# Pull ES node into a second stage deploy alpine container
FROM node:16-alpine

# For file download
RUN apk add --no-cache curl grep
RUN npm install -g snarkjs@0.7.0
COPY --from=builder /es-node/ /es-node/
RUN chmod +x /es-node/run.sh
WORKDIR /es-node

EXPOSE 9545 9222 30305/udp
