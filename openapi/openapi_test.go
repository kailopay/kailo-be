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
	for _, path := range []string{
		"/auth/register",
		"/auth/login",
		"/auth/google/login",
		"/auth/google/callback",
		"/auth/logout",
		"/auth/email/verify",
		"/auth/email/resend",
		"/auth/password/forgot",
		"/auth/password/reset",
		"/auth/password/change",
		"/auth/me",
		"/auth/me/avatar",
		"/v1/kyc",
		"/v1/kyc/inquiry",
		"/v1/api-keys",
		"/v1/api-keys/{id}",
		"/v1/webhook-endpoints",
		"/v1/webhook-endpoints/{id}",
		"/v1/onramps",
		"/v1/offramps",
		"/v1/orders",
		"/v1/orders/{id}",
		"/.well-known/stellar.toml",
		"/federation",
		"/sep24/info",
		"/sep24/transactions/deposit/interactive",
		"/sep24/transactions/withdraw/interactive",
		"/sep24/transaction",
		"/sep24/interactive/{id}",
		"/sep24/deposit",
		"/sep24/withdraw",
		"/callbacks/payments/xendit",
		"/callbacks/kyc/persona",
	} {
		if document.Paths.Find(path) == nil {
			t.Errorf("missing auth path %q", path)
		}
	}
	kyc := document.Paths.Find("/v1/kyc").Get
	if kyc == nil || kyc.Security == nil || len(*kyc.Security) != 1 {
		t.Fatal("/v1/kyc must use the kailoSession cookie security scheme")
	}
	if _, ok := (*kyc.Security)[0]["kailoSession"]; !ok {
		t.Fatal("/v1/kyc must use the kailoSession cookie security scheme")
	}
	apiKeys := document.Paths.Find("/v1/api-keys").Post
	if apiKeys == nil || apiKeys.Responses.Value("403") == nil {
		t.Fatal("POST /v1/api-keys must document the KYC-required response")
	}
	for _, path := range []string{"/v1/onramps", "/v1/offramps"} {
		operation := document.Paths.Find(path).Post
		if operation == nil || operation.Responses.Value("403") == nil {
			t.Fatalf("POST %s must document the KYC-required response", path)
		}
	}
	for _, path := range []string{
		"/sep24/transactions/deposit/interactive",
		"/sep24/transactions/withdraw/interactive",
	} {
		operation := document.Paths.Find(path).Post
		if operation == nil || operation.Responses.Value("403") == nil {
			t.Fatalf("POST %s must document the KYC-required response", path)
		}
	}
	webhookEndpoints := document.Paths.Find("/v1/webhook-endpoints")
	if webhookEndpoints == nil || webhookEndpoints.Post == nil || webhookEndpoints.Get == nil {
		t.Fatal("webhook endpoint registration and listing must be documented")
	}
	if webhookEndpoints.Post.RequestBody == nil || webhookEndpoints.Post.Responses.Value("201") == nil {
		t.Fatal("POST /v1/webhook-endpoints must document its request and creation response")
	}
	if document.Paths.Find("/v1/webhook-endpoints/{id}").Delete == nil {
		t.Fatal("DELETE /v1/webhook-endpoints/{id} must be documented")
	}
	for _, schema := range []string{
		"CreateWebhookEndpointRequest",
		"CreateWebhookEndpointResponse",
		"WebhookEndpoint",
		"WebhookEndpointListResponse",
		"WebhookEventType",
	} {
		if document.Components.Schemas[schema] == nil {
			t.Errorf("missing webhook schema %q", schema)
		}
	}
	kycInquiry := document.Paths.Find("/v1/kyc/inquiry").Post
	if kycInquiry == nil || kycInquiry.Responses.Value("500") == nil {
		t.Fatal("POST /v1/kyc/inquiry must document the internal error response")
	}
	if document.Paths.Find("/internal/auth/password-reset-completed") != nil {
		t.Error("the Auth0 password-reset webhook path must not exist")
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
