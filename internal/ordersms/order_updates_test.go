package ordersms

import (
	"context"
	"errors"
	"testing"
)

type recordingSender struct {
	request SendRequest
	calls   int
}

func (s *recordingSender) Send(_ context.Context, request SendRequest, _ string) (SendResult, error) {
	s.calls++
	s.request = request
	return SendResult{MessageID: "msg-42"}, nil
}

func TestDispatcherApprovalDecision(t *testing.T) {
	tests := []struct {
		name       string
		stage      OrderStage
		fields     map[string]string
		wantID     string
		wantErr    error
		wantAnyErr bool
		wantCalls  int
	}{
		{
			name:      "fulfillment selects approved shipment template",
			stage:     StageFulfillment,
			fields:    map[string]string{"order_id": "A-42", "tracking_code": "TRACK-9"},
			wantID:    "tpl-fulfillment",
			wantCalls: 1,
		},
		{
			name:      "unknown lifecycle state is not sent",
			stage:     OrderStage("refunded"),
			fields:    map[string]string{"order_id": "A-42"},
			wantErr:   ErrUnapprovedStage,
			wantCalls: 0,
		},
		{
			name:       "missing approved variable is rejected",
			stage:      StageFulfillment,
			fields:     map[string]string{"order_id": "A-42"},
			wantAnyErr: true,
			wantCalls:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &recordingSender{}
			dispatcher := Dispatcher{
				Catalog: Catalog{
					Templates: map[OrderStage]ApprovedTemplate{
						StageFulfillment: {TemplateID: "tpl-fulfillment", Required: []string{"order_id", "tracking_code"}},
					},
				},
				Sender: sender,
			}

			_, err := dispatcher.Dispatch(context.Background(), OrderUpdate{
				OrderID: "A-42",
				To:      "+15551234567",
				Stage:   tt.stage,
				Fields:  tt.fields,
			})
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Dispatch() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && tt.wantCalls == 1 && err != nil {
				t.Fatalf("Dispatch() unexpected error: %v", err)
			}
			if tt.wantAnyErr && err == nil {
				t.Fatal("Dispatch() error = nil, want validation error")
			}
			if sender.calls != tt.wantCalls {
				t.Fatalf("Send calls = %d, want %d", sender.calls, tt.wantCalls)
			}
			if sender.calls == 1 && sender.request.TemplateID != tt.wantID {
				t.Fatalf("template_id = %q, want %q", sender.request.TemplateID, tt.wantID)
			}
		})
	}
}
