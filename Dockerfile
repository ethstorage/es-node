# Build ES node in a stock Go builder container
FROM golang:1.20-alpine as builder

RUN apk add --no-cache gcc musl-dev linux-headers

# Get dependencies - will also be cached if we won't change go.mod/go.sum
COPY go.mod /es-node/
COPY go.sum /es-node/
RUN cd /es-node && go mod download

ADD . /es-node
RUN cd /es-node/cmd/es-node && go build

# Pull ES node into a second stage deploy alpine container
FROM node:16-alpine

RUN apk add --no-cache curl grep

COPY --from=builder /es-node/* /es-node/
RUN chmod +x /es-node/run.sh

EXPOSE 9545 9222 9222/udp
