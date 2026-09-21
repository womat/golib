package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type payload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestEncode(t *testing.T) {
	rec := httptest.NewRecorder()

	Encode(rec, http.StatusCreated, payload{Name: "sensor", Count: 3})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var got payload
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v (%q)", err, rec.Body.String())
	}
	if got.Name != "sensor" || got.Count != 3 {
		t.Errorf("decoded %+v, want {sensor 3}", got)
	}
}

func TestEncodeMarshalFailure(t *testing.T) {
	rec := httptest.NewRecorder()

	// A channel cannot be marshalled.
	Encode(rec, http.StatusOK, make(chan int))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !json.Valid(rec.Body.Bytes()) {
		t.Errorf("the fallback body is not valid JSON: %q", rec.Body.String())
	}
}

func TestDecode(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"sensor","count":3}`))
	rec := httptest.NewRecorder()

	got, err := Decode[payload](rec, req)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Name != "sensor" || got.Count != 3 {
		t.Errorf("decoded %+v, want {sensor 3}", got)
	}
}

func TestDecodeInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":`))
	rec := httptest.NewRecorder()

	if _, err := Decode[payload](rec, req); err == nil {
		t.Error("Decode accepted a truncated body")
	}
}

func TestDecodeRejectsTrailingData(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"a"} and then some`))
	rec := httptest.NewRecorder()

	if _, err := Decode[payload](rec, req); err == nil {
		t.Error("Decode accepted data after the JSON value")
	}
}

func TestDecodeRejectsOversizedBody(t *testing.T) {
	body := `{"name":"` + strings.Repeat("x", maxRequestBody+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()

	if _, err := Decode[payload](rec, req); err == nil {
		t.Errorf("Decode read a body of %d bytes although the limit is %d", len(body), maxRequestBody)
	}
}
