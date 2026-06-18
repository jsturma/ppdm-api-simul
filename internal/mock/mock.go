package mock

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Generator struct {
	schemas       map[string]any
	maxDepth      int
	maxArrayItems int
	refsSeen      map[string]struct{}
}

func New(schemas map[string]any) *Generator {
	return &Generator{
		schemas:       schemas,
		maxDepth:      10,
		maxArrayItems: 1,
		refsSeen:      make(map[string]struct{}),
	}
}

func (g *Generator) FromSchema(schema map[string]any, depth int) any {
	if schema == nil {
		return nil
	}
	return g.generate(g.normalize(schema, depth), depth, true)
}

func (g *Generator) FromResponse(schema map[string]any, example any, depth int) any {
	if example != nil {
		return example
	}
	return g.FromSchema(schema, depth)
}

func (g *Generator) Paginated(schema map[string]any, example any, page, pageSize int) any {
	body := g.FromResponse(schema, example, 0)
	obj, ok := body.(map[string]any)
	if !ok {
		return body
	}

	if content, ok := obj["content"].([]any); ok && len(content) > pageSize {
		obj["content"] = content[:pageSize]
	}

	normalized := g.normalize(schema, 0)
	properties, _ := normalized["properties"].(map[string]any)
	pageSchema, _ := properties["page"].(map[string]any)
	pageObj, _ := g.FromSchema(pageSchema, 1).(map[string]any)
	if pageObj == nil {
		pageObj = map[string]any{}
	}

	total := 0
	if content, ok := obj["content"].([]any); ok {
		total = len(content)
	}
	if total == 0 {
		total = pageSize
	}
	totalPages := 0
	if total > 0 {
		totalPages = 1
	}

	pageObj["number"] = page
	pageObj["size"] = pageSize
	pageObj["totalElements"] = total
	pageObj["totalPages"] = totalPages
	pageObj["maxPageableElements"] = firstInt(pageObj["maxPageableElements"], 10000)
	pageObj["queryState"] = firstString(pageObj["queryState"], "END")
	obj["page"] = pageObj

	return obj
}

func (g *Generator) normalize(schema map[string]any, depth int) map[string]any {
	if schema == nil {
		return nil
	}
	if depth > g.maxDepth {
		return schema
	}

	if ref, ok := schema["$ref"].(string); ok {
		target := g.lookupRef(ref)
		if target == nil {
			return schema
		}
		return g.normalize(target, depth+1)
	}

	if allOf, ok := schema["allOf"].([]any); ok {
		merged := map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []any{},
		}
		for _, item := range allOf {
			child, ok := item.(map[string]any)
			if !ok {
				continue
			}
			mergeSchemas(merged, g.normalize(child, depth+1))
		}
		return merged
	}

	return schema
}

func mergeSchemas(dst, src map[string]any) {
	if src == nil {
		return
	}
	dstProps, _ := dst["properties"].(map[string]any)
	if dstProps == nil {
		dstProps = map[string]any{}
		dst["properties"] = dstProps
	}
	srcProps, _ := src["properties"].(map[string]any)
	for name, prop := range srcProps {
		dstProps[name] = prop
	}

	dstRequired := toStringSet(dst["required"])
	for name := range toStringSet(src["required"]) {
		dstRequired[name] = struct{}{}
	}
	if len(dstRequired) > 0 {
		required := make([]any, 0, len(dstRequired))
		for name := range dstRequired {
			required = append(required, name)
		}
		dst["required"] = required
	}

	if t, ok := src["type"].(string); ok {
		dst["type"] = t
	}
}

func (g *Generator) generate(schema map[string]any, depth int, required bool) any {
	if schema == nil {
		return nil
	}
	if depth > g.maxDepth {
		if required {
			return g.fallback(schema)
		}
		return nil
	}

	schema = g.normalize(schema, depth)

	if example, ok := schema["example"]; ok {
		return example
	}
	if def, ok := schema["default"]; ok {
		return def
	}
	if value, ok := schema["const"]; ok {
		return value
	}

	if oneOf, ok := schema["oneOf"].([]any); ok && len(oneOf) > 0 {
		if child, ok := oneOf[0].(map[string]any); ok {
			return g.generate(g.normalize(child, depth+1), depth+1, required)
		}
	}
	if anyOf, ok := schema["anyOf"].([]any); ok && len(anyOf) > 0 {
		if child, ok := anyOf[0].(map[string]any); ok {
			return g.generate(g.normalize(child, depth+1), depth+1, required)
		}
	}

	if nullable, isNullable := schema["nullable"].(bool); isNullable && nullable && !required {
		return nil
	}

	schemaType, _ := schema["type"].(string)
	if schemaType == "object" || schema["properties"] != nil {
		return g.object(schema, depth)
	}
	switch schemaType {
	case "array":
		return g.array(schema, depth)
	case "string":
		return g.stringValue(schema)
	case "integer":
		return g.integer(schema)
	case "number":
		return g.number(schema)
	case "boolean":
		return true
	default:
		if schema["properties"] != nil {
			return g.object(schema, depth)
		}
		if required {
			return g.fallback(schema)
		}
		return nil
	}
}

func (g *Generator) object(schema map[string]any, depth int) map[string]any {
	result := make(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	requiredSet := toStringSet(schema["required"])

	for name, rawProp := range properties {
		propSchema, ok := rawProp.(map[string]any)
		if !ok {
			continue
		}
		_, isRequired := requiredSet[name]
		if !isRequired && depth > 3 {
			continue
		}
		value := g.generate(propSchema, depth+1, isRequired)
		if value == nil && isRequired {
			value = g.fallback(propSchema)
		}
		if value != nil {
			result[name] = value
		}
	}

	for name := range requiredSet {
		if _, ok := result[name]; ok {
			continue
		}
		propSchema, _ := properties[name].(map[string]any)
		if propSchema == nil {
			propSchema = map[string]any{"type": "string"}
		}
		result[name] = g.generate(propSchema, depth+1, true)
	}

	return result
}

func (g *Generator) array(schema map[string]any, depth int) []any {
	itemSchema, _ := schema["items"].(map[string]any)
	if itemSchema == nil {
		itemSchema = map[string]any{"type": "string"}
	}

	count := 1
	if minItems, ok := asInt(schema["minItems"]); ok && minItems > 0 {
		count = minItems
	}
	if count > g.maxArrayItems {
		count = g.maxArrayItems
	}

	items := make([]any, count)
	for i := 0; i < count; i++ {
		items[i] = g.generate(itemSchema, depth+1, true)
	}
	return items
}

func (g *Generator) fallback(schema map[string]any) any {
	schema = g.normalize(schema, 0)
	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		return enum[0]
	}
	switch schema["type"] {
	case "string":
		return g.stringValue(schema)
	case "integer":
		return g.integer(schema)
	case "number":
		return g.number(schema)
	case "boolean":
		return false
	case "array":
		return []any{}
	case "object":
		return map[string]any{}
	default:
		if schema["properties"] != nil {
			return g.object(schema, 0)
		}
		return map[string]any{}
	}
}

func (g *Generator) lookupRef(ref string) map[string]any {
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		return nil
	}
	name := ref[len(prefix):]
	if _, seen := g.refsSeen[name]; seen {
		return nil
	}
	target, ok := g.schemas[name].(map[string]any)
	if !ok {
		return nil
	}
	g.refsSeen[name] = struct{}{}
	defer delete(g.refsSeen, name)
	return target
}

func (g *Generator) stringValue(schema map[string]any) string {
	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		return fmt.Sprint(enum[0])
	}

	if format, ok := schema["format"].(string); ok {
		switch format {
		case "date-time", "date":
			return time.Now().UTC().Format(time.RFC3339)
		case "uuid":
			return newUUID()
		case "uri":
			return "https://ppdm-simulator.local/api/v2"
		}
	}

	if pattern, ok := schema["pattern"].(string); ok {
		if matched, _ := regexp.MatchString(pattern, "abc.def.ghi"); matched {
			return "abc.def.ghi"
		}
		if desc, _ := schema["description"].(string); strings.Contains(desc, "JWT") {
			return "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJwcGRtLXNpbXVsYXRvciJ9.signature"
		}
	}

	desc := strings.ToLower(fmt.Sprint(schema["description"]))
	switch {
	case strings.Contains(desc, "username"):
		return "admin"
	case strings.Contains(desc, "password"):
		return "password"
	case strings.Contains(desc, "token"), strings.Contains(desc, "jti"):
		return newUUID()
	case strings.Contains(desc, "scope"):
		return "admin"
	case strings.Contains(desc, "type") && strings.Contains(desc, "token"):
		return "Bearer"
	case strings.Contains(desc, "query"):
		return "END"
	}

	return "string"
}

func (g *Generator) integer(schema map[string]any) int {
	if def, ok := asInt(schema["default"]); ok {
		return def
	}
	if minimum, ok := asInt(schema["minimum"]); ok {
		return minimum
	}
	return 0
}

func (g *Generator) number(schema map[string]any) float64 {
	if minimum, ok := schema["minimum"].(float64); ok {
		return minimum
	}
	return 0
}

func toStringSet(value any) map[string]struct{} {
	set := map[string]struct{}{}
	items, ok := value.([]any)
	if !ok {
		return set
	}
	for _, item := range items {
		if name, ok := item.(string); ok {
			set[name] = struct{}{}
		}
	}
	return set
}

func asInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func firstInt(value any, fallback int) int {
	if v, ok := asInt(value); ok {
		return v
	}
	return fallback
}

func firstString(value any, fallback string) string {
	if s, ok := value.(string); ok && s != "" {
		return s
	}
	return fallback
}

func NewUUID() string {
	return newUUID()
}

func NewTokenSuffix() string {
	return newTokenSuffix()
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func newTokenSuffix() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
