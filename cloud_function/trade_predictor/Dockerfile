FROM python:3.11-slim

WORKDIR /app

COPY cloud_function/trade_predictor/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY cloud_function/trade_predictor/app/ ./app/

# Copy pre-trained trade predictor model
COPY models/trade_predictor.pkl /app/models/trade_predictor.pkl

ENV PORT=8080
ENV FUNCTION_TARGET=predict_trade
ENV TRADE_PREDICTOR_MODEL=/app/models/trade_predictor.pkl

CMD exec functions-framework --target=predict_trade --source=app/main.py --port=$PORT
