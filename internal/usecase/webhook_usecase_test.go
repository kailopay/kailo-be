package usecase

import "testing"

func TestPublicWebhookEventTypeMapsStableOrderLifecycleEvents(t *testing.T) {
	tests := []struct {
		internal string
		public   string
		ok       bool
	}{
		{internal: "order.created", public: EventOrderCreated, ok: true},
		{internal: "checkout.created", public: EventOrderPaymentPending, ok: true},
		{internal: "payment.confirmed", public: EventOrderPaymentConfirmed, ok: true},
		{internal: "asset.received", public: EventOrderAssetReceived, ok: true},
		{internal: "stellar.transfer_requested", public: EventOrderProcessing, ok: true},
		{internal: "retirement.requested", public: EventOrderProcessing, ok: true},
		{internal: "stellar.transfer_confirmed", public: EventOrderCompleted, ok: true},
		{internal: "payout.simulated", public: EventOrderCompleted, ok: true},
		{internal: "checkout.failed", public: EventOrderFailed, ok: true},
		{internal: "asset.invalid", public: EventOrderFailed, ok: true},
		{internal: "retirement.failed", public: EventOrderFailed, ok: true},
		{internal: "stellar.failed", public: EventOrderFailed, ok: true},
		{internal: "order.expired", public: EventOrderExpired, ok: true},
		{internal: "deposit.instructions_issued", ok: false},
		{internal: "retirement.confirmed", ok: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.internal, func(t *testing.T) {
			public, ok := PublicWebhookEventType(testCase.internal)
			if public != testCase.public || ok != testCase.ok {
				t.Fatalf("PublicWebhookEventType(%q) = %q/%v, want %q/%v", testCase.internal, public, ok, testCase.public, testCase.ok)
			}
		})
	}
}
