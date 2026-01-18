package delta

// Product represents a trading product on Delta Exchange
type Product struct {
	ID                int    `json:"id"`
	Symbol            string `json:"symbol"`
	Description       string `json:"description"`
	ProductType       string `json:"product_type"`
	QuotingAssetID    int    `json:"quoting_asset_id"`
	SettlingAssetID   int    `json:"settling_asset_id"`
	QuotingAsset      Asset  `json:"quoting_asset"`
	SettlingAsset     Asset  `json:"settling_asset"`
	TickSize          string `json:"tick_size"`
	ContractValue     string `json:"contract_value"`
	InitialMargin     string `json:"initial_margin"`
	MaintenanceMargin string `json:"maintenance_margin"`
	ImpactSize        int    `json:"impact_size"`
	MakerCommission   string `json:"maker_commission_rate"`
	TakerCommission   string `json:"taker_commission_rate"`
	IsActive          bool   `json:"is_active"`
}

// Asset represents an asset on Delta Exchange
type Asset struct {
	ID               int    `json:"id"`
	Symbol           string `json:"symbol"`
	Name             string `json:"name"`
	Precision        int    `json:"precision"`
	MinWithdrawLimit string `json:"minimum_withdrawal_limit"`
}

// Ticker represents real-time ticker data
type Ticker struct {
	Symbol      string  `json:"symbol"`
	ProductID   int     `json:"product_id"`
	Close       float64 `json:"close,string"`
	High        float64 `json:"high,string"`
	Low         float64 `json:"low,string"`
	MarkPrice   float64 `json:"mark_price,string"`
	Open        float64 `json:"open,string"`
	Size        float64 `json:"size"`
	Timestamp   int64   `json:"timestamp"`
	Turnover    float64 `json:"turnover,string"`
	Volume      float64 `json:"volume"`
	FundingRate float64 `json:"funding_rate,string"` // 8-hourly funding rate for perpetuals
}

// Candle represents OHLCV data
type Candle struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// Order represents an order on Delta Exchange
type Order struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	Size           int    `json:"size"`
	UnfilledSize   int    `json:"unfilled_size"`
	Side           string `json:"side"` // "buy" or "sell"
	OrderType      string `json:"order_type"`
	LimitPrice     string `json:"limit_price"`
	StopOrderType  string `json:"stop_order_type,omitempty"`
	StopPrice      string `json:"stop_price,omitempty"`
	PaidCommission string `json:"paid_commission"`
	ReduceOnly     bool   `json:"reduce_only"`
	ClientOrderID  string `json:"client_order_id,omitempty"`
	State          string `json:"state"`
	CreatedAt      string `json:"created_at"`
	ProductID      int    `json:"product_id"`
	ProductSymbol  string `json:"product_symbol"`
}

// Position represents a position on Delta Exchange
type Position struct {
	UserID          int64  `json:"user_id"`
	Size            int    `json:"size"`
	EntryPrice      string `json:"entry_price"`
	Margin          string `json:"margin"`
	Liquidation     string `json:"liquidation_price"`
	Bankruptcy      string `json:"bankruptcy_price"`
	RealizedPnL     string `json:"realized_pnl"`
	UnrealizedPnL   string `json:"unrealized_pnl"`
	RealizedFunding string `json:"realized_funding"`
	ProductID       int    `json:"product_id"`
	ProductSymbol   string `json:"product_symbol"`
}

// Wallet represents wallet balance
type Wallet struct {
	AssetID          int    `json:"asset_id"`
	AssetSymbol      string `json:"asset_symbol"`
	AvailableBalance string `json:"available_balance"`
	Balance          string `json:"balance"`
	BlockedMargin    string `json:"blocked_margin"`
	OrderMargin      string `json:"order_margin"`
	PositionMargin   string `json:"position_margin"`
	Commission       string `json:"commission"`
	UserID           int64  `json:"user_id"`
}

// WalletResponse represents the wallet API response
type WalletResponse struct {
	Meta   WalletMeta `json:"meta"`
	Result []Wallet   `json:"result"`
}

// WalletMeta contains metadata for wallet response
type WalletMeta struct {
	NetEquity string `json:"net_equity"`
}

// OrderRequest represents a request to place an order
type OrderRequest struct {
	ProductID     int    `json:"product_id,omitempty"`
	ProductSymbol string `json:"product_symbol,omitempty"`
	Size          int    `json:"size"`
	Side          string `json:"side"`       // "buy" or "sell"
	OrderType     string `json:"order_type"` // "limit_order", "market_order"
	LimitPrice    string `json:"limit_price,omitempty"`
	StopOrderType string `json:"stop_order_type,omitempty"`
	StopPrice     string `json:"stop_price,omitempty"`
	TimeInForce   string `json:"time_in_force,omitempty"` // "gtc", "ioc", "fok"
	PostOnly      bool   `json:"post_only,omitempty"`
	ReduceOnly    bool   `json:"reduce_only,omitempty"`
	ClientOrderID string `json:"client_order_id,omitempty"`

	// Bracket order fields
	BracketStopLossPrice        string `json:"bracket_stop_loss_price,omitempty"`
	BracketStopLossLimitPrice   string `json:"bracket_stop_loss_limit_price,omitempty"`
	BracketTakeProfitPrice      string `json:"bracket_take_profit_price,omitempty"`
	BracketTakeProfitLimitPrice string `json:"bracket_take_profit_limit_price,omitempty"`
}

// MarketRegime represents market state (legacy, kept for compatibility)
type MarketRegime string

const (
	RegimeBull    MarketRegime = "bull"
	RegimeBear    MarketRegime = "bear"
	RegimeRanging MarketRegime = "ranging"
	RegimeHighVol MarketRegime = "high_volatility"
	RegimeLowVol  MarketRegime = "low_volatility"
)
