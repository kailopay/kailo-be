package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSEP24DepositDisclosesPersonaGateInsteadOfSyntheticKYC(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewSep24Handler(Sep24Config{DepositAccount: "G" + "A"})
	router := gin.New()
	router.POST("/sep24/deposit", handler.Deposit)

	request := httptest.NewRequest(http.MethodPost, "/sep24/deposit?asset_code=XLM", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if _, exists := payload["kyc_simulated"]; exists {
		t.Fatal("response still exposes the synthetic KYC flag")
	}
	if required, ok := payload["kyc_required"].(bool); !ok || !required {
		t.Fatalf("kyc_required = %#v, want true", payload["kyc_required"])
	}
	if message, ok := payload["message"].(string); !ok || message != "Complete Persona identity verification before creating a value-moving order." {
		t.Fatalf("message = %#v", payload["message"])
	}
}
