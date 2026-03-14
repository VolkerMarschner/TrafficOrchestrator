# ============================================================================
# Traffic Orchestrator — additional Terraform variables
#
# These extend the variables already defined in the azure_lab_env repo.
# ============================================================================

variable "trafficorch_ttl" {
  description = "Seconds an agent keeps running without a master connection (standalone TTL). 0 = run forever."
  type        = number
  default     = 600
}

variable "linux_vm_profiles" {
  description = <<-EOT
    List of Traffic Orchestrator profile names to assign to Linux workload VMs.
    Assignment is round-robin: VM index modulo list length.
    Available built-in profiles: web_tier, app_tier, db_tier, monitoring_server.
    Leave empty to skip profile assignment (direct rules only).
  EOT
  type    = list(string)
  default = ["web_tier", "app_tier", "db_tier"]
}

variable "windows_vm_profiles" {
  description = <<-EOT
    List of Traffic Orchestrator profile names to assign to Windows workload VMs.
    Assignment is round-robin.
    Available built-in profiles: windows_client, domain_controller.
  EOT
  type    = list(string)
  default = ["windows_client"]
}
