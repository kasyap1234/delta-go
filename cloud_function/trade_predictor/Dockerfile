FROM python:3.13-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app/ ./app/

# Create models directory
RUN mkdir -p /app/models

ENV PORT=8082
ENV FUNCTION_TARGET=predict_trade
ENV TRADE_PREDICTOR_MODEL=/app/models/trade_predictor.pkl

CMD ["functions-framework", "--target=predict_trade", "--source=app/main.py", "--port=8082"]
