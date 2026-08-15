package ordersms

import (
	"context"
	"errors"
	"fmt"
)

type OrderStage string

const (
	StageCheckout    OrderStage = "checkout"
	StageFulfillment OrderStage = "fulfillment"
	StageReceipt     OrderStage = "receipt"
	StageUpdate      OrderStage = "order_update"
)

var ErrUnapprovedStage = errors.New("order stage is not approved for SMS")

type ApprovedTemplate struct {
	TemplateID string
	Required   []string
}

type Catalog struct {
	Templates map[OrderStage]ApprovedTemplate
}

type OrderUpdate struct {
	OrderID string            `json:"order_id"`
	To      string            `json:"to"`
	Stage   OrderStage        `json:"stage"`
	Fields  map[string]string `json:"fields"`
}

type Sender interface {
	Send(context.Context, SendRequest, string) (SendResult, error)
}

type Dispatcher struct {
	Catalog Catalog
	Sender  Sender
}

func (d Dispatcher) Dispatch(ctx context.Context, update OrderUpdate) (SendResult, error) {
	template, approved := d.Catalog.Templates[update.Stage]
	if !approved {
		return SendResult{}, ErrUnapprovedStage
	}
	if update.OrderID == "" || update.To == "" {
		return SendResult{}, errors.New("order_id and to are required")
	}
	for _, name := range template.Required {
		if update.Fields[name] == "" {
			return SendResult{}, fmt.Errorf("template field %q is required", name)
		}
	}

	return d.Sender.Send(ctx, SendRequest{
		To:           update.To,
		TemplateID:   template.TemplateID,
		TemplateVars: update.Fields,
	}, "order-"+update.OrderID+"-"+string(update.Stage))
}
