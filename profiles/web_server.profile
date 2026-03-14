# ============================================================
# Traffic Profile: Web / Application Server
#
# HTTP and HTTPS inbound listeners plus typical outbound
# connections to database and backend hosts.
# ============================================================

[META]
NAME        = web_server
DESCRIPTION = Web / application server — HTTP/HTTPS listeners and backend connections
VERSION     = 1.0
TAGS        = linux, web, http

[RULES]
# PROTO   ROLE     SRC   DST            PORT   INTV  CNT  #name

# --- Inbound HTTP / HTTPS ---
TCP       listen   SELF  -              80     -     -    #http-listener
TCP       listen   SELF  -              443    -     -    #https-listener

# --- Outbound to database ---
TCP       connect  SELF  group:database 3306   30    3    #mysql-query
TCP       connect  SELF  group:database 5432   30    3    #postgres-query

# --- Health-check / load balancer probes ---
TCP       listen   SELF  -              8080   -     -    #health-listener
