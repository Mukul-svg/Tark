package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"simplek8/internal/cache"
	"simplek8/internal/store"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

const (
	serviceDiscoveryCacheTTL = 30 * time.Second
	targetProbeTimeout       = 800 * time.Millisecond
)

type chatCompletionRequest struct {
	Model string `json:"model"`
}

type ProxyHandler struct {
	mu            sync.RWMutex
	defaultTarget string
	proxyByTarget map[string]*httputil.ReverseProxy
	store         store.Store
	cache         *cache.RedisCache
}

func NewProxyHandler(defaultTarget string, st store.Store, c *cache.RedisCache) *ProxyHandler {
	h := &ProxyHandler{
		store:         st,
		cache:         c,
		proxyByTarget: make(map[string]*httputil.ReverseProxy),
	}
	h.SetDefaultTarget(defaultTarget)
	return h
}

func (h *ProxyHandler) SetDefaultTarget(target string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.defaultTarget = strings.TrimSpace(target)
}

func (h *ProxyHandler) proxyForTarget(target string) *httputil.ReverseProxy {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}

	h.mu.RLock()
	if existing, ok := h.proxyByTarget[target]; ok {
		h.mu.RUnlock()
		return existing
	}
	h.mu.RUnlock()

	targetURL, err := url.Parse(target)
	if err != nil || targetURL.Scheme == "" || targetURL.Host == "" {
		return nil
	}

	p := httputil.NewSingleHostReverseProxy(targetURL)
	p.FlushInterval = -1
	originalDirector := p.Director
	p.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
	}
	p.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		slog.Error("proxy upstream error", "target", target, "error", err)
		http.Error(w, "upstream inference backend unavailable", http.StatusBadGateway)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, ok := h.proxyByTarget[target]; ok {
		return existing
	}
	h.proxyByTarget[target] = p
	return p
}

func (h *ProxyHandler) redisClient() *redis.Client {
	if h.cache == nil {
		return nil
	}
	return h.cache.Client()
}

func modelTargetsKey(model string) string {
	return "proxy:model:targets:" + strings.ToLower(model)
}

func modelRoundRobinKey(model string) string {
	return "proxy:model:rr:" + strings.ToLower(model)
}

func (h *ProxyHandler) invalidateModelCache(ctx context.Context, model string) {
	rdb := h.redisClient()
	if rdb == nil {
		return
	}
	if err := rdb.Del(ctx, modelTargetsKey(model), modelRoundRobinKey(model)).Err(); err != nil {
		slog.Warn("failed to invalidate model cache", "model", model, "error", err)
	}
}

func (h *ProxyHandler) evictProxyTarget(target string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.proxyByTarget, target)
}

func isTargetHealthy(target string) bool {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil || parsed.Host == "" {
		return false
	}

	hostPort := parsed.Host
	if !strings.Contains(hostPort, ":") {
		switch parsed.Scheme {
		case "https":
			hostPort = net.JoinHostPort(hostPort, "443")
		default:
			hostPort = net.JoinHostPort(hostPort, "80")
		}
	}

	conn, err := net.DialTimeout("tcp", hostPort, targetProbeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (h *ProxyHandler) filterHealthyTargets(targets []string) []string {
	healthy := make([]string, 0, len(targets))
	for _, target := range targets {
		if isTargetHealthy(target) {
			healthy = append(healthy, target)
		} else {
			h.evictProxyTarget(target)
		}
	}
	return healthy
}

func (h *ProxyHandler) setTargetsInCache(ctx context.Context, model string, targets []string) {
	rdb := h.redisClient()
	if rdb == nil {
		return
	}

	encoded, err := json.Marshal(targets)
	if err != nil {
		return
	}

	if err := rdb.Set(ctx, modelTargetsKey(model), encoded, serviceDiscoveryCacheTTL).Err(); err != nil {
		slog.Warn("failed to cache model targets", "model", model, "error", err)
	}
}

func (h *ProxyHandler) readTargetsFromCache(ctx context.Context, model string) ([]string, error) {
	rdb := h.redisClient()
	if rdb == nil {
		return nil, redis.Nil
	}

	raw, err := rdb.Get(ctx, modelTargetsKey(model)).Result()
	if err != nil {
		return nil, err
	}

	var targets []string
	if err := json.Unmarshal([]byte(raw), &targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func (h *ProxyHandler) selectRoundRobinTarget(ctx context.Context, model string, targets []string) string {
	if len(targets) == 1 {
		return targets[0]
	}

	rdb := h.redisClient()
	if rdb != nil {
		counter, err := rdb.Incr(ctx, modelRoundRobinKey(model)).Result()
		if err == nil {
			idx := int((counter - 1) % int64(len(targets)))
			return targets[idx]
		}
		slog.Warn("round-robin redis increment failed, using local fallback", "model", model, "error", err)
	}

	seed := time.Now().UnixNano()
	idx := int(seed % int64(len(targets)))
	if idx < 0 {
		idx = 0
	}
	return targets[idx]
}

func (h *ProxyHandler) resolveModelTarget(ctx context.Context, model string) (string, error) {
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" {
		h.mu.RLock()
		fallback := h.defaultTarget
		h.mu.RUnlock()
		if fallback == "" {
			return "", errors.New("inference backend is not configured")
		}
		return fallback, nil
	}

	targets, err := h.readTargetsFromCache(ctx, trimmedModel)
	if err == nil && len(targets) > 0 {
		healthyTargets := h.filterHealthyTargets(targets)
		if len(healthyTargets) > 0 {
			if len(healthyTargets) != len(targets) {
				h.setTargetsInCache(ctx, trimmedModel, healthyTargets)
			}
			return h.selectRoundRobinTarget(ctx, trimmedModel, healthyTargets), nil
		}

		h.invalidateModelCache(ctx, trimmedModel)
		slog.Warn("all cached model targets are unhealthy, falling back to database", "model", trimmedModel)
	}

	if err != nil && !errors.Is(err, redis.Nil) {
		slog.Warn("redis discovery failed, using database fallback", "model", trimmedModel, "error", err)
	}

	if h.store == nil {
		return "", fmt.Errorf("no routing target found for model %q", trimmedModel)
	}

	targets, err = h.store.ListActiveDeploymentTargets(ctx, trimmedModel)
	if err != nil {
		return "", fmt.Errorf("service discovery failed for model %q: %w", trimmedModel, err)
	}
	if len(targets) == 0 {
		return "", fmt.Errorf("no active deployment found for model %q", trimmedModel)
	}

	healthyTargets := h.filterHealthyTargets(targets)
	if len(healthyTargets) == 0 {
		h.invalidateModelCache(ctx, trimmedModel)
		return "", fmt.Errorf("no healthy deployment target found for model %q", trimmedModel)
	}

	h.setTargetsInCache(ctx, trimmedModel, healthyTargets)
	return h.selectRoundRobinTarget(ctx, trimmedModel, healthyTargets), nil
}

func parseChatModel(c echo.Context) (string, error) {
	if c.Request().Body == nil {
		return "", nil
	}

	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return "", err
	}
	c.Request().Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return "", nil
	}

	var req chatCompletionRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		return "", err
	}
	return req.Model, nil
}

func writeProxyError(c echo.Context, status int, code string, msg string) error {
	return c.JSON(status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": msg,
		},
	})
}

func (h *ProxyHandler) Proxy(c echo.Context) error {
	model, err := parseChatModel(c)
	if err != nil {
		return writeProxyError(c, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid chat completion payload")
	}

	target, err := h.resolveModelTarget(c.Request().Context(), model)
	if err != nil {
		return writeProxyError(c, http.StatusServiceUnavailable, "MODEL_UNAVAILABLE", err.Error())
	}

	proxy := h.proxyForTarget(target)
	if proxy == nil {
		return writeProxyError(c, http.StatusServiceUnavailable, "TARGET_INVALID", "resolved inference target is invalid: "+strconv.Quote(target))
	}

	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}
