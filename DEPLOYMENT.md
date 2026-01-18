# Cloud Build Deployment Guide

This guide explains how to set up three independent Cloud Build triggers to deploy the HMM Detector, Trade Predictor, and VM Bot separately, with the VM trigger automatically refreshing endpoints whenever any component changes.

## Architecture Overview

- **Two Cloud Functions (Gen2)**: HMM Detector and Trade Predictor
  - Deployed separately via independent Cloud Build triggers
  - Authenticated via IAM (only VM service account can invoke)
  - No outbound calls to external services
  
- **One VM (GCE)**: Delta Bot
  - Runs the trading bot logic
  - Has static external IP (for Delta Exchange connectivity)
  - Authenticates to Cloud Functions using GCP metadata server identity tokens
  - Triggered to rebuild/restart whenever Go code or either function changes

## Setup Steps

### 1. Create Cloud Build Triggers

Create three separate Cloud Build triggers in your GCP project:

#### Trigger 1: HMM Function Deploy (`function-hmm`)

**Configuration:**
- **Name:** `function-hmm`
- **Build Configuration:** `cloudbuild-hmm.yaml`
- **Included Files (path filter):**
  ```
  cloud_function/app/**
  cloud_function/requirements.txt
  python/regime_ml/src/regime_ml/**
  models/**
  cloudbuild-hmm.yaml
  ```
- **Branch Filter:** `^main$` (or your branch)
- **Substitutions (if needed):**
  - `_HMM_FUNCTION_NAME`: `hmm-detector` (default)
  - `_CLOUD_FUNCTION_REGION`: `us-central1` (default)
  - `_VM_SERVICE_ACCOUNT`: `delta-bot-vm` (default)

**Trigger this via:**
- Push to `main` with changes in `cloud_function/app/` or related files
- Manual trigger for testing

#### Trigger 2: Trade Predictor Function Deploy (`function-trade-predictor`)

**Configuration:**
- **Name:** `function-trade-predictor`
- **Build Configuration:** `cloudbuild-trade-predictor.yaml`
- **Included Files (path filter):**
  ```
  cloud_function/trade_predictor/**
  cloudbuild-trade-predictor.yaml
  ```
- **Branch Filter:** `^main$`
- **Substitutions (if needed):**
  - `_PREDICTOR_FUNCTION_NAME`: `trade-predictor` (default)
  - `_CLOUD_FUNCTION_REGION`: `us-central1` (default)
  - `_VM_SERVICE_ACCOUNT`: `delta-bot-vm` (default)

#### Trigger 3: VM Bot Deploy (`vm-deploy`)

**Configuration:**
- **Name:** `vm-deploy`
- **Build Configuration:** `cloudbuild-vm.yaml`
- **Included Files (path filter):**
  ```
  go/**
  cloud_function/app/**
  cloud_function/trade_predictor/**
  cloudbuild-vm.yaml
  ```
- **Branch Filter:** `^main$`
- **Substitutions (REQUIRED - set per deployment environment):**
  - `_GCE_INSTANCE_NAME`: `delta-bot` (or your instance name)
  - `_GCE_SSH_USER`: `debian` (default for Debian VMs)
  - `_GCE_ZONE`: `us-central1-a` (or your zone)
  - `_GCE_REGION`: `us-central1` (or your region)
  - `_GCE_MACHINE_TYPE`: `e2-micro` (default)
  - `_GCE_BOOT_DISK_TYPE`: `pd-standard` (default)
  - `_GCE_BOOT_DISK_SIZE_GB`: `10` (default)
  - `_VM_SERVICE_ACCOUNT`: `delta-bot-vm` (default)
  - `_SECRET_DELTA_API_KEY`: `delta-api-key` (secret name in Secret Manager)
  - `_SECRET_DELTA_API_SECRET`: `delta-api-secret` (secret name in Secret Manager)

### 2. Prerequisites

Before deploying, ensure:

1. **Service Account:** Create the VM service account:
   ```bash
   gcloud iam service-accounts create delta-bot-vm \
     --display-name="Delta Bot VM Service Account"
   ```

2. **Secrets in Google Secret Manager:**
   ```bash
   echo -n "your-api-key" | gcloud secrets create delta-api-key --data-file=-
   echo -n "your-api-secret" | gcloud secrets create delta-api-secret --data-file=-
   ```

3. **Cloud Build Service Account Permissions:**
   The Cloud Build service account needs permissions to:
   - Deploy Cloud Functions
   - Manage IAM bindings
   - Create/modify GCE instances
   - Access Secret Manager
   - Add static IPs

   Grant these roles:
   ```bash
   PROJECT_ID=$(gcloud config get-value project)
   CB_SA="${PROJECT_ID}@cloudbuild.gserviceaccount.com"
   
   gcloud projects add-iam-policy-binding $PROJECT_ID \
     --member="serviceAccount:${CB_SA}" \
     --role="roles/cloudfunctions.developer"
   
   gcloud projects add-iam-policy-binding $PROJECT_ID \
     --member="serviceAccount:${CB_SA}" \
     --role="roles/iam.securityAdmin"
   
   gcloud projects add-iam-policy-binding $PROJECT_ID \
     --member="serviceAccount:${CB_SA}" \
     --role="roles/compute.admin"
   
   gcloud projects add-iam-policy-binding $PROJECT_ID \
     --member="serviceAccount:${CB_SA}" \
     --role="roles/secretmanager.secretAccessor"
   ```

4. **API Enablement:**
   ```bash
   gcloud services enable cloudfunctions.googleapis.com
   gcloud services enable cloudbuild.googleapis.com
   gcloud services enable compute.googleapis.com
   gcloud services enable secretmanager.googleapis.com
   ```

### 3. Deployment Flow

**Option A: Parallel Independent Triggers (Simple)**

Triggers run in parallel on pushes that match their `includedFiles` patterns:

- Push changes to `cloud_function/app/` → `function-hmm` runs
- Push changes to `cloud_function/trade_predictor/` → `function-trade-predictor` runs
- Push changes to `go/` + either function → `vm-deploy` runs
- Result: Functions update independently; VM picks up latest URLs on its own trigger

**Option B: Sequential Chaining (Safer for URL Freshness)**

If you want to guarantee the VM always gets fresh URLs after function deploys:

1. Function triggers (`function-hmm`, `function-trade-predictor`) deploy independently
2. At the end of each function build config, add a step to invoke the `vm-deploy` trigger:
   ```bash
   - name: 'gcr.io/cloud-builders/gcloud'
     id: 'trigger-vm-refresh'
     entrypoint: 'bash'
     args:
       - '-c'
       - |
         gcloud builds submit \
           --config=cloudbuild-vm.yaml \
           --substitutions "_GCE_INSTANCE_NAME=delta-bot,_GCE_SSH_USER=debian"
   ```

**Recommended for your setup: Option A** (the `includedFiles` pattern in Trigger 3 already covers it).

### 4. How It Works

1. **Function Deploy (HMM/Predictor):**
   - Stages source code with dependencies and models
   - Deploys as Cloud Function Gen2 (Python 3.12)
   - Sets `--no-allow-unauthenticated` (IAM-only access)
   - Grants `roles/run.invoker` to the VM service account

2. **VM Deploy:**
   - Builds Go bot binary (static Linux binary)
   - Queries both Cloud Function URIs via `gcloud functions describe`
   - Fetches Delta Exchange secrets from Secret Manager
   - Provisions or updates GCE VM with static IP
   - Copies bot binary and environment files to VM
   - Installs bot and configures systemd service
   - Writes `/opt/delta-go/config/bot.env` with:
     - `HMM_ENDPOINT=...`
     - `TRADE_PREDICTOR_ENDPOINT=...`
     - `DELTA_API_KEY=...`
     - `DELTA_API_SECRET=...`

3. **VM Bot Execution:**
   - Runs as systemd service on GCE VM
   - Reads endpoints from `/opt/delta-go/config/bot.env`
   - Fetches GCP metadata server identity token (automatically authenticated)
   - Makes authenticated HTTPS calls to Cloud Functions with `Authorization: Bearer <ID_TOKEN>`
   - Connects to Delta Exchange using static external IP

### 5. Verification

After deploying, verify each component:

```bash
# Check HMM function
gcloud functions describe hmm-detector --region us-central1

# Check Trade Predictor function
gcloud functions describe trade-predictor --region us-central1

# Check VM
gcloud compute instances describe delta-bot --zone us-central1-a

# Check VM static IP
gcloud compute addresses describe delta-bot-ip --region us-central1

# SSH into VM and check bot status
gcloud compute ssh debian@delta-bot --zone us-central1-a
# Then on the VM:
sudo systemctl status delta-bot
cat /opt/delta-go/config/bot.env
```

### 6. Updating & Redeploying

- **To update HMM function:** Commit changes to `cloud_function/app/` → `function-hmm` trigger auto-runs
- **To update Trade Predictor:** Commit changes to `cloud_function/trade_predictor/` → `function-trade-predictor` trigger auto-runs
- **To update bot logic:** Commit changes to `go/` → `vm-deploy` trigger auto-runs, rebuilds bot, and restarts it
- **To manually refresh VM endpoints:** Trigger `vm-deploy` manually to re-fetch the latest function URLs

### 7. Security Notes

- **IAM-Only Access:** Only the VM service account (`delta-bot-vm`) can invoke the Cloud Functions (verified by GCP metadata service)
- **Static IP:** The VM has a reserved static external IP for consistent Delta Exchange connectivity
- **Secrets:** Delta API credentials are stored in Google Secret Manager and fetched securely during VM provisioning
- **No Serverless Egress Control Needed:** Only the VM calls Delta Exchange; serverless functions call neither Delta nor other external services

### 8. Troubleshooting

**VM can't call functions:**
- Check Cloud Build logs for IAM binding errors
- Verify `delta-bot-vm` service account has `roles/run.invoker` on both functions
- Ensure VM is running and can reach metadata server: `curl http://metadata.google.internal/computeMetadata/v1/`

**Function URLs not updating on VM:**
- Check that the `get-function-urls` step in `cloudbuild-vm.yaml` completed successfully
- SSH to VM and cat `/opt/delta-go/config/bot.env` to see what URLs are written
- Manually trigger `vm-deploy` to force refresh

**VM provisioning fails:**
- Check that the service account has `roles/compute.admin` and `roles/iam.securityAdmin`
- Verify that `_GCE_INSTANCE_NAME` and `_GCE_SSH_USER` substitutions are set correctly
- Check Cloud Build logs for detailed error messages
