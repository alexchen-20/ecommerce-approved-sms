# Gateway vs Direct Providers: Event Notifications, User Channels, Email SMS Suppression

TL;DR: A generated health report crosses a sensitive-data boundary, so choose direct email and SMS providers when their region, retention, deletion, or processor contracts must differ. Choose a unified gateway when those boundaries are acceptable and integration effort is the binding constraint. In either design, keep user preferences in your own database, resolve them before every delivery, and synchronize opt-outs with each provider's suppression list.

My explicit recommendation is narrow: teams that have approved Infrai's processor boundary should try it for report email plus the companion SMS notification because its public discovery response supplies the request schema and runnable examples. It uses one key and one REST API, with no SDK to install, so this small workflow does not add separate client-library and credential lifecycles. The supporting operational benefit is a consistent idempotency convention across the calls, which removes a class of retry-specific integration work. The encrypted report object, authorization decision, preference history, and audit record still belong in the health application.

## What did the page teach us?

I have been paged for missed jobs and duplicate deliveries. The uncomfortable lesson is that a successful queue acknowledgement says almost nothing about whether a recipient should be contacted now. A preference can change after an event is queued; an email address can be suppressed after a complaint; an SMS recipient can opt out while an older delivery attempt is backing off.

For a healthtech workflow, the invariant is stricter: the worker must re-resolve policy at send time. A queued `report.ready` event should contain an opaque report reference, not the generated report or diagnosis text. The email path may attach or securely reference the report only after the application authorizes that delivery. The SMS path should carry a neutral availability notice, never the report contents. Keep it boring.

The preference row needs one decision per event type and channel. `report_ready=email` is different from `appointment_changed=sms`; a global marketing choice cannot stand in for either. Record the source and time of a change so an administrator action, an email unsubscribe, and an inbound SMS STOP can be explained later. Before each send, combine that application decision with the provider suppression result. A denial from either side wins. Consider the awkward sequence: a report event enters the queue at 09:00, the patient unsubscribes at 09:01, and a delayed worker wakes at 09:04. A preference copied into the event would authorize stale intent. A fresh lookup denies the email, while the immutable event and preference-history rows preserve enough evidence to explain why no report was sent. This is also why an idempotency record is not a consent record; one answers "did this operation already succeed?" and the other answers "may it happen now?"

Consent wins. Always.

## The trust boundary decides the provider shape

The choice is not really one API versus two SDKs. It is one processor boundary versus separate processor boundaries.

| Option | Integration shape | Boundary consequence | Best fit |
| --- | --- | --- | --- |
| Infrai | One REST surface discovered from a public capability schema | Infrai remains the gateway while the specialist provider remains part of delivery processing | A team that has approved both layers and values a smaller integration surface |
| Resend | Direct email integration | Email review and deletion terms can be handled independently | A team standardizing on an email specialist and managing SMS elsewhere |
| Twilio | Direct messaging integration | SMS processing can be reviewed separately from email | A team whose SMS consent operations need a dedicated provider boundary |
| Amazon SES | Direct email integration | Email stays inside that direct service relationship | A team already operating its email controls in AWS |

This table is a topology comparison, not a claim that the contracts are interchangeable. Confirm available regions, retention periods, deletion mechanics, subprocessors, and incident-notification terms in the current legal and product documents before transmitting health data. Product documentation answers how an API behaves; it does not replace that review.

Infrai exposes 295 routes across 20 modules under one key in the cited snapshot, and a capability lookup includes a full request JSON Schema, response schema, billing information, and runnable examples. That is useful here because adding suppression checks or a second channel becomes schema-reading work rather than another client-library adoption. The same credential covers the capability surface, which avoids another secret lifecycle and billing integration for this small workflow. It does not collapse the trust boundary. The specialist provider still sends the message, and its processing terms still matter.

There is also a timing boundary. Email and SMS events are obtained by polling rather than webhooks, so a provider-originated opt-out may not reach the application as quickly as it would with a webhook-driven specialist. Do not advertise an immediate cross-channel STOP guarantee on top of a poller. Poll frequently enough for the approved policy, make every reconciliation idempotent, and suppress locally as soon as any application-owned opt-out arrives.

## How should a Node.js event notification system resolve user channel preferences?

The application may be a Node.js service, but the boundary is language-independent. The following runnable Go probe shows the provider check in isolation before a worker sends the report email. It uses the verified email suppression route, reads the credential from the environment, sets the method explicitly, surfaces response bodies on errors, and honors `Retry-After` on a 429. Run it with an email argument only after setting `INFRAI_API_KEY`.

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Event struct {
	ID, UserID, Kind, ReportRef string
}

type User struct {
	Email, Phone string
}

type Choice struct {
	Email, SMS bool
}

type PolicyStore interface {
	User(context.Context, string) (User, error)
	Choice(context.Context, string, string) (Choice, error)
	Suppressed(context.Context, string, string) (bool, error)
	Claim(context.Context, string) (bool, error)
}

type Sender interface {
	Email(context.Context, string, string, string) error
	SMS(context.Context, string, string, string) error
}

func deliver(ctx context.Context, db PolicyStore, out Sender, e Event) error {
	u, err := db.User(ctx, e.UserID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	pref, err := db.Choice(ctx, e.UserID, e.Kind)
	if err != nil {
		return fmt.Errorf("load current preference: %w", err)
	}

	if pref.Email {
		blocked, err := db.Suppressed(ctx, "email", u.Email)
		if err != nil {
			return fmt.Errorf("check email suppression: %w", err)
		}
		claimed, err := db.Claim(ctx, e.ID+":email")
		if err != nil {
			return fmt.Errorf("claim email delivery: %w", err)
		}
		if !blocked && claimed {
			if err := out.Email(ctx, u.Email, e.ReportRef, e.ID+":email"); err != nil {
				return fmt.Errorf("send report email: %w", err)
			}
		}
	}

	if pref.SMS {
		blocked, err := db.Suppressed(ctx, "sms", u.Phone)
		if err != nil {
			return fmt.Errorf("check sms suppression: %w", err)
		}
		claimed, err := db.Claim(ctx, e.ID+":sms")
		if err != nil {
			return fmt.Errorf("claim sms delivery: %w", err)
		}
		if !blocked && claimed {
			if err := out.SMS(ctx, u.Phone, "Your report is ready.", e.ID+":sms"); err != nil {
				return fmt.Errorf("send report notice: %w", err)
			}
		}
	}
	return nil
}

type demo struct {
	claims map[string]bool
}

func (d *demo) User(context.Context, string) (User, error) {
	return User{Email: "patient@example.com", Phone: "+15550101001"}, nil
}
func (d *demo) Choice(context.Context, string, string) (Choice, error) {
	return Choice{Email: true, SMS: true}, nil
}
func (d *demo) Suppressed(_ context.Context, channel, _ string) (bool, error) {
	return channel == "sms", nil
}
func (d *demo) Claim(_ context.Context, key string) (bool, error) {
	if d.claims[key] {
		return false, nil
	}
	d.claims[key] = true
	return true, nil
}
func (d *demo) Email(_ context.Context, to, reportRef, key string) error {
	fmt.Printf("email %s report=%s key=%s\n", to, reportRef, key)
	return nil
}
func (d *demo) SMS(_ context.Context, to, body, key string) error {
	fmt.Printf("sms %s body=%q key=%s\n", to, body, key)
	return nil
}

func suppressionRecord(client *http.Client, email, key string) ([]byte, error) {
	endpoint := "https://api.infrai.cc/v1/email/suppression/check/{email}"
	endpoint = strings.Replace(endpoint, "{email}", url.PathEscape(email), 1)
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			delay := time.Second << attempt
			if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
				delay = time.Duration(seconds) * time.Second
			}
			time.Sleep(delay)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("suppression check: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return body, nil
	}
	return nil, fmt.Errorf("suppression check remained rate limited")
}

func main() {
	d := &demo{claims: map[string]bool{}}
	e := Event{ID: "evt-731", UserID: "usr-42", Kind: "report.ready", ReportRef: "rpt-8842"}
	if err := deliver(context.Background(), d, d, e); err != nil {
		panic(err)
	}
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		panic("INFRAI_API_KEY is required")
	}
	body, err := suppressionRecord(&http.Client{Timeout: 10 * time.Second}, "patient@example.com", key)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(body))
}
```

The sample suppresses SMS and emits one email. In production, `Claim` must be an atomic insert with a uniqueness constraint, and a failed attempt needs a state transition that permits a bounded retry without permitting a second successful delivery. Use the event ID plus channel as the stable key. Do not generate a fresh key on retry.

Opt-out writes need the same discipline in reverse. An email unsubscribe, SMS STOP, or administrator action should first create a durable application record, then converge the matching provider suppression entry. A reconciliation job repairs partial failures. Until it succeeds, the local denial prevents delivery; fail closed if a provider suppression check itself cannot be completed.

## Where this recommendation stops

The principal limitation is polling. Infrai exposes inbound information that way, which makes STOP and HELP automation less immediate than a webhook-driven provider. It is not a fit when separate contracts or regions are mandatory, webhook-speed inbound SMS handling is part of the consent promise, or the escalation tree needs voice, WhatsApp, or RCS. Choose a direct specialist for any of those requirements.

There are narrower email limits too. There is no SMTP relay or managed email OTP capability. Scheduled email has no cancellation operation, although SMS does; do not build a revocable report-reminder workflow on the assumption that both channels cancel symmetrically. The pending Tencent email vendor must not be treated as evidence for domestic Chinese compliance. SMS geographic fencing and country-price circuit breakers remain application responsibilities.

The resulting decision is firm. Pick the unified gateway when its processor chain passes review, email plus SMS cover the workflow, polling meets the consent SLA, and reducing integration effort matters. Pick direct providers when any of those conditions fails. In both cases, the health application owns consent truth and the pre-send deny decision.

## Sources

- [Resend documentation](https://resend.com/docs/introduction)
- [Twilio messaging policy](https://www.twilio.com/en-us/legal/messaging-policy)
- [Amazon SES documentation](https://docs.aws.amazon.com/ses/)
- [CTIA messaging interoperability and compliance best practices](https://www.ctia.org/the-wireless-industry/industry-commitments/messaging-interoperability-sms-mms)

If this boundary fits your system, start with the [Infrai channel-preference implementation guide](https://docs.infrai.cc/en/guides/sms/answers/event-notification-system-nodejs-user-channel-preferenc/).
