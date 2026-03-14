# ============================================================
# Traffic Profile: Microsoft SQL Server
#
# Standalone or clustered SQL Server instance.  Covers the
# default instance listener, SQL Server Browser (UDP), and
# outbound replication / Always On Availability Group traffic
# to peer nodes.
#
# Assign to hosts tagged  #tag:mssql  in [TARGETS].
# ============================================================

[META]
NAME        = mssql_server
DESCRIPTION = Microsoft SQL Server — default instance and Always On replication
VERSION     = 1.0
EXTENDS     = base_windows
TAGS        = windows, database, mssql, sqlserver

[RULES]
# PROTO   ROLE     SRC   DST            PORT   INTV  CNT  #name

# --- SQL Server default instance ---
TCP       listen   SELF  -              1433   -     -    #mssql-listener

# --- SQL Server Browser (named instance discovery) ---
UDP       listen   SELF  -              1434   -     -    #mssql-browser

# --- SQL Server Analysis Services (SSAS) ---
TCP       listen   SELF  -              2383   -     -    #ssas-listener

# --- SQL Server Reporting Services (SSRS) ---
TCP       listen   SELF  -              80     -     -    #ssrs-http
TCP       listen   SELF  -              443    -     -    #ssrs-https

# --- Always On / database mirroring endpoint ---
TCP       listen   SELF  -              5022   -     -    #alwayson-endpoint

# --- Outbound: Always On replication to peer nodes ---
TCP       connect  SELF  group:mssql    5022   10    1    #alwayson-peer

# --- Outbound: Active Directory authentication ---
TCP       connect  SELF  group:dc       389    20    2    #ldap-auth
UDP       connect  SELF  group:dc       53     10    2    #dns-query
