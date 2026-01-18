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
