#!/usr/bin/env bash
# =============================================================================
# Traffic Orchestrator — Automated Deployment Script
#
# Integrates Traffic Orchestrator into an existing azure_lab_env environment.
#
# Usage:
#   ./deploy.sh [OPTIONS]
#
# Options:
#   -l, --lab-dir DIR        Path to the azure_lab_env directory
#                            (default: ../azure_lab_env relative to this script)
#   -b, --binary PATH        Path to trafficorch-linux-amd64 binary
#                            (default: ../trafficorch-linux-amd64)
#   -p, --profiles DIR       Path to the profiles/ directory
#                            (default: ../profiles)
#   --tf-only                Run Terraform only, skip Ansible
#   --ansible-only           Run Ansible only (Terraform must have run before)
#   --destroy                Tear down: remove TrafficOrchestrator from all hosts
#                            and revert Terraform additions
#   -h, --help               Show this help message
#
# Prerequisites:
#   - terraform (>= 1.3)
#   - ansible (>= 2.12)  with community.windows collection
#   - azure_lab_env already initialised (providers, SSH keys present)
#   - azure-cli authenticated (az login) or AZURE_* env vars set
#
# Example:
#   ./deploy.sh --lab-dir ~/azure_lab_env --binary ../trafficorch-linux-amd64
# =============================================================================

set -euo pipefail

# ── Constants ──────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

TERRAFORM_DIR="${SCRIPT_DIR}/terraform"
ANSIBLE_DIR="${SCRIPT_DIR}/ansible"

# ── Defaults ──────────────────────────────────────────────────────────────

LAB_DIR="${REPO_ROOT}/../azure_lab_env"
BINARY_PATH="${REPO_ROOT}/trafficorch-linux-amd64"
PROFILES_DIR="${REPO_ROOT}/profiles"
RUN_TERRAFORM=true
RUN_ANSIBLE=true
DESTROY=false

# ── Colours ───────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

log()     { echo -e "${BLUE}[deploy]${NC} $*"; }
success() { echo -e "${GREEN}[deploy]${NC} ✓ $*"; }
warn()    { echo -e "${YELLOW}[deploy]${NC} ⚠ $*"; }
error()   { echo -e "${RED}[deploy]${NC} ✗ $*" >&2; }
header()  { echo -e "\n${BOLD}═══ $* ═══${NC}\n"; }

# ── Argument parsing ──────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
  case "$1" in
    -l|--lab-dir)      LAB_DIR="$2";      shift 2 ;;
    -b|--binary)       BINARY_PATH="$2";  shift 2 ;;
    -p|--profiles)     PROFILES_DIR="$2"; shift 2 ;;
    --tf-only)         RUN_ANSIBLE=false; shift   ;;
    --ansible-only)    RUN_TERRAFORM=false; shift  ;;
    --destroy)         DESTROY=true;      shift   ;;
    -h|--help)         sed -n '/^# ====/,/^# ====/p' "$0" | head -40; exit 0 ;;
    *) error "Unknown option: $1"; exit 1 ;;
  esac
done

# ── Prerequisite checks ────────────────────────────────────────────────────

check_prereqs() {
  header "Checking prerequisites"

  local missing=0

  for cmd in terraform ansible ansible-playbook jq; do
    if command -v "$cmd" &>/dev/null; then
      success "$cmd: $(command -v "$cmd")"
    else
      error "$cmd: not found in PATH"
      missing=$((missing + 1))
    fi
  done

  # Resolve and validate paths
  LAB_DIR="$(realpath "${LAB_DIR}")"
  BINARY_PATH="$(realpath "${BINARY_PATH}")"
  PROFILES_DIR="$(realpath "${PROFILES_DIR}")"

  if [[ ! -d "$LAB_DIR" ]]; then
    error "azure_lab_env directory not found: $LAB_DIR"
    error "Clone it first: git clone https://github.com/VolkerMarschner/azure_lab_env $LAB_DIR"
    missing=$((missing + 1))
  else
    success "Lab directory: $LAB_DIR"
    # Verify it looks like azure_lab_env
    if [[ ! -f "$LAB_DIR/main.tf" ]]; then
      error "$LAB_DIR/main.tf not found — is this an azure_lab_env directory?"
      missing=$((missing + 1))
    fi
  fi

  if [[ ! -f "$BINARY_PATH" ]]; then
    error "TrafficOrchestrator binary not found: $BINARY_PATH"
    error "Build it first: make build-linux (from the TrafficOrchestrator repo root)"
    missing=$((missing + 1))
  else
    success "Binary: $BINARY_PATH"
    # Print version
    "$BINARY_PATH" --version 2>/dev/null || true
  fi

  if [[ ! -d "$PROFILES_DIR" ]]; then
    error "Profiles directory not found: $PROFILES_DIR"
    missing=$((missing + 1))
  else
    local profile_count
    profile_count=$(find "$PROFILES_DIR" -name "*.profile" | wc -l | tr -d ' ')
    success "Profiles directory: $PROFILES_DIR ($profile_count profiles)"
  fi

  if [[ $missing -gt 0 ]]; then
    error "$missing prerequisite(s) missing — aborting"
    exit 1
  fi

  # Check Ansible Windows collection
  if ! ansible-galaxy collection list 2>/dev/null | grep -q "community.windows"; then
    warn "community.windows Ansible collection not found"
    warn "Install with: ansible-galaxy collection install community.windows"
  fi
}

# ── Terraform phase ────────────────────────────────────────────────────────

run_terraform() {
  header "Terraform — provisioning infrastructure"

  # Copy our Terraform additions to the lab directory
  log "Copying Terraform additions to $LAB_DIR..."
  cp "${TERRAFORM_DIR}/trafficorch.tf"              "$LAB_DIR/"
  cp "${TERRAFORM_DIR}/variables-trafficorch.tf"    "$LAB_DIR/"
  cp "${TERRAFORM_DIR}/outputs-trafficorch.tf"      "$LAB_DIR/"
  mkdir -p "$LAB_DIR/templates"
  cp "${TERRAFORM_DIR}/templates/to.conf.tftpl"     "$LAB_DIR/templates/"
  success "Terraform files copied"

  # Apply
  cd "$LAB_DIR"
  log "Running terraform init..."
  terraform init -upgrade -input=false

  log "Running terraform apply..."
  terraform apply -auto-approve -input=false

  success "Terraform apply complete"
}

# ── Extract Terraform outputs ──────────────────────────────────────────────

extract_outputs() {
  header "Extracting Terraform outputs"

  cd "$LAB_DIR"

  TRAFFICORCH_PSK="$(terraform output -raw trafficorch_psk)"
  MASTER_PRIVATE_IP="$(terraform output -raw jumphost_private_ip)"
  MASTER_PUBLIC_IP="$(terraform output -raw jumphost_public_ip)"
  STATUS_URL="$(terraform output -raw trafficorch_status_url)"
  INVENTORY_FILE="${LAB_DIR}/inventory"

  if [[ ! -f "$INVENTORY_FILE" ]]; then
    error "Ansible inventory not found at $INVENTORY_FILE"
    error "Run terraform apply first to generate the inventory"
    exit 1
  fi

  log "Master private IP : $MASTER_PRIVATE_IP"
  log "Master public IP  : $MASTER_PUBLIC_IP"
  log "Status URL        : $STATUS_URL"
  log "Inventory         : $INVENTORY_FILE"
  success "Outputs extracted"
}

# ── Generate Ansible group_vars ────────────────────────────────────────────

write_group_vars() {
  header "Writing Ansible group_vars"

  mkdir -p "${ANSIBLE_DIR}/group_vars"

  cat > "${ANSIBLE_DIR}/group_vars/all.yml" <<EOF
# Auto-generated by deploy.sh — DO NOT COMMIT
# Regenerate by running deploy.sh again
trafficorch_psk: "${TRAFFICORCH_PSK}"
trafficorch_master_ip: "${MASTER_PRIVATE_IP}"
trafficorch_master_public_ip: "${MASTER_PUBLIC_IP}"
trafficorch_binary_local: "${BINARY_PATH}"
trafficorch_profiles_local: "${PROFILES_DIR}"
EOF
  chmod 0600 "${ANSIBLE_DIR}/group_vars/all.yml"
  success "group_vars/all.yml written (permissions: 600)"
}

# ── Ansible phase ──────────────────────────────────────────────────────────

run_ansible() {
  header "Ansible — deploying Traffic Orchestrator"

  local inventory="$INVENTORY_FILE"

  # Verify connectivity to jumphost before starting
  log "Testing Ansible connectivity to jumphost..."
  if ! ansible jumphost -i "$inventory" -m ping --timeout=20 &>/dev/null; then
    warn "Jumphost ping failed — VMs may still be booting, waiting 30s..."
    sleep 30
    ansible jumphost -i "$inventory" -m ping --timeout=30
  fi
  success "Jumphost reachable"

  # Step 1: Master on jumphost
  log "Step 1/3 — Deploying master on jumphost..."
  ansible-playbook \
    -i "$inventory" \
    "${ANSIBLE_DIR}/trafficorch-master-setup.yml" \
    --diff
  success "Master deployed"

  # Step 2: Linux agents
  log "Step 2/3 — Deploying Linux agents..."
  ansible-playbook \
    -i "$inventory" \
    "${ANSIBLE_DIR}/trafficorch-agents-linux.yml" \
    --diff
  success "Linux agents deployed"

  # Step 3: Windows agents (if any Windows hosts exist in inventory)
  if grep -q '^\[windows\]' "$inventory" && \
     grep -A5 '^\[windows\]' "$inventory" | grep -q '^[^[]'; then
    log "Step 3/3 — Deploying Windows agents..."
    ansible-playbook \
      -i "$inventory" \
      "${ANSIBLE_DIR}/trafficorch-agents-windows.yml" \
      --diff
    success "Windows agents deployed"
  else
    log "Step 3/3 — No Windows hosts in inventory, skipping"
  fi
}

# ── Destroy phase ──────────────────────────────────────────────────────────

run_destroy() {
  header "Removing Traffic Orchestrator from all hosts"
  warn "This will stop all agents and remove the master, but will NOT destroy VMs"

  cd "$LAB_DIR"

  local inventory="${LAB_DIR}/inventory"
  if [[ -f "$inventory" ]]; then
    # Stop and disable Linux agents
    ansible linux_workload -i "$inventory" -b \
      -m systemd -a "name=trafficorch-agent state=stopped enabled=false" \
      --ignore-errors || true

    # Stop and disable Windows agents
    if grep -q '^\[windows\]' "$inventory"; then
      ansible windows -i "$inventory" \
        -m win_scheduled_task -a "name=TrafficOrchestrator state=absent" \
        --ignore-errors || true
    fi

    # Stop master
    ansible jumphost -i "$inventory" -b \
      -m systemd -a "name=trafficorch-master state=stopped enabled=false" \
      --ignore-errors || true
  fi

  # Remove Terraform additions
  rm -f "$LAB_DIR/trafficorch.tf" \
        "$LAB_DIR/variables-trafficorch.tf" \
        "$LAB_DIR/outputs-trafficorch.tf" \
        "$LAB_DIR/to.conf" \
        "$LAB_DIR/templates/to.conf.tftpl"

  # Remove generated group_vars
  rm -f "${ANSIBLE_DIR}/group_vars/all.yml"

  success "Traffic Orchestrator removed"
  warn "Run 'terraform apply' in $LAB_DIR to clean up the NSG rule and PSK resource"
}

# ── Status / summary ───────────────────────────────────────────────────────

print_summary() {
  header "Deployment complete"

  echo -e "${GREEN}Traffic Orchestrator is running in your lab environment.${NC}"
  echo ""
  echo "  Master:     ${MASTER_PUBLIC_IP} (jumphost)"
  echo "  Status URL: ${STATUS_URL}"
  echo ""
  echo "  Check agent status:"
  echo "    curl ${STATUS_URL} | jq ."
  echo ""
  echo "  SSH into jumphost and run:"
  echo "    /usr/local/bin/trafficorch --status"
  echo "    journalctl -u trafficorch-master -f"
  echo ""
  echo "  Agents self-update automatically when you redeploy the master binary."
}

# ── Main ───────────────────────────────────────────────────────────────────

main() {
  echo -e "\n${BOLD}Traffic Orchestrator — Automated Deployment${NC}"
  echo -e "Lab directory  : ${LAB_DIR}"
  echo -e "Binary         : ${BINARY_PATH}"
  echo -e "Profiles       : ${PROFILES_DIR}"
  echo ""

  check_prereqs

  if [[ "$DESTROY" == "true" ]]; then
    # Outputs may not exist yet for destroy; try to extract them
    extract_outputs 2>/dev/null || true
    run_destroy
    exit 0
  fi

  if [[ "$RUN_TERRAFORM" == "true" ]]; then
    run_terraform
  fi

  extract_outputs
  write_group_vars

  if [[ "$RUN_ANSIBLE" == "true" ]]; then
    run_ansible
    print_summary
  else
    log "Skipping Ansible (--tf-only mode)"
    log "Run Ansible manually:"
    log "  ansible-playbook -i ${INVENTORY_FILE} ${ANSIBLE_DIR}/trafficorch-master-setup.yml"
    log "  ansible-playbook -i ${INVENTORY_FILE} ${ANSIBLE_DIR}/trafficorch-agents-linux.yml"
    log "  ansible-playbook -i ${INVENTORY_FILE} ${ANSIBLE_DIR}/trafficorch-agents-windows.yml"
  fi
}

main
