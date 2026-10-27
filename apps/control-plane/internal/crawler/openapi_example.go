package crawler

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// mustJSON encodes node properties without HTML escaping, so URLs keep a
// readable "&" in the builder.
func mustJSON(v interface{}) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// exampleBody builds a request body for a JSON media type: the document's
// own example if it has one, otherwise one synthesised from the schema.
func (s *OpenAPISpec) exampleBody(mt MediaType) string {
	if len(mt.Example) > 0 && string(mt.Example) != "null" {
		return prettyJSON(mt.Example)
	}
	if len(mt.Examples) > 0 {
		names := make([]string, 0, len(mt.Examples))
		for n := range mt.Examples {
			names = append(names, n)
		}
		sort.Strings(names)
		if v := mt.Examples[names[0]].Value; len(v) > 0 && string(v) != "null" {
			return prettyJSON(v)
		}
	}
	if v := s.sample(mt.Schema, 0); v != nil {
		b, _ := json.MarshalIndent(v, "", "  ")
		return string(b)
	}
	return "{}"
}

func prettyJSON(raw json.RawMessage) string {
	var v interface{}
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

type schemaObject struct {
	Ref        string                     `json:"$ref"`
	Type       interface{}                `json:"type"` // a string, or a list in OpenAPI 3.1
	Example    interface{}                `json:"example"`
	Default    interface{}                `json:"default"`
	Enum       []interface{}              `json:"enum"`
	Format     string                     `json:"format"`
	Properties map[string]json.RawMessage `json:"properties"`
	Items      json.RawMessage            `json:"items"`
	AllOf      []json.RawMessage          `json:"allOf"`
	OneOf      []json.RawMessage          `json:"oneOf"`
	AnyOf      []json.RawMessage          `json:"anyOf"`
}

// sample produces an example value for a JSON schema, following $ref into
// components (OpenAPI 3) or definitions (Swagger 2).
func (s *OpenAPISpec) sample(raw json.RawMessage, depth int) interface{} {
	if len(raw) == 0 || depth > 6 {
		return nil
	}
	var sch schemaObject
	if json.Unmarshal(raw, &sch) != nil {
		return nil
	}
	if sch.Ref != "" {
		name := sch.Ref[strings.LastIndex(sch.Ref, "/")+1:]
		if r, ok := s.Components.Schemas[name]; ok {
			return s.sample(r, depth+1)
		}
		if r, ok := s.Definitions[name]; ok {
			return s.sample(r, depth+1)
		}
		return nil
	}
	switch {
	case sch.Example != nil:
		return sch.Example
	case sch.Default != nil:
		return sch.Default
	case len(sch.Enum) > 0:
		return sch.Enum[0]
	case len(sch.AllOf) > 0:
		merged := map[string]interface{}{}
		for _, part := range sch.AllOf {
			if m, ok := s.sample(part, depth+1).(map[string]interface{}); ok {
				for k, v := range m {
					merged[k] = v
				}
			}
		}
		return merged
	case len(sch.OneOf) > 0:
		return s.sample(sch.OneOf[0], depth+1)
	case len(sch.AnyOf) > 0:
		return s.sample(sch.AnyOf[0], depth+1)
	}

	typ, _ := sch.Type.(string)
	if list, ok := sch.Type.([]interface{}); ok && len(list) > 0 {
		typ, _ = list[0].(string)
	}
	if typ == "" && len(sch.Properties) > 0 {
		typ = "object"
	}
	switch typ {
	case "object":
		obj := map[string]interface{}{}
		for name, p := range sch.Properties {
			obj[name] = s.sample(p, depth+1)
		}
		return obj
	case "array":
		if item := s.sample(sch.Items, depth+1); item != nil {
			return []interface{}{item}
		}
		return []interface{}{}
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "string":
		switch sch.Format {
		case "date-time":
			return "2026-01-01T00:00:00Z"
		case "date":
			return "2026-01-01"
		case "email":
			return "user@example.com"
		case "uuid":
			return "00000000-0000-0000-0000-000000000000"
		}
		return "string"
	}
	return nil
}
