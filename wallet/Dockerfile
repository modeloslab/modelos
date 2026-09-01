# syntax=docker/dockerfile:1

FROM rust:1.98.0-alpine AS rust-toolchain

FROM golang:1.27.0-alpine AS toolchain

RUN apk add --no-cache curl build-base musl-dev openssl-dev pkgconfig

COPY --from=rust-toolchain /usr/local/cargo /usr/local/cargo
COPY --from=rust-toolchain /usr/local/rustup /usr/local/rustup
ENV PATH="/usr/local/cargo/bin:${PATH}" \
    RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo

RUN sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -b /usr/local/bin v3.46.4

WORKDIR /app

COPY Taskfile.yml Taskfile.yml
COPY plonky2/ plonky2/
COPY pearl-blake3/ pearl-blake3/
COPY zk-pow/ zk-pow/
COPY xmss/ xmss/

RUN task build:zk-gobind build:libxmss

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -tags xmss,zkpow -o /usr/local/bin/wallet ./wallet
# dropwtxmgr: drops the wallet's transaction store (keeps keys/addresses) to force a full
# rescan — needed after a chain reset so the wallet discards stale UTXOs from the old chain.
RUN go build -tags xmss,zkpow -o /usr/local/bin/dropwtxmgr ./wallet/cmd/dropwtxmgr

FROM alpine:latest

RUN apk add --no-cache ca-certificates libgcc libstdc++
RUN adduser -D -s /bin/sh wallet

COPY --from=toolchain /usr/local/bin/wallet /usr/local/bin/wallet
COPY --from=toolchain /usr/local/bin/dropwtxmgr /usr/local/bin/dropwtxmgr

RUN mkdir -p /wallet-data && chown -R wallet:wallet /wallet-data

USER wallet
WORKDIR /wallet-data

EXPOSE 8336

ENTRYPOINT ["wallet"]
