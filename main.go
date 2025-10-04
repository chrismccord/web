package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jaytaylor/html2text"
	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/log"
)

const DEFAULT_TRUNCATE_AFTER = 100000

type FormInput struct {
	Name  string
	Value string
}

type Config struct {
	URL            string
	Profile        string
	FormID         string
	Inputs         []FormInput
	AfterSubmitURL string
	JSCode         string
	ScreenshotPath string
	TruncateAfter  int
	RawFlag        bool
}

func main() {
	config := parseArgs()

	if config.URL == "" {
		printHelp()
		os.Exit(1)
	}

	// Ensure Firefox and geckodriver are installed
	err := ensureFirefox()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up Firefox: %v\n", err)
		os.Exit(1)
	}

	err = ensureGeckodriver()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up geckodriver: %v\n", err)
		os.Exit(1)
	}

	// Process the request
	result, err := processRequest(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing request: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}

func ensureFirefox() error {
	// Get home directory for our isolated Firefox installation
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home directory: %v", err)
	}

	firefoxDir := filepath.Join(homeDir, ".web-firefox")

	// Platform-specific Firefox paths and URLs
	var firefoxExec string
	var firefoxUrl string
	var firefoxSubdir string

	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			firefoxSubdir = "firefox"
			firefoxExec = filepath.Join(firefoxDir, firefoxSubdir, "Nightly.app", "Contents", "MacOS", "firefox")
			firefoxUrl = "https://playwright.azureedge.net/builds/firefox/1482/firefox-mac-arm64.zip"
		} else {
			firefoxSubdir = "firefox"
			firefoxExec = filepath.Join(firefoxDir, firefoxSubdir, "Nightly.app", "Contents", "MacOS", "firefox")
			firefoxUrl = "https://playwright.azureedge.net/builds/firefox/1482/firefox-mac.zip"
		}
	case "linux":
		firefoxSubdir = "firefox"
		firefoxExec = filepath.Join(firefoxDir, firefoxSubdir, "firefox", "firefox")
		firefoxUrl = "https://playwright.azureedge.net/builds/firefox/1482/firefox-linux.zip"
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	// Check if Firefox executable exists
	if _, err := os.Stat(firefoxExec); err == nil {
		return nil
	}

	// Download and extract Firefox
	fmt.Println("Firefox not found, downloading...")
	err = downloadFirefox(firefoxUrl, firefoxDir)
	if err != nil {
		return fmt.Errorf("failed to download Firefox: %v", err)
	}

	// Verify the executable exists after download
	if _, err := os.Stat(firefoxExec); err != nil {
		return fmt.Errorf("Firefox executable not found after download: %s", firefoxExec)
	}

	fmt.Printf("Firefox downloaded to: %s\n", firefoxDir)
	return nil
}

func ensureGeckodriver() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home directory: %v", err)
	}

	geckoDir := filepath.Join(homeDir, ".web-firefox", "geckodriver")
	var geckoExec string
	var geckoUrl string

	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			geckoExec = filepath.Join(geckoDir, "geckodriver")
			geckoUrl = "https://github.com/mozilla/geckodriver/releases/download/v0.35.0/geckodriver-v0.35.0-macos-aarch64.tar.gz"
		} else {
			geckoExec = filepath.Join(geckoDir, "geckodriver")
			geckoUrl = "https://github.com/mozilla/geckodriver/releases/download/v0.35.0/geckodriver-v0.35.0-macos.tar.gz"
		}
	case "linux":
		geckoExec = filepath.Join(geckoDir, "geckodriver")
		geckoUrl = "https://github.com/mozilla/geckodriver/releases/download/v0.35.0/geckodriver-v0.35.0-linux64.tar.gz"
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	// Check if geckodriver exists
	if _, err := os.Stat(geckoExec); err == nil {
		return nil
	}

	// Download and extract geckodriver
	fmt.Println("Geckodriver not found, downloading...")
	err = downloadAndExtractTarGz(geckoUrl, geckoDir)
	if err != nil {
		return fmt.Errorf("failed to download geckodriver: %v", err)
	}

	// Make executable
	if err := os.Chmod(geckoExec, 0755); err != nil {
		return fmt.Errorf("failed to make geckodriver executable: %v", err)
	}

	fmt.Printf("Geckodriver downloaded to: %s\n", geckoDir)
	return nil
}

func downloadAndExtractTarGz(url, destDir string) error {
	// Create destination directory
	err := os.MkdirAll(destDir, 0755)
	if err != nil {
		return fmt.Errorf("could not create directory %s: %v", destDir, err)
	}

	// Download the tar.gz file
	fmt.Printf("Downloading from %s...\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("could not download: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create temporary file
	tempFile, err := os.CreateTemp("", "geckodriver-*.tar.gz")
	if err != nil {
		return fmt.Errorf("could not create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy download to temp file
	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		return fmt.Errorf("could not save download: %v", err)
	}

	tempFile.Close()

	// Extract using tar command
	fmt.Println("Extracting geckodriver...")
	return extractTarGz(tempFile.Name(), destDir)
}

func extractTarGz(src, dest string) error {
	// Use system tar command for simplicity
	cmd := fmt.Sprintf("tar -xzf %s -C %s", src, dest)
	if err := runCommand(cmd); err != nil {
		return fmt.Errorf("failed to extract tar.gz: %v", err)
	}
	return nil
}

func runCommand(cmd string) error {
	// Simple command execution
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	proc := &os.Process{}
	attr := &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}

	// Find the executable
	executable, err := findExecutable(parts[0])
	if err != nil {
		return err
	}

	proc, err = os.StartProcess(executable, parts, attr)
	if err != nil {
		return err
	}

	state, err := proc.Wait()
	if err != nil {
		return err
	}

	if !state.Success() {
		return fmt.Errorf("command failed: %s", cmd)
	}

	return nil
}

func findExecutable(name string) (string, error) {
	// Simple path search
	paths := []string{"/bin", "/usr/bin", "/usr/local/bin"}
	for _, dir := range paths {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("executable not found: %s", name)
}

func downloadFirefox(url, destDir string) error {
	// Create destination directory
	err := os.MkdirAll(destDir, 0755)
	if err != nil {
		return fmt.Errorf("could not create directory %s: %v", destDir, err)
	}

	// Download the zip file
	fmt.Printf("Downloading Firefox from %s...\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("could not download Firefox: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create temporary file
	tempFile, err := os.CreateTemp("", "firefox-*.zip")
	if err != nil {
		return fmt.Errorf("could not create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy download to temp file
	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		return fmt.Errorf("could not save download: %v", err)
	}

	tempFile.Close()

	// Extract the zip file
	fmt.Println("Extracting Firefox...")
	return extractZip(tempFile.Name(), destDir)
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	// Create destination directory
	os.MkdirAll(dest, 0755)

	// Extract files
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}

		path := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.FileInfo().Mode())
			rc.Close()
			continue
		}

		// Create directories for file
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			rc.Close()
			return err
		}

		// Create the file
		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.FileInfo().Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

func processRequest(config Config) (string, error) {
	baseURL := ensureProtocol(config.URL)

	// Get Firefox and geckodriver paths
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %v", err)
	}

	firefoxDir := filepath.Join(homeDir, ".web-firefox")
	geckoDriverPath := filepath.Join(firefoxDir, "geckodriver", "geckodriver")

	var firefoxExec string
	switch runtime.GOOS {
	case "darwin":
		firefoxExec = filepath.Join(firefoxDir, "firefox", "Nightly.app", "Contents", "MacOS", "firefox")
	case "linux":
		firefoxExec = filepath.Join(firefoxDir, "firefox", "firefox", "firefox")
	}

	// Start geckodriver service
	service, err := selenium.NewGeckoDriverService(geckoDriverPath, 4444)
	if err != nil {
		return "", fmt.Errorf("could not start geckodriver service: %v", err)
	}
	defer service.Stop()

	// Configure Firefox with profile
	profileDir := filepath.Join(homeDir, ".web-firefox", "profiles", config.Profile)
	os.MkdirAll(profileDir, 0755)

	caps := selenium.Capabilities{
		"browserName": "firefox",
		"moz:firefoxOptions": map[string]interface{}{
			"binary": firefoxExec,
			"args":   []string{"-headless", "-profile", profileDir},
			"prefs": map[string]interface{}{
				"devtools.console.stdout.content": true,
			},
			"log": map[string]interface{}{
				"level": "trace",
			},
		},
	}

	// Create WebDriver
	wd, err := selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d", 4444))
	if err != nil {
		return "", fmt.Errorf("could not create webdriver: %v", err)
	}
	defer wd.Quit()

	// Navigate to page
	if err := wd.Get(baseURL); err != nil {
		return "", fmt.Errorf("could not navigate to %s: %v", baseURL, err)
	}

	// Inject console capture script
	_, err = wd.ExecuteScript(`
		if (!window.__consoleMessages) {
			window.__consoleMessages = [];
			['log', 'warn', 'error', 'info', 'debug'].forEach(function(method) {
				var original = console[method];
				console[method] = function() {
					var args = Array.prototype.slice.call(arguments);
					var message = args.map(function(arg) {
						if (typeof arg === 'object') {
							try { return JSON.stringify(arg); }
							catch(e) { return String(arg); }
						}
						return String(arg);
					}).join(' ');
					window.__consoleMessages.push({
						level: method,
						message: message
					});
					original.apply(console, arguments);
				};
			});
		}
	`, nil)
	if err != nil {
		fmt.Printf("Warning: Could not inject console capture: %v\n", err)
	}

	// Detect LiveView pages
	isLiveView, err := wd.ExecuteScript("return document.querySelector('[data-phx-session]') !== null", nil)
	if err != nil {
		isLiveView = false
	}

	if isLiveView.(bool) {
		fmt.Println("Detected Phoenix LiveView page, waiting for connection...")
		// Wait for Phoenix LiveView to connect
		err = waitForSelector(wd, ".phx-connected", 10*time.Second)
		if err != nil {
			fmt.Printf("Warning: Could not detect LiveView connection: %v\n", err)
		} else {
			fmt.Println("Phoenix LiveView connected")
		}

		// Set up navigation tracking using Phoenix events for all page interactions
		_, err = wd.ExecuteScript(`
			if (!window.__phxNavigationState) {
				window.__phxNavigationState = { loading: false };
				document.addEventListener('phx:page-loading-start', function() {
					window.__phxNavigationState.loading = true;
				});
				document.addEventListener('phx:page-loading-stop', function() {
					window.__phxNavigationState.loading = false;
				});
			}
		`, nil)
		if err != nil {
			fmt.Printf("Warning: Could not inject Phoenix navigation listeners: %v\n", err)
		}
	}

	// Handle form submission if specified
	if config.FormID != "" && len(config.Inputs) > 0 {
		err = handleForm(wd, config, isLiveView.(bool))
		if err != nil {
			return "", fmt.Errorf("error handling form: %v", err)
		}
	}

	// Execute JavaScript if provided
	if config.JSCode != "" {
		// Store current URL before executing JS
		currentURL, _ := wd.CurrentURL()

		_, err = wd.ExecuteScript(config.JSCode, nil)
		if err != nil {
			fmt.Printf("Warning: JavaScript execution failed: %v\n", err)
		}

		// Wait for navigation based on page type
		if isLiveView.(bool) {
			// For LiveView pages, wait for navigation using Phoenix events
			fmt.Println("Waiting for Phoenix LiveView navigation...")

			// First, wait briefly for loading to potentially start
			time.Sleep(100 * time.Millisecond)

			// Check if navigation started
			err = waitForFunction(wd, "return window.__phxNavigationState && window.__phxNavigationState.loading === true", 1*time.Second)
			if err != nil {
				// No navigation event detected, check if URL changed
				newURL, _ := wd.CurrentURL()
				if newURL != currentURL {
					fmt.Println("URL changed, waiting for page to stabilize...")
					time.Sleep(500 * time.Millisecond)
				} else {
					fmt.Println("Info: No navigation detected (in-place LiveView update)")
				}
			} else {
				// Navigation started, wait for it to complete
				err = waitForFunction(wd, "return window.__phxNavigationState && window.__phxNavigationState.loading === false", 10*time.Second)
				if err != nil {
					fmt.Printf("Warning: Navigation did not complete within timeout: %v\n", err)
				} else {
					fmt.Println("Phoenix LiveView navigation completed")
				}
			}
		} else {
			// For non-LiveView pages, wait for traditional navigation
			fmt.Println("Waiting for page navigation...")

			// Brief delay to allow navigation to start
			time.Sleep(200 * time.Millisecond)

			// Wait for URL to change or timeout
			navigationOccurred := false
			err = wd.WaitWithTimeout(func(wd selenium.WebDriver) (bool, error) {
				newURL, err := wd.CurrentURL()
				if err != nil {
					return false, nil
				}
				if newURL != currentURL {
					navigationOccurred = true
					return true, nil
				}
				return false, nil
			}, 5*time.Second)

			if navigationOccurred {
				// Wait for page to be fully loaded
				fmt.Println("Navigation detected, waiting for page load...")
				err = waitForFunction(wd, "return document.readyState === 'complete'", 5*time.Second)
				if err != nil {
					fmt.Printf("Warning: Page load wait timed out: %v\n", err)
				} else {
					fmt.Println("Page load completed")
				}
			} else {
				fmt.Println("Info: No navigation detected (page update without URL change)")
			}
		}
	}

	// Take screenshot if requested
	if config.ScreenshotPath != "" {
		screenshot, err := wd.Screenshot()
		if err != nil {
			return "", fmt.Errorf("error taking screenshot: %v", err)
		}
		err = os.WriteFile(config.ScreenshotPath, screenshot, 0644)
		if err != nil {
			return "", fmt.Errorf("error saving screenshot: %v", err)
		}
		fmt.Printf("Screenshot saved to %s\n", config.ScreenshotPath)
	}

	// Navigate to after-submit URL if provided
	if config.AfterSubmitURL != "" {
		fmt.Printf("Navigating to after-submit URL: %s\n", config.AfterSubmitURL)
		if err := wd.Get(config.AfterSubmitURL); err != nil {
			return "", fmt.Errorf("could not navigate to after-submit URL: %v", err)
		}
	}

	// Get page content
	content, err := wd.PageSource()
	if err != nil {
		return "", fmt.Errorf("could not get page content: %v", err)
	}

	// Collect ALL logs: console logs (console.log/warn/error) AND browser logs (JS errors, network errors)
	var consoleMessages []string

	// 1. Collect console.log/warn/error messages from our injected capture
	capturedLogs, err := wd.ExecuteScript("return window.__consoleMessages || []", nil)
	if err == nil {
		if logArray, ok := capturedLogs.([]interface{}); ok {
			for _, logEntry := range logArray {
				if logMap, ok := logEntry.(map[string]interface{}); ok {
					level := "LOG"
					if lvl, ok := logMap["level"].(string); ok {
						level = strings.ToUpper(lvl)
						// Normalize 'warn' to 'warning' to match expected format
						if level == "WARN" {
							level = "WARNING"
						}
					}
					message := ""
					if msg, ok := logMap["message"].(string); ok {
						message = msg
					}
					consoleMessages = append(consoleMessages, fmt.Sprintf("[%s] %s", level, message))
				}
			}
		}
	}

	// 2. Collect browser logs (JavaScript errors, security errors, network errors, etc.)
	browserLogs, err := wd.Log(log.Browser)
	if err == nil {
		for _, logEntry := range browserLogs {
			level := strings.ToUpper(string(logEntry.Level))
			// Only include WARN, ERROR, SEVERE logs from browser to avoid noise
			if level == "WARNING" || level == "WARN" || level == "ERROR" || level == "SEVERE" {
				consoleMessages = append(consoleMessages, fmt.Sprintf("[%s] %s", level, logEntry.Message))
			}
		}
	}

	// Return raw HTML if requested
	if config.RawFlag {
		return content, nil
	}

	// Convert HTML to markdown
	text, err := html2text.FromString(content)
	if err != nil {
		return "", fmt.Errorf("could not convert HTML to text: %v", err)
	}

	// Clean and format the markdown
	markdown := cleanMarkdown(text)

	// Truncate if specified
	if len(markdown) > config.TruncateAfter {
		markdown = markdown[:config.TruncateAfter] + fmt.Sprintf("\n\n... (output truncated after %d chars, full content was %d chars)", config.TruncateAfter, len(text))
	}

	// Add header with URL and console messages
	result := fmt.Sprintf("==========================\n%s\n==========================\n\n%s", baseURL, markdown)

	// Add console messages if any
	if len(consoleMessages) > 0 {
		result += "\n\n" + strings.Repeat("=", 50) + "\nCONSOLE OUTPUT:\n" + strings.Repeat("=", 50) + "\n"
		for _, msg := range consoleMessages {
			result += msg + "\n"
		}
	}

	return result, nil
}

// waitForSelector waits for an element matching the selector to appear
func waitForSelector(wd selenium.WebDriver, selector string, timeout time.Duration) error {
	return wd.WaitWithTimeout(func(wd selenium.WebDriver) (bool, error) {
		_, err := wd.FindElement(selenium.ByCSSSelector, selector)
		return err == nil, nil
	}, timeout)
}

// waitForFunction waits for a JavaScript condition to be true
func waitForFunction(wd selenium.WebDriver, jsCode string, timeout time.Duration) error {
	return wd.WaitWithTimeout(func(wd selenium.WebDriver) (bool, error) {
		result, err := wd.ExecuteScript(jsCode, nil)
		if err != nil {
			return false, nil
		}
		if boolResult, ok := result.(bool); ok {
			return boolResult, nil
		}
		return false, nil
	}, timeout)
}

func handleForm(wd selenium.WebDriver, config Config, isLiveView bool) error {
	// Fill form inputs
	for _, input := range config.Inputs {
		selector := fmt.Sprintf("#%s input[name='%s']", config.FormID, input.Name)
		elem, err := wd.FindElement(selenium.ByCSSSelector, selector)
		if err != nil {
			return fmt.Errorf("could not find input %s: %v", input.Name, err)
		}
		if err := elem.Clear(); err != nil {
			return fmt.Errorf("could not clear input %s: %v", input.Name, err)
		}
		if err := elem.SendKeys(input.Value); err != nil {
			return fmt.Errorf("could not fill input %s: %v", input.Name, err)
		}
	}

	if isLiveView {
		// For LiveView, use Phoenix event-based navigation tracking
		formSelector := fmt.Sprintf("#%s", config.FormID)
		formElem, err := wd.FindElement(selenium.ByCSSSelector, formSelector)
		if err != nil {
			return fmt.Errorf("could not find LiveView form: %v", err)
		}

		// Submit the form by pressing Enter
		if err := formElem.SendKeys(selenium.EnterKey); err != nil {
			return fmt.Errorf("could not submit LiveView form: %v", err)
		}

		// Wait for Phoenix navigation to complete (phx:page-loading-start -> phx:page-loading-stop)
		fmt.Println("Waiting for Phoenix LiveView navigation...")

		// First, wait for loading to start (with short timeout)
		err = waitForFunction(wd, "return window.__phxNavigationState && window.__phxNavigationState.loading === true", 2*time.Second)
		if err != nil {
			fmt.Printf("Info: No navigation detected (this is normal for in-place updates)\n")
		} else {
			// If navigation started, wait for it to complete
			err = waitForFunction(wd, "return window.__phxNavigationState && window.__phxNavigationState.loading === false", 10*time.Second)
			if err != nil {
				fmt.Printf("Warning: Navigation did not complete within timeout: %v\n", err)
			} else {
				fmt.Println("Phoenix LiveView navigation completed")
			}
		}

		fmt.Println("LiveView form submitted")
	} else {
		// For regular forms, click submit button or press enter
		submitSelector := fmt.Sprintf("#%s input[type='submit'], #%s button[type='submit']", config.FormID, config.FormID)
		elem, err := wd.FindElement(selenium.ByCSSSelector, submitSelector)
		if err != nil {
			// Try pressing Enter on the form if no submit button
			formSelector := fmt.Sprintf("#%s", config.FormID)
			formElem, err := wd.FindElement(selenium.ByCSSSelector, formSelector)
			if err != nil {
				return fmt.Errorf("could not submit form: %v", err)
			}
			if err := formElem.SendKeys(selenium.EnterKey); err != nil {
				return fmt.Errorf("could not submit form: %v", err)
			}
		} else {
			if err := elem.Click(); err != nil {
				return fmt.Errorf("could not click submit button: %v", err)
			}
		}
		fmt.Println("Form submitted")
	}

	return nil
}

func parseArgs() Config {
	config := Config{
		TruncateAfter: DEFAULT_TRUNCATE_AFTER,
		Profile:       "default",
	}

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "--help":
			printHelp()
			os.Exit(0)
		case "--raw":
			config.RawFlag = true
		case "--truncate-after":
			if i+1 < len(args) {
				val, err := strconv.Atoi(args[i+1])
				if err == nil && val > 0 {
					config.TruncateAfter = val
				}
				i++
			}
		case "--screenshot":
			if i+1 < len(args) {
				config.ScreenshotPath = args[i+1]
				i++
			}
		case "--form":
			if i+1 < len(args) {
				config.FormID = args[i+1]
				i++
			}
		case "--input":
			if i+1 < len(args) {
				name := args[i+1]
				i++
				if i+1 < len(args) && args[i+1] == "--value" {
					i++
					if i+1 < len(args) {
						value := args[i+1]
						config.Inputs = append(config.Inputs, FormInput{Name: name, Value: value})
						i++
					}
				}
			}
		case "--value":
			// Skip, handled with --input
		case "--after-submit":
			if i+1 < len(args) {
				config.AfterSubmitURL = ensureProtocol(args[i+1])
				i++
			}
		case "--js":
			if i+1 < len(args) {
				config.JSCode = args[i+1]
				i++
			}
		case "--profile":
			if i+1 < len(args) {
				config.Profile = args[i+1]
				i++
			}
		default:
			if config.URL == "" && !strings.HasPrefix(arg, "--") {
				config.URL = arg
			}
		}
	}

	return config
}

func printHelp() {
	fmt.Printf(`web - portable web scraper for llms

Usage: web <url> [options]

Options:
  --help                     Show this help message
  --raw                      Output raw page instead of converting to markdown
  --truncate-after <number>  Truncate output after <number> characters and append a notice (default: %d)
  --screenshot <filepath>    Take a screenshot of the page and save it to the given filepath
  --form <id>                The id of the form for inputs
  --input <name>             Specify the name attribute for a form input field
  --value <value>            Provide the value to fill for the last --input field
  --after-submit <url>       After form submission and navigation, load this URL before converting to markdown
  --js <code>                Execute JavaScript code on the page after it loads
  --profile <name>           Use or create named session profile (default: "default")

Phoenix LiveView Support:
This tool automatically detects Phoenix LiveView applications and properly handles:
- Connection waiting (.phx-connected)
- Form submissions with loading states
- State management between interactions

Examples:
  web https://example.com
  web https://example.com --screenshot page.png --truncate-after 5000
  web localhost:4000/login --form login_form --input email --value test@example.com --input password --value secret
`, DEFAULT_TRUNCATE_AFTER)
}

// Ensure URL has protocol
func ensureProtocol(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "http://" + url
	}
	return url
}

// Clean markdown
func cleanMarkdown(markdown string) string {
	// Format headers properly
	markdown = strings.ReplaceAll(markdown, "\n# ", "\n# ")
	markdown = strings.ReplaceAll(markdown, "\n## ", "\n## ")
	markdown = strings.ReplaceAll(markdown, "\n### ", "\n### ")

	// Collapse multiple blank lines
	for strings.Contains(markdown, "\n\n\n") {
		markdown = strings.ReplaceAll(markdown, "\n\n\n", "\n\n")
	}

	// Normalize list bullets
	lines := strings.Split(markdown, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "- ") {
			lines[i] = "- " + strings.TrimPrefix(strings.TrimPrefix(line, "* "), "- ")
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}
