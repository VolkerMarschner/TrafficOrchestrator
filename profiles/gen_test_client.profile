# ============================================================
# Traffic Profile: Generic Test Client
#
# Connects to a set of custom test ports (TCP + UDP) that are
# not assigned to any well-known protocol.  Use this profile
# on client-side hosts in connectivity / firewall test
# scenarios.  Pair with gen_test_server on the server side.
#
# Ports covered:
#   54    — custom test port A (TCP + UDP)
#   79    — custom test port B (TCP + UDP)
#   5588  — custom test port C (TCP + UDP)
#   8855  — custom test port D (TCP + UDP)
#
# Tag client nodes with  #tag:gen_test_client  in [TARGETS].
# ============================================================

[META]
NAME        = gen_test_client
DESCRIPTION = Generic test client — connects to custom test ports (54, 79, 5588, 8855) TCP+UDP
VERSION     = 1.0
TAGS        = test, generic, client, connectivity

[RULES]
# PROTO   ROLE     SRC   DST                    PORT   INTV  CNT  #name

# --- Port 54 ---
TCP       connect  SELF  group:gen_test_server  54     30    1    #test-tcp-54
UDP       connect  SELF  group:gen_test_server  54     30    1    #test-udp-54

# --- Port 79 ---
TCP       connect  SELF  group:gen_test_server  79     30    1    #test-tcp-79
UDP       connect  SELF  group:gen_test_server  79     30    1    #test-udp-79

# --- Port 5588 ---
TCP       connect  SELF  group:gen_test_server  5588   30    1    #test-tcp-5588
UDP       connect  SELF  group:gen_test_server  5588   30    1    #test-udp-5588

# --- Port 8855 ---
TCP       connect  SELF  group:gen_test_server  8855   30    1    #test-tcp-8855
UDP       connect  SELF  group:gen_test_server  8855   30    1    #test-udp-8855
