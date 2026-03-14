# ============================================================
# Traffic Profile: Microsoft Exchange Server
#
# On-premises Exchange (2016/2019) handling inbound SMTP,
# client access (OWA, ActiveSync, Autodiscover via HTTPS),
# and inter-server mail flow within a DAG.
#
# Assign to hosts tagged  #tag:exchange  in [TARGETS].
# ============================================================

[META]
NAME        = exchange_server
DESCRIPTION = Microsoft Exchange Server — mail flow, client access and DAG replication
VERSION     = 1.0
EXTENDS     = base_windows
TAGS        = windows, exchange, email, messaging

[RULES]
# PROTO   ROLE     SRC   DST               PORT   INTV  CNT  #name

# --- SMTP inbound (MTA and inter-org relay) ---
TCP       listen   SELF  -                 25     -     -    #smtp-inbound

# --- SMTP submission (authenticated clients) ---
TCP       listen   SELF  -                 587    -     -    #smtp-submission

# --- IMAP4 (plain + SSL) ---
TCP       listen   SELF  -                 143    -     -    #imap4
TCP       listen   SELF  -                 993    -     -    #imap4s

# --- POP3 (plain + SSL) ---
TCP       listen   SELF  -                 110    -     -    #pop3
TCP       listen   SELF  -                 995    -     -    #pop3s

# --- HTTP redirect + HTTPS (OWA / EAS / Autodiscover / EWS) ---
TCP       listen   SELF  -                 80     -     -    #http-redirect
TCP       listen   SELF  -                 443    -     -    #https-cas

# --- Outbound SMTP to internet / relay ---
TCP       connect  SELF  ANY               25     30    2    #smtp-outbound

# --- Outbound: inter-server mail flow within DAG ---
TCP       connect  SELF  group:exchange    25     20    3    #dag-smtp
TCP       connect  SELF  group:exchange    64327  10    1    #dag-replication

# --- Outbound: Active Directory / DNS ---
TCP       connect  SELF  group:dc          389    20    2    #ldap-auth
TCP       connect  SELF  group:dc          636    20    1    #ldaps-auth
UDP       connect  SELF  group:dc          53     10    2    #dns-query
TCP       connect  SELF  group:dc          88     15    2    #kerberos
