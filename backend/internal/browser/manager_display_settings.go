package browser

import (
	"encoding/json"
	"errors"
)

type browserDisplaySettings struct {
	viewportMode string
	frameRate    int
	quality      int
}

func defaultBrowserDisplaySettings() browserDisplaySettings {
	return browserDisplaySettings{viewportMode: "auto", frameRate: 10, quality: 55}
}

func (p *process) displaySettings() browserDisplaySettings {
	settings := p.display
	defaults := defaultBrowserDisplaySettings()
	if settings.viewportMode != "auto" && settings.viewportMode != "fixed" {
		settings.viewportMode = defaults.viewportMode
	}
	if settings.frameRate == 0 {
		settings.frameRate = defaults.frameRate
	}
	if settings.quality == 0 {
		settings.quality = defaults.quality
	}
	return settings
}

func (p *process) addDisplaySettings(event map[string]any) {
	settings := p.displaySettings()
	event["viewportMode"] = settings.viewportMode
	event["frameRate"] = settings.frameRate
	event["quality"] = settings.quality
}

func (m *Manager) configureDisplay(current *viewer, command map[string]json.RawMessage) error {
	mode := rawString(command["viewportMode"])
	frameRate, frameRateOK := integerRaw(command["frameRate"])
	quality, qualityOK := integerRaw(command["quality"])
	width, widthOK := integerRaw(command["width"])
	height, heightOK := integerRaw(command["height"])
	generation := rawString(command["generation"])
	primaryIntent, _ := optionalBool(command["primaryIntent"])
	if (mode != "auto" && mode != "fixed") || !frameRateOK || !supportedFrameRate(frameRate) || !qualityOK || !supportedQuality(quality) || !widthOK || !heightOK {
		m.rejectDisplaySettings(current, "invalid_browser_settings", "The browser display settings are invalid.")
		return nil
	}
	size := normalizeViewport(viewportSize{Width: width, Height: height})
	if size.Width != width || size.Height != height || (mode == "fixed" && !supportedViewportPreset(size)) {
		m.rejectDisplaySettings(current, "invalid_browser_settings", "The browser display settings are invalid.")
		return nil
	}

	m.mu.Lock()
	if _, ok := m.viewers[current]; !ok || m.process != current.runtime {
		m.mu.Unlock()
		return errors.New("browser worker unavailable")
	}
	runtime := current.runtime
	if generation == "" || generation != runtime.generation {
		m.enqueueLocked(current, m.displaySettingsRejectedLocked(current, "stale_browser_generation", "The remote browser generation has changed."))
		m.mu.Unlock()
		return nil
	}
	if !primaryIntent || (m.primaryKnown && m.primary != current.clientID) {
		m.enqueueLocked(current, m.displaySettingsRejectedLocked(current, "not_primary_client", "Only the primary client can change browser display settings."))
		m.mu.Unlock()
		return nil
	}
	if runtime.pageStatus == "none" || runtime.pageStatus == "closed" || runtime.pageStatus == "closing" {
		m.enqueueLocked(current, m.displaySettingsRejectedLocked(current, "no_browser_page", "Open a browser page before changing display settings."))
		m.mu.Unlock()
		return nil
	}

	previousPrimary := m.primary
	previousPrimaryKnown := m.primaryKnown
	previousViewport := runtime.viewport
	previousSettings := runtime.displaySettings()
	if mode == "fixed" {
		runtime.viewport = size
	}
	if err := writeRuntimeLocked(runtime, map[string]any{
		"type": "settings", "width": size.Width, "height": size.Height,
		"frameRate": frameRate, "quality": quality,
	}); err != nil {
		runtime.viewport = previousViewport
		runtime.display = previousSettings
		m.primary = previousPrimary
		m.primaryKnown = previousPrimaryKnown
		m.mu.Unlock()
		return err
	}
	runtime.display = browserDisplaySettings{viewportMode: mode, frameRate: frameRate, quality: quality}
	runtime.viewport = size
	m.primary = current.clientID
	m.primaryKnown = true
	if previousPrimary != m.primary || !previousPrimaryKnown {
		m.broadcastPrimaryLocked()
	}
	m.broadcastStateLocked(runtime)
	m.mu.Unlock()
	return nil
}

func (m *Manager) rejectDisplaySettings(current *viewer, code, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.viewers[current]; !ok {
		return
	}
	m.enqueueLocked(current, m.displaySettingsRejectedLocked(current, code, message))
}

func (m *Manager) displaySettingsRejectedLocked(current *viewer, code, message string) map[string]any {
	settings := current.runtime.displaySettings()
	event := map[string]any{
		"type": "settings_rejected", "code": code, "error": message,
		"generation": current.runtime.generation,
		"width":      current.runtime.viewport.Width, "height": current.runtime.viewport.Height,
		"viewportMode": settings.viewportMode, "frameRate": settings.frameRate, "quality": settings.quality,
		"primary": m.primaryKnown && m.primary == current.clientID,
	}
	return event
}

func supportedFrameRate(value int) bool {
	return value == 5 || value == 10 || value == 15
}

func supportedQuality(value int) bool {
	return value == 40 || value == 55 || value == 70
}

func supportedViewportPreset(size viewportSize) bool {
	switch size {
	case viewportSize{Width: 1280, Height: 720}, viewportSize{Width: 1440, Height: 900}, viewportSize{Width: 1600, Height: 900}, viewportSize{Width: 1920, Height: 1080}:
		return true
	default:
		return false
	}
}
