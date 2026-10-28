package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/observability"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRiskHTTPActualResponseSchemas(t *testing.T) {
	f := newRiskHTTP(t)
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "..", "packages", "shared-contracts", "openapi", "aegis-api.yaml"))
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))
	preview := f.call(t, "POST", "/risk-assessment/preview", "network_operator", "{}", 200)
	require.NoError(t, doc.Components.Schemas["RiskPreviewEnvelope"].Value.VisitJSON(preview))
	root := f.call(t, "POST", "/risk-assessment", "system_administrator", "{}", 201)
	require.NoError(t, doc.Components.Schemas["RiskRecordEnvelope"].Value.VisitJSON(root))
	id := nestedString(t, root, "data", "assessment", "id")
	for _, tc := range []struct{ suffix, schema string }{
		{"/risk-assessment", "RiskAssessmentEnvelope"}, {"/risk-assessments/" + id, "RiskAssessmentEnvelope"},
		{"/risk-assessment/findings", "RiskFindingsEnvelope"}, {"/risk-assessments/" + id + "/findings", "RiskFindingsEnvelope"}, {"/risk-assessments", "RiskHistoryEnvelope"},
	} {
		body := f.call(t, "GET", tc.suffix, "network_operator", "", 200)
		require.NoError(t, doc.Components.Schemas[tc.schema].Value.VisitJSON(body), tc.suffix)
	}
	for _, role := range []string{"", "viewer"} {
		want := 401
		if role == "viewer" {
			want = 403
		}
		body := f.call(t, "GET", "/risk-assessment", role, "", want)
		require.NoError(t, doc.Components.Schemas["Error"].Value.VisitJSON(body))
	}
}

func TestRiskHTTPScopedMediaTypeContract(t *testing.T) {
	f := newRiskHTTP(t)
	const origin = "https://risk.example"
	router := Routes(f.h, &observability.Metrics{}, RouterConfig{AllowedOrigins: []string{origin}, RequestTimeout: 15 * time.Second})
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "..", "packages", "shared-contracts", "openapi", "aegis-api.yaml"))
	require.NoError(t, err)
	before := f.evidence(t)
	for i, tc := range []struct {
		method, path string
		json         bool
	}{
		{"POST", f.base() + "/risk-assessment", true},
		{"POST", f.base() + "/risk-assessment/preview", true},
		{"POST", "/api/v1/auth/login", false},
		{"POST", f.base() + "/policy-evaluation", false},
		{"POST", f.base() + "/risk-assessments", false},
		{"POST", f.base() + "/risk-assessment-extra", false},
		{"POST", f.base() + "/risk-assessment/preview/extra", false},
		{"POST", f.base() + "/risk-assessment/", false},
		{"POST", f.base() + "/risk-assessment/findings", false},
		{"POST", f.base() + "/%72isk-assessment", false},
		{"POST", "/api/v2/devices/" + f.device + "/change-events/" + f.eventID + "/risk-assessment", false},
		{"POST", "/api/v1/devices//change-events/" + f.eventID + "/risk-assessment", false},
		{"POST", "/risk-assessment", false},
		{"PUT", f.base() + "/risk-assessment", false},
	} {
		t.Run(fmt.Sprintf("exact_scope_%02d", i), func(t *testing.T) {
			id := uuid.NewString()
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("RBAC_HTTP_SECRET"))
			r.Header.Set("Origin", origin)
			r.Header.Set("Content-Type", "text/plain; canary=RBAC_HTTP_SECRET")
			r.Header.Set("X-Request-ID", id)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			require.Equal(t, 415, w.Code)
			require.Equal(t, origin, w.Header().Get("Access-Control-Allow-Origin"))
			require.Equal(t, "Origin", w.Header().Get("Vary"))
			require.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
			require.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
			require.Equal(t, "600", w.Header().Get("Access-Control-Max-Age"))
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
			require.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
			require.Equal(t, "no-referrer", w.Header().Get("Referrer-Policy"))
			require.Contains(t, w.Header().Get("Content-Security-Policy"), "default-src 'none'")
			require.Equal(t, id, w.Header().Get("X-Request-ID"))
			require.NotContains(t, w.Body.String(), "RBAC_HTTP_SECRET")
			if tc.json {
				require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
				var body map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				require.Equal(t, "UNSUPPORTED_MEDIA_TYPE", nestedString(t, body, "error", "code"))
				require.Equal(t, "Unsupported request content type.", nestedString(t, body, "error", "message"))
				require.Equal(t, id, nestedString(t, body, "error", "request_id"))
				require.NoError(t, doc.Components.Schemas["Error"].Value.VisitJSON(body))
				path := "/api/v1/devices/{deviceID}/change-events/{eventID}/risk-assessment"
				if tc.path == f.base()+"/risk-assessment/preview" {
					path += "/preview"
				}
				schema := doc.Paths.Value(path).Post.Responses.Value("415").Value.Content["application/json"].Schema.Value
				require.NoError(t, schema.VisitJSON(body), "actual 415 matches the declared operation response")
			} else {
				require.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
				require.Equal(t, "unsupported content type\n", w.Body.String(), "legacy and lookalike routes keep the exact original response")
			}
		})
	}
	for _, tc := range []struct {
		origin string
		want   int
	}{
		{origin, 204}, {"https://untrusted.example", 403},
	} {
		r := httptest.NewRequest("OPTIONS", f.base()+"/risk-assessment", http.NoBody)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		require.Equal(t, tc.want, w.Code)
		if tc.want == 204 {
			require.Equal(t, origin, w.Header().Get("Access-Control-Allow-Origin"))
			require.Empty(t, w.Body.String())
		} else {
			require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
			require.Equal(t, "origin not allowed\n", w.Body.String())
		}
	}
	for _, media := range []string{"application/json", "application/xml"} {
		w := f.serve(context.Background(), "POST", f.base()+"/risk-assessment", "system_administrator", `{"unexpected":true}`, media)
		require.Equal(t, 400, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, "VALIDATION_FAILED", nestedString(t, body, "error", "code"))
		require.NoError(t, doc.Components.Schemas["Error"].Value.VisitJSON(body))
	}
	f.call(t, "POST", "/risk-assessment", "", "{}", 401)
	require.Equal(t, before, f.evidence(t), "global content rejection never reaches risk persistence")
}

func TestRiskHTTPDisabledAndDeviceGroupScope(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "wrong group", true: "disabled actor"}[disabled], func(t *testing.T) {
			f := newRiskHTTP(t)
			root := f.call(t, "POST", "/risk-assessment", "network_administrator", "{}", 201)
			id := nestedString(t, root, "data", "assessment", "id")
			var err error
			if disabled {
				_, err = f.db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, f.users["network_administrator"])
			} else {
				_, err = f.db.Exec(`INSERT INTO user_device_groups(user_id,device_group) VALUES(?,'elsewhere')`, f.users["network_administrator"])
			}
			require.NoError(t, err)
			before := f.evidence(t)
			for _, tc := range []struct{ method, suffix string }{
				{"POST", "/risk-assessment/preview"}, {"POST", "/risk-assessment"}, {"GET", "/risk-assessment"}, {"GET", "/risk-assessment/findings"}, {"GET", "/risk-assessments"}, {"GET", "/risk-assessments/" + id}, {"GET", "/risk-assessments/" + id + "/findings"},
			} {
				f.call(t, tc.method, tc.suffix, "network_administrator", "", 403)
			}
			require.Equal(t, before, f.evidence(t))
			f.call(t, "GET", "/risk-assessment", "system_administrator", "", 200)
		})
	}
}
