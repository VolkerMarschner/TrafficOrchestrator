# ============================================================
# Traffic Profile: Windows Domain Client
#
# Standard Windows workstation joined to an Active Directory
# domain. Generates outbound authentication and file-access
# traffic toward DCs and file servers.
#
# Inherits common background traffic from base_windows.
# Assign to workstations / client VMs in [ASSIGNMENTS].
# ============================================================

[META]
NAME        = windows_client
DESCRIPTION = Windows workstation joined to AD domain
VERSION     = 1.0
EXTENDS     = base_windows
TAGS        = windows, client, workstation

[RULES]
# PROTO   ROLE     SRC   DST              PORT   INTV  CNT  #name

# --- DNS queries to DC ---
UDP       connect  SELF  group:dc         53     10    2    #dns-query

# --- LDAP authentication ---
TCP       connect  SELF  group:dc         389    20    3    #ldap-query
TCP       connect  SELF  group:dc         636    20    1    #ldaps-query

# --- Kerberos ---
TCP       connect  SELF  group:dc         88     15    2    #kerberos-tcp
UDP       connect  SELF  group:dc         88     15    2    #kerberos-udp

# --- SMB to DC (SYSVOL / NETLOGON) ---
TCP       connect  SELF  group:dc         445    60    2    #smb-dc

# --- SMB to file servers ---
TCP       connect  SELF  group:fileserver 445    60    5    #smb-files

# --- RDP inbound (someone connecting to this client) ---
TCP       listen   SELF  -                3389   -     -    #rdp-inbound
