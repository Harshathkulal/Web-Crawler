package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// Crawler manages the web crawling process
type Crawler struct {
	baseURL         *url.URL
	visited         map[string]bool
	visitedMutex    sync.RWMutex
	maxDepth        int
	outputDirectory string
	wg              sync.WaitGroup
	concurrencyChan chan struct{}
}

// NewCrawler creates a new web crawler
func NewCrawler(baseURLStr string, maxDepth int, maxConcurrency int) (*Crawler, error) {
	parsedURL, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, err
	}

	// Create output directory
	outputDir := filepath.Join("crawl_output", parsedURL.Hostname())
	os.MkdirAll(outputDir, os.ModePerm)

	return &Crawler{
		baseURL:         parsedURL,
		visited:         make(map[string]bool),
		maxDepth:        maxDepth,
		outputDirectory: outputDir,
		concurrencyChan: make(chan struct{}, maxConcurrency),
	}, nil
}

// Crawl starts the web crawling process
func (c *Crawler) Crawl() {
	c.wg.Add(1)
	go c.crawlPage(c.baseURL.String(), 0)
	c.wg.Wait()
}

// crawlPage recursively crawls web pages
func (c *Crawler) crawlPage(pageURL string, depth int) {
	defer c.wg.Done()

	// Check depth and already visited
	if depth > c.maxDepth {
		return
	}

	c.visitedMutex.Lock()
	if c.visited[pageURL] {
		c.visitedMutex.Unlock()
		return
	}
	c.visited[pageURL] = true
	c.visitedMutex.Unlock()

	// Limit concurrency
	c.concurrencyChan <- struct{}{}
	defer func() { <-c.concurrencyChan }()

	// Fetch the page
	resp, err := http.Get(pageURL)
	if err != nil {
		fmt.Printf("Error fetching %s: %v\n", pageURL, err)
		return
	}
	defer resp.Body.Close()

	// Save page content
	if err := c.savePage(pageURL, resp); err != nil {
		fmt.Printf("Error saving page %s: %v\n", pageURL, err)
	}

	// Parse HTML and extract links
	links := c.extractLinks(resp.Body, pageURL)

	// Crawl extracted links concurrently
	for _, link := range links {
		// Ensure link is within the same domain
		if c.isValidLink(link) {
			c.wg.Add(1)
			go c.crawlPage(link, depth+1)
		}
	}
}

// extractLinks finds all unique links in the page
func (c *Crawler) extractLinks(body io.Reader, baseURL string) []string {
	doc, err := html.Parse(body)
	if err != nil {
		return []string{}
	}

	links := make(map[string]bool)
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					link, err := url.Parse(attr.Val)
					if err == nil {
						// Convert to absolute URL
						absoluteURL := c.baseURL.ResolveReference(link)
						fullURL := absoluteURL.String()
						
						// Clean URL
						fullURL = strings.TrimRight(fullURL, "/")
						
						links[fullURL] = true
					}
					break
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			extract(child)
		}
	}
	extract(doc)

	// Convert map to slice
	uniqueLinks := make([]string, 0, len(links))
	for link := range links {
		uniqueLinks = append(uniqueLinks, link)
	}

	return uniqueLinks
}

// isValidLink checks if the link is within the same domain
func (c *Crawler) isValidLink(linkURL string) bool {
	parsedLink, err := url.Parse(linkURL)
	if err != nil {
		return false
	}

	// Check if link is within the same domain
	return parsedLink.Hostname() == c.baseURL.Hostname() && 
		   !strings.Contains(parsedLink.Path, "@") && 
		   (strings.HasPrefix(parsedLink.Scheme, "http"))
}

// savePage saves the page content to a file
func (c *Crawler) savePage(pageURL string, resp *http.Response) error {
	// Create safe filename
	parsedURL, err := url.Parse(pageURL)
	if err != nil {
		return err
	}

	// Generate filename
	filename := strings.ReplaceAll(parsedURL.Path, "/", "_")
	if filename == "" || filename == "_" {
		filename = "index"
	}
	filename = filepath.Join(c.outputDirectory, filename+".html")

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(filename, body, 0644)
}

func main() {
	// Configuration
	startURL := "https://www.example.com"
	maxDepth := 2
	maxConcurrency := 10

	// Create crawler
	crawler, err := NewCrawler(startURL, maxDepth, maxConcurrency)
	if err != nil {
		fmt.Printf("Error creating crawler: %v\n", err)
		return
	}

	// Start crawling
	fmt.Printf("Starting crawl of %s\n", startURL)
	startTime := time.Now()
	crawler.Crawl()
	duration := time.Since(startTime)

	fmt.Printf("\nCrawl completed in %v\n", duration)
	fmt.Println("Pages saved in 'crawl_output' directory")
}