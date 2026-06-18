package mock_test

import (
	"encoding/json"
	"testing"

	"ppdm-simul/internal/loader"
	"ppdm-simul/internal/mock"
)

func TestAssetResponseHasRequiredFields(t *testing.T) {
	bundle, err := loader.LoadDirectory("../../openapi-json")
	if err != nil {
		t.Fatalf("load specs: %v", err)
	}

	var schemas map[string]any
	for _, s := range bundle.Schemas {
		if _, ok := s["Asset"]; ok {
			schemas = s
			break
		}
	}
	if schemas == nil {
		t.Fatal("Asset schema not found")
	}

	gen := mock.New(schemas)
	assetSchema := schemas["Asset"].(map[string]any)
	body, ok := gen.FromSchema(assetSchema, 0).(map[string]any)
	if !ok {
		t.Fatal("expected object response")
	}

	for _, field := range []string{"details", "name", "type"} {
		if _, exists := body[field]; !exists {
			t.Fatalf("missing required field %q", field)
		}
	}
}

func TestAccessTokenResponseHasRequiredFields(t *testing.T) {
	bundle, err := loader.LoadDirectory("../../openapi-json")
	if err != nil {
		t.Fatalf("load specs: %v", err)
	}

	var schemas map[string]any
	for _, s := range bundle.Schemas {
		if _, ok := s["AccessToken"]; ok {
			schemas = s
			break
		}
	}

	gen := mock.New(schemas)
	body, ok := gen.FromSchema(schemas["AccessToken"].(map[string]any), 0).(map[string]any)
	if !ok {
		t.Fatal("expected object response")
	}

	for _, field := range []string{"access_token", "token_type"} {
		if _, exists := body[field]; !exists {
			t.Fatalf("missing required field %q", field)
		}
	}
}

func TestAssetsEndpointUsesResponseSchema(t *testing.T) {
	bundle, err := loader.LoadDirectory("../../openapi-json")
	if err != nil {
		t.Fatalf("load specs: %v", err)
	}

	op := bundle.Match("GET", "/api/v2/assets")
	if op == nil {
		t.Fatal("assets operation not found")
	}

	status, schema := op.SuccessResponse()
	if status != 200 || schema == nil {
		t.Fatalf("unexpected success response: %d", status)
	}

	gen := mock.New(op.Schemas)
	body, ok := gen.Paginated(schema, nil, 1, 5).(map[string]any)
	if !ok {
		t.Fatal("expected paginated object")
	}

	if _, ok := body["content"]; !ok {
		t.Fatal("missing content field")
	}
	if _, ok := body["page"]; !ok {
		t.Fatal("missing page field")
	}

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("empty response body")
	}
}
