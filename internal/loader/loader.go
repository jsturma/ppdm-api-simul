package loader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Operation struct {
	Method       string
	PathTemplate string
	PathPattern  *regexp.Regexp
	Operation    map[string]any
	SpecTitle    string
	SpecVersion  string
	Schemas      map[string]any
}

type ResponseContent struct {
	StatusCode  int
	ContentType string
	Schema      map[string]any
	Example     any
}

func (o *Operation) OperationID() string {
	if v, ok := o.Operation["operationId"].(string); ok {
		return v
	}
	return ""
}

func (o *Operation) Security() []any {
	if v, ok := o.Operation["security"]; ok {
		if list, ok := v.([]any); ok {
			return list
		}
	}
	return nil
}

func (o *Operation) SuccessResponse() (int, map[string]any) {
	for _, status := range []string{"200", "201", "202", "204"} {
		if content, ok := o.ResponseContent(status); ok {
			if content.StatusCode == 204 {
				return 204, nil
			}
			return content.StatusCode, content.Schema
		}
	}
	return 404, nil
}

func (o *Operation) ResponseContent(status string) (ResponseContent, bool) {
	responses, ok := o.Operation["responses"].(map[string]any)
	if !ok {
		return ResponseContent{}, false
	}

	raw, ok := responses[status]
	if !ok {
		return ResponseContent{}, false
	}
	response, ok := raw.(map[string]any)
	if !ok {
		return ResponseContent{}, false
	}

	code := 200
	switch status {
	case "201":
		code = 201
	case "202":
		code = 202
	case "204":
		return ResponseContent{StatusCode: 204}, true
	case "400":
		code = 400
	case "401":
		code = 401
	case "403":
		code = 403
	case "404":
		code = 404
	case "409":
		code = 409
	case "416":
		code = 416
	case "500":
		code = 500
	case "503":
		code = 503
	default:
		if n, err := strconv.Atoi(status); err == nil {
			code = n
		}
	}

	content, ok := response["content"].(map[string]any)
	if !ok {
		return ResponseContent{StatusCode: code}, true
	}

	for _, contentType := range []string{"application/json", "text/csv", "application/octet-stream"} {
		media, ok := content[contentType].(map[string]any)
		if !ok {
			continue
		}
		result := ResponseContent{
			StatusCode:  code,
			ContentType: contentType,
		}
		if schema, ok := media["schema"].(map[string]any); ok {
			result.Schema = schema
		}
		if example, ok := media["example"]; ok {
			result.Example = example
		}
		return result, true
	}

	return ResponseContent{StatusCode: code}, true
}

type Bundle struct {
	Operations []Operation
	Schemas    map[string]map[string]any
}

func (b *Bundle) Match(method, path string) *Operation {
	method = strings.ToUpper(method)
	for i := range b.Operations {
		op := &b.Operations[i]
		if op.Method == method && op.PathPattern.MatchString(path) {
			return op
		}
	}
	return nil
}

func pathToPattern(pathTemplate string) *regexp.Regexp {
	segments := strings.Split(pathTemplate, "/")
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			parts = append(parts, `[^/]+`)
		} else {
			parts = append(parts, regexp.QuoteMeta(segment))
		}
	}
	pattern := "^/" + strings.Join(parts, "/") + "$"
	return regexp.MustCompile(pattern)
}

func LoadDirectory(directory string) (*Bundle, error) {
	entries, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)

	bundle := &Bundle{
		Schemas: make(map[string]map[string]any),
	}

	for _, specPath := range entries {
		data, err := os.ReadFile(specPath)
		if err != nil {
			return nil, err
		}

		var spec map[string]any
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, err
		}

		info, _ := spec["info"].(map[string]any)
		title, _ := info["title"].(string)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(specPath), ".json")
		}
		version, _ := info["version"].(string)
		if version == "" {
			version = "unknown"
		}

		schemas := map[string]any{}
		if components, ok := spec["components"].(map[string]any); ok {
			if s, ok := components["schemas"].(map[string]any); ok {
				schemas = s
			}
		}
		bundle.Schemas[title+":"+version] = schemas

		paths, ok := spec["paths"].(map[string]any)
		if !ok {
			continue
		}

		for pathTemplate, methods := range paths {
			methodMap, ok := methods.(map[string]any)
			if !ok {
				continue
			}
			for method, rawOp := range methodMap {
				if strings.HasPrefix(method, "x-") {
					continue
				}
				operation, ok := rawOp.(map[string]any)
				if !ok {
					continue
				}
				bundle.Operations = append(bundle.Operations, Operation{
					Method:       strings.ToUpper(method),
					PathTemplate: pathTemplate,
					PathPattern:  pathToPattern(pathTemplate),
					Operation:    operation,
					SpecTitle:    title,
					SpecVersion:  version,
					Schemas:      schemas,
				})
			}
		}
	}

	return bundle, nil
}
