package tokeninfoproxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

const testTokenInfo = `{"access_token": "xxx","cn": "John Doe","expires_in": 42,"grant_type": "password","realm":"/services","scope":["uid","cn"],"token_type":"Bearer","uid":"jdoe"}` + "\n"

func TestProxy(t *testing.T) {
	var upstream string

	handler := func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json;charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testTokenInfo))
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	upstream = fmt.Sprintf("http://%s", server.Listener.Addr())
	url, _ := url.Parse(upstream)
	h := NewTokenInfoProxyHandler(url, 0, time.Second*0, time.Second*1)
	invalid := `{"error":"invalid_request","error_description":"Access Token not valid"}` + "\n"
	for _, it := range []struct {
		query    string
		wantCode int
		wantBody string
	}{
		{"/oauth2/tokeninfo", http.StatusBadRequest, invalid},
		{"/oauth2/tokeninfo?access_token", http.StatusBadRequest, invalid},
		{"/oauth2/tokeninfo?access_token=foo", http.StatusOK, testTokenInfo},
	} {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", "http://example.com"+it.query, nil)
		h.ServeHTTP(w, r)

		if w.Code != it.wantCode {
			t.Errorf("Wrong status code. Wanted %d, got %d", it.wantCode, w.Code)
		}

		if w.Body.String() != it.wantBody {
			t.Errorf("Wrong response body. Wanted %q, got %s", it.wantBody, w.Body.String())
		}
	}
}

func TestHostHeader(t *testing.T) {
	var upstream string

	handler := func(w http.ResponseWriter, req *http.Request) {
		if req.Host == "example.com" {
			t.Fatalf("Received the wrong Host header: %s", req.Host)
		}
		if req.URL.Path != "/upstream-tokeninfo" {
			t.Fatalf("Received the wrong path: %s", req.URL.Path)
		}
		if req.URL.RawQuery != "access_token=foo" {
			t.Fatalf("Received the wrong query: %s", req.URL.RawQuery)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	upstream = fmt.Sprintf("http://%s/upstream-tokeninfo", server.Listener.Addr())
	url, _ := url.Parse(upstream)
	h := NewTokenInfoProxyHandler(url, 0, time.Second*0, time.Second*1)

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "http://example.com/oauth2/tokeninfo?access_token=foo", nil)
	h.ServeHTTP(w, r)
}

func TestCache(t *testing.T) {
	var upstream string
	var upstreamCalls int

	handler := func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json;charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testTokenInfo))
		upstreamCalls++
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	upstream = fmt.Sprintf("http://%s", server.Listener.Addr())
	url, _ := url.Parse(upstream)
	h := NewTokenInfoProxyHandler(url, 10, 1*time.Second, time.Second*1)
	for i, it := range []struct {
		query     string
		wantCode  int
		wantBody  string
		wantCache string
	}{
		{"/oauth2/tokeninfo?access_token=foo", http.StatusOK, testTokenInfo, "MISS"},
		{"/oauth2/tokeninfo?access_token=bar", http.StatusOK, testTokenInfo, "MISS"},
		{"/oauth2/tokeninfo?access_token=foo", http.StatusOK, testTokenInfo, "HIT"},
		{"/oauth2/tokeninfo?access_token=foo", http.StatusOK, testTokenInfo, "MISS"},
	} {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", "http://example.com"+it.query, nil)
		h.ServeHTTP(w, r)

		if w.Code != it.wantCode {
			t.Errorf("Wrong status code. Wanted %d, got %d", it.wantCode, w.Code)
		}

		if w.Body.String() != it.wantBody {
			t.Errorf("Wrong response body. Wanted %q, got %s", it.wantBody, w.Body.String())
		}

		if w.Header().Get("X-Cache") != it.wantCache {
			t.Errorf("Wrong cache header in call %d. Wanted %q, got %s", i, it.wantCache, w.Header().Get("X-Cache"))
		}
		if i == 2 {
			time.Sleep(2 * time.Second)
		}
	}
	if upstreamCalls > 3 {
		t.Errorf("Second request for 'foo' token should have been cached, but we got %d calls to upstream", upstreamCalls)
	}
}

func TestCacheDisabled(t *testing.T) {
	var upstream string
	var counter int

	handler := func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json;charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testTokenInfo))
		counter++
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	upstream = fmt.Sprintf("http://%s", server.Listener.Addr())
	url, _ := url.Parse(upstream)
	h := NewTokenInfoProxyHandler(url, 10, 0, time.Second*1)
	for _, it := range []struct {
		query     string
		wantCode  int
		wantBody  string
		wantCache string
	}{
		{"/oauth2/tokeninfo?access_token=foo", http.StatusOK, testTokenInfo, "MISS"},
		{"/oauth2/tokeninfo?access_token=bar", http.StatusOK, testTokenInfo, "MISS"},
		{"/oauth2/tokeninfo?access_token=foo", http.StatusOK, testTokenInfo, "MISS"},
	} {
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", "http://example.com"+it.query, nil)
		h.ServeHTTP(w, r)

		if w.Code != it.wantCode {
			t.Errorf("Wrong status code. Wanted %d, got %d", it.wantCode, w.Code)
		}

		if w.Body.String() != it.wantBody {
			t.Errorf("Wrong response body. Wanted %q, got %s", it.wantBody, w.Body.String())
		}

		if w.Header().Get("X-Cache") != it.wantCache {
			t.Errorf("Wrong cache header. Wanted %q, got %s", it.wantCache, w.Header().Get("X-Cache"))
		}
	}
	if counter < 3 {
		t.Errorf("Second request for 'foo' token should NOT have been cached, but we only got %d calls to upstream", counter)
	}
}

func TestUpstreamTimeout(t *testing.T) {
	handler := func(w http.ResponseWriter, req *http.Request) {
		time.Sleep(10 * time.Millisecond)
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	upstream := fmt.Sprintf("http://%s", server.Listener.Addr())
	url, _ := url.Parse(upstream)
	h := NewTokenInfoProxyHandler(url, 0, 0, time.Millisecond*1)

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/oauth2/tokeninfo?access_token=foo", nil)

	h.ServeHTTP(w, r)

	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("Response code should be 504 Gateway Timeout but was %d %s instead", w.Code, http.StatusText(w.Code))
	}
}
