package rollup

import "math/big"

type EsConfig struct {
	L2ChainID *big.Int `json:"l2_chain_id"`
	// Required to identify the L2 network and create p2p signatures unique for this chain.
	// L2ChainID *big.Int `json:"l2_chain_id"`
}

// // CheckL2ChainID checks that the configured L2 chain ID matches the client's chain ID.
// func (cfg *EsConfig) CheckL2ChainID(ctx context.Context, client L2Client) error {
// 	id, err := client.ChainID(ctx)
// 	if err != nil {
// 		return err
// 	}
// 	if cfg.L2ChainID.Cmp(id) != 0 {
// 		return fmt.Errorf("incorrect L2 RPC chain id, expected from config %d, obtained from client %d", cfg.L2ChainID, id)
// 	}
// 	return nil
// }

// // ValidateL2Config checks L2 config variables for errors.
// func (cfg *EsConfig) ValidateL2Config(ctx context.Context, client L2Client) error {
// 	// Validate the L2 Client Chain ID
// 	if err := cfg.CheckL2ChainID(ctx, client); err != nil {
// 		return err
// 	}

// 	// Validate the Rollup L2 Genesis Blockhash
// 	if err := cfg.CheckL2GenesisBlockHash(ctx, client); err != nil {
// 		return err
// 	}

// 	return nil
// }

func PrefixEnvVar(prefix, suffix string) string {
	return prefix + "_" + suffix
}
