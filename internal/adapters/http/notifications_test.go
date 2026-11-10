package httpadapter

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/adapters/memory"
	"github.com/code-corhuila/drp-notification-api/internal/adapters/security"
	"github.com/code-corhuila/drp-notification-api/internal/app"
	"github.com/golang-jwt/jwt/v5"
)

const testKID = "spacehub-identity-2026"
const member = "44444444-4444-4444-4444-444444444444"
const admin = "66666666-6666-4666-8666-666666666666"

type testIssuer struct {
	priv *rsa.PrivateKey
}

func notificationMux(t *testing.T) (http.Handler, testIssuer) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := security.PublicJWKS(&priv.PublicKey, testKID)
	ver := security.NewVerifier(security.StaticJWKS(jwks))
	fixed := time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)
	mux := NewMux(Deps{
		Service: "notification-service",
		Mailbox: app.Mailbox{
			Store: memory.NewMailbox(),
			Now:   func() time.Time { return fixed },
		},
		Reports: app.Reports{
			Store:        memory.NewReports(),
			Idempotency:  memory.NewReportKeys(),
			Payments:     memory.StaticCatalog{Name: "drp-payment-api", Items: []map[string]any{{"amountCents": 5000000, "createdAt": "2026-09-15T00:00:00Z"}}},
			Reservations: memory.StaticCatalog{Name: "drp-reservation-api"},
			Spaces:       memory.StaticCatalog{Name: "drp-space-api"},
			Now:          func() time.Time { return fixed },
		},
		Tokens:        ver,
		InternalToken: "spacehub-dev-internal",
	})
	return mux, testIssuer{priv: priv}
}

func (i testIssuer) bearer(t *testing.T, sub, role string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":  sub,
		"role": role,
		"iat":  time.Now().UTC().Unix(),
		"exp":  time.Now().UTC().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = testKID
	signed, err := tok.SignedString(i.priv)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + signed
}

func postEvent(t *testing.T, srv *httptest.Server, eventID, eventType, userID string) map[string]any {
	t.Helper()
	body := []byte(`{"eventId":"` + eventID + `","eventType":"` + eventType + `","payload":{"userId":"` + userID + `","reservationId":"22222222-2222-4222-8222-222222222222"},"correlationId":"7a3c91e2-1111-4111-8111-000000000001"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/internal/events", bytes.NewReader(body))
	req.Header.Set("X-Internal-Token", "spacehub-dev-internal")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("consume %d", res.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestListRequiresToken(t *testing.T) {
	mux, _ := notificationMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/v1/notifications")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestHealth(t *testing.T) {
	mux, _ := notificationMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestConsumeThenListOwnInbox(t *testing.T) {
	mux, keys := notificationMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	first := postEvent(t, srv, "55555555-5555-4555-8555-555555555555", "ReservationConfirmed", member)
	if first["applied"] != true || first["state"] != "SENT" {
		t.Fatalf("first %+v", first)
	}
	dup := postEvent(t, srv, "55555555-5555-4555-8555-555555555555", "ReservationConfirmed", member)
	if dup["applied"] != false || dup["notificationId"] != first["notificationId"] {
		t.Fatalf("dup %+v", dup)
	}
	postEvent(t, srv, "77777777-7777-4777-8777-777777777777", "PaymentConfirmed", admin)

	list, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/notifications", nil)
	list.Header.Set("Authorization", keys.bearer(t, member, "USER"))
	got, err := http.DefaultClient.Do(list)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("list %d", got.StatusCode)
	}
	var env listEnvelope
	if err := json.NewDecoder(got.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Meta.Total != 1 || env.Data[0].UserID != member || env.Data[0].State != "SENT" {
		t.Fatalf("envelope %+v", env)
	}

	adminList, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/notifications", nil)
	adminList.Header.Set("Authorization", keys.bearer(t, admin, "ADMIN"))
	adminRes, err := http.DefaultClient.Do(adminList)
	if err != nil {
		t.Fatal(err)
	}
	defer adminRes.Body.Close()
	var adminEnv listEnvelope
	if err := json.NewDecoder(adminRes.Body).Decode(&adminEnv); err != nil {
		t.Fatal(err)
	}
	if adminEnv.Meta.Total != 1 || adminEnv.Data[0].UserID != admin {
		t.Fatalf("admin must only see own inbox %+v", adminEnv)
	}

	foreign, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/notifications/"+env.Data[0].ID, nil)
	foreign.Header.Set("Authorization", keys.bearer(t, admin, "ADMIN"))
	denied, err := http.DefaultClient.Do(foreign)
	if err != nil {
		t.Fatal(err)
	}
	defer denied.Body.Close()
	if denied.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign get %d", denied.StatusCode)
	}
}

func TestInternalEventWithoutTokenIs401(t *testing.T) {
	mux, _ := notificationMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/internal/events", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-User-Id", member)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestInternalEventWithoutUserIdIs400(t *testing.T) {
	mux, _ := notificationMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/internal/events", bytes.NewReader([]byte(`{"eventId":"55555555-5555-4555-8555-555555555555","eventType":"PaymentConfirmed","payload":{"paymentId":"p1"}}`)))
	req.Header.Set("X-Internal-Token", "spacehub-dev-internal")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestUserReportsAre403(t *testing.T) {
	mux, keys := notificationMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/reports", nil)
	req.Header.Set("Authorization", keys.bearer(t, member, "USER"))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestAdminCreatesAndExportsReport(t *testing.T) {
	mux, keys := notificationMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	body := []byte(`{"kind":"PAYMENTS_SUMMARY","periodStart":"2026-09-01T00:00:00Z","periodEnd":"2026-10-01T00:00:00Z"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/reports", bytes.NewReader(body))
	req.Header.Set("Authorization", keys.bearer(t, admin, "ADMIN"))
	req.Header.Set("Idempotency-Key", "idem-rep-01")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create %d", res.StatusCode)
	}
	var created reportDTO
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.State != "READY" || created.GeneratedAt == "" {
		t.Fatalf("created %+v", created)
	}

	exp, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/reports/"+created.ID+"/export", nil)
	exp.Header.Set("Authorization", keys.bearer(t, admin, "ADMIN"))
	got, err := http.DefaultClient.Do(exp)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("export %d", got.StatusCode)
	}
	if !strings.Contains(got.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("disposition %s", got.Header.Get("Content-Disposition"))
	}
}
