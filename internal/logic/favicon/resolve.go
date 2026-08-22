package favicon

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Resolve 解析目标站点可用的 favicon 地址。
// 流程：抓取页面 HTML 解析 <link rel=icon> 候选 -> 追加 /favicon.ico 兜底 -> 逐个探测可用性。
// 全程带超时与响应大小限制，避免慢站点拖垮服务。
func Resolve(ctx context.Context, rawURL string) (string, error) {
	pageURL, err := normalizeHTTPURL(rawURL)
	if err != nil {
		return "", err
	}
	client := httpClient()
	var candidates []string

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL.String(), nil)
	if err == nil {
		req.Header.Set("User-Agent", "DockerCopilot favicon resolver")
		if resp, derr := client.Do(req); derr == nil {
			defer resp.Body.Close()
			contentType := strings.ToLower(resp.Header.Get("Content-Type"))
			// 仅读取前 512KB，防止大页面耗内存
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			if resp.StatusCode >= 200 && resp.StatusCode < 400 && strings.Contains(contentType, "html") {
				candidates = append(candidates, extractIconLinks(pageURL, string(body))...)
			}
		}
	}

	// 追加标准根路径兜底
	fallback := pageURL.ResolveReference(&url.URL{Path: "/favicon.ico"}).String()
	candidates = appendUnique(candidates, fallback)

	for _, candidate := range candidates {
		if probe(ctx, client, candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到可用 favicon")
}

// normalizeHTTPURL 归一化并校验 URL，仅允许 http/https。
func normalizeHTTPURL(rawURL string) (*url.URL, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return nil, fmt.Errorf("URL 不能为空")
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("URL 格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("仅支持 http/https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return nil, fmt.Errorf("URL 缺少主机")
	}
	return parsed, nil
}

// httpClient 返回带各级超时的 HTTP 客户端。
func httpClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   3 * time.Second,
				KeepAlive: 10 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   3 * time.Second,
			ResponseHeaderTimeout: 4 * time.Second,
			IdleConnTimeout:       5 * time.Second,
		},
	}
}

// probe 探测候选 URL 是否为可用图片资源。
func probe(ctx context.Context, client *http.Client, candidate string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "DockerCopilot favicon resolver")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return false
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
	}
	return strings.HasPrefix(mediaType, "image/") || mediaType == "application/octet-stream"
}
