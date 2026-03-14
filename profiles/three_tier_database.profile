# ============================================================
# Traffic Profile: 3-Tier Web App — Database Tier
#
# Backend database server accepting connections from the
# application tier.  Covers PostgreSQL, MySQL, and MS SQL.
# Also models streaming replication to a replica / standby.
#
# Part of the three-tier set:
#   three_tier_frontend  → three_tier_appserver → three_tier_database
#
# Assign to hosts tagged  #tag:database  in [TARGETS].
# ============================================================

[META]
NAME        = three_tier_database
DESCRIPTION = 3-tier web app: database tier (PostgreSQL / MySQL / MSSQL)
VERSION     = 1.0
EXTENDS     = base_linux
TAGS        = linux, database, backend

[RULES]
# PROTO   ROLE     SRC   DST               PORT   INTV  CNT  #name

# --- PostgreSQL listener ---
TCP       listen   SELF  -                 5432   -     -    #postgres-listener

# --- MySQL / MariaDB listener ---
TCP       listen   SELF  -                 3306   -     -    #mysql-listener

# --- MS SQL Server listener ---
TCP       listen   SELF  -                 1433   -     -    #mssql-listener

# --- PostgreSQL streaming replication to replica ---
TCP       connect  SELF  group:database    5432   10    1    #pg-replication

# --- MySQL binlog replication to replica ---
TCP       connect  SELF  group:database    3306   10    1    #mysql-replication

# --- Backup agent outbound ---
TCP       connect  SELF  group:backup      3000   120   1    #db-backup
