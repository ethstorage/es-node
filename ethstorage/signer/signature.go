package signer

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strings"

	opcrypto "github.com/ethereum-optimism/optimism/op-service/crypto"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	hdwallet "github.com/ethereum-optimism/go-ethereum-hdwallet"
)

func PrivateKeySignerFn(key *ecdsa.PrivateKey, chainID *big.Int) bind.SignerFn {
	from := crypto.PubkeyToAddress(key.PublicKey)
	signer := types.LatestSignerForChainID(chainID)
	return func(address common.Address, tx *types.Transaction) (*types.Transaction, error) {
		if address != from {
			return nil, bind.ErrNotAuthorized
		}
		signature, err := crypto.Sign(signer.Hash(tx).Bytes(), key)
		if err != nil {
			return nil, err
		}
		return tx.WithSignature(signer, signature)
	}
}

// SignerFactoryFromConfig considers three ways that signers are created & then creates single factory from those config options.
// It can either take a remote signer (via CLIConfig) or it can be provided either a mnemonic + derivation path or a private key.
// It prefers the remote signer, then the mnemonic or private key (only one of which can be provided).
func SignerFactoryFromConfig(signerConfig CLIConfig) (opcrypto.SignerFactory, common.Address, error) {
	var signer opcrypto.SignerFactory
	var fromAddress common.Address
	if signerConfig.RemoteEnabled() {
		signerClient, err := NewSignerClient(signerConfig.Endpoint)
		if err != nil {
			return nil, common.Address{}, fmt.Errorf("failed to create the signer client: %w", err)
		}
		fromAddress = common.HexToAddress(signerConfig.Address)
		signer = func(chainID *big.Int) opcrypto.SignerFn {
			return func(ctx context.Context, address common.Address, tx *types.Transaction) (*types.Transaction, error) {
				if !bytes.Equal(address[:], fromAddress[:]) {
					return nil, fmt.Errorf("attempting to sign for %s, expected %s: ", address, signerConfig.Address)
				}
				return signerClient.SignTransaction(ctx, chainID, address, tx)
			}
		}
	} else {
		var privKey *ecdsa.PrivateKey
		var err error

		if signerConfig.PrivateKey != "" && signerConfig.Mnemonic != "" {
			return nil, common.Address{}, errors.New("cannot specify both a private key and a mnemonic")
		}
		if signerConfig.PrivateKey == "" {
			// Parse l2output wallet private key and L2OO contract address.
			wallet, err := hdwallet.NewFromMnemonic(signerConfig.Mnemonic)
			if err != nil {
				return nil, common.Address{}, fmt.Errorf("failed to parse mnemonic: %w", err)
			}

			privKey, err = wallet.PrivateKey(accounts.Account{
				URL: accounts.URL{
					Path: signerConfig.HDPath,
				},
			})
			if err != nil {
				return nil, common.Address{}, fmt.Errorf("failed to create a wallet: %w", err)
			}
		} else {
			privKey, err = crypto.HexToECDSA(strings.TrimPrefix(signerConfig.PrivateKey, "0x"))
			if err != nil {
				return nil, common.Address{}, fmt.Errorf("failed to parse the private key: %w", err)
			}
		}
		fromAddress = crypto.PubkeyToAddress(privKey.PublicKey)
		signer = func(chainID *big.Int) opcrypto.SignerFn {
			s := PrivateKeySignerFn(privKey, chainID)
			return func(_ context.Context, addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
				return s(addr, tx)
			}
		}
	}

	return signer, fromAddress, nil
}
