# Installation

## Requirements

- [Go](https://golang.org) 1.26 or newer
- [Rust](https://rustup.rs) toolchain (for ZK verification library)
- C compiler (for XMSS library)
- [Task](https://taskfile.dev) runner

## Build from Source

Clone the repository and build the blockchain binaries:

```bash
git clone https://github.com/pearl-research-labs/pearl.git
cd pearl
task build:blockchain
```

Binaries are placed in `bin/`:
- `modelosd` — full node
- `prlctl` — CLI control tool
- `oyster` — wallet daemon

To build only the node:

```bash
task build:modelosd
```

## Startup

modelosd will run and start downloading the block chain with no extra
configuration necessary. See the
[configuration documentation](configuration.md) for advanced options.

```bash
./bin/modelosd
```
