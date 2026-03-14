# ============================================================
# Traffic Profile: Windows Active Directory Domain Controller
#
# Covers inbound service ports (listen) and outbound DC-to-DC
# replication traffic (connect). Inherits base Windows traffic
# from the base_windows profile via EXTENDS.
#
# Assign to hosts tagged  #tag:dc  in [TARGETS].
# ============================================================

[META]
NAME        = domain_controller
DESCRIPTION = Windows Active Directory Domain Controller — services and replication
VERSION     = 1.0
EXTENDS     = base_windows
TAGS        = windows, active-directory, dc

[RULES]
# PROTO   ROLE     SRC   DST           PORT   INTV  CNT  #name

# --- DNS ---
UDP       listen   SELF  -             53     -     -    #dns-listener
TCP       listen   SELF  -             53     -     -    #dns-tcp-listener
UDP       connect  SELF  group:dc      53     15    2    #dns-dc-replication

# --- LDAP / LDAPS ---
TCP       listen   SELF  -             389    -     -    #ldap-listener
TCP       listen   SELF  -             636    -     -    #ldaps-listener
TCP       connect  SELF  group:dc      389    15    3    #ldap-dc-replication

# --- Kerberos ---
TCP       listen   SELF  -             88     -     -    #kerberos-tcp-listener
UDP       listen   SELF  -             88     -     -    #kerberos-udp-listener

# --- Global Catalog ---
TCP       listen   SELF  -             3268   -     -    #gc-listener
TCP       listen   SELF  -             3269   -     -    #gc-ssl-listener

# --- SMB / SYSVOL / NETLOGON ---
TCP       listen   SELF  -             445    -     -    #smb-listener
TCP       connect  SELF  group:dc      445    30    2    #smb-dc-replication

# --- RPC endpoint mapper ---
TCP       listen   SELF  -             135    -     -    #rpc-listener
