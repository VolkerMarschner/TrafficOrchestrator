# ============================================================
# Traffic Profile: 3-Tier Web App — Application Server
#
# Middle tier: accepts requests from the frontend/LB layer,
# connects to the database tier and optional caching / message
# broker services.  Typical Java EE / .NET / Node workload.
#
# Part of the three-tier set:
#   three_tier_frontend  → three_tier_appserver → three_tier_database
#
# Assign to hosts tagged  #tag:appserver  in [TARGETS].
# ============================================================

[META]
NAME        = three_tier_appserver
DESCRIPTION = 3-tier web app: application / business logic tier
VERSION     = 1.0
EXTENDS     = base_linux
TAGS        = linux, web, appserver, middleware

[RULES]
# PROTO   ROLE     SRC   DST               PORT   INTV  CNT  #name

# --- Inbound from load balancer / frontend ---
TCP       listen   SELF  -                 8080   -     -    #app-http-listener
TCP       listen   SELF  -                 8443   -     -    #app-https-listener

# --- JMX / management console (internal only) ---
TCP       listen   SELF  -                 9999   -     -    #jmx-mgmt

# --- Outbound: relational database ---
TCP       connect  SELF  group:database    5432   30    3    #db-postgres
TCP       connect  SELF  group:database    3306   30    3    #db-mysql
TCP       connect  SELF  group:database    1433   30    3    #db-mssql

# --- Outbound: distributed cache (Redis / Memcached) ---
TCP       connect  SELF  group:cache       6379   15    2    #redis-cache
TCP       connect  SELF  group:cache       11211  15    2    #memcached

# --- Outbound: message broker (RabbitMQ / Kafka) ---
TCP       connect  SELF  group:mq          5672   20    2    #rabbitmq-amqp
TCP       connect  SELF  group:mq          9092   20    2    #kafka-broker

# --- Outbound: external APIs (HTTPS) ---
TCP       connect  SELF  ANY               443    60    1    #ext-api-https
