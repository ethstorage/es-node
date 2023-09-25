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