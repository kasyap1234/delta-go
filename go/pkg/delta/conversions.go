package delta

import (
	"fmt"
	"math"
	"strconv"
)

// ParseContractValue parses the string contract value from Product to float64
func ParseContractValue(p *Product) (float64, error) {
	if p == nil {
		return 0, fmt.Errorf("product is nil")
	}
	if p.ContractValue == "" {
		return 0, fmt.Errorf("contract value is empty")
	}
	cv, err := strconv.ParseFloat(p.ContractValue, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse contract value '%s': %w", p.ContractValue, err)
	}
	return cv, nil
}

// NotionalToContracts converts a notional USD amount to number of contracts
// Formula: Contracts = Notional / (Price * ContractValue) for Linear Futures
// Note: This implementation assumes Linear Futures (Inverse contracts would be different)
func NotionalToContracts(notionalUSD float64, price float64, product *Product) (int, error) {
	if price <= 0 {
		return 0, fmt.Errorf("price must be positive")
	}
	cv, err := ParseContractValue(product)
	if err != nil {
		return 0, err
	}
	if cv <= 0 {
		return 0, fmt.Errorf("contract value must be positive")
	}

	// Calculate contracts
	contracts := notionalUSD / (price * cv)

	// Round down to nearest integer to be safe (avoid over-exposure)
	return int(math.Floor(contracts)), nil
}

// ContractsToNotional converts number of contracts to notional USD amount
// Formula: Notional = Contracts * Price * ContractValue for Linear Futures
func ContractsToNotional(contracts int, price float64, product *Product) (float64, error) {
	if price <= 0 {
		return 0, fmt.Errorf("price must be positive")
	}
	cv, err := ParseContractValue(product)
	if err != nil {
		return 0, err
	}

	return float64(contracts) * price * cv, nil
}

// MockProduct returns a Product with typical contract values for backtesting
// without requiring live API calls. These values are based on Delta Exchange specs.
func MockProduct(symbol string) *Product {
	// Default values based on Delta Exchange contract specifications
	// https://docs.delta.exchange
	switch symbol {
	case "BTCUSD", "BTCINR":
		return &Product{
			ID:            27,
			Symbol:        symbol,
			ProductType:   "perpetual_futures",
			ContractValue: "0.001", // 1 contract = 0.001 BTC
			TickSize:      "0.5",
			SettlingAsset: Asset{Symbol: "USDT"},
		}
	case "ETHUSD", "ETHINR":
		return &Product{
			ID:            139,
			Symbol:        symbol,
			ProductType:   "perpetual_futures",
			ContractValue: "0.01", // 1 contract = 0.01 ETH
			TickSize:      "0.05",
			SettlingAsset: Asset{Symbol: "USDT"},
		}
	case "SOLUSD", "SOLINR":
		return &Product{
			ID:            259,
			Symbol:        symbol,
			ProductType:   "perpetual_futures",
			ContractValue: "0.1", // 1 contract = 0.1 SOL
			TickSize:      "0.01",
			SettlingAsset: Asset{Symbol: "USDT"},
		}
	default:
		// Generic default for unknown symbols
		return &Product{
			ID:            0,
			Symbol:        symbol,
			ProductType:   "perpetual_futures",
			ContractValue: "0.001",
			TickSize:      "0.01",
			SettlingAsset: Asset{Symbol: "USDT"},
		}
	}
}
