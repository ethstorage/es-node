# Build ES node in a stock Go builder container
FROM golang:1.21 as builder
ADD . /es-node
WORKDIR /es-node
RUN make

# Pull ES node into a second stage deploy alpine container
FROM alpine:latest
COPY --from=builder /es-node/build/ /es-node/build/
RUN apk add --no-cache curl grep libstdc++ gcompat libgomp

# Entrypoint
COPY --from=builder /es-node/run.sh /es-node/
RUN chmod +x /es-node/run.sh
WORKDIR /es-node

EXPOSE 9545 9222 9222/udp
