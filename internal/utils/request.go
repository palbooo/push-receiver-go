package utils

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxRetryTimeout = 15 * time.Second
	retryStep       = 5 * time.Second
)

// RequestOptions contains options for making HTTP requests
type RequestOptions struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    []byte
	Form    map[string]string
	Client  *http.Client
}

// RequestWithRetry performs an HTTP request with automatic retry logic
func RequestWithRetry(opts RequestOptions) ([]byte, error) {
	return retry(0, opts)
}

func retry(retryCount int, opts RequestOptions) ([]byte, error) {
	client := opts.Client
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	// Prepare request body
	var bodyReader io.Reader
	if len(opts.Body) > 0 {
		bodyReader = bytes.NewReader(opts.Body)
	} else if len(opts.Form) > 0 {
		formData := url.Values{}
		for k, v := range opts.Form {
			formData.Set(k, v)
		}
		bodyReader = strings.NewReader(formData.Encode())
		if opts.Headers == nil {
			opts.Headers = make(map[string]string)
		}
		if _, exists := opts.Headers["Content-Type"]; !exists {
			opts.Headers["Content-Type"] = "application/x-www-form-urlencoded"
		}
	}

	// Create request
	req, err := http.NewRequest(opts.Method, opts.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range opts.Headers {
		req.Header.Set(key, value)
	}

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		timeout := time.Duration(retryCount) * retryStep
		if timeout > maxRetryTimeout {
			timeout = maxRetryTimeout
		}

		fmt.Printf("Request failed: %v\n", err)
		fmt.Printf("Retrying in %v seconds\n", timeout.Seconds())

		time.Sleep(timeout)
		return retry(retryCount+1, opts)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		timeout := time.Duration(retryCount) * retryStep
		if timeout > maxRetryTimeout {
			timeout = maxRetryTimeout
		}

		fmt.Printf("Request failed with status %d: %s\n", resp.StatusCode, string(body))
		fmt.Printf("Retrying in %v seconds\n", timeout.Seconds())

		time.Sleep(timeout)
		return retry(retryCount+1, opts)
	}

	return body, nil
}

// SimpleRequest performs a simple HTTP request without retry logic
func SimpleRequest(opts RequestOptions) ([]byte, error) {
	client := opts.Client
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	// Prepare request body
	var bodyReader io.Reader
	if len(opts.Body) > 0 {
		bodyReader = bytes.NewReader(opts.Body)
	} else if len(opts.Form) > 0 {
		formData := url.Values{}
		for k, v := range opts.Form {
			formData.Set(k, v)
		}
		bodyReader = strings.NewReader(formData.Encode())
		if opts.Headers == nil {
			opts.Headers = make(map[string]string)
		}
		if _, exists := opts.Headers["Content-Type"]; !exists {
			opts.Headers["Content-Type"] = "application/x-www-form-urlencoded"
		}
	}

	// Create request
	req, err := http.NewRequest(opts.Method, opts.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range opts.Headers {
		req.Header.Set(key, value)
	}

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

