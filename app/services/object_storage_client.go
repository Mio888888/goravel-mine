package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

type objectStorageClient struct {
	config StorageConfig
	client *http.Client
}

func newObjectStorageClient(config StorageConfig) *objectStorageClient {
	return &objectStorageClient{config: config, client: &http.Client{Timeout: 30 * time.Second}}
}

func (c *objectStorageClient) Put(source, objectPath, mimeType string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	endpoint, err := c.objectURL(objectPath)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mimeType)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(content)))
	c.sign(req, content)
	return c.do(req)
}

func (c *objectStorageClient) Delete(objectPath string) error {
	endpoint, err := c.objectURL(objectPath)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	c.sign(req, nil)
	return c.do(req)
}

func (c *objectStorageClient) PublicURL(objectPath string) string {
	if c.config.BaseURL != "" {
		return strings.TrimRight(c.config.BaseURL, "/") + "/" + objectPath
	}
	endpoint, err := c.objectURL(objectPath)
	if err != nil {
		return "/storage/" + objectPath
	}
	return endpoint
}

func (c *objectStorageClient) do(req *http.Request) error {
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
	return fmt.Errorf("object storage request failed: %s %s", res.Status, strings.TrimSpace(string(body)))
}

func (c *objectStorageClient) objectURL(objectPath string) (string, error) {
	endpoint := strings.TrimRight(c.config.Endpoint, "/")
	if endpoint == "" {
		return "", fmt.Errorf("storage endpoint is empty")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	cleanPath := strings.TrimLeft(path.Join(c.config.Bucket, objectPath), "/")
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + cleanPath
	return parsed.String(), nil
}

func (c *objectStorageClient) sign(req *http.Request, payload []byte) {
	if c.config.AccessKey == "" || c.config.SecretKey == "" {
		return
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	region := c.config.Region
	if region == "" {
		region = "us-east-1"
	}
	payloadHash := sha256Hex(payload)
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	signedHeaders, canonicalHeaders := canonicalHeaders(req.Header)
	canonicalRequest := strings.Join([]string{
		req.Method,
		uriEncode(req.URL.EscapedPath()),
		canonicalQuery(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	scope := strings.Join([]string{date, region, "s3", "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(signingKey(c.config.SecretKey, date, region), stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.config.AccessKey, scope, signedHeaders, signature,
	))
}

func canonicalHeaders(headers http.Header) (string, string) {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, strings.ToLower(key))
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		values := headers.Values(key)
		sort.Strings(values)
		lines = append(lines, key+":"+strings.Join(values, ","))
	}
	return strings.Join(keys, ";"), strings.Join(lines, "\n") + "\n"
}

func canonicalQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0)
	for _, key := range keys {
		for _, value := range values[key] {
			parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(value))
		}
	}
	return strings.Join(parts, "&")
}

func signingKey(secret, date, region string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func uriEncode(value string) string {
	escaped := strings.ReplaceAll(value, "%2F", "/")
	if escaped == "" {
		return "/"
	}
	return escaped
}
