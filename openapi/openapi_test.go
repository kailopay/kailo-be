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
		"/auth",
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
		"/v1/developer/overview",
		"/v1/developer/analytics",
		"/v1/developer/revenue/summary",
		"/v1/developer/revenue/entries",
		"/v1/developer/orders",
		"/v1/developer/wallets",
		"/v1/developer/wallets/{id}",
		"/v1/webhook-endpoints",
		"/v1/webhook-endpoints/{id}",
		"/v1/webhook-deliveries",
		"/v1/webhook-deliveries/{id}/replay",
		"/v1/onramps",
		"/v1/offramps",
		"/v1/orders",
		"/v1/orders/{id}",
		"/.well-known/stellar.toml",
		"/federation",
		"/sep24/info",
		"/sep24/transactions/deposit/interactive",
		"/sep24/transactions/withdraw/interactive",
		"/sep24/transactions",
		"/sep24/transaction",
		"/sep24/interactive/{id}",
		"/sep24/deposit",
		"/sep24/withdraw",
		"/sep38/info",
		"/sep38/prices",
		"/sep38/price",
		"/sep38/quote",
		"/sep38/quote/{id}",
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
		if operation == nil || operation.Responses.Value("403") == nil || operation.Security == nil {
			t.Fatalf("POST %s must document the KYC-required response", path)
		}
		if _, ok := (*operation.Security)[0]["sep10Bearer"]; !ok {
			t.Fatalf("POST %s must use SEP-10 bearer security", path)
		}
	}
	sep10 := document.Paths.Find("/auth")
	if sep10 == nil || sep10.Get == nil || sep10.Post == nil {
		t.Fatal("/auth must document both SEP-10 challenge and exchange")
	}
	if document.Components.SecuritySchemes["sep10Bearer"] == nil {
		t.Fatal("missing SEP-10 bearer security scheme")
	}
	firmQuote := document.Paths.Find("/sep38/quote")
	if firmQuote == nil || firmQuote.Post == nil || firmQuote.Post.Security == nil {
		t.Fatal("POST /sep38/quote must document authenticated firm quotes")
	}
	if document.Paths.Find("/sep38/quote/{id}").Get == nil {
		t.Fatal("GET /sep38/quote/{id} must be documented")
	}
	for _, schema := range []string{
		"SEP24TransactionListResponse",
		"SEP38InfoResponse",
		"SEP38PricesResponse",
		"SEP38PriceResponse",
		"SEP38QuoteRequest",
		"SEP38QuoteResponse",
		"SEP38Error",
	} {
		if document.Components.Schemas[schema] == nil {
			t.Errorf("missing anchor schema %q", schema)
		}
	}
	webhookEndpoints := document.Paths.Find("/v1/webhook-endpoints")
	if webhookEndpoints == nil || webhookEndpoints.Post == nil || webhookEndpoints.Get == nil {
		t.Fatal("webhook endpoint registration and listing must be documented")
	}
	if webhookEndpoints.Post.RequestBody == nil || webhookEndpoints.Post.Responses.Value("201") == nil {
		t.Fatal("POST /v1/webhook-endpoints must document its request and creation response")
	}
	webhookEndpointDetail := document.Paths.Find("/v1/webhook-endpoints/{id}")
	if webhookEndpointDetail.Delete == nil || webhookEndpointDetail.Get == nil || webhookEndpointDetail.Post == nil {
		t.Fatal("GET, POST, and DELETE /v1/webhook-endpoints/{id} must be documented")
	}
	if document.Paths.Find("/v1/webhook-deliveries").Get == nil || document.Paths.Find("/v1/webhook-deliveries/{id}/replay").Post == nil {
		t.Fatal("webhook delivery listing and replay must be documented")
	}
	for _, schema := range []string{
		"CreateWebhookEndpointRequest",
		"CreateWebhookEndpointResponse",
		"WebhookEndpoint",
		"WebhookEndpointListResponse",
		"WebhookEventType",
		"DeveloperOverviewResponse",
		"DeveloperAnalyticsResponse",
		"DeveloperRevenueSummaryResponse",
		"DeveloperRevenueEntriesResponse",
		"DeveloperOrdersResponse",
		"DeveloperWallet",
		"WebhookDelivery",
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
