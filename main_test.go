package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	testBinary   string
	testProfile  string
	testServerURL string
	initialized  bool
	serverOnce   sync.Once
)

// startTestServer starts a local HTTP server for testing
func startTestServer() {
	serverOnce.Do(func() {
		mux := http.NewServeMux()

		// Basic HTML page
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
<h1>Test Page</h1>
<p>This is a test page for web scraping.</p>
<div id="content">Test content here</div>
</body>
</html>`)
		})

		// Page with form
		mux.HandleFunc("/form", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Test Form</title></head>
<body>
<form id="test-form">
<input name="username" type="text">
<input name="password" type="password">
<button type="submit">Submit</button>
</form>
</body>
</html>`)
		})

		// Page with LiveView simulation
		mux.HandleFunc("/liveview", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>LiveView Test</title></head>
<body>
<div data-phx-session="test-session" class="phx-connected">
<h1>LiveView Page</h1>
</div>
</body>
</html>`)
		})

		// Start server on port 9999
		go http.ListenAndServe(":9999", mux)
		testServerURL = "http://localhost:9999"

		// Give server time to start
		time.Sleep(100 * time.Millisecond)
	})
}

// setupTest ensures the binary is built and available for testing
func setupTest(t *testing.T) {
	if !initialized {
		// Detect platform and set binary name
		platform := strings.ToLower(runtime.GOOS)
		arch := runtime.GOARCH
		if arch == "amd64" {
			arch = "amd64"
		}
		testBinary = fmt.Sprintf("web-%s-%s", platform, arch)

		// Build the project if binary doesn't exist
		if _, err := os.Stat(testBinary); os.IsNotExist(err) {
			t.Logf("Building project...")
			cmd := exec.Command("./build.sh")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("Failed to build project: %v\nOutput: %s", err, output)
			}
		}

		// Verify binary exists and is executable
		if _, err := os.Stat(testBinary); err != nil {
			t.Fatalf("Binary %s not found after build", testBinary)
		}

		// Make executable
		if err := os.Chmod(testBinary, 0755); err != nil {
			t.Fatalf("Failed to make binary executable: %v", err)
		}

		// Generate unique test profile name
		testProfile = fmt.Sprintf("test-profile-%d", time.Now().UnixNano())

		t.Logf("Testing binary: %s", testBinary)
		t.Logf("Test profile: %s", testProfile)

		initialized = true
	}

	// Start test server
	startTestServer()
}

// runWeb executes the web binary with given arguments and returns stdout, stderr, and error
func runWeb(args ...string) (string, string, error) {
	cmd := exec.Command("./"+testBinary, args...)
	cmd.Env = os.Environ()
	
	stdout, err := cmd.Output()
	stderr := ""
	if exitError, ok := err.(*exec.ExitError); ok {
		stderr = string(exitError.Stderr)
	}
	
	return string(stdout), stderr, err
}

func TestBasicScraping(t *testing.T) {
	setupTest(t)

	stdout, stderr, err := runWeb(testServerURL, "--truncate-after", "500")
	if err != nil {
		t.Fatalf("Basic scraping failed: %v\nStderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "Test Page") {
		t.Errorf("Expected content not found in output")
	}

	if !strings.Contains(stdout, testServerURL) {
		t.Errorf("Expected URL header not found in output")
	}
}

func TestRawHTMLOutput(t *testing.T) {
	setupTest(t)

	stdout, stderr, err := runWeb(testServerURL, "--raw", "--truncate-after", "500")
	if err != nil {
		t.Fatalf("Raw HTML output failed: %v\nStderr: %s", err, stderr)
	}

	// Selenium's PageSource returns DOM representation without DOCTYPE
	// Check for essential HTML structure instead
	if !strings.Contains(stdout, "<html>") {
		t.Errorf("Expected HTML tag not found in raw output")
	}

	if !strings.Contains(stdout, "<title>Test Page</title>") {
		t.Errorf("Expected title tag not found in raw output")
	}

	if !strings.Contains(stdout, "<body>") {
		t.Errorf("Expected body tag not found in raw output")
	}
}

func TestJavaScriptExecution(t *testing.T) {
	setupTest(t)

	testMessage := "test-message-12345"
	stdout, stderr, err := runWeb(
		testServerURL,
		"--js", fmt.Sprintf("console.log('%s'); document.title = 'Modified Title';", testMessage),
		"--truncate-after", "300",
	)
	if err != nil {
		t.Fatalf("JavaScript execution failed: %v\nStderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, testMessage) {
		t.Errorf("Console message '%s' not found in output. Got: %s", testMessage, stdout)
	}

	if !strings.Contains(stdout, "CONSOLE OUTPUT:") {
		t.Errorf("Console output section not found")
	}
}

func TestScreenshotFunctionality(t *testing.T) {
	setupTest(t)
	
	screenshotFile := fmt.Sprintf("test-screenshot-%d.png", time.Now().UnixNano())
	defer os.Remove(screenshotFile) // Cleanup
	
	stdout, stderr, err := runWeb(
		testServerURL,
		"--screenshot", screenshotFile,
		"--truncate-after", "100",
	)
	if err != nil {
		t.Fatalf("Screenshot functionality failed: %v\nStderr: %s", err, stderr)
	}
	
	if !strings.Contains(stdout, fmt.Sprintf("Screenshot saved to %s", screenshotFile)) {
		t.Errorf("Screenshot save message not found in output")
	}
	
	// Verify file exists and has content
	info, err := os.Stat(screenshotFile)
	if err != nil {
		t.Fatalf("Screenshot file not created: %v", err)
	}
	
	if info.Size() == 0 {
		t.Errorf("Screenshot file is empty")
	}
}

func TestProfileSessionPersistence(t *testing.T) {
	setupTest(t)
	
	profile := fmt.Sprintf("test-session-%d", time.Now().UnixNano())
	testKey := "test-key"
	testValue := "test-value-12345"
	
	// Store value in localStorage
	_, stderr, err := runWeb(
		"--profile", profile,
		testServerURL,
		"--js", fmt.Sprintf("localStorage.setItem('%s', '%s'); console.log('Stored:', localStorage.getItem('%s'));", testKey, testValue, testKey),
		"--truncate-after", "100",
	)
	if err != nil {
		t.Fatalf("Failed to store value in profile: %v\nStderr: %s", err, stderr)
	}
	
	// Retrieve value from localStorage
	stdout, stderr, err := runWeb(
		"--profile", profile,
		testServerURL, 
		"--js", fmt.Sprintf("console.log('Retrieved:', localStorage.getItem('%s'));", testKey),
		"--truncate-after", "200",
	)
	if err != nil {
		t.Fatalf("Failed to retrieve value from profile: %v\nStderr: %s", err, stderr)
	}
	
	if !strings.Contains(stdout, fmt.Sprintf("Retrieved: %s", testValue)) {
		t.Errorf("Session persistence failed. Expected 'Retrieved: %s' in output. Got: %s", testValue, stdout)
	}
	
	// Cleanup
	defer func() {
		homeDir, _ := os.UserHomeDir()
		profileDir := filepath.Join(homeDir, ".web-firefox", "profiles", profile)
		os.RemoveAll(profileDir)
	}()
}

func TestProfileIsolation(t *testing.T) {
	setupTest(t)
	
	profile1 := fmt.Sprintf("test-profile1-%d", time.Now().UnixNano())
	profile2 := fmt.Sprintf("test-profile2-%d", time.Now().UnixNano())
	testKey := "isolation-test-key"
	
	// Store value in profile1
	_, stderr, err := runWeb(
		"--profile", profile1,
		testServerURL,
		"--js", fmt.Sprintf("localStorage.setItem('%s', 'profile1-value');", testKey),
		"--truncate-after", "100",
	)
	if err != nil {
		t.Fatalf("Failed to store value in profile1: %v\nStderr: %s", err, stderr)
	}
	
	// Check that profile2 doesn't see the value
	stdout, stderr, err := runWeb(
		"--profile", profile2,
		testServerURL,
		"--js", fmt.Sprintf("console.log('Profile2 sees:', localStorage.getItem('%s'));", testKey),
		"--truncate-after", "200",
	)
	if err != nil {
		t.Fatalf("Failed to check profile2: %v\nStderr: %s", err, stderr)
	}
	
	if !strings.Contains(stdout, "Profile2 sees: null") {
		t.Errorf("Profile isolation failed. Profile2 should not see profile1's data. Got: %s", stdout)
	}
	
	// Cleanup
	defer func() {
		homeDir, _ := os.UserHomeDir()
		os.RemoveAll(filepath.Join(homeDir, ".web-firefox", "profiles", profile1))
		os.RemoveAll(filepath.Join(homeDir, ".web-firefox", "profiles", profile2))
	}()
}

func TestFormHandling(t *testing.T) {
	setupTest(t)

	stdout, stderr, err := runWeb(
		testServerURL+"/form",
		"--js", `
			const form = document.querySelector('#test-form');
			console.log('Form found with', form.querySelectorAll('input').length, 'inputs');
		`,
		"--truncate-after", "300",
	)
	if err != nil {
		t.Fatalf("Form handling test failed: %v\nStderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "Form found with 2 inputs") {
		t.Errorf("Form detection failed. Expected 'Form found with 2 inputs'. Got: %s", stdout)
	}
}

func TestHelpCommand(t *testing.T) {
	t.Parallel()
	setupTest(t)
	
	stdout, stderr, err := runWeb("--help")
	if err != nil {
		t.Fatalf("Help command failed: %v\nStderr: %s", err, stderr)
	}
	
	expectedStrings := []string{
		"Usage: web",
		"--help",
		"--raw", 
		"--screenshot",
		"--js",
		"--profile",
		"Phoenix LiveView Support:",
	}
	
	for _, expected := range expectedStrings {
		if !strings.Contains(stdout, expected) {
			t.Errorf("Help output missing expected string '%s'", expected)
		}
	}
}

func TestPhoenixLiveViewDetection(t *testing.T) {
	setupTest(t)
	
	stdout, stderr, err := runWeb(
		testServerURL+"/liveview",
		"--js", `
			const div = document.querySelector('[data-phx-session]');
			if (div) {
				console.log('LiveView element detected');
			}
		`,
		"--truncate-after", "200",
	)
	if err != nil {
		t.Fatalf("Phoenix LiveView detection test failed: %v\nStderr: %s", err, stderr)
	}
	
	if !strings.Contains(stdout, "LiveView element detected") {
		t.Errorf("LiveView detection failed. Got: %s", stdout)
	}
}

func TestMultipleConsoleMessageTypes(t *testing.T) {
	setupTest(t)
	
	stdout, stderr, err := runWeb(
		testServerURL,
		"--js", `
			console.log('info message');
			console.warn('warning message'); 
			console.error('error message');
		`,
		"--truncate-after", "400",
	)
	if err != nil {
		t.Fatalf("Multiple console message types test failed: %v\nStderr: %s", err, stderr)
	}
	
	expectedMessages := []string{
		"[LOG] info message",
		"[WARNING] warning message", 
		"[ERROR] error message",
	}
	
	for _, expected := range expectedMessages {
		if !strings.Contains(stdout, expected) {
			t.Errorf("Console message '%s' not found in output", expected)
		}
	}
}

func TestContentTruncation(t *testing.T) {
	setupTest(t)
	
	stdout, stderr, err := runWeb(testServerURL, "--truncate-after", "50")
	if err != nil {
		t.Fatalf("Content truncation test failed: %v\nStderr: %s", err, stderr)
	}
	
	if !strings.Contains(stdout, "output truncated after 50 chars") {
		t.Errorf("Truncation message not found. Expected 'output truncated after 50 chars'. Got: %s", stdout)
	}
}

// TestAll runs a comprehensive test to ensure everything works together
func TestAll(t *testing.T) {
	setupTest(t)
	
	// Run a complex test combining multiple features
	screenshotFile := fmt.Sprintf("test-all-%d.png", time.Now().UnixNano())
	defer os.Remove(screenshotFile)
	
	profile := fmt.Sprintf("test-all-%d", time.Now().UnixNano())
	defer func() {
		homeDir, _ := os.UserHomeDir()
		profileDir := filepath.Join(homeDir, ".web-firefox", "profiles", profile)
		os.RemoveAll(profileDir)
	}()
	
	stdout, stderr, err := runWeb(
		"--profile", profile,
		testServerURL,
		"--screenshot", screenshotFile,
		"--js", `
			console.log('Starting comprehensive test');
			localStorage.setItem('comprehensive', 'test-passed');
			const div = document.createElement('div');
			div.innerHTML = 'Test content added';
			document.body.appendChild(div);
			console.log('Test completed successfully');
		`,
		"--truncate-after", "500",
	)
	if err != nil {
		t.Fatalf("Comprehensive test failed: %v\nStderr: %s", err, stderr)
	}
	
	// Verify multiple aspects
	checks := []string{
		"Starting comprehensive test",
		"Test completed successfully", 
		fmt.Sprintf("Screenshot saved to %s", screenshotFile),
		testServerURL,
	}
	
	for _, check := range checks {
		if !strings.Contains(stdout, check) {
			t.Errorf("Comprehensive test missing check: '%s'", check)
		}
	}
	
	// Verify screenshot was created
	if _, err := os.Stat(screenshotFile); err != nil {
		t.Errorf("Screenshot file not created in comprehensive test")
	}
}