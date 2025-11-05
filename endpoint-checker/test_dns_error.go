package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

func checkDNSError(url string) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("\n=== Testing URL: %s ===\n", url)
		fmt.Printf("Error: %v\n", err)
		fmt.Printf("Error Type: %T\n", err)

		// Method 1: errors.As
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			fmt.Printf("✓ Method 1 (errors.As): DNS Error detected\n")
			fmt.Printf("  DNS Error details: %+v\n", dnsErr)
		} else {
			fmt.Printf("✗ Method 1 (errors.As): NOT a DNS Error\n")
		}

		// Method 2: String matching
		if strings.Contains(err.Error(), "no such host") {
			fmt.Printf("✓ Method 2 (string match 'no such host'): DNS Error detected\n")
		} else {
			fmt.Printf("✗ Method 2 (string match 'no such host'): NOT detected\n")
		}

		// Method 3: Check for lookup
		if strings.Contains(err.Error(), "lookup") {
			fmt.Printf("✓ Method 3 (string match 'lookup'): Lookup issue detected\n")
		} else {
			fmt.Printf("✗ Method 3 (string match 'lookup'): NOT detected\n")
		}

		// Unwrap and check
		fmt.Printf("\nUnwrapping error chain:\n")
		unwrapped := err
		depth := 0
		for unwrapped != nil {
			fmt.Printf("  [%d] Type: %T, Value: %v\n", depth, unwrapped, unwrapped)

			// Check if this level is DNSError
			if dnsErr, ok := unwrapped.(*net.DNSError); ok {
				fmt.Printf("      ✓ Found DNSError at depth %d: %+v\n", depth, dnsErr)
			}

			unwrapped = errors.Unwrap(unwrapped)
			depth++
			if depth > 10 {
				fmt.Println("      (stopping after 10 levels)")
				break
			}
		}

		return
	}
	defer resp.Body.Close()
	fmt.Printf("✓ URL %s returned status: %d\n", url, resp.StatusCode)
}

func main() {
	fmt.Println("Testing DNS Error Detection")
	fmt.Println("============================")

	// Test with non-existent domain
	checkDNSError("https://nonexisting.host.invalid.example.com")

	// Test with another non-existent domain
	checkDNSError("https://this-domain-does-not-exist-12345.com")

	// Test with valid domain (should succeed)
	fmt.Println("\n\n=== Testing with valid domain ===")
	checkDNSError("https://www.google.com")
}
