package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// writeFile creates a temporary file with the given content and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
	return path
}

const minimalProfile = `
[META]
NAME        = test_profile
DESCRIPTION = Unit test profile
VERSION     = 1.0
TAGS        = unit, test

[RULES]
TCP  connect  SELF  group:web  443  10  loop  #https-out
UDP  listen   SELF  -         53   -   -     #dns-listener
`

// ─── LoadProfile ──────────────────────────────────────────────────────────────

func TestLoadProfile_Basic(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "test_profile.profile", minimalProfile)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() unexpected error: %v", err)
	}

	if p.Meta.Name != "test_profile" {
		t.Errorf("Meta.Name = %q, want %q", p.Meta.Name, "test_profile")
	}
	if p.Meta.Description != "Unit test profile" {
		t.Errorf("Meta.Description = %q", p.Meta.Description)
	}
	if p.Meta.Version != "1.0" {
		t.Errorf("Meta.Version = %q", p.Meta.Version)
	}
	if len(p.Meta.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(p.Meta.Tags))
	}
	if len(p.Rules) != 2 {
		t.Fatalf("Rules len = %d, want 2", len(p.Rules))
	}

	// First rule: TCP connect
	r0 := p.Rules[0]
	if r0.Protocol != "TCP" || r0.Role != "connect" || r0.Port != 443 || r0.Interval != 10 || r0.Count != -1 {
		t.Errorf("Rule[0] mismatch: %+v", r0)
	}
	if r0.Name != "https-out" {
		t.Errorf("Rule[0].Name = %q, want %q", r0.Name, "https-out")
	}

	// Second rule: UDP listen
	r1 := p.Rules[1]
	if r1.Protocol != "UDP" || r1.Role != "listen" || r1.Port != 53 {
		t.Errorf("Rule[1] mismatch: %+v", r1)
	}
}

func TestLoadProfile_NameFromFilename(t *testing.T) {
	// Profile with no NAME in [META] — should derive name from filename.
	content := `
[META]
DESCRIPTION = no name set

[RULES]
TCP  connect  SELF  -  80  5  loop
`
	dir := t.TempDir()
	path := writeFile(t, dir, "derived_name.profile", content)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Meta.Name != "derived_name" {
		t.Errorf("Name = %q, want %q", p.Meta.Name, "derived_name")
	}
}

func TestLoadProfile_NoRulesSection(t *testing.T) {
	content := `
[META]
NAME = empty_profile
`
	dir := t.TempDir()
	path := writeFile(t, dir, "empty.profile", content)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Rules) != 0 {
		t.Errorf("Expected 0 rules, got %d", len(p.Rules))
	}
}

func TestLoadProfile_Extends(t *testing.T) {
	content := `
[META]
NAME    = child
EXTENDS = parent_profile
`
	dir := t.TempDir()
	path := writeFile(t, dir, "child.profile", content)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Meta.Extends != "parent_profile" {
		t.Errorf("Extends = %q, want %q", p.Meta.Extends, "parent_profile")
	}
}

func TestLoadProfile_InvalidProtocol(t *testing.T) {
	content := `
[META]
NAME = bad

[RULES]
HTTP  connect  SELF  -  80  5  loop
`
	dir := t.TempDir()
	path := writeFile(t, dir, "bad.profile", content)

	_, err := LoadProfile(path)
	if err == nil {
		t.Fatal("Expected error for invalid protocol, got nil")
	}
	if !strings.Contains(err.Error(), "protocol") {
		t.Errorf("Error should mention 'protocol', got: %v", err)
	}
}

func TestLoadProfile_InvalidPort(t *testing.T) {
	content := `
[META]
NAME = badport

[RULES]
TCP  connect  SELF  -  99999  5  loop
`
	dir := t.TempDir()
	path := writeFile(t, dir, "badport.profile", content)

	_, err := LoadProfile(path)
	if err == nil {
		t.Fatal("Expected error for invalid port, got nil")
	}
}

func TestLoadProfile_TooFewFields(t *testing.T) {
	content := `
[META]
NAME = short

[RULES]
TCP  connect  SELF
`
	dir := t.TempDir()
	path := writeFile(t, dir, "short.profile", content)

	_, err := LoadProfile(path)
	if err == nil {
		t.Fatal("Expected error for too-few fields, got nil")
	}
}

func TestLoadProfile_InvalidRole(t *testing.T) {
	content := `
[META]
NAME = badrole

[RULES]
TCP  relay  SELF  -  443  5  loop
`
	dir := t.TempDir()
	path := writeFile(t, dir, "badrole.profile", content)

	_, err := LoadProfile(path)
	if err == nil {
		t.Fatal("Expected error for invalid role, got nil")
	}
	if !strings.Contains(err.Error(), "role") {
		t.Errorf("Error should mention 'role', got: %v", err)
	}
}

func TestLoadProfile_CountVariants(t *testing.T) {
	content := `
[META]
NAME = counts

[RULES]
TCP  connect  SELF  -  80  0  loop  #forever-loop
TCP  connect  SELF  -  81  0  -     #dash-means-loop
TCP  connect  SELF  -  82  0  5     #count-5
`
	dir := t.TempDir()
	path := writeFile(t, dir, "counts.profile", content)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Rules) != 3 {
		t.Fatalf("len(Rules) = %d, want 3", len(p.Rules))
	}
	if p.Rules[0].Count != -1 { // "loop"
		t.Errorf("Rules[0].Count = %d, want -1 (loop)", p.Rules[0].Count)
	}
	if p.Rules[1].Count != -1 { // "-"
		t.Errorf("Rules[1].Count = %d, want -1 (loop)", p.Rules[1].Count)
	}
	if p.Rules[2].Count != 5 {
		t.Errorf("Rules[2].Count = %d, want 5", p.Rules[2].Count)
	}
}

func TestLoadProfile_FileNotFound(t *testing.T) {
	_, err := LoadProfile("/no/such/file.profile")
	if err == nil {
		t.Fatal("Expected error for missing file, got nil")
	}
}

// ─── LoadProfileDir ───────────────────────────────────────────────────────────

func TestLoadProfileDir_Empty(t *testing.T) {
	dir := t.TempDir()
	profiles, err := LoadProfileDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("Expected 0 profiles in empty dir, got %d", len(profiles))
	}
}

func TestLoadProfileDir_MultipleProfiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "alpha.profile", `
[META]
NAME = alpha
[RULES]
TCP  listen  SELF  -  80  -  -
`)
	writeFile(t, dir, "beta.profile", `
[META]
NAME = beta
[RULES]
UDP  listen  SELF  -  53  -  -
`)
	// Non-profile file should be ignored.
	writeFile(t, dir, "notes.txt", "this is not a profile")

	profiles, err := LoadProfileDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("Expected 2 profiles, got %d", len(profiles))
	}
	if _, ok := profiles["alpha"]; !ok {
		t.Error("Missing profile 'alpha'")
	}
	if _, ok := profiles["beta"]; !ok {
		t.Error("Missing profile 'beta'")
	}
}

func TestLoadProfileDir_ParseError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.profile", `
[META]
NAME = bad

[RULES]
INVALID_PROTO  connect  SELF  -  80  5  loop
`)

	_, err := LoadProfileDir(dir)
	if err == nil {
		t.Fatal("Expected error when profile in dir is invalid, got nil")
	}
}

func TestLoadProfileDir_NonExistentDir(t *testing.T) {
	_, err := LoadProfileDir("/no/such/dir")
	if err == nil {
		t.Fatal("Expected error for non-existent dir, got nil")
	}
}

// ─── FlattenProfile ───────────────────────────────────────────────────────────

func TestFlattenProfile_NoExtends(t *testing.T) {
	profiles := map[string]*Profile{
		"simple": {
			Meta:  ProfileMeta{Name: "simple"},
			Rules: []*ProfileRule{{Protocol: "TCP", Role: "listen", Port: 80}},
		},
	}
	rules, err := FlattenProfile("simple", profiles, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}
}

func TestFlattenProfile_WithExtends(t *testing.T) {
	profiles := map[string]*Profile{
		"base": {
			Meta:  ProfileMeta{Name: "base"},
			Rules: []*ProfileRule{{Protocol: "TCP", Role: "listen", Port: 80, Name: "from-base"}},
		},
		"child": {
			Meta:  ProfileMeta{Name: "child", Extends: "base"},
			Rules: []*ProfileRule{{Protocol: "UDP", Role: "listen", Port: 53, Name: "from-child"}},
		},
	}
	rules, err := FlattenProfile("child", profiles, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("Expected 2 rules (1 inherited + 1 own), got %d", len(rules))
	}
	// Parent rules must come first.
	if rules[0].Name != "from-base" {
		t.Errorf("First rule should be inherited from base, got %q", rules[0].Name)
	}
	if rules[1].Name != "from-child" {
		t.Errorf("Second rule should be from child, got %q", rules[1].Name)
	}
}

func TestFlattenProfile_CircularExtends(t *testing.T) {
	profiles := map[string]*Profile{
		"a": {Meta: ProfileMeta{Name: "a", Extends: "b"}},
		"b": {Meta: ProfileMeta{Name: "b", Extends: "a"}},
	}
	_, err := FlattenProfile("a", profiles, nil)
	if err == nil {
		t.Fatal("Expected circular-extends error, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("Error should mention 'circular', got: %v", err)
	}
}

func TestFlattenProfile_MissingProfile(t *testing.T) {
	profiles := map[string]*Profile{}
	_, err := FlattenProfile("nonexistent", profiles, nil)
	if err == nil {
		t.Fatal("Expected error for missing profile, got nil")
	}
}

func TestFlattenProfile_MissingParent(t *testing.T) {
	profiles := map[string]*Profile{
		"child": {Meta: ProfileMeta{Name: "child", Extends: "ghost_parent"}},
	}
	_, err := FlattenProfile("child", profiles, nil)
	if err == nil {
		t.Fatal("Expected error for missing parent profile, got nil")
	}
}

// ─── ResolveProfileRules ──────────────────────────────────────────────────────

func buildTestProfiles() map[string]*Profile {
	return map[string]*Profile{
		"web": {
			Meta: ProfileMeta{Name: "web"},
			Rules: []*ProfileRule{
				{Protocol: "TCP", Role: "listen", Port: 80, Name: "http-listen"},
				{Protocol: "TCP", Role: "connect", Src: "SELF", Dst: "group:db", Port: 5432, Interval: 10, Count: -1, Name: "db-connect"},
			},
		},
		"db": {
			Meta: ProfileMeta{Name: "db"},
			Rules: []*ProfileRule{
				{Protocol: "TCP", Role: "listen", Port: 5432, Name: "pg-listen"},
			},
		},
	}
}

func TestResolveProfileRules_Listen(t *testing.T) {
	profiles := buildTestProfiles()
	targetMap := map[string]string{"web1": "10.0.0.1"}
	tagMap := map[string][]string{}

	rules, err := ResolveProfileRules(profiles, []string{"db"}, "10.0.0.2", targetMap, tagMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}
	if rules[0].Role != "listen" || rules[0].Port != 5432 {
		t.Errorf("Expected listen on 5432, got %+v", rules[0])
	}
}

func TestResolveProfileRules_ConnectSelf(t *testing.T) {
	profiles := map[string]*Profile{
		"client": {
			Meta: ProfileMeta{Name: "client"},
			Rules: []*ProfileRule{
				{Protocol: "TCP", Role: "connect", Src: "SELF", Dst: "10.0.0.99", Port: 8080, Count: -1},
			},
		},
	}
	targetMap := map[string]string{}
	tagMap := map[string][]string{}

	rules, err := ResolveProfileRules(profiles, []string{"client"}, "10.0.0.1", targetMap, tagMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}
	if rules[0].Source != "10.0.0.1" {
		t.Errorf("Source should be agent IP, got %q", rules[0].Source)
	}
	if rules[0].Target != "10.0.0.99" {
		t.Errorf("Target should be 10.0.0.99, got %q", rules[0].Target)
	}
}

func TestResolveProfileRules_ConnectGroup(t *testing.T) {
	profiles := map[string]*Profile{
		"web": {
			Meta: ProfileMeta{Name: "web"},
			Rules: []*ProfileRule{
				{Protocol: "TCP", Role: "connect", Src: "SELF", Dst: "group:db", Port: 5432, Count: -1},
			},
		},
	}
	targetMap := map[string]string{"db1": "10.0.1.1", "db2": "10.0.1.2"}
	tagMap := map[string][]string{"db": {"10.0.1.1", "10.0.1.2"}}

	rules, err := ResolveProfileRules(profiles, []string{"web"}, "10.0.0.1", targetMap, tagMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should expand to one rule per IP in the group.
	if len(rules) != 2 {
		t.Fatalf("Expected 2 rules (one per DB IP), got %d", len(rules))
	}
	targets := map[string]bool{rules[0].Target: true, rules[1].Target: true}
	if !targets["10.0.1.1"] || !targets["10.0.1.2"] {
		t.Errorf("Expected both DB IPs as targets, got %v", targets)
	}
}

func TestResolveProfileRules_ConnectAny(t *testing.T) {
	profiles := map[string]*Profile{
		"broadcast": {
			Meta: ProfileMeta{Name: "broadcast"},
			Rules: []*ProfileRule{
				{Protocol: "TCP", Role: "connect", Src: "SELF", Dst: "ANY", Port: 9000, Count: -1},
			},
		},
	}
	targetMap := map[string]string{"h1": "10.0.0.1", "h2": "10.0.0.2"}
	tagMap := map[string][]string{}

	rules, err := ResolveProfileRules(profiles, []string{"broadcast"}, "10.0.0.99", targetMap, tagMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ANY expands to all entries in targetMap.
	if len(rules) != len(targetMap) {
		t.Errorf("Expected %d rules for ANY, got %d", len(targetMap), len(rules))
	}
}

func TestResolveProfileRules_SrcNotSelf_OtherAgent(t *testing.T) {
	// Rule with Src != SELF that does not match the current agentIP should be skipped.
	profiles := map[string]*Profile{
		"specific": {
			Meta: ProfileMeta{Name: "specific"},
			Rules: []*ProfileRule{
				{Protocol: "TCP", Role: "connect", Src: "10.0.0.5", Dst: "10.0.0.9", Port: 8080, Count: -1},
			},
		},
	}
	targetMap := map[string]string{}
	tagMap := map[string][]string{}

	rules, err := ResolveProfileRules(profiles, []string{"specific"}, "10.0.0.1", targetMap, tagMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// agentIP (10.0.0.1) != Src (10.0.0.5) → rule must be skipped.
	if len(rules) != 0 {
		t.Errorf("Expected 0 rules (Src mismatch), got %d", len(rules))
	}
}

func TestResolveProfileRules_UnknownProfile(t *testing.T) {
	profiles := buildTestProfiles()
	_, err := ResolveProfileRules(profiles, []string{"nonexistent"}, "10.0.0.1",
		map[string]string{}, map[string][]string{})
	if err == nil {
		t.Fatal("Expected error for unknown profile, got nil")
	}
}

func TestResolveProfileRules_EmptyProfileNames(t *testing.T) {
	profiles := buildTestProfiles()
	rules, err := ResolveProfileRules(profiles, []string{}, "10.0.0.1",
		map[string]string{}, map[string][]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("Expected 0 rules for empty profile list, got %d", len(rules))
	}
}

// ─── LookupAssignments ────────────────────────────────────────────────────────

func TestLookupAssignments_DirectIP(t *testing.T) {
	assignments := map[string][]string{
		"10.0.0.5": {"web_tier", "app_tier"},
	}
	targetMap := map[string]string{}

	result := LookupAssignments("10.0.0.5", assignments, targetMap)
	if len(result) != 2 {
		t.Fatalf("Expected 2 profiles, got %d", len(result))
	}
	if result[0] != "web_tier" || result[1] != "app_tier" {
		t.Errorf("Unexpected profiles: %v", result)
	}
}

func TestLookupAssignments_NamedTarget(t *testing.T) {
	assignments := map[string][]string{
		"webserver": {"web_tier"},
	}
	targetMap := map[string]string{
		"webserver": "10.0.0.10",
	}

	result := LookupAssignments("10.0.0.10", assignments, targetMap)
	if len(result) != 1 || result[0] != "web_tier" {
		t.Errorf("Expected [web_tier], got %v", result)
	}
}

func TestLookupAssignments_NotFound(t *testing.T) {
	assignments := map[string][]string{
		"10.0.0.1": {"profile_a"},
	}
	targetMap := map[string]string{}

	result := LookupAssignments("10.0.0.99", assignments, targetMap)
	if result != nil {
		t.Errorf("Expected nil, got %v", result)
	}
}

func TestLookupAssignments_EmptyAssignments(t *testing.T) {
	result := LookupAssignments("10.0.0.1",
		map[string][]string{},
		map[string]string{})
	if result != nil {
		t.Errorf("Expected nil for empty assignments, got %v", result)
	}
}

// ─── ParseExtendedConfigV2 — profile-related sections ───────────────────────

func TestParseExtendedConfigV2_ProfileDir_Relative(t *testing.T) {
	dir := t.TempDir()
	profDir := filepath.Join(dir, "profiles")
	if err := os.Mkdir(profDir, 0755); err != nil {
		t.Fatal(err)
	}

	confContent := `
[MASTER]
PORT = 9000
PSK  = TestPSK12345678

PROFILE_DIR = profiles
`
	confPath := writeFile(t, dir, "to.conf", confContent)

	cfg, err := ParseExtendedConfigV2(confPath)
	if err != nil {
		t.Fatalf("ParseExtendedConfigV2() error: %v", err)
	}

	// PROFILE_DIR must be absolute and point to the directory next to to.conf.
	if !filepath.IsAbs(cfg.ProfileDir) {
		t.Errorf("ProfileDir should be absolute, got %q", cfg.ProfileDir)
	}
	if cfg.ProfileDir != profDir {
		t.Errorf("ProfileDir = %q, want %q", cfg.ProfileDir, profDir)
	}
}

func TestParseExtendedConfigV2_ProfileDir_Absolute(t *testing.T) {
	dir := t.TempDir()
	profDir := filepath.Join(dir, "abs_profiles")
	if err := os.Mkdir(profDir, 0755); err != nil {
		t.Fatal(err)
	}

	confContent := "PORT = 9000\nPSK = TestPSK12345678\nPROFILE_DIR = " + profDir + "\n"
	confPath := writeFile(t, dir, "to.conf", confContent)

	cfg, err := ParseExtendedConfigV2(confPath)
	if err != nil {
		t.Fatalf("ParseExtendedConfigV2() error: %v", err)
	}
	if cfg.ProfileDir != profDir {
		t.Errorf("Absolute ProfileDir changed: got %q, want %q", cfg.ProfileDir, profDir)
	}
}

func TestParseExtendedConfigV2_Assignments(t *testing.T) {
	dir := t.TempDir()
	confContent := `
PORT = 9000
PSK  = TestPSK12345678

[TARGETS]
web1 = 10.0.0.1

[ASSIGNMENTS]
web1     = web_tier, app_tier
10.0.0.2 = db_tier
`
	confPath := writeFile(t, dir, "to.conf", confContent)

	cfg, err := ParseExtendedConfigV2(confPath)
	if err != nil {
		t.Fatalf("ParseExtendedConfigV2() error: %v", err)
	}

	web1Profiles, ok := cfg.Assignments["web1"]
	if !ok {
		t.Fatal("Assignment for 'web1' not found")
	}
	if len(web1Profiles) != 2 || web1Profiles[0] != "web_tier" || web1Profiles[1] != "app_tier" {
		t.Errorf("web1 profiles = %v, want [web_tier app_tier]", web1Profiles)
	}

	dbProfiles, ok := cfg.Assignments["10.0.0.2"]
	if !ok {
		t.Fatal("Assignment for '10.0.0.2' not found")
	}
	if len(dbProfiles) != 1 || dbProfiles[0] != "db_tier" {
		t.Errorf("10.0.0.2 profiles = %v, want [db_tier]", dbProfiles)
	}
}

func TestParseExtendedConfigV2_TagMap(t *testing.T) {
	dir := t.TempDir()
	confContent := `
PORT = 9000
PSK  = TestPSK12345678

[TARGETS]
db1 = 10.0.1.1  #tag:db, tag:primary
db2 = 10.0.1.2  #tag:db, tag:replica
`
	confPath := writeFile(t, dir, "to.conf", confContent)

	cfg, err := ParseExtendedConfigV2(confPath)
	if err != nil {
		t.Fatalf("ParseExtendedConfigV2() error: %v", err)
	}

	dbIPs := cfg.TagMap["db"]
	if len(dbIPs) != 2 {
		t.Fatalf("TagMap[db] len = %d, want 2", len(dbIPs))
	}
	found := map[string]bool{dbIPs[0]: true, dbIPs[1]: true}
	if !found["10.0.1.1"] || !found["10.0.1.2"] {
		t.Errorf("TagMap[db] = %v, want both DB IPs", dbIPs)
	}

	primary := cfg.TagMap["primary"]
	if len(primary) != 1 || primary[0] != "10.0.1.1" {
		t.Errorf("TagMap[primary] = %v, want [10.0.1.1]", primary)
	}
}

// ─── Integration: LoadProfileDir then ResolveProfileRules ────────────────────

func TestIntegration_LoadAndResolve(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "server.profile", `
[META]
NAME = server

[RULES]
TCP  listen   SELF  -          80   -   -    #http
TCP  connect  SELF  group:db   5432 15  loop #db-query
`)
	writeFile(t, dir, "db.profile", `
[META]
NAME = db

[RULES]
TCP  listen  SELF  -  5432  -  -  #pg
`)

	profiles, err := LoadProfileDir(dir)
	if err != nil {
		t.Fatalf("LoadProfileDir error: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("Expected 2 profiles, got %d", len(profiles))
	}

	targetMap := map[string]string{"dbhost": "10.0.1.5"}
	tagMap := map[string][]string{"db": {"10.0.1.5"}}

	// Resolve rules for a "server" agent at 10.0.0.1.
	rules, err := ResolveProfileRules(profiles, []string{"server"}, "10.0.0.1", targetMap, tagMap)
	if err != nil {
		t.Fatalf("ResolveProfileRules error: %v", err)
	}

	// Expect: 1 listen rule (http) + 1 connect rule (db-query → 10.0.1.5)
	if len(rules) != 2 {
		t.Fatalf("Expected 2 resolved rules, got %d", len(rules))
	}

	listenCount := 0
	connectCount := 0
	for _, r := range rules {
		switch r.Role {
		case "listen":
			listenCount++
		case "connect":
			connectCount++
			if r.Target != "10.0.1.5" {
				t.Errorf("Connect target = %q, want %q", r.Target, "10.0.1.5")
			}
		}
	}
	if listenCount != 1 || connectCount != 1 {
		t.Errorf("Expected 1 listen + 1 connect, got %d + %d", listenCount, connectCount)
	}
}
