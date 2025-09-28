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

	"github.com/playwright-community/playwright-go"
	"github.com/jaytaylor/html2text"
)

const DEFAULT_TRUNCATE_AFTER = 100000

type FormInput struct {
	Name  string
	Value string
}

type Config struct {
	URL           string
	Profile       string
	FormID        string
	Inputs        []FormInput
	AfterSubmitURL string
	JSCode        string
	ScreenshotPath string
	TruncateAfter int
	RawFlag       bool
}

func main() {
	config := parseArgs()

	if config.URL == "" {
		printHelp()
		os.Exit(1)
	}

	// Ensure Firefox is installed
	err := ensureFirefox()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up Firefox: %v\n", err)
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
		fmt.Printf("Using cached Firefox at: %s\n", firefoxDir)
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
	
	// Get Firefox executable path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %v", err)
	}

	firefoxDir := filepath.Join(homeDir, ".web-firefox")
	var firefoxExec string
	
	switch runtime.GOOS {
	case "darwin":
		firefoxExec = filepath.Join(firefoxDir, "firefox", "Nightly.app", "Contents", "MacOS", "firefox")
	case "linux":
		firefoxExec = filepath.Join(firefoxDir, "firefox", "firefox", "firefox")
	}
	
	pw, err := playwright.Run()
	if err != nil {
		return "", fmt.Errorf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	// Create profile directory for session persistence
	profileDir := filepath.Join(homeDir, ".web-firefox", "profiles", config.Profile)
	os.MkdirAll(profileDir, 0755)

	// Launch Firefox with persistent context for session storage
	context, err := pw.Firefox.LaunchPersistentContext(profileDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:       playwright.Bool(true),
		ExecutablePath: playwright.String(firefoxExec),
	})
	if err != nil {
		return "", fmt.Errorf("could not launch Firefox with persistent context: %v", err)
	}
	defer context.Close()

	// Create new page
	page, err := context.NewPage()
	if err != nil {
		return "", fmt.Errorf("could not create page: %v", err)
	}

	// Set up console message listener
	var consoleMessages []string
	page.On("console", func(msg playwright.ConsoleMessage) {
		consoleMessages = append(consoleMessages, fmt.Sprintf("[%s] %s", strings.ToUpper(msg.Type()), msg.Text()))
	})

	// Navigate to page
	_, err = page.Goto(baseURL)
	if err != nil {
		return "", fmt.Errorf("could not navigate to %s: %v", baseURL, err)
	}

	// Detect LiveView pages
	isLiveView, err := page.Evaluate(`document.querySelector("[data-phx-session]") !== null`)
	if err != nil {
		isLiveView = false
	}
	
	if isLiveView.(bool) {
		fmt.Println("Detected Phoenix LiveView page, waiting for connection...")
		// Wait for Phoenix LiveView to connect
		_, err = page.WaitForSelector(".phx-connected", playwright.PageWaitForSelectorOptions{
			Timeout: playwright.Float(10000), // 10 seconds
		})
		if err != nil {
			fmt.Printf("Warning: Could not detect LiveView connection: %v\n", err)
		} else {
			fmt.Println("Phoenix LiveView connected")
		}
	}

	// Handle form submission if specified
	if config.FormID != "" && len(config.Inputs) > 0 {
		err = handleForm(page, config, isLiveView.(bool))
		if err != nil {
			return "", fmt.Errorf("error handling form: %v", err)
		}
	}

	// Execute JavaScript if provided
	if config.JSCode != "" {
		_, err = page.Evaluate(config.JSCode)
		if err != nil {
			fmt.Printf("Warning: JavaScript execution failed: %v\n", err)
		}
	}

	// Take screenshot if requested
	if config.ScreenshotPath != "" {
		_, err = page.Screenshot(playwright.PageScreenshotOptions{
			Path:     &config.ScreenshotPath,
			FullPage: playwright.Bool(true),
		})
		if err != nil {
			return "", fmt.Errorf("error taking screenshot: %v", err)
		}
		fmt.Printf("Screenshot saved to %s\n", config.ScreenshotPath)
	}

	// Navigate to after-submit URL if provided
	if config.AfterSubmitURL != "" {
		fmt.Printf("Navigating to after-submit URL: %s\n", config.AfterSubmitURL)
		_, err = page.Goto(config.AfterSubmitURL)
		if err != nil {
			return "", fmt.Errorf("could not navigate to after-submit URL: %v", err)
		}
	}

	// Get page content
	content, err := page.Content()
	if err != nil {
		return "", fmt.Errorf("could not get page content: %v", err)
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

func handleForm(page playwright.Page, config Config, isLiveView bool) error {
	// Fill form inputs
	for _, input := range config.Inputs {
		selector := fmt.Sprintf("#%s input[name='%s']", config.FormID, input.Name)
		err := page.Fill(selector, input.Value)
		if err != nil {
			return fmt.Errorf("could not fill input %s: %v", input.Name, err)
		}
	}

	if isLiveView {
		// For LiveView, submit form and wait for loading states
		formSelector := fmt.Sprintf("#%s", config.FormID)
		
		// Submit the form
		err := page.Locator(formSelector).Press("Enter")
		if err != nil {
			return fmt.Errorf("could not submit LiveView form: %v", err)
		}

		// Wait for LiveView loading states to complete
		_, err = page.WaitForFunction("() => !document.querySelector('.phx-submit-loading')", playwright.PageWaitForFunctionOptions{
			Timeout: playwright.Float(10000),
		})
		if err != nil {
			fmt.Printf("Warning: Could not wait for submit loading: %v\n", err)
		}
		_, err = page.WaitForFunction("() => !document.querySelector('.phx-change-loading')", playwright.PageWaitForFunctionOptions{
			Timeout: playwright.Float(5000),
		})
		if err != nil {
			fmt.Printf("Warning: Could not wait for change loading: %v\n", err)
		}
		
		fmt.Println("LiveView form submitted and loading completed")
	} else {
		// For regular forms, click submit button or press enter
		submitSelector := fmt.Sprintf("#%s input[type='submit'], #%s button[type='submit']", config.FormID, config.FormID)
		err := page.Click(submitSelector)
		if err != nil {
			// Try pressing Enter on the form if no submit button
			formSelector := fmt.Sprintf("#%s", config.FormID)
			err = page.Locator(formSelector).Press("Enter")
			if err != nil {
				return fmt.Errorf("could not submit form: %v", err)
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
