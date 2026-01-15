"""
Walk-Forward Backtesting Engine

This simulator steps through historical data candle-by-candle,
calling the actual HMM model and Go strategy logic to test
the complete trading system without look-ahead bias.

Usage:
    uv run python backtest.py --symbols BTCUSD,ETHUSD,SOLUSD --months 6
"""

import os
import json
import argparse
import requests
import numpy as np
import pandas as pd
from datetime import datetime, timedelta
from dataclasses import dataclass
from typing import List, Dict, Optional, Tuple

from regime_ml import HMMMarketDetector, DeltaDataFetcher, load_model


@dataclass
class Trade:
    """Represents a single trade"""
    symbol: str
    side: str
    entry_time: datetime
    entry_price: float
    size: float
    stop_loss: float
    take_profit: float
    regime: str
    confidence: float
    
    exit_time: Optional[datetime] = None
    exit_price: Optional[float] = None
    exit_reason: Optional[str] = None
    pnl: float = 0.0
    pnl_pct: float = 0.0


@dataclass
class Position:
    """Active position"""
    symbol: str
    side: str
    entry_price: float
    size: float
    stop_loss: float
    take_profit: float
    entry_time: datetime


@dataclass 
class BacktestConfig:
    """Backtest configuration"""
    symbols: List[str]
    start_date: datetime
    end_date: datetime
    initial_capital: float = 10000.0
    leverage: int = 10
    risk_per_trade_pct: float = 1.0
    commission_pct: float = 0.05
    slippage_pct: float = 0.02
    min_confidence: float = 0.6
    hmm_lookback: int = 200
    strategy_server_url: str = "http://localhost:8081"
    daily_loss_limit_pct: float = -5.0
    min_hmm_confidence: float = 0.7
    models_dir: str = "../../models"


class TradeSimulator:
    """Simulates trade execution and tracks P&L"""
    
    def __init__(self, config: BacktestConfig):
        self.config = config
        self.capital = config.initial_capital
        self.equity = config.initial_capital
        self.positions: Dict[str, Position] = {}
        self.trades: List[Trade] = []
        self.equity_curve: List[Tuple[datetime, float]] = []
        
        self.daily_start_balance = config.initial_capital
        self.current_day = None
        self.is_daily_limit_hit = False
        
    def calculate_position_size(self, price: float, stop_loss: float) -> float:
        """Calculate position size based on risk"""
        risk_amount = self.capital * (self.config.risk_per_trade_pct / 100)
        risk_per_unit = abs(price - stop_loss)
        
        if risk_per_unit == 0:
            return 0
            
        size = (risk_amount / risk_per_unit) * self.config.leverage
        max_size = (self.capital * 0.2 * self.config.leverage) / price
        return min(size, max_size)
    
    def check_daily_reset(self, current_ts: datetime):
        """Check if we need to reset daily tracking"""
        current_day = current_ts.date()
        if self.current_day is None or current_day > self.current_day:
            self.current_day = current_day
            self.daily_start_balance = self.capital
            self.is_daily_limit_hit = False
    
    def can_trade_today(self) -> Tuple[bool, str]:
        """Check if trading is allowed"""
        if self.is_daily_limit_hit:
            return False, "daily loss limit hit"
        
        if self.daily_start_balance > 0:
            daily_pnl_pct = ((self.capital - self.daily_start_balance) / self.daily_start_balance) * 100
            if daily_pnl_pct <= self.config.daily_loss_limit_pct:
                self.is_daily_limit_hit = True
                return False, f"daily loss {daily_pnl_pct:.2f}% exceeds limit"
        
        return True, ""
    
    def open_position(
        self,
        symbol: str,
        side: str,
        price: float,
        stop_loss: float,
        take_profit: float,
        regime: str,
        confidence: float,
        timestamp: datetime
    ) -> Optional[Trade]:
        """Open a new position"""
        if symbol in self.positions:
            return None
            
        if side == "buy":
            entry_price = price * (1 + self.config.slippage_pct / 100)
        else:
            entry_price = price * (1 - self.config.slippage_pct / 100)
        
        size = self.calculate_position_size(entry_price, stop_loss)
        if size <= 0:
            return None
            
        commission = entry_price * size * (self.config.commission_pct / 100)
        self.capital -= commission
        
        position = Position(
            symbol=symbol,
            side=side,
            entry_price=entry_price,
            size=size,
            stop_loss=stop_loss,
            take_profit=take_profit,
            entry_time=timestamp
        )
        self.positions[symbol] = position
        
        trade = Trade(
            symbol=symbol,
            side=side,
            entry_time=timestamp,
            entry_price=entry_price,
            size=size,
            stop_loss=stop_loss,
            take_profit=take_profit,
            regime=regime,
            confidence=confidence
        )
        
        return trade
    
    def check_exits(self, symbol: str, high: float, low: float, close: float, timestamp: datetime) -> Optional[Trade]:
        """Check if position should be exited"""
        if symbol not in self.positions:
            return None
            
        pos = self.positions[symbol]
        exit_price = None
        exit_reason = None
        
        if pos.side == "buy":
            if low <= pos.stop_loss:
                exit_price = pos.stop_loss
                exit_reason = "stop_loss"
            elif high >= pos.take_profit:
                exit_price = pos.take_profit
                exit_reason = "take_profit"
        else:
            if high >= pos.stop_loss:
                exit_price = pos.stop_loss
                exit_reason = "stop_loss"
            elif low <= pos.take_profit:
                exit_price = pos.take_profit
                exit_reason = "take_profit"
                
        if exit_price:
            return self._close_position(symbol, exit_price, exit_reason, timestamp)
            
        return None
    
    def close_position(self, symbol: str, price: float, reason: str, timestamp: datetime) -> Optional[Trade]:
        """Close position at market price"""
        return self._close_position(symbol, price, reason, timestamp)
    
    def _close_position(self, symbol: str, price: float, reason: str, timestamp: datetime) -> Optional[Trade]:
        """Internal close position logic"""
        if symbol not in self.positions:
            return None
            
        pos = self.positions.pop(symbol)
        
        if pos.side == "buy":
            exit_price = price * (1 - self.config.slippage_pct / 100)
            pnl = (exit_price - pos.entry_price) * pos.size
        else:
            exit_price = price * (1 + self.config.slippage_pct / 100)
            pnl = (pos.entry_price - exit_price) * pos.size
            
        commission = exit_price * pos.size * (self.config.commission_pct / 100)
        pnl -= commission
        
        self.capital += pnl
        self.equity = self.capital + self._unrealized_pnl({symbol: price})
        
        for trade in reversed(self.trades):
            if trade.symbol == symbol and trade.exit_time is None:
                trade.exit_time = timestamp
                trade.exit_price = exit_price
                trade.exit_reason = reason
                trade.pnl = pnl
                trade.pnl_pct = (pnl / (pos.entry_price * pos.size)) * 100
                return trade
                
        return None
    
    def _unrealized_pnl(self, prices: Dict[str, float]) -> float:
        """Calculate unrealized P&L for open positions"""
        pnl = 0.0
        for symbol, pos in self.positions.items():
            if symbol in prices:
                if pos.side == "buy":
                    pnl += (prices[symbol] - pos.entry_price) * pos.size
                else:
                    pnl += (pos.entry_price - prices[symbol]) * pos.size
        return pnl
    
    def update_equity(self, prices: Dict[str, float], timestamp: datetime):
        """Update equity curve"""
        self.equity = self.capital + self._unrealized_pnl(prices)
        self.equity_curve.append((timestamp, self.equity))


class BacktestEngine:
    """Main backtesting engine"""
    
    def __init__(self, config: BacktestConfig):
        self.config = config
        self.simulator = TradeSimulator(config)
        self.detectors: Dict[str, HMMMarketDetector] = {}
        self.data: Dict[str, pd.DataFrame] = {}
        
        for symbol in config.symbols:
            model_path = f"{config.models_dir}/hmm_model_{symbol}.pkl"
            if os.path.exists(model_path):
                self.detectors[symbol] = load_model(model_path)
                print(f"Loaded HMM model for {symbol}")
            else:
                print(f"Warning: No HMM model for {symbol}, will train on-the-fly")
                self.detectors[symbol] = HMMMarketDetector(n_states=5)
    
    def calculate_atr(self, candles: pd.DataFrame, period: int = 14) -> float:
        """Calculate Average True Range"""
        if len(candles) < period + 1:
            return 0
        
        high = candles['high'].values
        low = candles['low'].values
        close = candles['close'].values
        
        tr = np.zeros(len(candles))
        for i in range(1, len(candles)):
            tr[i] = max(
                high[i] - low[i],
                abs(high[i] - close[i-1]),
                abs(low[i] - close[i-1])
            )
        
        atr = np.mean(tr[-period:])
        return atr
    
    def fetch_data(self):
        """Fetch historical data for all symbols"""
        fetcher = DeltaDataFetcher()
        
        for symbol in self.config.symbols:
            print(f"Fetching data for {symbol}...")
            df = fetcher.fetch_candles(
                symbol=symbol,
                resolution="1h",
                start=self.config.start_date,
                end=self.config.end_date
            )
            self.data[symbol] = df.sort_values('timestamp').reset_index(drop=True)
            print(f"  Loaded {len(df)} candles")
    
    def call_strategy_server(self, candles: pd.DataFrame, regime: str, symbol: str) -> Optional[Dict]:
        """Call Go strategy server"""
        try:
            candle_data = [
                {
                    "time": int(row['timestamp'].timestamp()),
                    "open": row['open'],
                    "high": row['high'],
                    "low": row['low'],
                    "close": row['close'],
                    "volume": row['volume']
                }
                for _, row in candles.iterrows()
            ]
            
            response = requests.post(
                f"{self.config.strategy_server_url}/analyze",
                json={
                    "candles": candle_data,
                    "regime": regime,
                    "symbol": symbol
                },
                timeout=5
            )
            
            if response.status_code == 200:
                return response.json()
        except Exception as e:
            print(f"Strategy server error: {e}")
            
        return None
    
    def run(self) -> 'PerformanceReport':
        """Run the backtest"""
        print("\n" + "=" * 60)
        print("WALK-FORWARD BACKTEST")
        print("=" * 60)
        print(f"Period: {self.config.start_date.date()} to {self.config.end_date.date()}")
        print(f"Symbols: {', '.join(self.config.symbols)}")
        print(f"Initial capital: ${self.config.initial_capital:,.2f}")
        print()
        
        min_len = min(len(self.data[s]) for s in self.config.symbols)
        
        for i in range(self.config.hmm_lookback, min_len):
            if i % 500 == 0:
                print(f"Processing candle {i}/{min_len}...")
            
            current_ts = self.data[self.config.symbols[0]].iloc[i]['timestamp']
            
            self.simulator.check_daily_reset(current_ts)
            
            for symbol in list(self.simulator.positions.keys()):
                candle = self.data[symbol].iloc[i]
                trade = self.simulator.check_exits(
                    symbol,
                    candle['high'],
                    candle['low'],
                    candle['close'],
                    current_ts
                )
                if trade:
                    print(f"  EXIT {symbol}: {trade.exit_reason}, P&L: ${trade.pnl:.2f}")
            
            can_trade, reason = self.simulator.can_trade_today()
            if not can_trade:
                continue
            
            best_signal = None
            best_score = 0
            best_symbol = None
            best_regime = None
            
            for symbol in self.config.symbols:
                if symbol in self.simulator.positions:
                    continue
                    
                df = self.data[symbol]
                candles = df.iloc[i - self.config.hmm_lookback:i + 1]
                
                detector = self.detectors[symbol]
                hmm_result = detector.detect_regime(
                    opens=candles['open'].values,
                    highs=candles['high'].values,
                    lows=candles['low'].values,
                    closes=candles['close'].values,
                    volumes=candles['volume'].values
                )
                
                regime = hmm_result.get('regime', 'ranging')
                hmm_conf = hmm_result.get('confidence', 0.5)
                
                if hmm_conf < self.config.min_hmm_confidence:
                    continue
                
                signal = self.call_strategy_server(candles.tail(200), regime, symbol)
                
                if signal and signal.get('action') in ['buy', 'sell']:
                    strat_conf = signal.get('confidence', 0)
                    total_score = (strat_conf * 0.6) + (hmm_conf * 0.4)
                    
                    if total_score >= self.config.min_confidence and total_score > best_score:
                        best_signal = signal
                        best_score = total_score
                        best_symbol = symbol
                        best_regime = regime
            
            if best_signal and best_symbol:
                if i + 1 < len(self.data[best_symbol]):
                    next_candle = self.data[best_symbol].iloc[i + 1]
                    entry_price = next_candle['open']
                    
                    trade = self.simulator.open_position(
                        symbol=best_symbol,
                        side=best_signal['side'],
                        price=entry_price,
                        stop_loss=best_signal.get('stop_loss', entry_price * 0.98),
                        take_profit=best_signal.get('take_profit', entry_price * 1.04),
                        regime=best_regime,
                        confidence=best_score,
                        timestamp=next_candle['timestamp']
                    )
                    
                    if trade:
                        self.simulator.trades.append(trade)
                        print(f"  ENTRY {best_symbol}: {trade.side} @ ${trade.entry_price:.2f} (score: {best_score:.2f})")
            
            current_prices = {s: self.data[s].iloc[i]['close'] for s in self.config.symbols}
            self.simulator.update_equity(current_prices, current_ts)
        
        for symbol in list(self.simulator.positions.keys()):
            last_price = self.data[symbol].iloc[-1]['close']
            last_ts = self.data[symbol].iloc[-1]['timestamp']
            self.simulator.close_position(symbol, last_price, "end_of_backtest", last_ts)
        
        return PerformanceReport(self.simulator, self.config)


class PerformanceReport:
    """Generate performance metrics from backtest"""
    
    def __init__(self, simulator: TradeSimulator, config: BacktestConfig):
        self.simulator = simulator
        self.config = config
        self.metrics = self._calculate_metrics()
    
    def _calculate_metrics(self) -> Dict:
        """Calculate all performance metrics"""
        trades = self.simulator.trades
        equity_curve = self.simulator.equity_curve
        
        if not trades:
            return {"error": "No trades executed"}
        
        total_trades = len(trades)
        winning_trades = len([t for t in trades if t.pnl > 0])
        losing_trades = len([t for t in trades if t.pnl < 0])
        
        gross_profit = sum(t.pnl for t in trades if t.pnl > 0)
        gross_loss = abs(sum(t.pnl for t in trades if t.pnl < 0))
        
        net_pnl = sum(t.pnl for t in trades)
        total_return_pct = (net_pnl / self.config.initial_capital) * 100
        
        win_rate = (winning_trades / total_trades) * 100 if total_trades > 0 else 0
        profit_factor = gross_profit / gross_loss if gross_loss > 0 else float('inf')
        
        avg_win = gross_profit / winning_trades if winning_trades > 0 else 0
        avg_loss = gross_loss / losing_trades if losing_trades > 0 else 0
        
        equity_values = [e[1] for e in equity_curve]
        peak = equity_values[0]
        max_dd = 0
        for eq in equity_values:
            if eq > peak:
                peak = eq
            dd = (peak - eq) / peak * 100
            if dd > max_dd:
                max_dd = dd
        
        if len(equity_values) > 1:
            returns = np.diff(equity_values) / equity_values[:-1]
            sharpe = (np.mean(returns) / np.std(returns)) * np.sqrt(24 * 365) if np.std(returns) > 0 else 0
        else:
            sharpe = 0
        
        durations = []
        for t in trades:
            if t.exit_time and t.entry_time:
                duration = (t.exit_time - t.entry_time).total_seconds() / 3600
                durations.append(duration)
        avg_duration = np.mean(durations) if durations else 0
        
        return {
            "total_trades": total_trades,
            "winning_trades": winning_trades,
            "losing_trades": losing_trades,
            "win_rate_pct": round(win_rate, 2),
            "profit_factor": round(profit_factor, 2),
            "net_pnl": round(net_pnl, 2),
            "total_return_pct": round(total_return_pct, 2),
            "max_drawdown_pct": round(max_dd, 2),
            "sharpe_ratio": round(sharpe, 2),
            "avg_win": round(avg_win, 2),
            "avg_loss": round(avg_loss, 2),
            "avg_duration_hours": round(avg_duration, 1),
            "final_equity": round(self.simulator.equity, 2)
        }
    
    def print_report(self):
        """Print formatted report"""
        m = self.metrics
        
        print("\n" + "=" * 60)
        print("BACKTEST RESULTS")
        print("=" * 60)
        
        print(f"\nPerformance Summary")
        print(f"  Initial Capital:    ${self.config.initial_capital:,.2f}")
        print(f"  Final Equity:       ${m.get('final_equity', 0):,.2f}")
        print(f"  Net P&L:            ${m.get('net_pnl', 0):,.2f}")
        print(f"  Total Return:       {m.get('total_return_pct', 0):+.2f}%")
        
        print(f"\nRisk Metrics")
        print(f"  Sharpe Ratio:       {m.get('sharpe_ratio', 0):.2f}")
        print(f"  Max Drawdown:       {m.get('max_drawdown_pct', 0):.2f}%")
        print(f"  Profit Factor:      {m.get('profit_factor', 0):.2f}")
        
        print(f"\nTrade Statistics")
        print(f"  Total Trades:       {m.get('total_trades', 0)}")
        print(f"  Win Rate:           {m.get('win_rate_pct', 0):.1f}%")
        print(f"  Avg Win:            ${m.get('avg_win', 0):,.2f}")
        print(f"  Avg Loss:           ${m.get('avg_loss', 0):,.2f}")
        print(f"  Avg Duration:       {m.get('avg_duration_hours', 0):.1f} hours")
        
        print("\n" + "=" * 60)
    
    def save_report(self, path: str):
        """Save report to JSON"""
        report = {
            "config": {
                "symbols": self.config.symbols,
                "start_date": self.config.start_date.isoformat(),
                "end_date": self.config.end_date.isoformat(),
                "initial_capital": self.config.initial_capital
            },
            "metrics": self.metrics,
            "trades": [
                {
                    "symbol": t.symbol,
                    "side": t.side,
                    "entry_time": t.entry_time.isoformat(),
                    "entry_price": t.entry_price,
                    "exit_time": t.exit_time.isoformat() if t.exit_time else None,
                    "exit_price": t.exit_price,
                    "exit_reason": t.exit_reason,
                    "pnl": t.pnl,
                    "regime": t.regime,
                    "confidence": t.confidence
                }
                for t in self.simulator.trades
            ]
        }
        
        with open(path, 'w') as f:
            json.dump(report, f, indent=2)
        print(f"\nReport saved to {path}")


def main():
    parser = argparse.ArgumentParser(description='Walk-Forward Backtest')
    parser.add_argument('--symbols', type=str, default='BTCUSD,ETHUSD,SOLUSD',
                      help='Comma-separated symbols')
    parser.add_argument('--months', type=int, default=6,
                      help='Months to backtest')
    parser.add_argument('--capital', type=float, default=10000,
                      help='Initial capital')
    parser.add_argument('--leverage', type=int, default=10,
                      help='Leverage')
    parser.add_argument('--output', type=str, default='backtest_results.json',
                      help='Output file path')
    parser.add_argument('--models-dir', type=str, default='../../models',
                      help='Directory containing HMM models')
    
    args = parser.parse_args()
    
    end_date = datetime.now()
    start_date = end_date - timedelta(days=args.months * 30)
    
    config = BacktestConfig(
        symbols=[s.strip() for s in args.symbols.split(',')],
        start_date=start_date,
        end_date=end_date,
        initial_capital=args.capital,
        leverage=args.leverage,
        models_dir=args.models_dir
    )
    
    engine = BacktestEngine(config)
    engine.fetch_data()
    report = engine.run()
    
    report.print_report()
    report.save_report(args.output)


if __name__ == "__main__":
    main()
