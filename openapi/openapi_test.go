package openapi

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestDocumentValidatesAuthContract(t *testing.T) {
	loader := openapi3.NewLoader()
	document, err := loader.LoadFromFile("openapi.yaml")
	if err != nil {
		t.Fatalf("loading OpenAPI document: %v", err)
	}
	if err := document.Validate(t.Context()); err != nil {
		t.Fatalf("validating OpenAPI document: %v", err)
	}
	for _, path := range []string{"/auth/login", "/auth/callback", "/auth/logout", "/auth/me"} {
		if document.Paths.Find(path) == nil {
			t.Errorf("missing auth path %q", path)
		}
	}
	me := document.Paths.Find("/auth/me").Get
	if me == nil || me.Security == nil || len(*me.Security) != 1 {
		t.Fatal("/auth/me must use the kailoSession cookie security scheme")
	}
	if _, ok := (*me.Security)[0]["kailoSession"]; !ok {
		t.Fatal("/auth/me must use the kailoSession cookie security scheme")
	}
	for name := range document.Components.Schemas {
		if containsSensitiveAuthValue(name) {
			t.Errorf("sensitive authentication value exposed as schema %q", name)
		}
	}
	for _, pathItem := range document.Paths.Map() {
		for _, operation := range pathItem.Operations() {
			for _, parameter := range operation.Parameters {
				if parameter == nil || parameter.Value == nil {
					continue
				}
				if containsSensitiveAuthValue(parameter.Value.Name) {
					t.Errorf("sensitive authentication value exposed as parameter %q", parameter.Value.Name)
				}
			}
		}
	}
}

func containsSensitiveAuthValue(value string) bool {
	value = strings.ToLower(value)
	for _, forbidden := range []string{"access_token", "refresh_token", "id_token", "authorization_code", "state", "nonce", "code_verifier"} {
		if strings.Contains(value, forbidden) {
			return true
		}
	}
	return false
}
