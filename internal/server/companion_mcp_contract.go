package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/channing771/mornlea/internal/companion"
	mcpv1 "github.com/channing771/mornlea/packages/contracts/companion-agent/mcp-v1"
)

type companionMCPContract struct {
	ProtocolVersion   string
	EndpointPath      string
	RequestBodyBytes  int
	WireResponseBytes int
	Tools             []companionMCPToolContract
}

type companionMCPToolContract struct {
	Name                 string
	InputSchema          json.RawMessage
	OutputSchema         json.RawMessage
	CanonicalResultBytes int
}

type companionMCPManifest struct {
	ApplicationContractVersion string `json:"application_contract_version"`
	MCPProtocolVersion         string `json:"mcp_protocol_version"`
	EndpointPath               string `json:"endpoint_path"`
	Stateless                  bool   `json:"stateless"`
	JSONResponse               bool   `json:"json_response"`
	SSE                        bool   `json:"sse"`
	Sessions                   bool   `json:"sessions"`
	Limits                     struct {
		RequestBodyBytes  int `json:"request_body_bytes"`
		WireResponseBytes int `json:"wire_response_bytes"`
		PlanInputBytes    int `json:"plan_input_bytes"`
	} `json:"limits"`
	Tools []struct {
		Name                 string   `json:"name"`
		InputSchema          string   `json:"input_schema"`
		ResultSchema         string   `json:"result_schema"`
		DomainResultCodes    []string `json:"domain_result_codes"`
		CanonicalResultBytes int      `json:"canonical_result_bytes"`
	} `json:"tools"`
}

type companionMCPSchema struct {
	Definitions map[string]any `json:"$defs"`
}

func loadCompanionMCPContract() (companionMCPContract, error) {
	var manifest companionMCPManifest
	if err := decodeEmbeddedMCPJSON(mcpv1.ManifestJSON(), &manifest); err != nil {
		return companionMCPContract{}, fmt.Errorf("server: MCP manifest 非法: %w", err)
	}
	var schema companionMCPSchema
	if err := decodeEmbeddedMCPJSON(mcpv1.SchemaJSON(), &schema); err != nil {
		return companionMCPContract{}, fmt.Errorf("server: MCP schema 非法: %w", err)
	}
	if manifest.ApplicationContractVersion != "v1" || manifest.MCPProtocolVersion != "2025-11-25" ||
		manifest.EndpointPath != "/mcp" || !manifest.Stateless || !manifest.JSONResponse || manifest.SSE || manifest.Sessions ||
		manifest.Limits.RequestBodyBytes != 256<<10 || manifest.Limits.WireResponseBytes != 160<<10 ||
		manifest.Limits.PlanInputBytes != 64<<10 || len(schema.Definitions) == 0 {
		return companionMCPContract{}, fmt.Errorf("server: MCP contract metadata 不匹配")
	}
	wantNames := companion.PlanningToolNames()
	if len(manifest.Tools) != len(wantNames) {
		return companionMCPContract{}, fmt.Errorf("server: MCP tool 数量=%d", len(manifest.Tools))
	}
	wantDomainCodes := map[string][]string{
		companion.ToolGetPlanningContext: {},
		companion.ToolListAffordances:    {},
		companion.ToolInspectInventory:   {},
		companion.ToolFindVisibleBlocks:  {companion.ValidatorUnknownBlock},
		companion.ToolQueryTerrain:       {companion.ValidatorOutOfBounds},
		companion.ToolValidatePlan: {
			companion.ValidatorInvalidSchema,
			companion.ValidatorOutOfBounds,
			companion.ValidatorUnknownPlayer,
			companion.ValidatorUnmineableTarget,
			companion.ValidatorUnknownBlock,
			companion.ValidatorMissingItem,
			companion.ValidatorSnapshotMismatch,
		},
	}
	contract := companionMCPContract{
		ProtocolVersion: manifest.MCPProtocolVersion, EndpointPath: manifest.EndpointPath,
		RequestBodyBytes: manifest.Limits.RequestBodyBytes, WireResponseBytes: manifest.Limits.WireResponseBytes,
		Tools: make([]companionMCPToolContract, 0, len(manifest.Tools)),
	}
	seen := make(map[string]struct{}, len(manifest.Tools))
	for index, tool := range manifest.Tools {
		if tool.Name != wantNames[index] || tool.InputSchema == "" || tool.ResultSchema == "" ||
			!slices.Equal(tool.DomainResultCodes, wantDomainCodes[tool.Name]) ||
			tool.CanonicalResultBytes != companion.PlanningToolCanonicalLimit(tool.Name) {
			return companionMCPContract{}, fmt.Errorf("server: MCP tool[%d] manifest 不匹配", index)
		}
		if _, duplicate := seen[tool.Name]; duplicate {
			return companionMCPContract{}, fmt.Errorf("server: MCP tool %q 重复", tool.Name)
		}
		seen[tool.Name] = struct{}{}
		input, ok := schema.Definitions[tool.InputSchema]
		if !ok {
			return companionMCPContract{}, fmt.Errorf("server: MCP input schema %q 缺失", tool.InputSchema)
		}
		output, ok := schema.Definitions[tool.ResultSchema]
		if !ok {
			return companionMCPContract{}, fmt.Errorf("server: MCP result schema %q 缺失", tool.ResultSchema)
		}
		resolvedInput, err := resolveCompanionMCPSchema(input, schema.Definitions, map[string]bool{tool.InputSchema: true})
		if err != nil {
			return companionMCPContract{}, err
		}
		resolvedOutput, err := resolveCompanionMCPSchema(output, schema.Definitions, map[string]bool{tool.ResultSchema: true})
		if err != nil {
			return companionMCPContract{}, err
		}
		inputObject, ok := resolvedInput.(map[string]any)
		if !ok || inputObject["type"] != "object" {
			return companionMCPContract{}, fmt.Errorf("server: MCP tool %q input 非 object", tool.Name)
		}
		inputJSON, err := json.Marshal(resolvedInput)
		if err != nil {
			return companionMCPContract{}, err
		}
		outputJSON, err := json.Marshal(resolvedOutput)
		if err != nil {
			return companionMCPContract{}, err
		}
		contract.Tools = append(contract.Tools, companionMCPToolContract{
			Name: tool.Name, InputSchema: inputJSON, OutputSchema: outputJSON,
			CanonicalResultBytes: tool.CanonicalResultBytes,
		})
	}
	if err := validateCompanionMCPPlaceEnum(contract.Tools); err != nil {
		return companionMCPContract{}, err
	}
	return contract, nil
}

func decodeEmbeddedMCPJSON(data []byte, target any) error {
	if err := validateCompanionMCPJSONShape(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("存在尾随 JSON")
		}
		return err
	}
	return nil
}

func resolveCompanionMCPSchema(value any, definitions map[string]any, stack map[string]bool) (any, error) {
	switch current := value.(type) {
	case map[string]any:
		if len(current) == 1 {
			if reference, ok := current["$ref"].(string); ok {
				const prefix = "#/$defs/"
				if len(reference) <= len(prefix) || reference[:len(prefix)] != prefix {
					return nil, fmt.Errorf("server: MCP schema 含外部 reference")
				}
				target := reference[len(prefix):]
				definition, ok := definitions[target]
				if !ok || stack[target] {
					return nil, fmt.Errorf("server: MCP schema reference %q 非法", target)
				}
				nextStack := make(map[string]bool, len(stack)+1)
				for name, present := range stack {
					nextStack[name] = present
				}
				nextStack[target] = true
				return resolveCompanionMCPSchema(definition, definitions, nextStack)
			}
		}
		resolved := make(map[string]any, len(current))
		for key, item := range current {
			value, err := resolveCompanionMCPSchema(item, definitions, stack)
			if err != nil {
				return nil, err
			}
			resolved[key] = value
		}
		return resolved, nil
	case []any:
		resolved := make([]any, len(current))
		for index, item := range current {
			value, err := resolveCompanionMCPSchema(item, definitions, stack)
			if err != nil {
				return nil, err
			}
			resolved[index] = value
		}
		return resolved, nil
	case nil, string, bool, json.Number:
		return current, nil
	default:
		return nil, fmt.Errorf("server: MCP schema 含非法 JSON 值 %T", value)
	}
}

func validateCompanionMCPPlaceEnum(tools []companionMCPToolContract) error {
	var validatorSchema any
	for _, tool := range tools {
		if tool.Name == companion.ToolValidatePlan {
			if err := json.Unmarshal(tool.InputSchema, &validatorSchema); err != nil {
				return err
			}
			break
		}
	}
	root, ok := validatorSchema.(map[string]any)
	if !ok {
		return fmt.Errorf("server: validate_plan schema 缺失")
	}
	properties, _ := root["properties"].(map[string]any)
	plan, _ := properties["plan"].(map[string]any)
	planProperties, _ := plan["properties"].(map[string]any)
	steps, _ := planProperties["steps"].(map[string]any)
	items, _ := steps["items"].(map[string]any)
	oneOf, _ := items["oneOf"].([]any)
	var names []string
	for _, branch := range oneOf {
		object, _ := branch.(map[string]any)
		branchProperties, _ := object["properties"].(map[string]any)
		block, _ := branchProperties["block"].(map[string]any)
		rawEnum, ok := block["enum"].([]any)
		if !ok {
			continue
		}
		for _, rawName := range rawEnum {
			name, ok := rawName.(string)
			if !ok {
				return fmt.Errorf("server: place enum 非字符串")
			}
			names = append(names, name)
		}
	}
	if !slices.Equal(names, companion.PlanningPlaceBlockNames()) {
		return fmt.Errorf("server: place enum 与 core canonical registry 漂移")
	}
	return nil
}
