# web - shell command for simple LLM web browsing

shell-based web browser for LLMs that converts web pages to markdown, executes js, and interacts with pages.

```bash
# Convert a webpage to markdown
web https://example.com

# Take a screenshot while scraping
web https://example.com --screenshot page.png

# Execute JavaScript and capture log output along with markdown content
web https://example.com --js "console.log(document.title)"

# Fill and submit a form
web https://login.example.com \
    --form "login_form" \
    --input "username" --value "myuser" \
    --input "password" --value "mypass"
```

## Features

- **Self-contained executable** - Single native binary with no runtime dependencies
- **Auto headless firefox download** - Downloads Firefox drivers automatically on first run
- **Markdown conversion** - HTML to markdown conversion for optimized consumption by LLMs
- **JavaScript execution** - Full browser engine with arbitrary js execution
- **Phoenix LiveView support** - Detects and properly handles Phoenix LiveView applications
- **Screenshots** - Save full-page screenshots
- **Form filling** - Automated form interaction with LiveView-aware submissions
- **Session persistence** - Maintains cookies and authentication across runs with profiles

## Quick Start

1. **Build for current platform**:
   ```bash
   make              # Build ./web for your platform
   ./web https://example.com
   ```

You can then `sudo cp web /usr/local/bin` to make it available system wide

### Multi-platform Build

For releases or deployment to other systems:
```bash
make build        # Build all platforms
```
This creates:
- `web-darwin-arm64` - macOS Apple Silicon (M1/M2/M3)
- `web-darwin-amd64` - macOS Intel
- `web-linux-amd64` - Linux x86_64

## Usage Examples

```bash
# Basic scraping
web https://example.com

# Output raw HTML
web https://example.com --raw > output.html

# With truncation and screenshot
web example.com --screenshot screenshot.png --truncate-after 123

# Form submission with Phoenix LiveView support
web http://localhost:4000/users/log-in \
    --form "login_form" \
    --input "user[email]" --value "foo@bar" \
    --input "user[password]" --value "secret" \
    --after-submit "http://localhost:4000/authd/page"

# Execute JavaScript on the page
web example.com --js "document.querySelector('button').click()"

# Use named session profile
./web --profile "mysite" https://authenticated-site.com
```

## Options

```
Usage: web <url> [options]

Options:
  --help                     Show this help message
  --raw                      Output raw page instead of converting to markdown
  --truncate-after <number>  Truncate output after <number> characters and append a notice (default: 100000)
  --screenshot <filepath>    Take a screenshot of the page and save it to the given filepath
  --form <id>                The id of the form for inputs
  --input <name>             Specify the name attribute for a form input field
  --value <value>            Provide the value to fill for the last --input field
  --after-submit <url>       After form submission and navigation, load this URL before converting to markdown
  --js <code>                Execute JavaScript code on the page after it loads
  --profile <name>           Use or create named session profile (default: "default")
```

## Phoenix LiveView Support

This tool has special support for Phoenix LiveView applications:

- **Auto-detection** - Automatically detects LiveView pages via `[data-phx-session]` attribute
- **Connection waiting** - Waits for `.phx-connected` class before proceeding
- **Form handling** - Properly handles LiveView form submissions with loading states
- **State management** - Waits for `.phx-change-loading` and `.phx-submit-loading` to complete

## System Requirements

- **Linux x64 or macOS** (Ubuntu 18.04+, RHEL 7+, Debian 9+, Arch Linux, macOS 10.12+)
- **~100MB free space** (for Firefox on first run)

### Linux System Packages

On Linux, you may need to install system packages for Firefox:

```bash
# Ubuntu/Debian - Core packages for Firefox
sudo apt install libgtk-3-0 libdbus-glib-1-2 libx11-xcb1 libxcb1 libxcomposite1 libxcursor1 libxdamage1 libxi6 libxrandr2 libxss1 libxtst6 libxext6 libasound2 libatspi2.0-0 libdrm2 libxfixes3 libxrender1

# Additional packages for multimedia and fonts
sudo apt install libpulse0 libcanberra-gtk3-module packagekit-gtk3-module libdbusmenu-glib4 libdbusmenu-gtk3-4
```


## Testing

```bash
make test
```

This will build binaries for all platforms and run tests

## Development

### Available Commands

```bash
make              # Build for current platform (./web)
make build        # Build all platforms (darwin-arm64, darwin-amd64, linux-amd64)
make test         # Build and run tests
make clean        # Remove build artifacts
```

### Build Requirements

- **Go 1.21+** (for building only)

## Architecture

- **Single Go binary** - No runtime dependencies
- **Firefox engine** - Downloads Firefox drivers to `~/.web-firefox/` on first run
- **Profile isolation** - Sessions stored in `~/.web-firefox/profiles/`
- **Cross-platform** - Builds for macOS (Intel/ARM64) and Linux x86_64

## License

MIT License


