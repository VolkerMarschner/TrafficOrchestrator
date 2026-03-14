# ============================================================
# Traffic Profile: 3-Tier Web App — Frontend / Load Balancer
#
# Reverse proxy or load balancer that terminates public HTTP/S,
# forwards requests to the application tier, and exposes a
# health-check endpoint for upstream monitoring.
#
# Part of the three-tier set:
#   three_tier_frontend  → three_tier_appserver → three_tier_database
#
# Assign to hosts tagged  #tag:frontend  in [TARGETS].
# ============================================================

[META]
NAME        = three_tier_frontend
DESCRIPTION = 3-tier web app: load balancer / reverse proxy
VERSION     = 1.0
EXTENDS     = base_linux
TAGS        = linux, web, frontend, loadbalancer

[RULES]
# PROTO   ROLE     SRC   DST               PORT   INTV  CNT  #name

# --- Public-facing HTTP / HTTPS ---
TCP       listen   SELF  -                 80     -     -    #http-public
TCP       listen   SELF  -                 443    -     -    #https-public

# --- Health check / monitoring endpoint ---
TCP       listen   SELF  -                 8080   -     -    #health-check

# --- Outbound to application tier ---
TCP       connect  SELF  group:appserver   8080   5     5    #proxy-to-app-http
TCP       connect  SELF  group:appserver   8443   5     3    #proxy-to-app-https

# --- OCSP / CRL for TLS certificate validation ---
TCP       connect  SELF  ANY               80     300   1    #ocsp-check
