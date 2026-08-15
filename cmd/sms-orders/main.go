package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/example/ecommerce-approved-sms/internal/ordersms"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	catalog := ordersms.Catalog{
		Templates: map[ordersms.OrderStage]ordersms.ApprovedTemplate{
			ordersms.StageCheckout:    {TemplateID: mustEnv("SMS_TEMPLATE_CHECKOUT_ID"), Required: []string{"order_id", "payment_deadline"}},
			ordersms.StageFulfillment: {TemplateID: mustEnv("SMS_TEMPLATE_FULFILLMENT_ID"), Required: []string{"order_id", "tracking_code"}},
			ordersms.StageReceipt:     {TemplateID: mustEnv("SMS_TEMPLATE_RECEIPT_ID"), Required: []string{"order_id", "amount"}},
			ordersms.StageUpdate:      {TemplateID: mustEnv("SMS_TEMPLATE_UPDATE_ID"), Required: []string{"order_id", "status"}},
		},
	}
	dispatcher := ordersms.Dispatcher{Catalog: catalog, Sender: ordersms.NewClient(apiKey)}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders/sms", func(w http.ResponseWriter, r *http.Request) {
		var update ordersms.OrderUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err := dispatcher.Dispatch(r.Context(), update)
		if err != nil {
			status := http.StatusUnprocessableEntity
			var apiErr *ordersms.APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
				status = apiErr.StatusCode
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, result)
	})

	addr := ":8080"
	log.Printf("order SMS service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func mustEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
