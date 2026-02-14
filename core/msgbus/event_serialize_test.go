package msgbus

import (
	"testing"

	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

// ============================================================================
// Tests for ReqBalanceSnapshot Serialization
// ============================================================================

func TestReqBalanceSnapshotSize(t *testing.T) {
	tests := []struct {
		name     string
		snapshot event.RespBalanceSnapshot
	}{
		{
			name: "empty balances",
			snapshot: event.RespBalanceSnapshot{
				AccountID: 1,
				Balances:  nil,
			},
		},
		{
			name: "single balance",
			snapshot: event.RespBalanceSnapshot{
				AccountID: 1,
				Balances: []common.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
				},
			},
		},
		{
			name: "multiple balances",
			snapshot: event.RespBalanceSnapshot{
				AccountID: 123,
				Balances: []common.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
					{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
					{TokenID: 3, Available: 100.0, Locked: 50.0, Total: 150.0},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := RespBalanceSnapshotSize(&tt.snapshot)
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
		snapshot event.RespBalanceSnapshot
	}{
		{
			name: "empty balances",
			snapshot: event.RespBalanceSnapshot{
				AccountID: 1,
				Balances:  []common.Balance{},
			},
		},
		{
			name: "single balance",
			snapshot: event.RespBalanceSnapshot{
				AccountID: 42,
				Balances: []common.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
				},
			},
		},
		{
			name: "multiple balances",
			snapshot: event.RespBalanceSnapshot{
				AccountID: 123,
				Balances: []common.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
					{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
					{TokenID: 3, Available: 100.12345678, Locked: 50.87654321, Total: 150.99999999},
				},
			},
		},
		{
			name: "large account ID",
			snapshot: event.RespBalanceSnapshot{
				AccountID: 999999999,
				Balances: []common.Balance{
					{TokenID: 1000, Available: 0.00000001, Locked: 0.0, Total: 0.00000001},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate size and allocate buffer
			size := RespBalanceSnapshotSize(&tt.snapshot)
			buf := make([]byte, size)

			// Serialize
			written := SerializeRespBalanceSnapshot(buf, &tt.snapshot)
			if uint64(written) != size {
				t.Errorf("SerializeRespBalanceSnapshot wrote %d bytes, expected %d", written, size)
			}

			// Deserialize
			result := DeserializeRespBalanceSnapshot(buf)

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
	snapshot := event.RespBalanceSnapshot{
		AccountID: 123,
		Balances: []common.Balance{
			{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
			{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
			{TokenID: 3, Available: 100.0, Locked: 50.0, Total: 150.0},
		},
	}

	size := RespBalanceSnapshotSize(&snapshot)
	buf := make([]byte, size)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		SerializeRespBalanceSnapshot(buf, &snapshot)
	}
}

func BenchmarkDeserializeRespBalanceSnapshot(b *testing.B) {
	snapshot := event.RespBalanceSnapshot{
		AccountID: 123,
		Balances: []common.Balance{
			{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
			{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
			{TokenID: 3, Available: 100.0, Locked: 50.0, Total: 150.0},
		},
	}

	size := RespBalanceSnapshotSize(&snapshot)
	buf := make([]byte, size)
	SerializeRespBalanceSnapshot(buf, &snapshot)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = DeserializeRespBalanceSnapshot(buf)
	}
}

// ============================================================================
// Tests for Order Event Serialization
// ============================================================================

func TestSerializeDeserializeOrderAccepted(t *testing.T) {
	e := event.OrderAccepted{
		ClientOrderID: 12345,
		OrderID:       67890,
		CreatedAt:     1672531200000000000,
	}

	size := OrderAcceptedSize()
	buf := make([]byte, size)

	written := SerializeOrderAccepted(buf, &e)
	if uint64(written) != size {
		t.Errorf("SerializeOrderAccepted wrote %d bytes, expected %d", written, size)
	}

	result := DeserializeOrderAccepted(buf)

	if result.ClientOrderID != e.ClientOrderID {
		t.Errorf("ClientOrderID mismatch: got %d, want %d", result.ClientOrderID, e.ClientOrderID)
	}
	if result.OrderID != e.OrderID {
		t.Errorf("OrderID mismatch: got %d, want %d", result.OrderID, e.OrderID)
	}
	if result.CreatedAt != e.CreatedAt {
		t.Errorf("CreatedAt mismatch: got %d, want %d", result.CreatedAt, e.CreatedAt)
	}
}

func TestSerializeDeserializeOrderPartiallyFilled(t *testing.T) {
	e := event.OrderPartiallyFilled{
		ClientOrderID: 12345,
		OrderID:       67890,
		ExecutedQty:   0.5,
		UpdatedAt:     1672531200000000000,
	}

	size := OrderPartiallyFilledSize()
	buf := make([]byte, size)

	written := SerializeOrderPartiallyFilled(buf, &e)
	if uint64(written) != size {
		t.Errorf("SerializeOrderPartiallyFilled wrote %d bytes, expected %d", written, size)
	}

	result := DeserializeOrderPartiallyFilled(buf)

	if result.ClientOrderID != e.ClientOrderID {
		t.Errorf("ClientOrderID mismatch: got %d, want %d", result.ClientOrderID, e.ClientOrderID)
	}
	if result.OrderID != e.OrderID {
		t.Errorf("OrderID mismatch: got %d, want %d", result.OrderID, e.OrderID)
	}
	if result.ExecutedQty != e.ExecutedQty {
		t.Errorf("ExecutedQty mismatch: got %f, want %f", result.ExecutedQty, e.ExecutedQty)
	}
	if result.UpdatedAt != e.UpdatedAt {
		t.Errorf("UpdatedAt mismatch: got %d, want %d", result.UpdatedAt, e.UpdatedAt)
	}
}

func TestSerializeDeserializeOrderFilled(t *testing.T) {
	e := event.OrderFilled{
		ClientOrderID: 12345,
		OrderID:       67890,
		ExecutedQty:   1.0,
		UpdatedAt:     1672531200000000000,
	}

	size := OrderFilledSize()
	buf := make([]byte, size)

	written := SerializeOrderFilled(buf, &e)
	if uint64(written) != size {
		t.Errorf("SerializeOrderFilled wrote %d bytes, expected %d", written, size)
	}

	result := DeserializeOrderFilled(buf)

	if result.ClientOrderID != e.ClientOrderID {
		t.Errorf("ClientOrderID mismatch: got %d, want %d", result.ClientOrderID, e.ClientOrderID)
	}
	if result.OrderID != e.OrderID {
		t.Errorf("OrderID mismatch: got %d, want %d", result.OrderID, e.OrderID)
	}
	if result.ExecutedQty != e.ExecutedQty {
		t.Errorf("ExecutedQty mismatch: got %f, want %f", result.ExecutedQty, e.ExecutedQty)
	}
	if result.UpdatedAt != e.UpdatedAt {
		t.Errorf("UpdatedAt mismatch: got %d, want %d", result.UpdatedAt, e.UpdatedAt)
	}
}

func TestSerializeDeserializeOrderCanceled(t *testing.T) {
	e := event.OrderCanceled{
		ClientOrderID: 12345,
		OrderID:       67890,
		UpdatedAt:     1672531200000000000,
	}

	size := OrderCanceledSize()
	buf := make([]byte, size)

	written := SerializeOrderCanceled(buf, &e)
	if uint64(written) != size {
		t.Errorf("SerializeOrderCanceled wrote %d bytes, expected %d", written, size)
	}

	result := DeserializeOrderCanceled(buf)

	if result.ClientOrderID != e.ClientOrderID {
		t.Errorf("ClientOrderID mismatch: got %d, want %d", result.ClientOrderID, e.ClientOrderID)
	}
	if result.OrderID != e.OrderID {
		t.Errorf("OrderID mismatch: got %d, want %d", result.OrderID, e.OrderID)
	}
	if result.UpdatedAt != e.UpdatedAt {
		t.Errorf("UpdatedAt mismatch: got %d, want %d", result.UpdatedAt, e.UpdatedAt)
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

// ============================================================================
// Tests for BalanceUpdate Serialization
// ============================================================================

func TestBalanceUpdateSize(t *testing.T) {
	tests := []struct {
		name   string
		update event.BalanceUpdate
	}{
		{
			name: "empty balances",
			update: event.BalanceUpdate{
				AccountID: 1,
				Balances:  nil,
				UpdatedAt: 1672531200000000000,
			},
		},
		{
			name: "single balance",
			update: event.BalanceUpdate{
				AccountID: 1,
				Balances: []common.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
				},
				UpdatedAt: 1672531200000000000,
			},
		},
		{
			name: "multiple balances",
			update: event.BalanceUpdate{
				AccountID: 123,
				Balances: []common.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
					{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
					{TokenID: 3, Available: 100.0, Locked: 50.0, Total: 150.0},
				},
				UpdatedAt: 1672531200000000000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := BalanceUpdateSize(&tt.update)
			expectedSize := uint64(SizeOfBalanceUpdateHeader) + uint64(len(tt.update.Balances))*uint64(SizeOfBalance)
			if size != expectedSize {
				t.Errorf("BalanceUpdateSize() = %d, want %d", size, expectedSize)
			}
		})
	}
}

func TestSerializeDeserializeBalanceUpdate(t *testing.T) {
	tests := []struct {
		name   string
		update event.BalanceUpdate
	}{
		{
			name: "empty balances",
			update: event.BalanceUpdate{
				AccountID: 1,
				Balances:  []common.Balance{},
				UpdatedAt: 1672531200000000000,
			},
		},
		{
			name: "single balance",
			update: event.BalanceUpdate{
				AccountID: 42,
				Balances: []common.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
				},
				UpdatedAt: 1672531200000000000,
			},
		},
		{
			name: "multiple balances",
			update: event.BalanceUpdate{
				AccountID: 123,
				Balances: []common.Balance{
					{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
					{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
					{TokenID: 3, Available: 100.12345678, Locked: 50.87654321, Total: 150.99999999},
				},
				UpdatedAt: 1672531200000000000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate size and allocate buffer
			size := BalanceUpdateSize(&tt.update)
			buf := make([]byte, size)

			// Serialize
			written := SerializeBalanceUpdate(buf, &tt.update)
			if uint64(written) != size {
				t.Errorf("SerializeBalanceUpdate wrote %d bytes, expected %d", written, size)
			}

			// Deserialize
			result := DeserializeBalanceUpdate(buf)

			// Verify AccountID
			if result.AccountID != tt.update.AccountID {
				t.Errorf("AccountID mismatch: got %d, want %d", result.AccountID, tt.update.AccountID)
			}

			// Verify UpdatedAt
			if result.UpdatedAt != tt.update.UpdatedAt {
				t.Errorf("UpdatedAt mismatch: got %d, want %d", result.UpdatedAt, tt.update.UpdatedAt)
			}

			// Verify balances count
			if len(result.Balances) != len(tt.update.Balances) {
				t.Fatalf("Balances count mismatch: got %d, want %d", len(result.Balances), len(tt.update.Balances))
			}

			// Verify each balance
			for i := range tt.update.Balances {
				expected := tt.update.Balances[i]
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

func BenchmarkSerializeBalanceUpdate(b *testing.B) {
	update := event.BalanceUpdate{
		AccountID: 123,
		Balances: []common.Balance{
			{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
			{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
			{TokenID: 3, Available: 100.0, Locked: 50.0, Total: 150.0},
		},
		UpdatedAt: 1672531200000000000,
	}

	size := BalanceUpdateSize(&update)
	buf := make([]byte, size)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		SerializeBalanceUpdate(buf, &update)
	}
}

func BenchmarkDeserializeBalanceUpdate(b *testing.B) {
	update := event.BalanceUpdate{
		AccountID: 123,
		Balances: []common.Balance{
			{TokenID: 1, Available: 1.5, Locked: 0.5, Total: 2.0},
			{TokenID: 2, Available: 10.0, Locked: 0.0, Total: 10.0},
			{TokenID: 3, Available: 100.0, Locked: 50.0, Total: 150.0},
		},
		UpdatedAt: 1672531200000000000,
	}

	size := BalanceUpdateSize(&update)
	buf := make([]byte, size)
	SerializeBalanceUpdate(buf, &update)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = DeserializeBalanceUpdate(buf)
	}
}
