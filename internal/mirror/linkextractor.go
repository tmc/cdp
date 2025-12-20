package mirror

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// LinkExtractor extracts URLs from various sources (HTML, CSS, JS).
type LinkExtractor struct {
	baseURL *url.URL
	options *MirrorOptions
}

// NewLinkExtractor creates a new link extractor.
func NewLinkExtractor(baseURL *url.URL, opts *MirrorOptions) *LinkExtractor {
	return &LinkExtractor{
		baseURL: baseURL,
		options: opts,
	}
}

// ExtractedLink represents a discovered link with metadata.
type ExtractedLink struct {
	URL      string
	Type     LinkType
	Priority URLPriority
	Source   string // Where the link was found
}

// LinkType represents the type of link discovered.
type LinkType int

const (
	LinkTypeHTML LinkType = iota
	LinkTypeCSS
	LinkTypeJS
	LinkTypeImage
	LinkTypeFont
	LinkTypeMedia
	LinkTypeDocument
	LinkTypeAPI
	LinkTypeSitemap
	LinkTypeOther
)

// ExtractFromHTML extracts all links from HTML content.
func (le *LinkExtractor) ExtractFromHTML(content []byte, pageURL string) ([]ExtractedLink, error) {
	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var links []ExtractedLink
	var extractLinks func(*html.Node)
	extractLinks = func(n *html.Node) {
		if n.Type == html.ElementNode {
			extracted := le.extractFromNode(n, pageURL)
			links = append(links, extracted...)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractLinks(c)
		}
	}

	extractLinks(doc)
	return links, nil
}

// extractFromNode extracts links from a single HTML node.
func (le *LinkExtractor) extractFromNode(n *html.Node, pageURL string) []ExtractedLink {
	var links []ExtractedLink

	switch n.Data {
	case "a", "area":
		// Extract href
		if href := le.getAttr(n, "href"); href != "" {
			if resolved := le.resolveURL(href, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     LinkTypeHTML,
					Priority: PriorityNormal,
					Source:   "html:a",
				})
			}
		}

	case "link":
		// Extract stylesheet, icon, preload links
		if href := le.getAttr(n, "href"); href != "" {
			rel := le.getAttr(n, "rel")
			linkType := LinkTypeOther
			priority := PriorityLow

			if strings.Contains(rel, "stylesheet") {
				linkType = LinkTypeCSS
				priority = PriorityHigh
			} else if strings.Contains(rel, "icon") {
				linkType = LinkTypeImage
				priority = PriorityLow
			} else if strings.Contains(rel, "preload") || strings.Contains(rel, "prefetch") {
				priority = PriorityHigh
			}

			if resolved := le.resolveURL(href, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     linkType,
					Priority: priority,
					Source:   fmt.Sprintf("html:link[rel=%s]", rel),
				})
			}
		}

	case "script":
		// Extract script src
		if src := le.getAttr(n, "src"); src != "" {
			if resolved := le.resolveURL(src, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     LinkTypeJS,
					Priority: PriorityHigh,
					Source:   "html:script",
				})
			}
		}

	case "img":
		// Extract image src and srcset
		if src := le.getAttr(n, "src"); src != "" {
			if resolved := le.resolveURL(src, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     LinkTypeImage,
					Priority: PriorityNormal,
					Source:   "html:img",
				})
			}
		}
		if srcset := le.getAttr(n, "srcset"); srcset != "" {
			srcs := le.parseSrcSet(srcset)
			for _, src := range srcs {
				if resolved := le.resolveURL(src, pageURL); resolved != "" {
					links = append(links, ExtractedLink{
						URL:      resolved,
						Type:     LinkTypeImage,
						Priority: PriorityNormal,
						Source:   "html:img[srcset]",
					})
				}
			}
		}

	case "source":
		// Extract media source
		if src := le.getAttr(n, "src"); src != "" {
			if resolved := le.resolveURL(src, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     LinkTypeMedia,
					Priority: PriorityNormal,
					Source:   "html:source",
				})
			}
		}

	case "video", "audio":
		// Extract media src and poster
		if src := le.getAttr(n, "src"); src != "" {
			if resolved := le.resolveURL(src, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     LinkTypeMedia,
					Priority: PriorityNormal,
					Source:   fmt.Sprintf("html:%s", n.Data),
				})
			}
		}
		if poster := le.getAttr(n, "poster"); poster != "" {
			if resolved := le.resolveURL(poster, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     LinkTypeImage,
					Priority: PriorityLow,
					Source:   "html:video[poster]",
				})
			}
		}

	case "iframe", "embed":
		// Extract embedded content
		if src := le.getAttr(n, "src"); src != "" {
			if resolved := le.resolveURL(src, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     LinkTypeHTML,
					Priority: PriorityNormal,
					Source:   fmt.Sprintf("html:%s", n.Data),
				})
			}
		}

	case "object":
		// Extract object data
		if data := le.getAttr(n, "data"); data != "" {
			if resolved := le.resolveURL(data, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     LinkTypeOther,
					Priority: PriorityNormal,
					Source:   "html:object",
				})
			}
		}

	case "form":
		// Extract form action
		if action := le.getAttr(n, "action"); action != "" {
			if resolved := le.resolveURL(action, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     LinkTypeHTML,
					Priority: PriorityLow,
					Source:   "html:form",
				})
			}
		}
	}

	// Check for inline styles with url()
	if style := le.getAttr(n, "style"); style != "" {
		cssLinks := le.extractFromCSS([]byte(style), pageURL)
		links = append(links, cssLinks...)
	}

	return links
}

// ExtractFromCSS extracts URLs from CSS content.
func (le *LinkExtractor) ExtractFromCSS(content []byte, pageURL string) []ExtractedLink {
	return le.extractFromCSS(content, pageURL)
}

func (le *LinkExtractor) extractFromCSS(content []byte, pageURL string) []ExtractedLink {
	var links []ExtractedLink

	// Regex to match url() in CSS
	// Matches: url("..."), url('...'), url(...)
	urlRegex := regexp.MustCompile(`url\(\s*['"]?([^'")]+)['"]?\s*\)`)
	matches := urlRegex.FindAllSubmatch(content, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			urlStr := string(match[1])
			if resolved := le.resolveURL(urlStr, pageURL); resolved != "" {
				// Determine type based on URL
				linkType := LinkTypeOther
				if strings.HasSuffix(strings.ToLower(urlStr), ".woff") ||
					strings.HasSuffix(strings.ToLower(urlStr), ".woff2") ||
					strings.HasSuffix(strings.ToLower(urlStr), ".ttf") ||
					strings.HasSuffix(strings.ToLower(urlStr), ".otf") ||
					strings.HasSuffix(strings.ToLower(urlStr), ".eot") {
					linkType = LinkTypeFont
				} else if strings.Contains(strings.ToLower(urlStr), ".png") ||
					strings.Contains(strings.ToLower(urlStr), ".jpg") ||
					strings.Contains(strings.ToLower(urlStr), ".jpeg") ||
					strings.Contains(strings.ToLower(urlStr), ".gif") ||
					strings.Contains(strings.ToLower(urlStr), ".svg") ||
					strings.Contains(strings.ToLower(urlStr), ".webp") {
					linkType = LinkTypeImage
				}

				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     linkType,
					Priority: PriorityNormal,
					Source:   "css:url()",
				})
			}
		}
	}

	// Also extract @import statements
	importRegex := regexp.MustCompile(`@import\s+['"]([^'"]+)['"]`)
	importMatches := importRegex.FindAllSubmatch(content, -1)

	for _, match := range importMatches {
		if len(match) >= 2 {
			urlStr := string(match[1])
			if resolved := le.resolveURL(urlStr, pageURL); resolved != "" {
				links = append(links, ExtractedLink{
					URL:      resolved,
					Type:     LinkTypeCSS,
					Priority: PriorityHigh,
					Source:   "css:@import",
				})
			}
		}
	}

	return links
}

// ExtractFromJS extracts URLs from JavaScript content.
func (le *LinkExtractor) ExtractFromJS(content []byte, pageURL string) []ExtractedLink {
	var links []ExtractedLink

	// Simple regex-based extraction for common patterns
	// This is not a full JS parser, but catches common cases

	// Match fetch(), XMLHttpRequest, import(), require()
	patterns := []string{
		`fetch\(\s*['"]([^'"]+)['"]`,                  // fetch("url")
		`XMLHttpRequest.*\.open\([^,]+,\s*['"]([^'"]+)`, // xhr.open('GET', "url")
		`import\(\s*['"]([^'"]+)['"]`,                 // import("url")
		`require\(\s*['"]([^'"]+)['"]`,                // require("url")
		`src\s*[:=]\s*['"]([^'"]+)['"]`,               // src: "url" or src = "url"
		`href\s*[:=]\s*['"]([^'"]+)['"]`,              // href: "url"
		`url\s*[:=]\s*['"]([^'"]+)['"]`,               // url: "url"
		`window\.location\s*=\s*['"]([^'"]+)['"]`,     // window.location = "url"
	}

	for _, pattern := range patterns {
		regex := regexp.MustCompile(pattern)
		matches := regex.FindAllSubmatch(content, -1)

		for _, match := range matches {
			if len(match) >= 2 {
				urlStr := string(match[1])
				// Skip data: URLs and javascript: URLs
				if strings.HasPrefix(urlStr, "data:") || strings.HasPrefix(urlStr, "javascript:") {
					continue
				}
				if resolved := le.resolveURL(urlStr, pageURL); resolved != "" {
					// Try to determine type
					linkType := LinkTypeOther
					if strings.HasSuffix(strings.ToLower(urlStr), ".js") {
						linkType = LinkTypeJS
					} else if strings.HasSuffix(strings.ToLower(urlStr), ".json") {
						linkType = LinkTypeAPI
					}

					links = append(links, ExtractedLink{
						URL:      resolved,
						Type:     linkType,
						Priority: PriorityNormal,
						Source:   "js:pattern",
					})
				}
			}
		}
	}

	return links
}

// getAttr gets an attribute value from an HTML node.
func (le *LinkExtractor) getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// resolveURL resolves a potentially relative URL against a base URL.
func (le *LinkExtractor) resolveURL(href, baseURLStr string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}

	// Skip special URLs
	if strings.HasPrefix(href, "#") ||
		strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:") ||
		strings.HasPrefix(href, "javascript:") ||
		strings.HasPrefix(href, "data:") {
		return ""
	}

	// Parse base URL
	base, err := url.Parse(baseURLStr)
	if err != nil {
		base = le.baseURL
	}

	// Parse and resolve the URL
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}

	resolved := base.ResolveReference(u)
	return resolved.String()
}

// parseSrcSet parses an HTML srcset attribute.
func (le *LinkExtractor) parseSrcSet(srcset string) []string {
	var urls []string
	// srcset format: "url1 descriptor1, url2 descriptor2, ..."
	parts := strings.Split(srcset, ",")
	for _, part := range parts {
		// Take only the URL part (before any descriptor)
		tokens := strings.Fields(strings.TrimSpace(part))
		if len(tokens) > 0 {
			urls = append(urls, tokens[0])
		}
	}
	return urls
}
