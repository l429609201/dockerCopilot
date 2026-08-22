package favicon

import (
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// iconLinkRe 用于兜底提取 <link rel="...icon..." href="..."> 中的 href。
var iconLinkRe = regexp.MustCompile(`(?is)<link[^>]+rel=["'][^"']*icon[^"']*["'][^>]+href=["']([^"']+)["']`)

// extractIconLinks 解析 HTML，提取所有图标 link 的绝对地址。
func extractIconLinks(base *url.URL, htmlText string) []string {
	out := []string{}
	doc, err := html.Parse(strings.NewReader(htmlText))
	if err != nil {
		return out
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "link") {
			rel, href := "", ""
			for _, attr := range n.Attr {
				switch strings.ToLower(attr.Key) {
				case "rel":
					rel = strings.ToLower(attr.Val)
				case "href":
					href = strings.TrimSpace(attr.Val)
				}
			}
			if href != "" && isIconRel(rel) {
				if resolved, err := base.Parse(href); err == nil && resolved.Scheme != "" && resolved.Host != "" {
					out = appendUnique(out, resolved.String())
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if len(out) > 0 {
		return out
	}
	// 兜底：部分页面由脚本注入，用正则再兜一次简单 link 标签
	for _, match := range iconLinkRe.FindAllStringSubmatch(htmlText, -1) {
		if len(match) == 2 {
			if resolved, err := base.Parse(strings.TrimSpace(match[1])); err == nil && resolved.Scheme != "" && resolved.Host != "" {
				out = appendUnique(out, resolved.String())
			}
		}
	}
	return out
}

// isIconRel 判断 rel 是否为图标类。
func isIconRel(rel string) bool {
	rel = strings.ToLower(rel)
	return strings.Contains(rel, "icon") || strings.Contains(rel, "apple-touch-icon")
}

// appendUnique 去重追加非空字符串。
func appendUnique(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}
