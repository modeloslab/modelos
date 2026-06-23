# modelOS

[![Blockchain / Build and Test](https://github.com/modeloslab/modelos/actions/workflows/blockchain_ci.yml/badge.svg)](https://github.com/modeloslab/modelos/actions/workflows/blockchain_ci.yml)
[![Integration Tests CI](https://github.com/modeloslab/modelos/actions/workflows/integration_tests_ci.yml/badge.svg)](https://github.com/modeloslab/modelos/actions/workflows/integration_tests_ci.yml)
[![Miner CI](https://github.com/modeloslab/modelos/actions/workflows/miner_ci.yml/badge.svg)](https://github.com/modeloslab/modelos/actions/workflows/miner_ci.yml)
[![Miner GPU CI](https://github.com/modeloslab/modelos/actions/workflows/miner_gpu_ci.yml/badge.svg)](https://github.com/modeloslab/modelos/actions/workflows/miner_gpu_ci.yml)
[![Desktop Wallet CI/CD](https://github.com/modeloslab/modelos/actions/workflows/pearl-desktop-wallet.yml/badge.svg)](https://github.com/modeloslab/modelos/actions/workflows/pearl-desktop-wallet.yml)
[![Plonky2 Tests](https://github.com/modeloslab/modelos/actions/workflows/plonky2_ci.yml/badge.svg)](https://github.com/modeloslab/modelos/actions/workflows/plonky2_ci.yml)
[![Rust CI](https://github.com/modeloslab/modelos/actions/workflows/rust_ci.yml/badge.svg)](https://github.com/modeloslab/modelos/actions/workflows/rust_ci.yml)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

**modelOS is a verifiable Proof-of-Useful-Work Layer 1 for decentralised AI inference.**
Mining is real AI computation: instead of arbitrary hashing, miners run the matrix
multiplications that power neural-network inference, and every block carries a Plonky2
zero-knowledge proof that the computation was performed correctly. The energy spent on
consensus is not discarded — it becomes intelligence. The native asset, **MDL**, is earned
exclusively by performing genuine AI work.

modelOS builds the full infrastructure layer on top of the ZK-PoW foundation introduced by
the [Pearl protocol](https://arxiv.org/abs/2504.09971), and is **merge-mined with Pearl**
via AuxPoW — a single GPU run earns both MDL and Pearl rewards.

The network is organised into three interlocking layers:

- **Layer 1 — The Chain.** A UTXO proof-of-work blockchain secured by zero-knowledge proofs
  of real matrix multiplication. Difficulty is governed by **Colossus 2.0** (absolute ASERT,
  integer-exact, ~194 s target block time).
- **Layer 2 — The Market.** The same GPUs that mine the chain serve on-chain inference
  requests. Users lock an MDL fee; miners compete to serve the request and submit a proof;
  the first valid proof claims the fee. No API provider, no custodian.
- **Layer 3 — The Application Layer.** Nova Script AI-native smart contracts, an on-chain
  agentic layer, a Zcash-inspired privacy stack, and consumer apps (Compute chat, ModelCode).

This monorepo contains the full node, wallet, SPV light client, ZK proving system, GPU
miners, the inference pool, and supporting tools.

### Transaction model

modelOS uses a Bitcoin-style UTXO ledger with three transaction versions:

| Version | Name | Purpose |
|---------|------|---------|
| **v1** | Standard transfer | Moves MDL between addresses (Bitcoin-compatible). |
| **v3** | Inference Request | Locks an MDL fee under a covenant encoding the model tier, a prompt commitment, and the requester key — broadcasting an AI job to the market. |
| **v4** | Inference Proof | Carries the miner's proof of correct inference and claims the locked fee; the script releases payment only if the proof is valid and binds to the exact request. |

A request is settled by the first valid proof confirmed within a five-block window
(~16 minutes); if none arrives, the fee is automatically refunded to the requester.

## Roadmap

Layer 1 (ZK-PoW consensus, Colossus 2.0, AuxPoW merge-mining) and the core inference
marketplace (v1/v3/v4 transactions, open miner competition, automatic refunds, the
8B/14B/32B/70B consensus tiers) are **live**. The phases below are **under construction** and
developed in a **private repository** ahead of mainnet activation; each ships subject to
community governance. See [ROADMAP.md](ROADMAP.md) for the detailed status.

- **🚧 Market Depth** — expanding the inference market, model ecosystem, and EVM connectivity.
  - *Frontier model tiers* — MoE proof circuits for gpt-oss (120B / ~20B active), DeepSeek-V3
    & DeepSeek-R1 (671B / ~37B active), Qwen3-MoE, Llama-4 MoE, and Mixtral.
  - *Model hosting economy* — permissionless non-consensus hosting (paid per-hour or in bulk),
    image-model hosting (Flux, Stable Diffusion), staking/slash for uptime, and a decentralized
    GPU shard pool that serves 700B+ models split across many miners.
  - *Marketplace extensions* — Inference Futures, a LoRA adapter marketplace with automatic
    royalties, and a bidirectional MDL ↔ wMDL bridge so any EVM contract can fund inference.
- **🚧 Privacy Architecture** — a Zcash-inspired shielded pool across transfers, inference,
  model weights, and fine-tuning; shielded/private inference; zk-Inference proofs; and a
  general-purpose zkML layer.
- **🚧 Programmable AI** — Nova Script, a Solidity-like contract language with `INFER()`,
  `EMBED()`, and `CLASSIFY()` as native opcodes; the first contracts (Cognitive Swap,
  inference escrow, revenue-share); and the on-chain agentic layer (agent registry,
  contract-storage memory, multi-agent orchestration, bounties).
- **🚧 Network Intelligence** — the MDL-native model: a model trained, owned, and governed by
  the network itself through a recursive distillation loop, plus protocol upgrades
  (floating-point PoUW, post-quantum addresses).

## Repository Layout

| Directory | Description |
|-----------|-------------|
| [`node/`](node/) | **modelosd** — reference implementation of the modelOS protocol (full node, PoUW + inference marketplace) |
| [`wallet/`](wallet/) | **oyster** — HD wallet daemon with JSON-RPC and gRPC interfaces |
| [`spv/`](spv/) | modelOS light client — privacy-preserving SPV client using compact block filters |
| [`dnsseeder/`](dnsseeder/) | DNS seeder for the modelOS network |
| [`coredns-dnsseed/`](coredns-dnsseed/) | CoreDNS plugin — production DNS seeder |
| [`proxy/`](proxy/) | Caddy reverse-proxy sidecar for RPC TLS termination and rate limiting |
| [`xmss/`](xmss/) | XMSS post-quantum signature scheme (C + Go FFI) |
| [`zk-pow/`](zk-pow/) | ZK proof-of-work circuit and verifier (Rust, Plonky2/STARKy) |
| [`pearl-blake3/`](pearl-blake3/) | Blake3 hashing utilities (Rust) |
| [`plonky2/`](plonky2/) | Plonky2 SNARK proving system (Rust, vendored) |
| [`miner/`](miner/) | GPU mining + inference infrastructure — standalone miner, vLLM miner, inference pool, and provider bridge (Rust/C#/Python/CUDA) |
| [`py-pearl-mining/`](py-pearl-mining/) | Python bindings for the mining / proof primitives (Rust/PyO3) |
| [`apps/`](apps/) | Frontend applications (Compute chat app, desktop wallet — pnpm/Turborepo) |
| [`tools/`](tools/) | Go development tool dependencies |

## Prerequisites

- [Go](https://golang.org) 1.26 or newer
- [Rust](https://rustup.rs) toolchain (for ZK and hashing crates)
- C compiler (for XMSS library)
- [Python](https://python.org) 3.12 and [uv](https://docs.astral.sh/uv/) (for the GPU miner packages)
- [Task](https://taskfile.dev) runner
- [CUDA toolkit](https://developer.nvidia.com/cuda-toolkit) (for the GPU miner)

## Building

```bash
task build              # build everything (blockchain + GPU miner)
task build:blockchain   # modelosd, prlctl, oyster → bin/
task build:miner        # install GPU miner Python packages
task build:modelosd     # modelosd node daemon only
```

## Running a Node and Miner

The setup flow: **build** > **create wallet** > **start node** > **start miner**.

### 1. Create a wallet and get a mining address

```bash
./bin/oyster -u rpcuser -P rpcpass --create
```

Follow the prompts to set a passphrase and record your seed. Then start the
wallet and generate a Taproot mining address (an `mdl1p…` address):

```bash
./bin/oyster -u rpcuser -P rpcpass &
./bin/prlctl -u rpcuser -P rpcpass -s https://localhost:44207 getnewaddress
```

### 2. Start the node

```bash
./bin/modelosd \
  --rpcuser=rpcuser \
  --rpcpass=rpcpass \
  --rpclisten=0.0.0.0:44107 \
  --miningaddr=<your-taproot-address> \
  --txindex
```

Key flags: `--testnet` / `--simnet` for non-mainnet, `--notls` to disable TLS,
`--debuglevel=debug` for verbose logs. See `node/sample-modelos.conf` for all
options.

| Network  | RPC   | P2P   | Wallet Server |
|----------|-------|-------|---------------|
| Mainnet  | 44107 | 44108 | 44207         |
| Testnet  | 44109 | 44110 | 44209         |
| Testnet2 | 44111 | 44112 | 44211         |
| Simnet   | 18556 | 18555 | 18554         |
| Regtest  | 18334 | 18444 | 18332         |

### 3. Start the miner

The vLLM miner has two components: **modelos-gateway** (bridge to the node) and
**vllm-miner** (GPU mining + inference via vLLM). The gateway connects to
`modelosd` over JSON-RPC and exposes a mining interface on `/tmp/pearlgw.sock`
(UDS) or port 8337 (TCP, set `MINER_RPC_TRANSPORT=tcp`).

```bash
export PEARLD_RPC_URL="http://localhost:44107"
export PEARLD_RPC_USER="rpcuser"
export PEARLD_RPC_PASSWORD="rpcpass"
export PEARLD_MINING_ADDRESS="<your-taproot-address>"   # mdl1p… — receives MDL rewards + inference fees
modelos-gateway start
```

To run the full stack with Docker:

```bash
docker buildx build -t vllm_miner . -f miner/vllm-miner/Dockerfile

docker run --rm -it --gpus all --network host \
  -e PEARLD_RPC_URL=http://localhost:44107 \
  -e PEARLD_RPC_USER=rpcuser \
  -e PEARLD_RPC_PASSWORD=rpcpass \
  -v ~/.cache/huggingface:/root/.cache/huggingface \
  --shm-size 8g \
  vllm_miner:latest \
  deepseek-ai/DeepSeek-R1-Distill-Llama-70B \
  --host 0.0.0.0 --port 8000
```

> **The public GPU miner** lives in [`miner/vllm-miner/`](miner/vllm-miner/) — it serves AI
> inference and mines in the same GPU run, and is the supported way to contribute compute to
> the network. To mine against the modelOS pool instead of a local node, configure it with
> the `MODELOS_POOL_*` settings (see the miner's README); a single process drives all visible
> GPUs, one worker per card.

## Testing

```bash
task test               # run all tests (Go + Python)
task test:go            # Go tests with race detector
task test:python        # full Python test suite
task test:python:basic  # Python tests (excludes integration/perf/slow)
```

## Formatting and Linting

```bash
task fmt            # format all (Go + Rust + Python)
task lint:python    # lint Python code with ruff
task tidy           # tidy Go dependencies
```

Scoped variants are available: `task fmt:go`, `task fmt:rust`, `task fmt:python`,
`task lint:go`, `task lint:rust`, `task lint:python`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md).

## License

modelOS is licensed under the [copyfree](http://copyfree.org) ISC License.
See [LICENSE](LICENSE) for details.

## Acknowledgments

modelOS stands on the shoulders of **Pearl Research** and the entire Pearl team. Their
work on zero-knowledge Proof-of-Useful-Work — proving that real, useful computation can
replace wasteful hashing as the basis of consensus — is the foundation everything here is
built on. We are deeply grateful for their incredible research and engineering, and for
making it open; modelOS would not exist without it. Thank you. 🙏

modelOS is built on the ZK-PoW foundation introduced by the
[Pearl protocol](https://arxiv.org/abs/2504.09971), and its blockchain infrastructure was
originally forked from the following open-source projects:

- [btcd](https://github.com/btcsuite/btcd) — full node implementation
- [btcwallet](https://github.com/btcsuite/btcwallet) — wallet daemon
- [neutrino](https://github.com/lightninglabs/neutrino) — SPV light client
