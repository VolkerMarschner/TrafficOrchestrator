# ============================================================
# Traffic Profile: Generic Test Server
#
# Listens on a set of custom test ports (TCP + UDP) that are
# not assigned to any well-known protocol.  Use this profile
# on server-side hosts in connectivity / firewall test
# scenarios.  Pair with gen_test_client on the client side.
#
# Ports covered:
#   54    — custom test port A (TCP + UDP)
#   79    — custom test port B (TCP + UDP)
#   5588  — custom test port C (TCP + UDP)
#   8855  — custom test port D (TCP + UDP)
#
# Tag server nodes with  #tag:gen_test_server  in [TARGETS].
# ============================================================

[META]
NAME        = gen_test_server
DESCRIPTION = Generic test server — listens on custom test ports (54, 79, 5588, 8855) TCP+UDP
VERSION     = 1.0
TAGS        = test, generic, server, connectivity

[RULES]
# PROTO   ROLE     SRC   DST   PORT   INTV  CNT  #name

# --- Port 54 ---
TCP       listen   SELF  -     54     -     -    #test-tcp-54
UDP       listen   SELF  -     54     -     -    #test-udp-54

# --- Port 79 ---
TCP       listen   SELF  -     79     -     -    #test-tcp-79
UDP       listen   SELF  -     79     -     -    #test-udp-79

# --- Port 5588 ---
TCP       listen   SELF  -     5588   -     -    #test-tcp-5588
UDP       listen   SELF  -     5588   -     -    #test-udp-5588

# --- Port 8855 ---
TCP       listen   SELF  -     8855   -     -    #test-tcp-8855
UDP       listen   SELF  -     8855   -     -    #test-udp-8855
