[META]
NAME        = gen_test_client
DESCRIPTION = Generic test client — connects to TCP and UDP ports 54, 79, 5588 and 8855
              on all hosts tagged as "gen_test_server". Use together with gen_test_server
              to verify end-to-end traffic generation across non-standard port combinations
              in lab or simulation environments.
VERSION     = 1.0
TAGS        = generic, test, client, lab

[RULES]
# ── Port 54 ──────────────────────────────────────────────────────────────────
TCP  connect  SELF  group:gen_test_server  54    30  loop  #gen-test-tcp-54
UDP  connect  SELF  group:gen_test_server  54    30  loop  #gen-test-udp-54

# ── Port 79 ──────────────────────────────────────────────────────────────────
TCP  connect  SELF  group:gen_test_server  79    30  loop  #gen-test-tcp-79
UDP  connect  SELF  group:gen_test_server  79    30  loop  #gen-test-udp-79

# ── Port 5588 ─────────────────────────────────────────────────────────────────
TCP  connect  SELF  group:gen_test_server  5588  30  loop  #gen-test-tcp-5588
UDP  connect  SELF  group:gen_test_server  5588  30  loop  #gen-test-udp-5588

# ── Port 8855 ─────────────────────────────────────────────────────────────────
TCP  connect  SELF  group:gen_test_server  8855  30  loop  #gen-test-tcp-8855
UDP  connect  SELF  group:gen_test_server  8855  30  loop  #gen-test-udp-8855
