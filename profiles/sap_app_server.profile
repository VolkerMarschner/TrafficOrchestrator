# ============================================================
# Traffic Profile: SAP NetWeaver Application Server (ABAP)
#
# Covers inbound SAP GUI (DIAG), RFC gateway, message server,
# ICM HTTP/HTTPS listeners, and outbound connections to the
# SAP HANA or Oracle database tier and Active Directory.
#
# Assumes instance number 00. For other instance numbers adjust
# ports accordingly (3200+NN, 3300+NN, 3600+NN).
#
# Assign to hosts tagged  #tag:sap_app  in [TARGETS].
# ============================================================

[META]
NAME        = sap_app_server
DESCRIPTION = SAP NetWeaver ABAP Application Server — instance 00
VERSION     = 1.0
EXTENDS     = base_windows
TAGS        = windows, sap, abap, netweaver

[RULES]
# PROTO   ROLE     SRC   DST              PORT   INTV  CNT  #name

# --- SAP GUI / DIAG dispatcher (instance 00) ---
TCP       listen   SELF  -                3200   -     -    #sap-diag-listener

# --- SAP RFC gateway (instance 00) ---
TCP       listen   SELF  -                3300   -     -    #sap-rfc-gateway

# --- SAP Message Server (instance 00) ---
TCP       listen   SELF  -                3600   -     -    #sap-message-server
TCP       listen   SELF  -                3900   -     -    #sap-message-server-http

# --- SAP ICM — HTTP / HTTPS ---
TCP       listen   SELF  -                8000   -     -    #sap-icm-http
TCP       listen   SELF  -                44300  -     -    #sap-icm-https

# --- SAP Web Dispatcher (internal routing) ---
TCP       listen   SELF  -                8080   -     -    #sap-webdisp-http
TCP       listen   SELF  -                8443   -     -    #sap-webdisp-https

# --- Outbound: SAP HANA database ---
TCP       connect  SELF  group:sap_hana   30015  30    3    #hana-tenant-sql
TCP       connect  SELF  group:sap_hana   30013  60    1    #hana-system-db

# --- Outbound: Active Directory authentication ---
TCP       connect  SELF  group:dc         389    20    2    #ldap-auth
UDP       connect  SELF  group:dc         53     10    2    #dns-query
TCP       connect  SELF  group:dc         88     15    2    #kerberos

# --- SAP inter-app-server communication ---
TCP       connect  SELF  group:sap_app    3600   30    2    #sap-ms-connect
