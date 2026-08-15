package ordersms

import (
	"encoding/json"
	"testing"
)

func TestCapabilityRequestFields(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{name: "signature create", in: SignatureCreate{Name: "store"}, want: `{"name":"store"}`},
		{name: "template create", in: TemplateCreate{Name: "shipped", Body: "Order shipped"}, want: `{"name":"shipped","body":"Order shipped"}`},
		{name: "send", in: SendRequest{To: "+15551234567", TemplateID: "tpl-1", TemplateVars: map[string]string{"order_id": "A-42"}}, want: `{"to":"+15551234567","template_id":"tpl-1","template_vars":{"order_id":"A-42"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("Marshal() = %s, want %s", got, tt.want)
			}
		})
	}
}
