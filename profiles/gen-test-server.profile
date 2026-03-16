[META]
NAME        = gen_test_server
DESCRIPTION = Generic test server — listens on TCP and UDP ports 54, 79, 5588 and 8855.
              Use together with gen_test_client to verify end-to-end traffic generation
              across non-standard port combinations in lab or simulation environments.
VERSION     = 1.0
TAGS        = generic, test, server, lab

[RULES]
# ── Port 54 ──────────────────────────────────────────────────────────────────
TCP  listen  SELF  -  54    -  -  #gen-test-tcp-54
UDP  listen  SELF  -  54    -  -  #gen-test-udp-54

# ── Port 79 ──────────────────────────────────────────────────────────────────
TCP  listen  SELF  -  79    -  -  #gen-test-tcp-79
UDP  listen  SELF  -  79    -  -  #gen-test-udp-79

# ── Port 5588 ─────────────────────────────────────────────────────────────────
TCP  listen  SELF  -  5588  -  -  #gen-test-tcp-5588
UDP  listen  SELF  -  5588  -  -  #gen-test-udp-5588

# ── Port 8855 ─────────────────────────────────────────────────────────────────
TCP  listen  SELF  -  8855  -  -  #gen-test-tcp-8855
UDP  listen  SELF  -  8855  -  -  #gen-test-udp-8855
