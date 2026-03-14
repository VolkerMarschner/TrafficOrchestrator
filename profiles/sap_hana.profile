# ============================================================
# Traffic Profile: SAP HANA In-Memory Database
#
# Covers SQL/MDX client listeners, HANA XS / Web IDE HTTPS,
# SAP Host Agent management, and outbound System Replication
# traffic to a secondary HANA node.
#
# Assumes tenant DB on system number 00 (ports 300XX).
#
# Assign to hosts tagged  #tag:sap_hana  in [TARGETS].
# ============================================================

[META]
NAME        = sap_hana
DESCRIPTION = SAP HANA in-memory database — SQL listeners and system replication
VERSION     = 1.0
EXTENDS     = base_linux
TAGS        = linux, sap, hana, database

[RULES]
# PROTO   ROLE     SRC   DST               PORT   INTV  CNT  #name

# --- HANA System DB (nameserver) ---
TCP       listen   SELF  -                 30013  -     -    #hana-system-db

# --- HANA Tenant DB — SQL/MDX client access ---
TCP       listen   SELF  -                 30015  -     -    #hana-tenant-sql
TCP       listen   SELF  -                 30017  -     -    #hana-tenant-mdx

# --- HANA Studio / hdbsql internal ---
TCP       listen   SELF  -                 30001  -     -    #hana-nameserver

# --- HANA XS engine (HTTP/HTTPS) ---
TCP       listen   SELF  -                 8090   -     -    #hana-xs-http
TCP       listen   SELF  -                 4390   -     -    #hana-xs-https

# --- SAP Host Agent (instance management) ---
TCP       listen   SELF  -                 1128   -     -    #sap-hostagent-http
TCP       listen   SELF  -                 1129   -     -    #sap-hostagent-https

# --- HANA System Replication (HSR) to secondary node ---
TCP       connect  SELF  group:sap_hana    40001  15    1    #hsr-log-replay
TCP       connect  SELF  group:sap_hana    40002  15    1    #hsr-data-transfer

# --- Backup traffic to backup server ---
TCP       connect  SELF  group:backup      3000   120   1    #hana-backup
