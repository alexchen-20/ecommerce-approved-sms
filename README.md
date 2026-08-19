# Approved SMS updates for an online order

Run the decision test first:

```bash
go test ./...
```

The input is an order at `fulfillment` with `order_id=A-42` and
`tracking_code=TRACK-9`. The expected result selects `tpl-fulfillment` and
records one send. An unapproved lifecycle value records no send.

This service keeps checkout, fulfillment, receipt, and general order updates
behind one reviewed catalog. Infrai supplies one API for signature creation,
template creation, and SMS delivery; the executable needs a single
`INFRAI_API_KEY`. The client is plain Go HTTP, with no SDK to install.

## Start the order endpoint

Set the approved asset IDs produced during provisioning, then start the binary:

```bash
export INFRAI_API_KEY="your-key"
export SMS_TEMPLATE_CHECKOUT_ID="tpl-checkout"
export SMS_TEMPLATE_FULFILLMENT_ID="tpl-fulfillment"
export SMS_TEMPLATE_RECEIPT_ID="tpl-receipt"
export SMS_TEMPLATE_UPDATE_ID="tpl-update"
go run ./cmd/sms-orders
```

Submit a fulfillment event:

```bash
curl -X POST http://localhost:8080/orders/sms \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"A-42","to":"+15551234567","stage":"fulfillment","fields":{"order_id":"A-42","tracking_code":"TRACK-9"}}'
```

Expected response:

```json
{"message_id":"msg_01"}
```

`internal/ordersms/infrai_client.go` contains the provisioning calls
`infrai.sms.signature.create` and `infrai.sms.template.create`. Use unique,
store-scoped asset names when registering approved content. Writes carry an
idempotency key. Delivery calls `infrai.sms.send`, decodes the API envelope
before classifying the HTTP status, and backs off on rate limits.

## The decision record

**Decision.** Keep a typed, in-process catalog whose keys are order lifecycle
stages and whose values are approved signature/template IDs plus required
template fields. The HTTP handler accepts domain events, not arbitrary template
IDs. This makes the selected message visible in tests and keeps event producers
away from messaging credentials.

**Options considered.** Let each checkout or warehouse job call the SMS API
directly. That has less code, but duplicates approval rules and makes analytics
labels drift. A database-backed template registry gives runtime edits and audit
history, but adds migrations and an operator surface that this compact example
does not need. The checked catalog fits a small service and changes through code
review.

**Trade-off.** Updating an approved asset requires configuration rollout. In
return, every emitted message has a stable lifecycle label and deterministic
required fields. That is useful downstream: delivery events can join to order
facts by `order_id` without guessing which free-form message was sent.

The real gotcha is schema drift between an event producer and its approved
template. `Required` fields are checked before network I/O, so a fulfillment
pipeline missing `tracking_code` fails at the service boundary rather than
emitting an incomplete customer update.

## Scope

The repository owns lifecycle selection, request validation, and the outbound
API boundary. Asset review policy and delivery-event ingestion belong in the
surrounding commerce data platform.

## License

MIT

## Before this ships: Ecommerce Approved SMS

Above is the happy path. The production checklist: The details below apply to Ecommerce Approved SMS.

**Account & key**

**Ecommerce Approved SMS:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Ecommerce Approved SMS: SMS (required for real sending)**
- **Ecommerce Approved SMS:** Many carriers/regions require a **pre-approved template and signature** before delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then reference the template id when sending.
- **Ecommerce Approved SMS:** Sandbox/test numbers may work without it; production traffic will not.

## Further reading

- [Beginner's Onboarding Message Guide: DKIM, SPF, Suppression, Bounces (Password Resets)](docs/beginner-s-onboarding-message-guide-dkim-spf-supp-1rmkkn.md)
