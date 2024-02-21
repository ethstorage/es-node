# ZK Prover
### Introduction
ZK prover generates zk proofs to prove that a correct encoding mask is computed using Poseidon hash algorithm.

### Implementation
Currently the `Groth16` scheme is used as the prove system, and it is implemented by calling `snarkjs` through CLI in `zk_prover.go`.

### Environment requirements
The following installation is required before you can run a zk prover:
* node v16 or later
* snarkjs v0.7.0 global installation

Note that a product version `.zkey` file in `snarkjs` folder should be used in product environment instead of the version in git.

### Performance 

Rapidsnark vs snarkjs proofing time on my MBP (2.6 GHz 6-Core Intel Core i7) and AX101

proving time | snarkjs | rapidsnark
-- | -- | -- 
MBP | 32.6 s | 5 s
AX101 | 10.23 s | 2.45 s

Rapidsnark already fully uses all the CPU cores on the host machine, so we do not need to run multiple proves concurrently to improve the efficiency.

Go-rapidsnark also provides the go version of the witness calculator, so that our prover can go rid of the nodejs and snarkjs dependencies while gaining faster proving.

We also need to investigate the gnark to decide whether we want to choose it or rapidsnark, but at least we have a better alternative than snarkjs

source: https://github.com/ethstorage/go-ethstorage/pull/14#issuecomment-1590816080



### Performance updates

1. Tests on MacOS amd64 show no performance gain of go-rapidsnark.

 Implementation  | Proof per sample (mode 1) | Proof for 2 samples (mode 2) 
-- | -- | -- 
snarkjs | 43.8  |  60.1
go-rapidsnark + wazero.Circom2WZWitnessCalculator |  47.1 | 82.3
go-rapidsnark + wasmer.Circom2WitnessCalculator | 49.5 | 101.8

Note rapidsnark supports 2 witness calculator implementations which `wazero` wins, so `wazero` is used with go-rapidsnark in below tests.

2. Tests on MacOS arm64 (m3) show a slight performance gain of go-rapidsnark.

 Implementation  | Proof per sample (mode 1) | Proof for 2 samples (mode 2) 
-- | -- | -- 
snarkjs |  16.2 | 31.7
go-rapidsnark| 15.1 | 28.0 

3. Ubuntu (AX101)

 Implementation  | Proof per sample (mode 1) | Proof for 2 samples (mode 2) 
-- | -- | -- 
snarkjs | 12.6  |  24.4
go-rapidsnark |  5.7 | 14.1

4. With `-tags rapidsnark_asm` AX101 has an obvious improvement, esp. mode2：

 Implementation  | Proof per sample (mode 1) | Proof for 2 samples (mode 2) 
-- | -- | -- 
go-rapidsnark |  5.7 | 14.1
go-rapidsnark + `-tags rapidsnark_asm` |  4.6 | 8.2

But this is not the case for Mac. MacOS build has the optimization enabled according to [the go-rapidsnark doc](https://github.com/iden3/go-rapidsnark/tree/main/prover#performance-optimization-on-x86_64-hardware).

5. Tests on MacOS amd64, comparing to rapidsnark (c++)

Implementation|Proof per sample (mode 1)|Proof for 2 samples (mode 2)
-- | -- | --
snarkjs | 43.8 | 60.1 |
go-rapidsnark| 47.1 | 82.3 | 
rapidsnark| 8.2 | 16.7 |

