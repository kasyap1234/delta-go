"""
Delta Exchange Data Fetcher

Fetches historical OHLCV data from Delta Exchange API.
"""

import pandas as pd
import requests
from datetime import datetime, timedelta


class DeltaDataFetcher:
    """Fetches historical data from Delta Exchange API"""
    
    def __init__(self, base_url: str = "https://api.india.delta.exchange/v2"):
        self.base_url = base_url
        self.session = requests.Session()
        
    def fetch_candles(
        self, 
        symbol: str, 
        resolution: str, 
        start: datetime, 
        end: datetime
    ) -> pd.DataFrame:
        """
        Fetch OHLCV candles from Delta Exchange.
        
        Args:
            symbol: Trading pair (e.g., 'BTCUSD')
            resolution: Candle interval (e.g., '5m', '1h', '1d')
            start: Start datetime
            end: End datetime
            
        Returns:
            DataFrame with OHLCV data
        """
        all_candles = []
        current_start = start
        
        while current_start < end:
            chunk_end = min(current_start + timedelta(days=30), end)
            
            params = {
                'symbol': symbol,
                'resolution': resolution,
                'start': int(current_start.timestamp()),
                'end': int(chunk_end.timestamp())
            }
            
            try:
                response = self.session.get(
                    f"{self.base_url}/history/candles",
                    params=params,
                    timeout=30
                )
                response.raise_for_status()
                data = response.json()
                
                if data.get('success') and data.get('result'):
                    all_candles.extend(data['result'])
                    
            except Exception as e:
                print(f"Warning: Failed to fetch candles for {current_start}: {e}")
                
            current_start = chunk_end
            
        if not all_candles:
            raise ValueError("No candle data fetched")
            
        df = pd.DataFrame(all_candles)
        df['timestamp'] = pd.to_datetime(df['time'], unit='s')
        df = df.rename(columns={
            'open': 'open',
            'high': 'high', 
            'low': 'low',
            'close': 'close',
            'volume': 'volume'
        })
        
        for col in ['open', 'high', 'low', 'close', 'volume']:
            df[col] = pd.to_numeric(df[col], errors='coerce')
            
        df = df.sort_values('timestamp').drop_duplicates('timestamp')
        df = df.dropna()
        
        return df
