package auth

import (
	"net/http"
	"net/http/httptest"
	"tangoforge/internal/config"
	"testing"
)

// newCfg 构造带指定凭据的全局配置。
func newCfg(uiToken, apiToken string) *config.GlobalConfig {
	cfg := config.DefaultGlobalConfig()
	cfg.UIToken = uiToken
	cfg.APIToken = apiToken
	return &cfg
}

// req 构造请求并设置来源地址与头。
func req(remoteAddr string, setHeader func(h http.Header)) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	r.RemoteAddr = remoteAddr
	if setHeader != nil {
		setHeader(r.Header)
	}
	return r
}

func TestIdentify_UITokenLoopback(t *testing.T) {
	cfg := newCfg("ui-secret", "api-secret")
	r := req("127.0.0.1:5555", func(h http.Header) { h.Set("X-UI-Token", "ui-secret") })
	a, needAuth := Identify(cfg, r)
	if needAuth {
		t.Fatal("ui request should not need auth")
	}
	if a.Class != ClassUI || a.Name != "ui" {
		t.Fatalf("got %+v, want ui", a)
	}
}

func TestIdentify_UITokenLoopbackIPv6(t *testing.T) {
	cfg := newCfg("ui-secret", "api-secret")
	r := req("[::1]:5555", func(h http.Header) { h.Set("X-UI-Token", "ui-secret") })
	a, _ := Identify(cfg, r)
	if a.Class != ClassUI {
		t.Fatalf("got class %q, want ui (IPv6 loopback)", a.Class)
	}
}

func TestIdentify_UITokenWrongOnLoopback(t *testing.T) {
	cfg := newCfg("ui-secret", "api-secret")
	// 错误 UI-Token：回环下不构成 ui；无 X-Actor → unknown。
	r := req("127.0.0.1:5555", func(h http.Header) { h.Set("X-UI-Token", "wrong") })
	a, needAuth := Identify(cfg, r)
	if needAuth {
		t.Fatal("loopback request should not need auth")
	}
	if a.Class != ClassUnknown || a.Name != "unknown" {
		t.Fatalf("got %+v, want unknown", a)
	}
}

func TestIdentify_XActorOnLoopback(t *testing.T) {
	cfg := newCfg("ui-secret", "api-secret")
	// 回环 + X-Actor → agent（CLI 场景）。
	r := req("127.0.0.1:5555", func(h http.Header) { h.Set("X-Actor", "human") })
	a, _ := Identify(cfg, r)
	if a.Class != ClassAgent || a.Name != "human" {
		t.Fatalf("got %+v, want agent(human)", a)
	}
}

func TestIdentify_NoCredentialLoopback(t *testing.T) {
	cfg := newCfg("ui-secret", "api-secret")
	r := req("127.0.0.1:5555", nil)
	a, _ := Identify(cfg, r)
	if a.Class != ClassUnknown {
		t.Fatalf("got class %q, want unknown", a.Class)
	}
}

func TestIdentify_RemoteNoBearer(t *testing.T) {
	cfg := newCfg("ui-secret", "api-secret")
	r := req("192.168.1.5:1234", nil)
	_, needAuth := Identify(cfg, r)
	if !needAuth {
		t.Fatal("remote request without bearer must need auth (401)")
	}
}

func TestIdentify_RemoteWrongBearer(t *testing.T) {
	cfg := newCfg("ui-secret", "api-secret")
	r := req("192.168.1.5:1234", func(h http.Header) { h.Set("Authorization", "Bearer wrong-token") })
	_, needAuth := Identify(cfg, r)
	if !needAuth {
		t.Fatal("remote request with wrong bearer must need auth (401)")
	}
}

func TestIdentify_RemoteUITokenIgnored(t *testing.T) {
	// UI 凭据仅回环有效：非回环携带正确 UI-Token 而无 Bearer → 仍需 401。
	cfg := newCfg("ui-secret", "api-secret")
	r := req("192.168.1.5:1234", func(h http.Header) { h.Set("X-UI-Token", "ui-secret") })
	_, needAuth := Identify(cfg, r)
	if !needAuth {
		t.Fatal("ui token must not work remotely")
	}
}

func TestIdentify_RemoteValidBearer(t *testing.T) {
	cfg := newCfg("ui-secret", "api-secret")
	r := req("192.168.1.5:1234", func(h http.Header) { h.Set("Authorization", "Bearer api-secret") })
	a, needAuth := Identify(cfg, r)
	if needAuth {
		t.Fatal("valid bearer should pass")
	}
	if a.Class != ClassAgent || a.Name != "unknown" {
		t.Fatalf("got %+v, want agent(unknown)", a)
	}
}

func TestIdentify_RemoteValidBearerWithActor(t *testing.T) {
	cfg := newCfg("ui-secret", "api-secret")
	r := req("192.168.1.5:1234", func(h http.Header) {
		h.Set("Authorization", "Bearer api-secret")
		h.Set("X-Actor", "my-agent")
	})
	a, _ := Identify(cfg, r)
	if a.Class != ClassAgent || a.Name != "my-agent" {
		t.Fatalf("got %+v, want agent(my-agent)", a)
	}
}

func TestIdentify_RemoteAPITokenNotConfigured(t *testing.T) {
	// api_token 为空：远程一律 401（安全默认，QA P3-默认项 1）。
	cfg := newCfg("ui-secret", "")
	r := req("192.168.1.5:1234", func(h http.Header) { h.Set("Authorization", "Bearer anything") })
	_, needAuth := Identify(cfg, r)
	if !needAuth {
		t.Fatal("remote request must be rejected when api_token is empty")
	}
}

func TestIdentify_NilConfig(t *testing.T) {
	// cfg 为 nil：不 panic，按无凭据处理。
	r := req("127.0.0.1:5555", func(h http.Header) { h.Set("X-UI-Token", "x") })
	a, _ := Identify(nil, r)
	if a.Class != ClassUnknown {
		t.Fatalf("nil config: got %+v, want unknown", a)
	}
}

func TestFromMCP(t *testing.T) {
	a := FromMCP("tangoforge-mcp")
	if a.Class != ClassAgent || a.Name != "tangoforge-mcp" {
		t.Fatalf("got %+v, want agent(tangoforge-mcp)", a)
	}
}

func TestActorContext(t *testing.T) {
	// 未写入 → unknown 兜底。
	if a := ActorFrom(t.Context()); a.Class != ClassUnknown {
		t.Fatalf("empty ctx: got %+v, want unknown", a)
	}
	want := Actor{Name: "ui", Class: ClassUI}
	ctx := WithActor(t.Context(), want)
	if a := ActorFrom(ctx); a != want {
		t.Fatalf("got %+v, want %+v", a, want)
	}
}

func TestBearerToken(t *testing.T) {
	r := req("127.0.0.1:1", func(h http.Header) {
		h.Set("Authorization", "Bearer tok123")
	})
	if got := BearerToken(r); got != "tok123" {
		t.Fatalf("got %q, want tok123", got)
	}
	// 大小写敏感（RFC 7235 Bearer 大小写不敏感，但按字面解析）。
	r2 := req("127.0.0.1:1", func(h http.Header) {
		h.Set("Authorization", "bearer tok123")
	})
	if got := BearerToken(r2); got != "" {
		t.Fatalf("got %q, want empty for lowercase prefix", got)
	}
}

func TestSecureEqual(t *testing.T) {
	if !SecureEqual("abc", "abc") {
		t.Error("equal strings must match")
	}
	if SecureEqual("abc", "abd") {
		t.Error("different strings must not match")
	}
	if SecureEqual("", "a") {
		t.Error("empty vs non-empty must not match")
	}
	if !SecureEqual("", "") {
		t.Error("empty strings must match")
	}
}
