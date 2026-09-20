package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const inventorySchemaVersion = 1

// Inventory is the frozen language-neutral contract coverage list.
type Inventory struct {
	SchemaVersion int        `json:"schema_version"`
	Identities    Identities `json:"identities"`
	Families      []Family   `json:"families"`
}

// Identities pins current supported versions. Empty or zero values are
// incomplete evidence and fail acceptance.
type Identities struct {
	Protocol           int    `json:"protocol"`
	ChunkSchema        int    `json:"chunk_schema"`
	PlayerSchema       int    `json:"player_schema"`
	WorldMetadata      int    `json:"world_metadata"`
	CompanionsAISchema int    `json:"companions_ai_schema"`
	HostileMobsSchema  int    `json:"hostile_mobs_schema"`
	PassiveMobsSchema  int    `json:"passive_mobs_schema"`
	EngineABI          int    `json:"engine_abi"`
	RegionFormat       int    `json:"region_format"`
	AgentHTTP          string `json:"agent_http"`
	AgentMCP           string `json:"agent_mcp"`
}

// Family is one supported protocol, save, kernel, or agent contract.
type Family struct {
	ID                string   `json:"id"`
	Kind              string   `json:"kind"`
	Role              string   `json:"role"`
	CurrentVersion    string   `json:"current_version"`
	SupportedVersions []string `json:"supported_versions"`
	Source            string   `json:"source"`
	EventualOwner     string   `json:"eventual_owner"`
	NumericSemantics  string   `json:"numeric_semantics"`
	Fixtures          []string `json:"fixtures"`
}

// InventoryError lists every coverage, version, or fixture failure.
type InventoryError struct {
	Problems []string
}

func (err *InventoryError) Error() string {
	if err == nil || len(err.Problems) == 0 {
		return "runtime-oracle: inventory error"
	}
	return "runtime-oracle: " + strings.Join(err.Problems, "; ")
}

// LoadInventory reads a frozen inventory JSON file.
func LoadInventory(path string) (Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Inventory{}, fmt.Errorf("runtime-oracle: read inventory: %w", err)
	}
	var inventory Inventory
	if err := json.Unmarshal(data, &inventory); err != nil {
		return Inventory{}, fmt.Errorf("runtime-oracle: decode inventory: %w", err)
	}
	if inventory.SchemaVersion != inventorySchemaVersion {
		return Inventory{}, fmt.Errorf("runtime-oracle: inventory schema_version %d, want %d", inventory.SchemaVersion, inventorySchemaVersion)
	}
	return inventory, nil
}

// Reconcile compares a frozen inventory with families discovered from
// current registries and source. A missing supported family, version
// mismatch, extra inventory row, or missing fixture fails acceptance.
func Reconcile(root string, inventory Inventory, discovered []Family, live Identities) error {
	var problems []string
	problems = append(problems, identityProblems(inventory.Identities, live)...)

	if inventory.SchemaVersion != inventorySchemaVersion {
		problems = append(problems, fmt.Sprintf("inventory schema_version %d, want %d", inventory.SchemaVersion, inventorySchemaVersion))
	}

	inventoryByID := make(map[string]Family, len(inventory.Families))
	for _, family := range inventory.Families {
		if family.ID == "" {
			problems = append(problems, "inventory family has empty id")
			continue
		}
		if _, exists := inventoryByID[family.ID]; exists {
			problems = append(problems, "duplicate inventory family "+family.ID)
			continue
		}
		inventoryByID[family.ID] = family
	}

	discoveredByID := make(map[string]Family, len(discovered))
	for _, family := range discovered {
		discoveredByID[family.ID] = family
		listed, ok := inventoryByID[family.ID]
		if !ok {
			problems = append(problems, "uncovered family "+family.ID)
			continue
		}
		if listed.Kind != family.Kind {
			problems = append(problems, fmt.Sprintf("family %s kind %s does not match code %s", family.ID, listed.Kind, family.Kind))
		}
		if listed.Role != family.Role {
			problems = append(problems, fmt.Sprintf("family %s role %s does not match code %s", family.ID, listed.Role, family.Role))
		}
		if listed.CurrentVersion != family.CurrentVersion {
			problems = append(problems, fmt.Sprintf("family %s version %s does not match code %s", family.ID, listed.CurrentVersion, family.CurrentVersion))
		}
		if !sameStringSet(listed.SupportedVersions, family.SupportedVersions) {
			problems = append(problems, fmt.Sprintf("family %s supported versions %v do not match code %v", family.ID, listed.SupportedVersions, family.SupportedVersions))
		}
		if listed.Source != family.Source {
			problems = append(problems, fmt.Sprintf("family %s source %s does not match code %s", family.ID, listed.Source, family.Source))
		}
		if listed.EventualOwner != family.EventualOwner {
			problems = append(problems, fmt.Sprintf("family %s eventual owner %s does not match code %s", family.ID, listed.EventualOwner, family.EventualOwner))
		}
		if listed.NumericSemantics != family.NumericSemantics {
			problems = append(problems, fmt.Sprintf("family %s numeric semantics do not match code", family.ID))
		}
		if strings.TrimSpace(listed.Source) == "" {
			problems = append(problems, "family "+family.ID+" is missing source provenance")
		}
		if strings.TrimSpace(listed.EventualOwner) == "" {
			problems = append(problems, "family "+family.ID+" is missing eventual owner")
		}
		if strings.TrimSpace(listed.NumericSemantics) == "" {
			problems = append(problems, "family "+family.ID+" is missing numeric semantics")
		}
		if !sameStringSet(listed.Fixtures, family.Fixtures) {
			problems = append(problems, fmt.Sprintf("family %s fixtures %v do not match code %v", family.ID, listed.Fixtures, family.Fixtures))
		}
		if len(listed.Fixtures) == 0 {
			problems = append(problems, "family "+family.ID+" has no coverage fixture")
			continue
		}
		for _, fixture := range listed.Fixtures {
			if fixture == "" {
				problems = append(problems, "family "+family.ID+" has an empty fixture path")
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(fixture))); err != nil {
				problems = append(problems, fmt.Sprintf("family %s fixture %s is missing", family.ID, fixture))
			}
		}
	}

	for id := range inventoryByID {
		if _, ok := discoveredByID[id]; !ok {
			problems = append(problems, "inventory family "+id+" is not in current registries")
		}
	}

	problems = append(problems, requiredKindProblems(inventory.Families)...)
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return &InventoryError{Problems: problems}
}

func identityProblems(got, live Identities) []string {
	var problems []string
	if live.Protocol == 0 || live.ChunkSchema == 0 || live.PlayerSchema == 0 ||
		live.WorldMetadata == 0 || live.CompanionsAISchema == 0 || live.HostileMobsSchema == 0 ||
		live.PassiveMobsSchema == 0 || live.EngineABI == 0 || live.RegionFormat == 0 ||
		live.AgentHTTP == "" || live.AgentMCP == "" {
		problems = append(problems, "incomplete live contract identity")
	}
	if got.Protocol == 0 || got.ChunkSchema == 0 || got.PlayerSchema == 0 ||
		got.WorldMetadata == 0 || got.CompanionsAISchema == 0 || got.HostileMobsSchema == 0 ||
		got.PassiveMobsSchema == 0 || got.EngineABI == 0 || got.RegionFormat == 0 ||
		got.AgentHTTP == "" || got.AgentMCP == "" {
		problems = append(problems, "incomplete inventory identity")
	}
	if got.Protocol != live.Protocol {
		problems = append(problems, fmt.Sprintf("protocol version %d does not match code %d", got.Protocol, live.Protocol))
	}
	if got.ChunkSchema != live.ChunkSchema {
		problems = append(problems, fmt.Sprintf("chunk schema %d does not match code %d", got.ChunkSchema, live.ChunkSchema))
	}
	if got.PlayerSchema != live.PlayerSchema {
		problems = append(problems, fmt.Sprintf("player schema %d does not match code %d", got.PlayerSchema, live.PlayerSchema))
	}
	if got.WorldMetadata != live.WorldMetadata {
		problems = append(problems, fmt.Sprintf("world metadata %d does not match code %d", got.WorldMetadata, live.WorldMetadata))
	}
	if got.CompanionsAISchema != live.CompanionsAISchema {
		problems = append(problems, fmt.Sprintf("companions.ai schema %d does not match code %d", got.CompanionsAISchema, live.CompanionsAISchema))
	}
	if got.HostileMobsSchema != live.HostileMobsSchema {
		problems = append(problems, fmt.Sprintf("hostile_mobs schema %d does not match code %d", got.HostileMobsSchema, live.HostileMobsSchema))
	}
	if got.PassiveMobsSchema != live.PassiveMobsSchema {
		problems = append(problems, fmt.Sprintf("passive_mobs schema %d does not match code %d", got.PassiveMobsSchema, live.PassiveMobsSchema))
	}
	if got.EngineABI != live.EngineABI {
		problems = append(problems, fmt.Sprintf("engine ABI %d does not match code %d", got.EngineABI, live.EngineABI))
	}
	if got.RegionFormat != live.RegionFormat {
		problems = append(problems, fmt.Sprintf("region format %d does not match code %d", got.RegionFormat, live.RegionFormat))
	}
	if got.AgentHTTP != live.AgentHTTP {
		problems = append(problems, fmt.Sprintf("agent HTTP %s does not match code %s", got.AgentHTTP, live.AgentHTTP))
	}
	if got.AgentMCP != live.AgentMCP {
		problems = append(problems, fmt.Sprintf("agent MCP %s does not match code %s", got.AgentMCP, live.AgentMCP))
	}
	return problems
}

func requiredKindProblems(families []Family) []string {
	seen := map[string]bool{}
	roles := map[string]bool{}
	for _, family := range families {
		seen[family.Kind] = true
		roles[family.Role] = true
	}
	var problems []string
	for _, kind := range []string{"protocol", "save", "kernel", "agent"} {
		if !seen[kind] {
			problems = append(problems, "inventory is missing kind "+kind)
		}
	}
	for _, role := range []string{"input", "event"} {
		if !roles[role] {
			problems = append(problems, "inventory is missing semantic role "+role)
		}
	}
	return problems
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	for _, value := range right {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	return true
}
