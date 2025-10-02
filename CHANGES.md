# Web Tool Enhancements

## Summary of Changes

This PR adds two major features to the web tool to better support Phoenix authentication workflows and JavaScript-triggered navigation:

## Why These Changes Were Needed

### The Problem

**Phoenix's `phx.gen.auth` uses button attributes instead of hidden inputs:**

```html
<!-- Traditional approach (already supported by web tool) -->
<input type="hidden" name="user[remember_me]" value="true">
<button type="submit">Log in</button>

<!-- Phoenix approach (NOT supported before these changes) -->
<button name="user[remember_me]" value="true">Keep me logged in</button>
<button type="submit">Log in only this time</button>
```

**Before these changes**, the web tool could only:
- Fill text inputs via `--input`
- Click generic submit buttons (no way to choose which button)
- Execute JavaScript via `--js`, but would capture page state immediately (before navigation completed)

**This meant:**
1. **No way to click a specific button** when multiple buttons exist in a form
2. **JavaScript workarounds failed** because navigation happened after the page was captured
3. **Session persistence broken** because remember_me button couldn't be clicked properly

## The Solution

### 1. Button Click Support (`--button`)

Added ability to click specific form buttons by name and value attributes, which is essential for Phoenix forms that use button attributes instead of hidden inputs.

**New Parameters:**
- `--button <name>` - Specify the name attribute of the button to click
- `--button <name> --value <value>` - Click button with specific name AND value

**Implementation Details:**
- CSS selector escaping for brackets in attribute names (e.g., `user[remember_me]` → `user\[remember_me\]`)
- Form handler now triggers with button-only submissions (not just inputs)
- Automatic navigation wait after form submission

### 2. Navigation Wait Support (`--wait-for-navigation`)

Added ability to wait for navigation after JavaScript execution, solving the issue where JavaScript triggers form submissions or redirects.

**New Parameter:**
- `--wait-for-navigation [timeout_ms]` - Wait for URL change after JavaScript (default: 5000ms)

**Implementation Details:**
- Detects URL changes after JavaScript execution
- Waits for `document.readyState === 'complete'` after navigation
- Configurable timeout with sensible defaults

## Use Case: Phoenix Magic Link Authentication

The primary use case is Phoenix's `phx.gen.auth` magic link login flow, which requires:
1. Submitting email to request magic link
2. Clicking magic link to authenticate
3. **Clicking a button with `name="user[remember_me]" value="true"`** to persist session

### Complete Login Workflow Example

```bash
# Step 1: Request magic link via email
web http://localhost:4000/users/log-in \
  --profile "demo1" \
  --form "login_form_magic" \
  --input "user[email]" --value "demo1@example.com"

# Step 2: Extract magic link from development mailbox
MAGIC_LINK=$(web http://localhost:4000/dev/mailbox \
  --profile "demo1" \
  --raw | grep -o 'http://localhost:4000/users/log-in/[^"<]*' | tail -1)

# Step 3: Complete login with remember_me cookie
web "$MAGIC_LINK" \
  --profile "demo1" \
  --form "login_form" \
  --button "user[remember_me]" --value "true"

# Step 4: Access protected pages with persistent session
web http://localhost:4000/conversations --profile "demo1"
```

### Alternative: Using JavaScript with Navigation Wait

```bash
# Submit form with JavaScript and wait for navigation
web "$MAGIC_LINK" \
  --profile "demo1" \
  --js "setTimeout(() => {
    const form = document.getElementById('login_form');
    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = 'user[remember_me]';
    input.value = 'true';
    form.appendChild(input);
    form.submit();
  }, 1000)" \
  --wait-for-navigation 5000
```

## Technical Changes

### Modified Files
- `main.go` - All changes in single file

### Key Code Changes

1. **Config struct** - Added button and navigation fields:
```go
type Config struct {
    // ... existing fields
    ButtonName          string
    ButtonValue         string
    WaitForNavigation   bool
    NavigationTimeout   int
}
```

2. **Form handler** - Button click support:
```go
// If button name/value specified, try to find that specific button
if config.ButtonName != "" {
    // Escape brackets in CSS selector
    escapedName := strings.ReplaceAll(strings.ReplaceAll(config.ButtonName, "[", "\\["), "]", "\\]")
    buttonSelector := fmt.Sprintf("#%s button[name='%s'][value='%s']",
        config.FormID, escapedName, config.ButtonValue)
    elem, err = wd.FindElement(selenium.ByCSSSelector, buttonSelector)
    // ... click button
}
```

3. **JavaScript handler** - Navigation wait:
```go
if config.WaitForNavigation {
    currentURL, _ := wd.CurrentURL()

    // Wait for URL to change
    err = wd.WaitWithTimeout(func(wd selenium.WebDriver) (bool, error) {
        newURL, _ := wd.CurrentURL()
        return newURL != currentURL, nil
    }, timeout)

    // Wait for page load completion
    waitForFunction(wd, "return document.readyState === 'complete'", 3*time.Second)
}
```

4. **Form submission** - Auto navigation wait:
```go
// After button click, automatically wait for navigation
if err := elem.Click(); err != nil {
    return fmt.Errorf("could not click submit button: %v", err)
}

// Wait for navigation after form submission
time.Sleep(500 * time.Millisecond)
currentURL, _ := wd.CurrentURL()
err = wd.WaitWithTimeout(func(wd selenium.WebDriver) (bool, error) {
    newURL, _ := wd.CurrentURL()
    return newURL != currentURL, nil
}, 5*time.Second)
```

5. **Arg parsing** - New flags:
```go
case "--button":
    if i+1 < len(args) {
        config.ButtonName = args[i+1]
        i++
        if i+1 < len(args) && args[i+1] == "--value" {
            i++
            if i+1 < len(args) {
                config.ButtonValue = args[i+1]
                i++
            }
        }
    }

case "--wait-for-navigation":
    config.WaitForNavigation = true
    if i+1 < len(args) {
        if val, err := strconv.Atoi(args[i+1]); err == nil && val > 0 {
            config.NavigationTimeout = val
            i++
        }
    }
```

## Updated Help Text

```
Options:
  --help                        Show this help message
  --raw                         Output raw page instead of converting to markdown
  --truncate-after <number>     Truncate output after <number> characters and append a notice (default: 100000)
  --screenshot <filepath>       Take a screenshot of the page and save it to the given filepath
  --form <id>                   The id of the form for inputs
  --input <name>                Specify the name attribute for a form input field
  --value <value>               Provide the value to fill for the last --input field
  --button <name>               Specify the name attribute of the button to click (optional --value to match specific value)
  --after-submit <url>          After form submission and navigation, load this URL before converting to markdown
  --js <code>                   Execute JavaScript code on the page after it loads
  --wait-for-navigation [ms]    Wait for navigation after JavaScript execution (default timeout: 5000ms)
  --profile <name>              Use or create named session profile (default: "default")

Examples:
  web https://example.com
  web https://example.com --screenshot page.png --truncate-after 5000
  web localhost:4000/login --form login_form --input email --value test@example.com --input password --value secret
  web localhost:4000/confirm --form login_form --button "user[remember_me]" --value "true"
  web localhost:4000/confirm --js "document.querySelector('button').click()" --wait-for-navigation 3000
```

## Testing

### Automated Tests

Added comprehensive test coverage in `main_test.go`:

**TestButtonClick** - Validates `--button` flag:
- ✅ Creates form with multiple buttons with name/value attributes
- ✅ Clicks specific button: `--button "user[remember_me]" --value "true"`
- ✅ Verifies navigation occurs after button click
- ✅ Tests CSS selector escaping for brackets in button names

**TestWaitForNavigation** - Validates `--wait-for-navigation` flag:
- ✅ Tests JavaScript-triggered navigation with configurable timeout
- ✅ Verifies page waits for URL change and complete page load
- ✅ Tests with custom timeout: `--wait-for-navigation 3000`

All 14 tests pass successfully (~39s total).

### Manual Testing

Tested with Phoenix 1.8 application using `phx.gen.auth`:
- ✅ Magic link request and retrieval
- ✅ Button click with name/value attributes
- ✅ Session cookie persistence with `remember_me=true`
- ✅ JavaScript-triggered navigation
- ✅ Access to protected routes after authentication

## Breaking Changes

None. All changes are additive and backward compatible.

## Related Issues

This addresses common pain points when testing Phoenix authentication flows where:
1. Forms use button attributes instead of hidden inputs
2. JavaScript triggers navigation that needs to complete before capturing page state
3. Session cookies need to persist across tool invocations
