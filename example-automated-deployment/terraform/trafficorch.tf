# ============================================================================
# Traffic Orchestrator — Terraform additions for azure_lab_env
#
# Drop this file (and the other files in this directory) alongside the
# existing azure_lab_env Terraform configuration files.
#
# What this file adds:
#   • Random PSK for the control channel
#   • NSG rule allowing agents in the private subnet to reach the master
#     on the jumphost (ports 9000 + 9001)
#   • Generated to.conf written to the local working directory for Ansible
#   • Profile files copied for deployment
# ============================================================================

# ── PSK (generated once, stored in Terraform state) ─────────────────────────

resource "random_password" "trafficorch_psk" {
  length  = 32
  upper   = true
  lower   = true
  numeric = true
  special = false # avoid shell quoting issues in Ansible and config files
}

# ── NSG rule: allow agents → master on control + distribution ports ──────────
#
# The existing jumphost NSG only allows inbound SSH from anywhere.
# Agents (private subnet) need TCP 9000 (control) and TCP 9001 (binary dist).

resource "azurerm_network_security_rule" "trafficorch_control" {
  name                        = "allow-trafficorch-from-private"
  priority                    = 200
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_ranges     = ["9000", "9001"]
  source_address_prefix       = var.private_subnet_prefix[0]
  destination_address_prefix  = "*"
  resource_group_name         = azurerm_resource_group.main.name
  network_security_group_name = azurerm_network_security_group.jumphost.name
}

# ── Generate to.conf from template ──────────────────────────────────────────
#
# Terraform knows all IPs after apply — we use them to build the complete
# master config with [TARGETS] and [ASSIGNMENTS] already filled in.

resource "local_file" "trafficorch_conf" {
  content = templatefile("${path.module}/templates/to.conf.tftpl", {
    psk          = random_password.trafficorch_psk.result
    profile_dir  = "/opt/trafficorch/profiles"
    ttl          = var.trafficorch_ttl

    linux_names = [
      for i in range(var.linux_instance_count) :
      "${var.prefix}-WL-Linux-${i + 1}"
    ]
    linux_ips = [
      for nic in azurerm_network_interface.linux_workload :
      nic.ip_configuration[0].private_ip_address
    ]
    linux_profiles = var.linux_vm_profiles

    windows_names = [
      for i in range(var.windows_instance_count) :
      "${var.prefix}-WL-Windows-${i + 1}"
    ]
    windows_ips = [
      for nic in azurerm_network_interface.windows_workload :
      nic.ip_configuration[0].private_ip_address
    ]
    windows_profiles = var.windows_vm_profiles
  })

  filename        = "${path.module}/to.conf"
  file_permission = "0600"
}
