package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/segmentio/kafka-go"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakePublisher records every message it's asked to write, or returns a
// canned error, without touching a real Kafka broker.
type fakePublisher struct {
	messages []kafka.Message
	err      error
}

func (f *fakePublisher) WriteMessages(_ context.Context, msgs ...kafka.Message) error {
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, msgs...)
	return nil
}

func proxyStub(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("proxy was called for method %s; POST /api/jobs must not fall through to it", r.Method)
		w.WriteHeader(http.StatusInternalServerError)
	})
}

func postJob(t *testing.T, handler http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestHandleCreateJobPublishesVideoRequest(t *testing.T) {
	pub := &fakePublisher{}
	handler := handleCreateJob(pub, proxyStub(t), testLogger())

	rec := postJob(t, handler, `{"url":"https://example.com/v","title":"T","duration":42,"kind":"video","height":1080,"with_audio":true}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if len(pub.messages) != 1 {
		t.Fatalf("published %d messages, want 1", len(pub.messages))
	}

	var got jobRequest
	if err := json.Unmarshal(pub.messages[0].Value, &got); err != nil {
		t.Fatalf("published message is not valid jobRequest JSON: %v", err)
	}
	if got.URL != "https://example.com/v" || got.Height != 1080 || !got.WithAudio || got.Duration != 42 {
		t.Errorf("published request = %+v, unexpected fields", got)
	}
	if got.FileID == "" {
		t.Error("published request has empty FileID; handler should always generate one server-side")
	}

	var respBody map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if respBody["file_id"] != got.FileID {
		t.Errorf("response file_id = %q, want it to match the published request's %q", respBody["file_id"], got.FileID)
	}
}

func TestHandleCreateJobIgnoresClientSuppliedFileID(t *testing.T) {
	pub := &fakePublisher{}
	handler := handleCreateJob(pub, proxyStub(t), testLogger())

	postJob(t, handler, `{"file_id":"client-supplied","url":"https://example.com/v","kind":"audio","audio_format":"opus"}`)

	if len(pub.messages) != 1 {
		t.Fatalf("published %d messages, want 1", len(pub.messages))
	}
	var got jobRequest
	json.Unmarshal(pub.messages[0].Value, &got) //nolint:errcheck
	if got.FileID == "client-supplied" {
		t.Error("client-supplied file_id was used instead of a server-generated one")
	}
}

func TestHandleCreateJobValidation(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"invalid JSON", `not json`},
		{"missing url", `{"kind":"video","height":1080}`},
		{"invalid kind", `{"url":"https://x","kind":"gif"}`},
		{"video kind without height", `{"url":"https://x","kind":"video"}`},
		{"video kind with height 0", `{"url":"https://x","kind":"video","height":0}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub := &fakePublisher{}
			handler := handleCreateJob(pub, proxyStub(t), testLogger())
			rec := postJob(t, handler, tc.body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if len(pub.messages) != 0 {
				t.Error("invalid request was still published to Kafka")
			}
		})
	}
}

func TestHandleCreateJobAudioKindDoesNotRequireHeight(t *testing.T) {
	pub := &fakePublisher{}
	handler := handleCreateJob(pub, proxyStub(t), testLogger())

	rec := postJob(t, handler, `{"url":"https://example.com/v","kind":"audio","audio_format":"mp3"}`)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
}

func TestHandleCreateJobPublishFailureReturns500(t *testing.T) {
	pub := &fakePublisher{err: errors.New("broker unreachable")}
	handler := handleCreateJob(pub, proxyStub(t), testLogger())

	rec := postJob(t, handler, `{"url":"https://example.com/v","kind":"audio"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleCreateJobNonPostFallsThroughToProxy(t *testing.T) {
	called := false
	proxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := handleCreateJob(&fakePublisher{}, proxy, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("GET /api/jobs was not passed through to the proxy")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
