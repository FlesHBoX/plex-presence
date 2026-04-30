// In Go, every file belongs to a package. "main" is special — it's the entry point package.
// Think of it like the root index.php or the main script node runs.
package main

import (
	// "fmt" is Go's standard formatting/printing library — like console.log() or echo
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	// "time" is the standard library for durations, sleep, etc.
	"time"

	// External package for the system tray icon — installed via go get
	"github.com/getlantern/systray"
	"github.com/xeyossr/go-discordrpc/client"
	"golang.org/x/sys/windows/registry"
)

//go:embed icon.ico
var iconData []byte

// Constants are like PHP's define() or JS's const at module level.
// These are compile-time values that won't change at runtime.
const (
	plexHost        = "http://kino:32400" // replace with your server IP
	plexToken       = "CTBRFxV5VHykhZPT6zwq"
	pollInterval    = 5 * time.Second       // how often to check Plex — 5 seconds
	discordClientID = "1499228595753582612" // Discord Application ID for Rich Presence
)

// os.Hostname() returns the machine's hostname and an error.
// If it fails for some reason we fall back to the hardcoded name.
var playerName string

func init() {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Println("Could not detect hostname, using fallback")
		playerName = "Apophis"
	} else {
		playerName = hostname
		fmt.Println("Detected hostname:", playerName)
	}
}

// main() is the entry point — like index.php or the top level of a node script.
// systray.Run() takes two functions:
//   - onReady: called when the tray is initialized (like DOMContentLoaded)
//   - onExit:  called when the tray is shutting down (like a cleanup/destructor)
//
// Importantly, systray.Run() BLOCKS — it hands control to the tray and doesn't return.
// So everything else has to be kicked off from inside onReady.
func main() {
	systray.Run(onReady, onExit)
}

// onReady is called once the system tray is ready.
// This is where we set up the tray menu and kick off background processes.
func onReady() {
	// Sets the text label on the tray icon (not all OS/themes show this)
	systray.SetTitle("Plex Presence")
	// Sets the tooltip that appears when you hover the tray icon
	systray.SetTooltip("Plex Discord Presence")
	// Sets the icon for the tray
	systray.SetIcon(iconData)

	// AddMenuItem creates a menu entry. Returns an object we can interact with.
	// Think of it like creating a <li> in a dropdown menu.
	// First arg is the label, second is the tooltip for that item.
	mStatus := systray.AddMenuItem("Not Playing", "Current Plex status")
	// Disable() makes it unclickable — we're using it as a read-only status display
	mStatus.Disable()

	// AddSeparator adds a horizontal rule in the menu, like <hr>
	systray.AddSeparator()

	// The quit button — this one we DO want to be clickable
	mQuit := systray.AddMenuItem("Quit", "Quit Plex Presence")

	// "go" keyword starts a goroutine — think of it like setTimeout(fn, 0) in JS,
	// or running something async. It runs concurrently without blocking this function.
	// We pass mStatus so the poller can update the menu label as status changes.
	go pollPlex(mStatus)

	// Another goroutine — this one just waits for the quit button to be clicked.
	// mQuit.ClickedCh is a "channel" — like an event emitter that blocks until fired.
	// The <- operator receives from the channel (waits for a value to come through).
	// When it does, we call systray.Quit() to shut down.
	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()

	// Startup toggle menu item — checked state reflects current registry value
	mStartup := systray.AddMenuItem("Launch at startup", "Toggle launch at Windows startup")
	if isStartupEnabled() {
		mStartup.Check()
	}

	// Goroutine that listens for clicks on the startup menu item.
	// "for range channel" loops every time the channel receives a value —
	// like addEventListener for repeated clicks, vs the one-shot <- we use for quit.
	go func() {
		for range mStartup.ClickedCh {
			if isStartupEnabled() {
				disableStartup()
				mStartup.Uncheck()
			} else {
				enableStartup()
				mStartup.Check()
			}
		}
	}()
}

// onExit is called during shutdown cleanup — like a destructor or process.on('exit').
// Later we'll use this to clear the Discord presence before quitting.
func onExit() {
	fmt.Println("Exiting...")
	clearDiscord()
}

// These structs mirror the JSON shape Plex returns.
// json:"..." tags tell the parser which JSON key maps to which field.
// This is like defining a schema for JSON.parse() in JS, or casting in PHP.
type PlexPlayer struct {
	Title string `json:"title"` // machine name e.g. "Apophis"
	State string `json:"state"` // "playing" or "paused"
}

type PlexUser struct {
	Title string `json:"title"` // username e.g. "FlesHBoX"
}

type PlexMetadata struct {
	Title            string     `json:"title"`            // movie/episode title
	Type             string     `json:"type"`             // "movie" or "episode"
	Year             int        `json:"year"`             // release year
	Duration         int        `json:"duration"`         // total duration in ms
	ViewOffset       int        `json:"viewOffset"`       // current position in ms
	GrandparentTitle string     `json:"grandparentTitle"` // TV show name (empty for movies)
	ParentIndex      int        `json:"parentIndex"`      // season number (TV only)
	Index            int        `json:"index"`            // episode number (TV only)
	Player           PlexPlayer `json:"Player"`
	User             PlexUser   `json:"User"`
}

type PlexResponse struct {
	MediaContainer struct {
		Size     int            `json:"size"`
		Metadata []PlexMetadata `json:"Metadata"`
	} `json:"MediaContainer"`
}

// discordConnected tracks whether we have an active Discord RPC connection.
// It's a simple boolean flag — like a module-level variable in JS.
// We use a pointer-sized value here so goroutines can read it safely.
// var discordConnected bool = false
// discordClient holds our Discord RPC connection instance.
// nil means we're not connected.
var discordClient *client.Client

func pollPlex(statusItem *systray.MenuItem) {
	// Renamed to httpClient to avoid collision with the imported "client" package
	httpClient := &http.Client{Timeout: 5 * time.Second}

	for {
		url := fmt.Sprintf("%s/status/sessions?X-Plex-Token=%s", plexHost, plexToken)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Println("Error creating request:", err)
			time.Sleep(pollInterval)
			continue
		}

		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req) // httpClient here, not client
		if err != nil {
			fmt.Println("Error contacting Plex:", err)
			time.Sleep(pollInterval)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Println("Error reading response:", err)
			time.Sleep(pollInterval)
			continue
		}

		var plexResp PlexResponse
		if err := json.Unmarshal(body, &plexResp); err != nil {
			fmt.Println("Error parsing JSON:", err)
			time.Sleep(pollInterval)
			continue
		}

		var currentSession *PlexMetadata
		for i := range plexResp.MediaContainer.Metadata {
			session := &plexResp.MediaContainer.Metadata[i]
			if session.Player.Title == playerName {
				currentSession = session
				break
			}
		}

		if currentSession == nil {
			fmt.Println("Nothing playing on", playerName)
			statusItem.SetTitle("Not Playing")
			clearDiscord()
		} else {
			fmt.Printf("Now playing: %s (%s) - state: %s\n",
				currentSession.Title,
				currentSession.Type,
				currentSession.Player.State,
			)
			statusItem.SetTitle(currentSession.Title)

			// Only login if we aren't already connected
			if discordClient == nil {
				c := client.NewClient(discordClientID)
				if err := c.Login(); err != nil {
					fmt.Println("Error connecting to Discord:", err)
				} else {
					discordClient = c
				}
			}

			if discordClient != nil {
				updateDiscord(currentSession)
			}
		}

		time.Sleep(pollInterval)
	}
}

// updateDiscord takes the current session and pushes it to Discord Rich Presence.
// It's a separate function so pollPlex stays readable.
func updateDiscord(session *PlexMetadata) {
	now := time.Now()
	start := now.Add(-time.Duration(session.ViewOffset) * time.Millisecond)
	end := start.Add(time.Duration(session.Duration) * time.Millisecond)

	var details string
	var state string

	if session.Type == "episode" {
		details = session.GrandparentTitle
		state = fmt.Sprintf("S%02dE%02d - %s", session.ParentIndex, session.Index, session.Title)
	} else {
		details = fmt.Sprintf("%s (%d)", session.Title, session.Year)
		state = "Watching"
	}

	if session.Player.State == "paused" {
		state = state + " (Paused)"
	}

	err := discordClient.SetActivity(client.Activity{
		Type:       3, // 3 = Watching
		Details:    details,
		State:      state,
		LargeImage: "plex",
		LargeText:  "Plex Media Player",
		Timestamps: &client.Timestamps{
			Start: &start,
			End:   &end,
		},
	})

	if err != nil {
		fmt.Println("Error updating Discord presence:", err)
	}
}

// clearDiscord removes the Rich Presence from Discord.
// rich-go has no Logout, so we clear by setting an empty activity.
func clearDiscord() {
	if discordClient != nil {
		discordClient.Logout()
		discordClient = nil
	}
}

const startupRegKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const appName = "PlexPresence"

// isStartupEnabled checks if our app is registered to run at Windows startup
func isStartupEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, startupRegKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	_, _, err = k.GetStringValue(appName)
	return err == nil
}

// enableStartup adds our exe to the Windows startup registry key
func enableStartup() {
	k, err := registry.OpenKey(registry.CURRENT_USER, startupRegKey, registry.SET_VALUE)
	if err != nil {
		fmt.Println("Error opening registry:", err)
		return
	}
	defer k.Close()

	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("Error getting exe path:", err)
		return
	}

	k.SetStringValue(appName, exePath)
	fmt.Println("Startup enabled:", exePath)
}

// disableStartup removes our exe from the Windows startup registry key
func disableStartup() {
	k, err := registry.OpenKey(registry.CURRENT_USER, startupRegKey, registry.SET_VALUE)
	if err != nil {
		fmt.Println("Error opening registry:", err)
		return
	}
	defer k.Close()

	k.DeleteValue(appName)
	fmt.Println("Startup disabled")
}
