# ============================================================================
# Traffic Orchestrator — additional Terraform outputs
#
# These are consumed by deploy.sh to configure the Ansible playbooks.
# ============================================================================

output "trafficorch_psk" {
  description = "Pre-shared key for the TrafficOrchestrator control channel. Treat as a secret."
  value       = random_password.trafficorch_psk.result
  sensitive   = true
}

output "jumphost_private_ip" {
  description = "Private IP of the jumphost — used by agents as the master address within the VNet."
  value       = azurerm_network_interface.jumphost.ip_configuration[0].private_ip_address
}

output "jumphost_public_ip" {
  description = "Public IP of the jumphost — used for SSH access and the /agents status endpoint."
  value       = azurerm_public_ip.jumphost.ip_address
}

output "linux_workload_ips" {
  description = "Private IPs of all Linux workload VMs."
  value       = [for nic in azurerm_network_interface.linux_workload : nic.ip_configuration[0].private_ip_address]
}

output "windows_workload_ips" {
  description = "Private IPs of all Windows workload VMs."
  value       = [for nic in azurerm_network_interface.windows_workload : nic.ip_configuration[0].private_ip_address]
}

output "trafficorch_status_url" {
  description = "URL to query the agent registry (JSON) from outside the lab."
  value       = "http://${azurerm_public_ip.jumphost.ip_address}:9001/agents"
}
