FROM python:3.13-slim

WORKDIR /app

# Install the shared regime_ml package first
COPY python/regime_ml /opt/regime_ml
RUN pip install --no-cache-dir /opt/regime_ml

# Install Cloud Function dependencies
COPY cloud_function/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy the application code
COPY cloud_function/app/*.py .

# Copy pre-trained models
COPY models/ /app/models/

# Set environment variables
ENV PORT=8080
ENV FUNCTION_TARGET=detect_regime
ENV HMM_MODEL_DIR=/app/models

# Run the function
CMD exec functions-framework --target=detect_regime --port=$PORT
