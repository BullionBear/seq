package evbus

import (
	"testing"

	"github.com/BullionBear/seq/core/model/event"
)

// ============================================================================
// Tests for ReqBalanceSnapshot Serialization
// ============================================================================

func TestReqBalanceSnapshotSize(t *testing.T) {
	tests := []struct {
		name     string
		snapshot event.ReqBalanceSnapshot
	}{
		{
			name: "empty balances",
			snapshot: event.ReqBalanceSnapshot{
				AccountID: 1,
				Balances:  nil,
			},
		},
		{
			name: "single balance",
			snapshot: event.ReqBalanceSnapshot{
				AccountID: 1,
				Balances: []event.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
				},
			},
		},
		{
			name: "multiple balances",
			snapshot: event.ReqBalanceSnapshot{
				AccountID: 123,
				Balances: []event.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
					{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
					{TokenID: 3, Available: 100.0, Locked: 50.0, Total: 150.0},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := ReqBalanceSnapshotSize(&tt.snapshot)
			expectedSize := uint64(SizeOfReqBalanceSnapshotHeader) + uint64(len(tt.snapshot.Balances))*uint64(SizeOfBalance)
			if size != expectedSize {
				t.Errorf("ReqBalanceSnapshotSize() = %d, want %d", size, expectedSize)
			}
		})
	}
}

func TestSerializeDeserializeReqBalanceSnapshot(t *testing.T) {
	tests := []struct {
		name     string
		snapshot event.ReqBalanceSnapshot
	}{
		{
			name: "empty balances",
			snapshot: event.ReqBalanceSnapshot{
				AccountID: 1,
				Balances:  []event.Balance{},
			},
		},
		{
			name: "single balance",
			snapshot: event.ReqBalanceSnapshot{
				AccountID: 42,
				Balances: []event.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
				},
			},
		},
		{
			name: "multiple balances",
			snapshot: event.ReqBalanceSnapshot{
				AccountID: 123,
				Balances: []event.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
					{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
					{TokenID: 3, Available: 100.12345678, Locked: 50.87654321, Total: 150.99999999},
				},
			},
		},
		{
			name: "large account ID",
			snapshot: event.ReqBalanceSnapshot{
				AccountID: 999999999,
				Balances: []event.Balance{
					{TokenID: 1000, Available: 0.00000001, Locked: 0.0, Total: 0.00000001},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate size and allocate buffer
			size := ReqBalanceSnapshotSize(&tt.snapshot)
			buf := make([]byte, size)

			// Serialize
			written := SerializeReqBalanceSnapshot(buf, &tt.snapshot)
			if uint64(written) != size {
				t.Errorf("SerializeReqBalanceSnapshot wrote %d bytes, expected %d", written, size)
			}

			// Deserialize
			result := DeserializeReqBalanceSnapshot(buf)

			// Verify AccountID
			if result.AccountID != tt.snapshot.AccountID {
				t.Errorf("AccountID mismatch: got %d, want %d", result.AccountID, tt.snapshot.AccountID)
			}

			// Verify balances count
			if len(result.Balances) != len(tt.snapshot.Balances) {
				t.Fatalf("Balances count mismatch: got %d, want %d", len(result.Balances), len(tt.snapshot.Balances))
			}

			// Verify each balance
			for i := range tt.snapshot.Balances {
				expected := tt.snapshot.Balances[i]
				actual := result.Balances[i]

				if actual.TokenID != expected.TokenID {
					t.Errorf("Balance[%d].TokenID mismatch: got %d, want %d", i, actual.TokenID, expected.TokenID)
				}
				if actual.Available != expected.Available {
					t.Errorf("Balance[%d].Available mismatch: got %f, want %f", i, actual.Available, expected.Available)
				}
				if actual.Locked != expected.Locked {
					t.Errorf("Balance[%d].Locked mismatch: got %f, want %f", i, actual.Locked, expected.Locked)
				}
				if actual.Total != expected.Total {
					t.Errorf("Balance[%d].Total mismatch: got %f, want %f", i, actual.Total, expected.Total)
				}
			}
		})
	}
}

func BenchmarkSerializeReqBalanceSnapshot(b *testing.B) {
	snapshot := event.ReqBalanceSnapshot{
		AccountID: 123,
		Balances: []event.Balance{
			{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
			{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
			{TokenID: 3, Available: 100.0, Locked: 50.0, Total: 150.0},
		},
	}

	size := ReqBalanceSnapshotSize(&snapshot)
	buf := make([]byte, size)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		SerializeReqBalanceSnapshot(buf, &snapshot)
	}
}

func BenchmarkDeserializeReqBalanceSnapshot(b *testing.B) {
	snapshot := event.ReqBalanceSnapshot{
		AccountID: 123,
		Balances: []event.Balance{
			{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
			{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
			{TokenID: 3, Available: 100.0, Locked: 50.0, Total: 150.0},
		},
	}

	size := ReqBalanceSnapshotSize(&snapshot)
	buf := make([]byte, size)
	SerializeReqBalanceSnapshot(buf, &snapshot)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = DeserializeReqBalanceSnapshot(buf)
	}
}

// ============================================================================
// Tests for OrderUpdate Serialization (existing but adding for completeness)
// ============================================================================

func TestSerializeDeserializeOrderUpdate(t *testing.T) {
	orderUpdate := event.OrderUpdate{
		ClientOrderID: 12345,
		OrderID:       67890,
		OrderStatus:   2, // PartiallyFilled
		ExecutedQty:   0.5,
		UpdatedAt:     1672531200000000000,
	}

	size := OrderUpdateSize()
	buf := make([]byte, size)

	written := SerializeOrderUpdate(buf, &orderUpdate)
	if uint64(written) != size {
		t.Errorf("SerializeOrderUpdate wrote %d bytes, expected %d", written, size)
	}

	result := DeserializeOrderUpdate(buf)

	if result.ClientOrderID != orderUpdate.ClientOrderID {
		t.Errorf("ClientOrderID mismatch: got %d, want %d", result.ClientOrderID, orderUpdate.ClientOrderID)
	}
	if result.OrderID != orderUpdate.OrderID {
		t.Errorf("OrderID mismatch: got %d, want %d", result.OrderID, orderUpdate.OrderID)
	}
	if result.OrderStatus != orderUpdate.OrderStatus {
		t.Errorf("OrderStatus mismatch: got %d, want %d", result.OrderStatus, orderUpdate.OrderStatus)
	}
	if result.ExecutedQty != orderUpdate.ExecutedQty {
		t.Errorf("ExecutedQty mismatch: got %f, want %f", result.ExecutedQty, orderUpdate.ExecutedQty)
	}
	if result.UpdatedAt != orderUpdate.UpdatedAt {
		t.Errorf("UpdatedAt mismatch: got %d, want %d", result.UpdatedAt, orderUpdate.UpdatedAt)
	}
}

// ============================================================================
// Tests for Fill Serialization
// ============================================================================

func TestSerializeDeserializeFill(t *testing.T) {
	fill := event.Fill{
		ClientOrderID: 12345,
		OrderID:       67890,
		FillID:        111,
		FilledQty:     0.5,
		FilledPrice:   50000.0,
		FeeCcyID:      1,
		FeeQty:        0.0001,
		FilledAt:      1672531200000000000,
	}

	size := FillSize()
	buf := make([]byte, size)

	written := SerializeFill(buf, &fill)
	if uint64(written) != size {
		t.Errorf("SerializeFill wrote %d bytes, expected %d", written, size)
	}

	result := DeserializeFill(buf)

	if result.ClientOrderID != fill.ClientOrderID {
		t.Errorf("ClientOrderID mismatch: got %d, want %d", result.ClientOrderID, fill.ClientOrderID)
	}
	if result.OrderID != fill.OrderID {
		t.Errorf("OrderID mismatch: got %d, want %d", result.OrderID, fill.OrderID)
	}
	if result.FillID != fill.FillID {
		t.Errorf("FillID mismatch: got %d, want %d", result.FillID, fill.FillID)
	}
	if result.FilledQty != fill.FilledQty {
		t.Errorf("FilledQty mismatch: got %f, want %f", result.FilledQty, fill.FilledQty)
	}
	if result.FilledPrice != fill.FilledPrice {
		t.Errorf("FilledPrice mismatch: got %f, want %f", result.FilledPrice, fill.FilledPrice)
	}
	if result.FeeCcyID != fill.FeeCcyID {
		t.Errorf("FeeCcyID mismatch: got %d, want %d", result.FeeCcyID, fill.FeeCcyID)
	}
	if result.FeeQty != fill.FeeQty {
		t.Errorf("FeeQty mismatch: got %f, want %f", result.FeeQty, fill.FeeQty)
	}
	if result.FilledAt != fill.FilledAt {
		t.Errorf("FilledAt mismatch: got %d, want %d", result.FilledAt, fill.FilledAt)
	}
}
