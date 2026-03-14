# Traffic Orchestrator — Automated Deployment Example

This directory contains an **example** of how to integrate Traffic Orchestrator into an
[azure_lab_env](https://github.com/VolkerMarschner/azure_lab_env) environment using
Terraform and Ansible.

> **This is an example only.** It is intended to illustrate one possible automated
> deployment pattern. Adapt paths, profile assignments, and NSG rules to match your
> own environment.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│  Azure Resource Group (azure_lab_env)                           │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Public Subnet                                           │   │
│  │                                                          │   │
│  │  ┌─────────────────────────────────────────────────┐    │   │
│  │  │  Jumphost (Linux)                               │    │   │
│  │  │                                                 │    │   │
│  │  │  trafficorch-master (systemd service)           │    │   │
│  │  │    TCP 9000  ← agent control channel            │    │   │
│  │  │    TCP 9001  ← binary distribution (HTTP)       │    │   │
│  │  └─────────────────────────────────────────────────┘    │   │
│  └──────────────────────────────────────────────────────────┘   │
│              ▲ TCP 9000 / 9001 (NSG rule added by Terraform)    │
│              │                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Private Subnet                                          │   │
│  │                                                          │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │   │
│  │  │ Linux WL-1   │  │ Linux WL-2   │  │ Windows WL-1  │  │   │
│  │  │ trafficorch  │  │ trafficorch  │  │ trafficorch   │  │   │
│  │  │ --agent      │  │ --agent      │  │ --agent       │  │   │
│  │  │ (systemd)    │  │ (systemd)    │  │ (Sched. Task) │  │   │
│  │  └──────────────┘  └──────────────┘  └───────────────┘  │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

**Traffic flow:**
- All agents connect outbound to the master on **port 9000** (control channel).
- On first deploy (or after an update), agents pull the binary from the master on **port 9001** (HTTP, no authentication — the binary is public within the VNet).
- The master generates a random **PSK** once via Terraform and distributes it to all agents via Ansible (never passed in the clear outside the VNet).

---

## Prerequisites

| Tool | Minimum version | Notes |
|------|-----------------|-------|
| `terraform` | 1.3 | Needs `azurerm` and `local` providers |
| `ansible` | 2.12 | With `community.windows` collection |
| `az` CLI | any | Authenticated (`az login`) or via `AZURE_*` env vars |
| `jq` | any | Used by `deploy.sh` for output parsing |

```bash
# Install Ansible Windows collection if not already present
ansible-galaxy collection install community.windows
```

The [azure_lab_env](https://github.com/VolkerMarschner/azure_lab_env) environment must
already be **initialised** (providers downloaded, SSH keys present, inventory generated).
Traffic Orchestrator is layered on top — it does not replace or recreate the base
infrastructure.

You also need the **trafficorch-linux-amd64** binary built from this repository:

```bash
# From the TrafficOrchestrator repo root
make build-linux
```

---

## Directory Structure

```
example-automated-deployment/
├── deploy.sh                        # One-shot orchestration script
│
├── terraform/
│   ├── trafficorch.tf               # PSK, NSG rule, to.conf generation
│   ├── variables-trafficorch.tf     # Customisation variables
│   ├── outputs-trafficorch.tf       # Exported values consumed by Ansible
│   └── templates/
│       └── to.conf.tftpl            # HCL template → master config
│
└── ansible/
    ├── group_vars/
    │   └── all.yml.example          # Documents variables written by deploy.sh
    ├── trafficorch-master-setup.yml # Deploys master on jumphost
    ├── trafficorch-agents-linux.yml # Deploys agents on Linux workload VMs
    └── trafficorch-agents-windows.yml # Deploys agents on Windows workload VMs
```

---

## Quick Start

### 1 — Clone both repositories side by side

```
~/
├── azure_lab_env/          ← existing lab environment
└── TrafficOrchestrator/    ← this repository
```

```bash
git clone https://github.com/VolkerMarschner/azure_lab_env ~/azure_lab_env
git clone https://github.com/VolkerMarschner/TrafficOrchestrator ~/TrafficOrchestrator
```

### 2 — Initialise azure_lab_env (if not done already)

Follow the instructions in the azure_lab_env README to:
- Run `terraform init && terraform apply` to create the base infrastructure
- Verify you can SSH to the jumphost

### 3 — Build the Traffic Orchestrator binary

```bash
cd ~/TrafficOrchestrator
make build-linux          # produces trafficorch-linux-amd64
```

### 4 — Run the deployment

```bash
cd ~/TrafficOrchestrator/example-automated-deployment
./deploy.sh
```

The script will:
1. Check all prerequisites
2. Copy the Terraform additions into `~/azure_lab_env/`
3. Run `terraform apply` to add the PSK, NSG rule, and `to.conf`
4. Extract Terraform outputs and write `ansible/group_vars/all.yml`
5. Run three Ansible playbooks in sequence:
   - `trafficorch-master-setup.yml` — installs master on jumphost
   - `trafficorch-agents-linux.yml` — installs agents on Linux VMs
   - `trafficorch-agents-windows.yml` — installs agents on Windows VMs (if any)
6. Print a summary with the status URL

### 5 — Verify

```bash
# From your local machine
MASTER_IP=$(cd ~/azure_lab_env && terraform output -raw jumphost_public_ip)
curl http://${MASTER_IP}:9001/agents | jq .

# Or SSH to the jumphost
ssh azureuser@${MASTER_IP}
  /usr/local/bin/trafficorch --status
  journalctl -u trafficorch-master -f
```

---

## Command-line Options

```
./deploy.sh [OPTIONS]

  -l, --lab-dir DIR        Path to azure_lab_env directory
                           (default: ../azure_lab_env)
  -b, --binary PATH        Path to trafficorch-linux-amd64 binary
                           (default: ../trafficorch-linux-amd64)
  -p, --profiles DIR       Path to the profiles/ directory
                           (default: ../profiles)
  --tf-only                Run Terraform only, skip Ansible
  --ansible-only           Run Ansible only (Terraform must have run before)
  --destroy                Remove Traffic Orchestrator from all hosts
                           and revert Terraform additions
  -h, --help               Show this help message
```

**Example with explicit paths:**

```bash
./deploy.sh \
  --lab-dir ~/azure_lab_env \
  --binary ~/TrafficOrchestrator/trafficorch-linux-amd64 \
  --profiles ~/TrafficOrchestrator/profiles
```

---

## Customising Profile Assignments

Edit `terraform/variables-trafficorch.tf` (or pass `-var` flags) to change which
Traffic Orchestrator profiles are assigned to VMs:

```hcl
# In your terraform.tfvars or via -var flags:

linux_vm_profiles = [
  "web_tier",
  "app_tier",
  "db_tier",
  "monitoring_server"
]

windows_vm_profiles = [
  "windows_client",
  "email_server"
]
```

Profiles are assigned **round-robin** across the VMs of each group. If you have
3 Linux VMs and specify `["web_tier", "app_tier", "db_tier"]`, each VM gets one
profile. With 4 VMs and 3 profiles, the cycle repeats from the beginning.

All available profiles are in the `profiles/` directory of this repository.

---

## Tearing Down

To stop all Traffic Orchestrator processes and revert the Terraform additions
(without destroying the base lab VMs):

```bash
./deploy.sh --destroy
```

This will:
1. Stop and disable all Linux agents (systemd)
2. Remove the Windows Scheduled Tasks (if any Windows hosts exist)
3. Stop and disable the master service
4. Delete the Terraform files that were copied to azure_lab_env
5. Remove the generated `group_vars/all.yml`

After `--destroy`, run `terraform apply` once in the azure_lab_env directory
to clean up the NSG rule and PSK resource from Azure.

---

## How It Works — Step by Step

### Terraform phase

The three Terraform files added to `azure_lab_env/` do the following:

| File | Purpose |
|------|---------|
| `trafficorch.tf` | Creates a random 32-character PSK, adds an NSG inbound rule (private subnet → jumphost on TCP 9000/9001), and renders `to.conf` via `templatefile()` |
| `variables-trafficorch.tf` | Declares `trafficorch_ttl`, `linux_vm_profiles`, and `windows_vm_profiles` |
| `outputs-trafficorch.tf` | Exports PSK, IPs, and status URL for use by Ansible |

The `to.conf` is generated entirely from Terraform — it already contains all VM IPs
(known after `terraform apply`) with correct profile assignments and the freshly
generated PSK.

### Ansible phase

**Master setup** (`trafficorch-master-setup.yml`):
- Copies the binary to `/usr/local/bin/trafficorch` (mode 0755)
- Copies all profiles to `/opt/trafficorch/profiles/`
- Copies the generated `to.conf` to `/opt/trafficorch/to.conf`
- Creates `/etc/trafficorch-master.env` with `TRAFFICORCH_PSK=...` (mode 0600)
- Installs and starts the `trafficorch-master` systemd service
- Waits for port 9001 to become available before returning

**Linux agent setup** (`trafficorch-agents-linux.yml`):
- Downloads the binary from the master's HTTP server on port 9001
- Verifies the SHA-256 checksum
- Creates `/opt/trafficorch/trafficorch.env` with the PSK (mode 0600)
- Installs and starts the `trafficorch-agent` systemd service
- Runs at `serial: 5` to avoid hammering port 9001

**Windows agent setup** (`trafficorch-agents-windows.yml`):
- Uses WinRM over HTTPS (configured by azure_lab_env)
- Downloads the binary from the master using `win_get_url`
- Verifies the SHA-256 checksum via `win_stat`
- Registers the agent as a **Windows Scheduled Task** (runs at boot as SYSTEM,
  restarts up to 10 times on failure — no NSSM required)
- All PSK-handling tasks use `no_log: true`

### Self-update

After initial deployment, agents check the master's version on every heartbeat.
If the master binary is newer, the master sends an `UPDATE_AVAILABLE` notification
and the agent:
1. Downloads the new binary from `http://master:9001/binary`
2. Verifies the SHA-256 checksum
3. Replaces itself atomically and restarts (Linux: `syscall.Exec`; Windows: helper `.bat`)

Re-deploying only the master binary (via `--ansible-only`) is enough to trigger a
rolling self-update across all connected agents.

---

## Security Notes

- The **PSK** is generated by Terraform, stored in Terraform state, and transmitted
  to hosts only via Ansible over SSH/WinRM (never in the clear over the internet).
- On Linux, the PSK lives in an environment file (`0600`, root-owned) sourced by
  systemd — it never appears in the service unit visible to `systemctl show`.
- On Windows, the PSK is passed as a Scheduled Task argument; `no_log: true` prevents
  it from appearing in Ansible output or logs.
- The binary distribution server (port 9001) is **unauthenticated** — it serves the
  binary to anyone who can reach the jumphost on that port. The NSG rule limits access
  to the private subnet only.
- The `group_vars/all.yml` file (written by `deploy.sh`) contains the PSK in plain
  text. It is mode `0600`, listed in `.gitignore`, and must never be committed.

---

## Troubleshooting

**`terraform apply` fails with "resource not found"**

The Terraform additions reference resources from `azure_lab_env` by name
(e.g. `azurerm_resource_group.main`, `azurerm_network_security_group.jumphost`).
If your azure_lab_env uses different resource names, edit `terraform/trafficorch.tf`
to match.

**Ansible cannot reach the jumphost**

```bash
cd ~/azure_lab_env
ansible jumphost -i inventory -m ping
```

If this fails, verify that `terraform apply` completed successfully and that the
jumphost's public IP is reachable.

**Ansible cannot reach private subnet VMs**

Private VMs are accessed via ProxyJump through the jumphost. Check that the
`ansible_ssh_common_args` in the azure_lab_env inventory includes the correct
ProxyJump directive and that the SSH key is available locally.

**Agents connect but show wrong profile**

The profile name written into `to.conf` comes from `linux_vm_profiles` /
`windows_vm_profiles` in the Terraform variables. Run `terraform output` after
apply to inspect the generated `to.conf`, then re-run Ansible to push any changes.

**Port 9001 not reachable from agents**

Verify the NSG rule was created:

```bash
cd ~/azure_lab_env
terraform show | grep trafficorch_control
```

If the rule is missing, run `terraform apply` again (the `trafficorch.tf` file
must be present in the azure_lab_env directory).
