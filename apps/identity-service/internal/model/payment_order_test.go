package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOrderResponseFinalAmountZeroSerialization(t *testing.T) {
	zero := 0.0
	now := time.Now()
	txHash := "ZERO_YUAN_COUPON"

	tests := []struct {
		name     string
		order    OrderResponse
		wantKey  string
		wantZero bool
	}{
		{
			name: "final_amount=0 should serialize to 0 not null/omitted",
			order: OrderResponse{
				ID:             uuid.New(),
				OrderNo:        "P202607061200001234",
				PlanID:         uuid.New(),
				PlanName:       "Test Plan",
				PeriodCode:     "month",
				AmountUSDT:     4.99,
				DiscountAmount: 4.99,
				FinalAmount:    0.0,
				PaymentMethod:  "usdt_trc20",
				Status:         PaymentStatusPaid,
				TxHash:         &txHash,
				PaidAmount:     &zero,
				PaidAt:         &now,
				ExpiresAt:      now.Add(2 * time.Hour),
				CreatedAt:      now,
			},
			wantKey:  "final_amount",
			wantZero: true,
		},
		{
			name: "final_amount=4.99 should serialize correctly",
			order: OrderResponse{
				ID:             uuid.New(),
				OrderNo:        "P202607061200015678",
				PlanID:         uuid.New(),
				PlanName:       "Test Plan",
				PeriodCode:     "month",
				AmountUSDT:     4.99,
				DiscountAmount: 0,
				FinalAmount:    4.99,
				PaymentMethod:  "usdt_trc20",
				Status:         PaymentStatusPending,
				ExpiresAt:      now.Add(2 * time.Hour),
				CreatedAt:      now,
			},
			wantKey:  "final_amount",
			wantZero: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.order)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}

			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}

			val, ok := m[tt.wantKey]
			if !ok {
				t.Errorf("key %q missing from JSON output: %s", tt.wantKey, string(data))
				return
			}

			if tt.wantZero {
				num, ok := val.(float64)
				if !ok {
					t.Errorf("final_amount should be number, got %T: %v", val, val)
					return
				}
				if num != 0 {
					t.Errorf("final_amount should be 0, got %v", num)
				}
			}

			t.Logf("JSON output: %s", string(data))
		})
	}
}

func TestNewOrderResponseWithZeroFinalAmount(t *testing.T) {
	order := &PaymentOrder{
		ID:             uuid.New(),
		OrderNo:        "P202607061200009999",
		UserID:         uuid.New(),
		PlanID:         uuid.New(),
		PlanName:       "Free Plan",
		PeriodCode:     "month",
		AmountUSDT:     0,
		DiscountAmount: 0,
		FinalAmount:    0,
		PayAddress:     "Txxxx",
		PayCurrency:    "USDT-TRC20",
		PaymentMethod:  "usdt_trc20",
		Status:         PaymentStatusPaid,
		ExpiresAt:      time.Now().Add(2 * time.Hour),
		CreatedAt:      time.Now(),
	}

	resp := NewOrderResponse(order)

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	val, ok := m["final_amount"]
	if !ok {
		t.Fatalf("final_amount key missing from JSON: %s", string(data))
	}

	num, ok := val.(float64)
	if !ok {
		t.Fatalf("final_amount should be float64, got %T: %v", val, val)
	}

	if num != 0 {
		t.Errorf("final_amount should be 0, got %v", num)
	}

	t.Logf("PASS: NewOrderResponse with FinalAmount=0 serializes correctly")
	t.Logf("JSON: %s", string(data))
}
