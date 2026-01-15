package delta

import (
	"encoding/json"
	"fmt"
)

// GetWalletBalances returns all wallet balances
func (c *Client) GetWalletBalances() (*WalletResponse, error) {
	resp, err := c.Get("/wallet/balances", nil)
	if err != nil {
		return nil, err
	}

	// Parse the full response including meta
	var walletResp WalletResponse

	// Parse result
	if err := json.Unmarshal(resp.Result, &walletResp.Result); err != nil {
		return nil, fmt.Errorf("failed to parse wallet result: %v", err)
	}

	// Parse meta if present
	if resp.Meta != nil {
		if err := json.Unmarshal(resp.Meta, &walletResp.Meta); err != nil {
			// Meta parsing is optional, don't fail
			walletResp.Meta = WalletMeta{}
		}
	}

	return &walletResp, nil
}

// GetWalletByAsset returns wallet balance for a specific asset
func (c *Client) GetWalletByAsset(assetSymbol string) (*Wallet, error) {
	walletResp, err := c.GetWalletBalances()
	if err != nil {
		return nil, err
	}

	for _, w := range walletResp.Result {
		if w.AssetSymbol == assetSymbol {
			return &w, nil
		}
	}

	return nil, fmt.Errorf("wallet for asset %s not found", assetSymbol)
}

// GetAvailableBalance returns available balance for trading
func (c *Client) GetAvailableBalance(assetSymbol string) (float64, error) {
	wallet, err := c.GetWalletByAsset(assetSymbol)
	if err != nil {
		return 0, err
	}

	var balance float64
	if _, err := fmt.Sscanf(wallet.AvailableBalance, "%f", &balance); err != nil {
		return 0, fmt.Errorf("failed to parse available balance: %v", err)
	}

	return balance, nil
}
